package site

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func newSite(t *testing.T, publicURL *url.URL) *Site {
	t.Helper()
	s, err := New(publicURL)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return s
}

func get(handler http.HandlerFunc, target string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestFormatINR(t *testing.T) {
	tests := map[int]string{
		0:         "₹0",
		7:         "₹7",
		999:       "₹999",
		1000:      "₹1,000",
		1499:      "₹1,499",
		99999:     "₹99,999",
		100000:    "₹1,00,000",
		1234567:   "₹12,34,567",
		12345678:  "₹1,23,45,678",
		123456789: "₹12,34,56,789",
	}
	for rupees, want := range tests {
		if got := formatINR(rupees); got != want {
			t.Errorf("formatINR(%d) = %q, want %q", rupees, got, want)
		}
	}
}

type jsonLDDoc struct {
	Context string `json:"@context"`
	Graph   []struct {
		Type   string `json:"@type"`
		Name   string `json:"name"`
		URL    string `json:"url"`
		Logo   string `json:"logo"`
		Offers []struct {
			Name     string `json:"name"`
			Price    string `json:"price"`
			Currency string `json:"priceCurrency"`
		} `json:"offers"`
	} `json:"@graph"`
}

var jsonLDBlock = regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`)

func parseJSONLD(t *testing.T, body string) jsonLDDoc {
	t.Helper()
	blocks := jsonLDBlock.FindAllStringSubmatch(body, -1)
	if len(blocks) != 1 {
		t.Fatalf("found %d JSON-LD blocks, want 1", len(blocks))
	}
	var doc jsonLDDoc
	if err := json.Unmarshal([]byte(blocks[0][1]), &doc); err != nil {
		t.Fatalf("JSON-LD %s does not parse: %v", blocks[0][1], err)
	}
	return doc
}

func TestPagesCarryTheirMetadata(t *testing.T) {
	s := newSite(t, &url.URL{Scheme: "https", Host: "seomarine.com"})
	tests := []struct {
		name      string
		serve     http.HandlerFunc
		canonical string
		cache     string
	}{
		{name: "landing", serve: s.ServeLanding, canonical: "https://seomarine.com/", cache: "no-cache"},
		{name: "pricing", serve: s.ServePricing, canonical: "https://seomarine.com/pricing", cache: "public, max-age=300"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := get(tt.serve, "/", nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			for header, want := range map[string]string{
				"Content-Type":            "text/html; charset=utf-8",
				"Cache-Control":           tt.cache,
				"Content-Security-Policy": contentSecurityPolicy,
				"X-Content-Type-Options":  "nosniff",
			} {
				if got := rec.Header().Get(header); got != want {
					t.Errorf("%s = %q, want %q", header, got, want)
				}
			}

			body := rec.Body.String()
			for _, want := range []string{
				`<html lang="en-IN">`,
				`<link rel="canonical" href="` + tt.canonical + `">`,
				`<meta property="og:url" content="` + tt.canonical + `">`,
				`<meta property="og:image" content="https://seomarine.com/site/social-card.png?v=` + s.versions["social-card.png"] + `">`,
				`<meta name="twitter:card" content="summary_large_image">`,
				`href="/sign-up"`,
				"₹1,499",
			} {
				if !strings.Contains(body, want) {
					t.Errorf("page lacks %s", want)
				}
			}
			for _, tag := range []string{"<title>", `<meta name="description" content="`, `<meta property="og:title" content="`, "<h1>", "<main "} {
				if n := strings.Count(body, tag); n != 1 {
					t.Errorf("page has %d %s, want exactly 1", n, tag)
				}
			}
			if strings.Contains(strings.ToLower(body), "openseo") {
				t.Error("page mentions OpenSEO")
			}

			doc := parseJSONLD(t, body)
			if doc.Context != "https://schema.org" || len(doc.Graph) != 2 {
				t.Fatalf("JSON-LD = %+v, want a schema.org graph of two nodes", doc)
			}
			org, app := doc.Graph[0], doc.Graph[1]
			if org.Type != "Organization" || org.URL != "https://seomarine.com/" || org.Logo != "https://seomarine.com/site/logo.svg" {
				t.Errorf("Organization = %+v", org)
			}
			if app.Type != "SoftwareApplication" || app.Name != "Seomarine" || len(app.Offers) != 2 {
				t.Fatalf("SoftwareApplication = %+v", app)
			}
			for i, want := range []string{"0", "1499"} {
				if o := app.Offers[i]; o.Price != want || o.Currency != "INR" {
					t.Errorf("offer %d = %+v, want %s INR", i, o, want)
				}
			}
		})
	}
}

// The public URL comes from configuration, but it reaches HTML attributes
// and a script block, so it must not be able to break out of either.
func TestPagesEscapeThePublicURL(t *testing.T) {
	hostile := &url.URL{Scheme: "https", Host: `x"><script>alert(1)</script>`}
	s := newSite(t, hostile)
	body := get(s.ServeLanding, "/", nil).Body.String()

	if strings.Contains(body, "<script>alert(1)") {
		t.Fatal("the public URL was written into the page unescaped")
	}
	if !strings.Contains(body, `<link rel="canonical" href="https://x%22%3e%3cscript%3ealert%281%29%3c%2Fscript%3e/">`) {
		t.Errorf("canonical link is not URL-escaped:\n%s", body[:strings.Index(body, "<style>")])
	}
	org := parseJSONLD(t, body).Graph[0]
	if want := hostile.String() + "/"; org.URL != want {
		t.Errorf("JSON-LD url = %q, want %q intact", org.URL, want)
	}
}

