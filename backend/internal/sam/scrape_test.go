package sam

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// testGuard allows the httptest origin and blocks everything else, like the
// real guard blocks private addresses.
type testGuard struct{ allowedHost string }

func (g testGuard) NormalizeAndValidateStartURL(_ context.Context, input string) (string, error) {
	if !strings.Contains(input, "://") {
		input = "http://" + input
	}
	parsed, err := url.Parse(input)
	if err != nil || parsed.Host != g.allowedHost {
		return "", errors.New("blocked by test guard")
	}
	return parsed.String(), nil
}

func (g testGuard) NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func newSite(t *testing.T, handler http.HandlerFunc) (scraper, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	return scraper{guard: testGuard{allowedHost: host}}, server
}

func TestHTMLToTextStripsMarkupAndDecodesEntities(t *testing.T) {
	page := `<html><head><title>A &amp; B</title><style>p{}</style><script>alert(1)</script></head>
<body><!-- hidden --><noscript>enable js</noscript><h1>Hello</h1> <p>Fish&nbsp;&amp; chips  &lt;3</p></body></html>`
	got := htmlToText(page)
	if strings.Contains(got, "alert") || strings.Contains(got, "hidden") || strings.Contains(got, "enable js") || strings.Contains(got, "p{}") {
		t.Fatalf("non-content survived: %q", got)
	}
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "& chips <3") {
		t.Fatalf("text = %q", got)
	}
	if title := extractTitle(page); title == nil || *title != "A & B" {
		t.Fatalf("title = %v", title)
	}
	if extractTitle("<p>no title</p>") != nil {
		t.Fatal("a missing title must be nil")
	}
}

func TestParseSitemapURLsKeepsSameOriginPagesOnly(t *testing.T) {
	xml := `<urlset>
<url><loc>https://a.test/one</loc></url>
<url><loc>/relative</loc></url>
<url><loc>https://other.test/x</loc></url>
<url><loc>https://a.test/sitemap-2.xml</loc></url>
</urlset>`
	got := parseSitemapURLs(xml, "https://a.test")
	want := []string{"https://a.test/one", "https://a.test/relative"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMapLinksListsHomepageThenSitemapAndRespectsLimit(t *testing.T) {
	s, server := newSite(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sitemap.xml" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `<urlset><url><loc>%[1]s/</loc></url><url><loc>%[1]s/a</loc></url><url><loc>%[1]s/b</loc></url></urlset>`, "http://"+r.Host)
	})
	urls, blocked := s.mapLinks(context.Background(), server.URL, 2)
	if blocked || len(urls) != 2 || !strings.HasSuffix(urls[1], "/a") {
		t.Fatalf("urls = %v blocked = %v", urls, blocked)
	}
}

func TestMapLinksFallsBackToHomepageWithoutSitemap(t *testing.T) {
	s, server := newSite(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	urls, blocked := s.mapLinks(context.Background(), server.URL, 10)
	if blocked || len(urls) != 1 {
		t.Fatalf("urls = %v blocked = %v", urls, blocked)
	}
}

func TestMapLinksReportsBlockedHost(t *testing.T) {
	s := scraper{guard: testGuard{allowedHost: "allowed.test"}}
	urls, blocked := s.mapLinks(context.Background(), "http://169.254.169.254/", 10)
	if !blocked || len(urls) != 0 {
		t.Fatalf("urls = %v blocked = %v", urls, blocked)
	}
}

func TestReadPagesSkipsBlockedAndEmptyPages(t *testing.T) {
	s, server := newSite(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			fmt.Fprint(w, "<title>Ok</title><p>"+strings.Repeat("word ", 2000)+"</p>")
		case "/empty":
			fmt.Fprint(w, "<script>x</script>")
		default:
			http.NotFound(w, r)
		}
	})
	pages, blocked := s.readPages(context.Background(), []string{
		server.URL + "/ok", server.URL + "/empty", server.URL + "/missing", "http://127.0.0.1:1/internal",
	}, 10)
	if blocked || len(pages) != 1 {
		t.Fatalf("pages = %+v blocked = %v", pages, blocked)
	}
	if n := len([]rune(pages[0].Text)); n != perPageChars {
		t.Fatalf("text length = %d, want %d", n, perPageChars)
	}
	if pages[0].Title == nil || *pages[0].Title != "Ok" {
		t.Fatalf("title = %v", pages[0].Title)
	}
}

func TestReadPagesHonorsLimitAndReportsBlockedWhenNothingReadable(t *testing.T) {
	var hits int
	s, server := newSite(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		fmt.Fprint(w, "<p>text</p>")
	})
	pages, _ := s.readPages(context.Background(), []string{server.URL + "/1", server.URL + "/2", server.URL + "/3"}, 2)
	if len(pages) != 2 || hits != 2 {
		t.Fatalf("pages = %d hits = %d, want 2 and 2", len(pages), hits)
	}
	if _, blocked := s.readPages(context.Background(), nil, 5); !blocked {
		t.Fatal("no pages read must report blocked")
	}
}

func TestFetchFollowsOneValidatedRedirect(t *testing.T) {
	s, server := newSite(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/old":
			http.Redirect(w, r, "/new", http.StatusMovedPermanently)
		case "/new":
			fmt.Fprint(w, "moved here")
		case "/loop1":
			http.Redirect(w, r, "/loop2", http.StatusFound)
		case "/loop2":
			http.Redirect(w, r, "/loop3", http.StatusFound)
		}
	})
	if body, ok := s.fetchText(context.Background(), server.URL+"/old"); !ok || body != "moved here" {
		t.Fatalf("redirect body = %q ok = %v", body, ok)
	}
	if _, ok := s.fetchText(context.Background(), server.URL+"/loop1"); ok {
		t.Fatal("a second redirect hop must not be followed")
	}
}

// The redirect target is reachable here, so only the guard stands between the
// fetcher and its content. A target that merely fails to connect would pass for
// the wrong reason.
func TestFetchRefusesRedirectsToBlockedHosts(t *testing.T) {
	var internalHits int
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internalHits++
		fmt.Fprint(w, "internal secret")
	}))
	t.Cleanup(internal.Close)
	s, server := newSite(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/metadata", http.StatusFound)
	})
	if body, ok := s.fetchText(context.Background(), server.URL); ok || body != "" {
		t.Fatalf("a redirect to a host the guard blocks was followed: %q", body)
	}
	if internalHits != 0 {
		t.Fatalf("the blocked host was contacted %d times", internalHits)
	}
}

func TestFetchRejectsOversizedBody(t *testing.T) {
	s, server := newSite(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", maxResponseBytes+1)))
	})
	if _, ok := s.fetchText(context.Background(), server.URL); ok {
		t.Fatal("an oversized body must be rejected, not truncated")
	}
}
