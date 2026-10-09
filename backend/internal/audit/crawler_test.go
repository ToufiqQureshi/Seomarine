package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// crawlerServer serves a small set of pages exercised by the crawler tests.
func crawlerServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Add("Link", `<https://example.com/canonical>; rel="canonical"`)
		w.Header().Set("X-Robots-Tag", "index, follow")
		_, _ = w.Write([]byte(`<html><head>
			<title>T</title>
			<meta name="description" content="D">
		</head><body><h1>H</h1>
			<a href="/internal">in</a>
			<a href="https://other.com/ext">out</a>
			<img src="/a.png">
			<p>` + auditFiller + `</p>
		</body></html>`))
	})
	mux.HandleFunc("/noindex", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><meta name="robots" content="noindex"></head><body><p>hi</p></body></html>`))
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/ok")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/pdf", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 binary"))
	})
	mux.HandleFunc("/blocked", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/limited", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func newTestCrawler(server *httptest.Server, maxHTMLBytes int) *Crawler {
	guard := &Guard{
		Resolver: fakeResolver{addrs: map[string][]string{"audit.test": {"93.184.216.34"}}},
		Client:   dialAllTo(server),
	}
	return NewCrawler(CrawlerOptions{Guard: guard, MaxHTMLBytes: maxHTMLBytes})
}

func TestCrawlerFetchPageAnalyzesHTML(t *testing.T) {
	crawler := newTestCrawler(crawlerServer(t), 0)
	result := crawler.FetchPage(context.Background(), "http://audit.test/ok")
	if result.FetchClass != FetchOK || result.StatusCode != 200 {
		t.Fatalf("class=%s status=%d", result.FetchClass, result.StatusCode)
	}
	if !result.IsHtml {
		t.Fatal("expected HTML")
	}
	if result.Title != "T" || result.H1Count != 1 {
		t.Fatalf("title=%q h1=%d", result.Title, result.H1Count)
	}
	if result.HeaderCanonicalURL != "https://example.com/canonical" {
		t.Fatalf("header canonical = %q", result.HeaderCanonicalURL)
	}
	if result.XRobotsTag != "index, follow" || !result.IsIndexable {
		t.Fatalf("xRobotsTag=%q indexable=%v", result.XRobotsTag, result.IsIndexable)
	}
	if result.ImagesTotal != 1 || result.ImagesMissingAlt != 1 {
		t.Fatalf("images total=%d missingAlt=%d", result.ImagesTotal, result.ImagesMissingAlt)
	}
	if len(result.Links) != 2 {
		t.Fatalf("links = %d", len(result.Links))
	}
	if result.ContentHash == "" {
		t.Fatal("expected a content hash")
	}
	if result.HTMLBytesRead == 0 {
		t.Fatal("expected a byte count")
	}
}

func TestCrawlerFetchPageNoindex(t *testing.T) {
	crawler := newTestCrawler(crawlerServer(t), 0)
	result := crawler.FetchPage(context.Background(), "http://audit.test/noindex")
	if result.IsIndexable {
		t.Fatal("expected the page to be noindex")
	}
}

func TestCrawlerFetchPageRedirectRecordsTarget(t *testing.T) {
	crawler := newTestCrawler(crawlerServer(t), 0)
	result := crawler.FetchPage(context.Background(), "http://audit.test/redirect")
	if result.StatusCode != http.StatusFound {
		t.Fatalf("status = %d", result.StatusCode)
	}
	if result.RedirectURL != "http://audit.test/ok" {
		t.Fatalf("redirectURL = %q", result.RedirectURL)
	}
	if result.IsHtml {
		t.Fatal("a redirect has no body to analyze")
	}
}

func TestCrawlerFetchPageNonHTML(t *testing.T) {
	crawler := newTestCrawler(crawlerServer(t), 0)
	result := crawler.FetchPage(context.Background(), "http://audit.test/pdf")
	if result.IsHtml {
		t.Fatal("a PDF must not be analyzed as HTML")
	}
	if result.StatusCode != 200 {
		t.Fatalf("status = %d", result.StatusCode)
	}
}

func TestCrawlerFetchPageClassifiesBlockAndRateLimit(t *testing.T) {
	crawler := newTestCrawler(crawlerServer(t), 0)
	if got := crawler.FetchPage(context.Background(), "http://audit.test/blocked").FetchClass; got != FetchBlocked {
		t.Fatalf("blocked class = %s", got)
	}
	if got := crawler.FetchPage(context.Background(), "http://audit.test/limited").FetchClass; got != FetchRateLimited {
		t.Fatalf("rate limited class = %s", got)
	}
}

func TestCrawlerFetchPageRejectsBlockedHost(t *testing.T) {
	crawler := newTestCrawler(crawlerServer(t), 0)
	result := crawler.FetchPage(context.Background(), "http://127.0.0.1/")
	if result.FetchClass != FetchError {
		t.Fatalf("class = %s, want error", result.FetchClass)
	}
}

func TestCrawlerTruncatesLargeBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>" + strings.Repeat("x", 5000) + "</body></html>"))
	}))
	defer server.Close()
	crawler := newTestCrawler(server, 100)
	result := crawler.FetchPage(context.Background(), "http://audit.test/")
	if result.HTMLBytesRead > 100 {
		t.Fatalf("read %d bytes, want at most 100", result.HTMLBytesRead)
	}
}

func TestCrawlerCrawlPageURLsPreservesOrder(t *testing.T) {
	crawler := newTestCrawler(crawlerServer(t), 0)
	urls := []string{"http://audit.test/ok", "http://audit.test/noindex", "http://audit.test/pdf"}
	results, err := crawler.CrawlPageURLs(context.Background(), urls, 2)
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d", len(results))
	}
	if results[0].URL != urls[0] || results[2].URL != urls[2] {
		t.Fatalf("results out of order: %+v", results)
	}
}

func TestCrawlerCrawlPageURLsHonorsCanceledContext(t *testing.T) {
	crawler := newTestCrawler(crawlerServer(t), 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := crawler.CrawlPageURLs(ctx, []string{"http://audit.test/ok"}, 1); err == nil {
		t.Fatal("expected the canceled context to surface")
	}
}

func TestLooksLikeHTMLAndPrefixSnippet(t *testing.T) {
	if !looksLikeHTML("  <!doctype html><html>") {
		t.Fatal("expected html sniff")
	}
	if looksLikeHTML("just text") {
		t.Fatal("plain text is not html")
	}
	if got := prefixSnippet("abcdef", 3); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := prefixSnippet("ab", 10); got != "ab" {
		t.Fatalf("got %q", got)
	}
}

func TestDecodeUTF8Prefix(t *testing.T) {
	// A truncated multibyte rune must be dropped rather than corrupted.
	bytes := []byte{'a', 0xe2, 0x82}
	if got := decodeUTF8Prefix(bytes); got != "a" {
		t.Fatalf("got %q", got)
	}
	if got := decodeUTF8Prefix([]byte("hello")); got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestGuardDefaults(t *testing.T) {
	guard := NewGuard()
	if guard.Resolver != nil {
		t.Fatal("NewGuard leaves the resolver nil so the default is used")
	}
	if guard.resolver() == nil {
		t.Fatal("resolver() must fall back to the default")
	}
	if guard.dialer() == nil {
		t.Fatal("dialer() must fall back to a default dialer")
	}
	if guard.NewClient(time.Second) == nil {
		t.Fatal("NewClient must return a client")
	}
}