func TestPagesAnswerConditionalRequests(t *testing.T) {
	s := newSite(t, &url.URL{Scheme: "https", Host: "seomarine.com"})
	etag := get(s.ServePricing, "/pricing", nil).Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	if rec := get(s.ServePricing, "/pricing", http.Header{"If-None-Match": {etag}}); rec.Code != http.StatusNotModified {
		t.Errorf("status with a matching ETag = %d, want 304", rec.Code)
	}
	if other := get(s.ServeLanding, "/", nil).Header().Get("ETag"); other == etag {
		t.Error("landing and pricing share an ETag")
	}
}

var assetURL = regexp.MustCompile(`/site/[^"?]+\?v=[0-9a-f]+`)

// Every asset a page references must be servable at that URL and cacheable
// for good; any other version of the URL must be revalidated.
func TestAssets(t *testing.T) {
	s := newSite(t, &url.URL{Scheme: "https", Host: "seomarine.com"})
	page := get(s.ServeLanding, "/", nil).Body.String() + get(s.ServePricing, "/pricing", nil).Body.String()
	refs := assetURL.FindAllString(page, -1)
	if len(refs) < 3 {
		t.Fatalf("pages reference %d assets, want the logo, the font and the social card", len(refs))
	}
	wantType := map[string]string{".svg": "image/svg+xml", ".png": "image/png", ".woff2": "font/woff2"}
	for _, ref := range refs {
		rec := get(s.ServeAsset, ref, nil)
		ext := ref[strings.LastIndex(ref, "."):strings.Index(ref, "?")]
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Errorf("%s: status = %d, %d bytes", ref, rec.Code, rec.Body.Len())
		}
		if got := rec.Header().Get("Content-Type"); got != wantType[ext] {
			t.Errorf("%s: Content-Type = %q, want %q", ref, got, wantType[ext])
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Errorf("%s: Cache-Control = %q", ref, got)
		}
	}

	stale := get(s.ServeAsset, "/site/logo.svg?v=0000000000000000", nil)
	if stale.Code != http.StatusOK || stale.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("stale version: status %d, Cache-Control %q; want 200 and no-cache", stale.Code, stale.Header().Get("Cache-Control"))
	}
	etag := stale.Header().Get("ETag")
	if rec := get(s.ServeAsset, "/site/logo.svg", http.Header{"If-None-Match": {etag}}); rec.Code != http.StatusNotModified {
		t.Errorf("revalidation with ETag %s: status = %d, want 304", etag, rec.Code)
	}

	for _, target := range []string{"/site/", "/site/missing.css", "/site/static/logo.svg", "/site/../site.go", "/site/LOGO.SVG"} {
		if rec := get(s.ServeAsset, target, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", target, rec.Code)
		}
	}
}
