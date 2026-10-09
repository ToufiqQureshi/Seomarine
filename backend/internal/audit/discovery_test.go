package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseRobotsTxt(t *testing.T) {
	text := "User-agent: *\nDisallow: /private\nAllow: /private/public\nSitemap: https://example.com/sitemap.xml\n"
	robots := parseRobotsTxt("https://example.com", &text)
	if !robots.IsAllowed("https://example.com/ok") {
		t.Error("expected /ok to be allowed")
	}
	if robots.IsAllowed("https://example.com/private/x") {
		t.Error("expected /private/x to be disallowed")
	}
	if !robots.IsAllowed("https://example.com/private/public") {
		t.Error("expected /private/public to be allowed")
	}
	if len(robots.SitemapURLs) != 1 || robots.SitemapURLs[0] != "https://example.com/sitemap.xml" {
		t.Errorf("sitemaps = %v", robots.SitemapURLs)
	}
}

func TestParseRobotsTxtNilAllowsEverything(t *testing.T) {
	robots := parseRobotsTxt("https://example.com", nil)
	if !robots.IsAllowed("https://example.com/anything") {
		t.Fatal("a missing robots.txt allows everything")
	}
}

func TestParseRobotsTxtWildcardAndEndAnchor(t *testing.T) {
	text := "User-agent: *\nDisallow: /*.pdf$\n"
	robots := parseRobotsTxt("https://example.com", &text)
	if robots.IsAllowed("https://example.com/docs/file.pdf") {
		t.Error("expected the .pdf to be disallowed")
	}
	if !robots.IsAllowed("https://example.com/docs/file.pdf?v=1") {
		t.Error("an end anchor must not match a query string")
	}
}

func TestParseRobotsTxtUserAgentSpecificity(t *testing.T) {
	text := "User-agent: Seomarine-Audit\nDisallow: /blocked\n\nUser-agent: *\nDisallow: /\n"
	robots := parseRobotsTxt("https://example.com", &text)
	if robots.IsAllowed("https://example.com/blocked") {
		t.Error("expected /blocked to be disallowed for our agent")
	}
	if !robots.IsAllowed("https://example.com/other") {
		t.Error("the specific group should win over the wildcard group")
	}
}

func TestParseRobotsTxtEmptyDisallowIsNoop(t *testing.T) {
	text := "User-agent: *\nDisallow:\n"
	robots := parseRobotsTxt("https://example.com", &text)
	if !robots.IsAllowed("https://example.com/anything") {
		t.Fatal("an empty Disallow allows everything")
	}
}

func TestParseSitemapXML(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/a</loc></url>
  <url><loc>https://example.com/b</loc></url>
</urlset>`
	sections := parseSitemapXML(body)
	if len(sections.URLs) != 2 || sections.URLs[0] != "https://example.com/a" {
		t.Fatalf("urls = %v", sections.URLs)
	}
	if len(sections.Sitemaps) != 0 {
		t.Fatalf("sitemaps = %v", sections.Sitemaps)
	}

	index := `<sitemapindex><sitemap><loc>https://example.com/s1.xml</loc></sitemap></sitemapindex>`
	sections = parseSitemapXML(index)
	if len(sections.Sitemaps) != 1 || sections.Sitemaps[0] != "https://example.com/s1.xml" {
		t.Fatalf("sitemaps = %v", sections.Sitemaps)
	}
}

func TestIsProbablySitemapXML(t *testing.T) {
	if !isProbablySitemapXML("application/xml", "") {
		t.Error("xml content type should qualify")
	}
	if !isProbablySitemapXML("", "  <?xml version=\"1.0\"?>") {
		t.Error("an xml prolog should qualify")
	}
	if !isProbablySitemapXML("", "<urlset>") {
		t.Error("a urlset root should qualify")
	}
	if isProbablySitemapXML("text/html", "<html>") {
		t.Error("an HTML page is not a sitemap")
	}
}

func TestDiscoverURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /private\nSitemap: http://audit.test/sitemap.xml\n"))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset>
				<url><loc>http://audit.test/b</loc></url>
				<url><loc>http://audit.test/a</loc></url>
				<url><loc>http://audit.test/private/hidden</loc></url>
			</urlset>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	guard := &Guard{Resolver: fakeResolver{addrs: map[string][]string{"audit.test": {"93.184.216.34"}}}}
	d := discoverer{guard: guard, client: dialAllTo(server), now: time.Now}
	urls, robotsText := d.DiscoverURLs(context.Background(), "http://audit.test", 10, nil)
	if robotsText == nil {
		t.Fatal("expected the robots.txt body")
	}
	want := []string{"http://audit.test/a", "http://audit.test/b", "http://audit.test/private/hidden"}
	if len(urls) != len(want) {
		t.Fatalf("urls = %v, want %v", urls, want)
	}
	for i := range want {
		if urls[i] != want[i] {
			t.Fatalf("urls = %v, want %v", urls, want)
		}
	}
}

func TestDiscoverURLsCapsAtMaxPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\n"))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset>
				<url><loc>http://audit.test/1</loc></url>
				<url><loc>http://audit.test/2</loc></url>
				<url><loc>http://audit.test/3</loc></url>
			</urlset>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	guard := &Guard{Resolver: fakeResolver{addrs: map[string][]string{"audit.test": {"93.184.216.34"}}}}
	d := discoverer{guard: guard, client: dialAllTo(server), now: time.Now}
	urls, _ := d.DiscoverURLs(context.Background(), "http://audit.test", 2, nil)
	if len(urls) != 2 {
		t.Fatalf("urls = %v, want 2", urls)
	}
}
