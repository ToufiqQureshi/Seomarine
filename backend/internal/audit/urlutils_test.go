package audit

import "testing"

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		base   string
		want   string
		wantOK bool
	}{
		{name: "strips fragment", url: "https://example.com/page#top", want: "https://example.com/page", wantOK: true},
		{name: "sorts query", url: "https://example.com/?b=2&a=1", want: "https://example.com/?a=1&b=2", wantOK: true},
		{name: "lowercases host", url: "https://Example.COM/Path", want: "https://example.com/Path", wantOK: true},
		{name: "keeps trailing slash", url: "https://example.com/path/", want: "https://example.com/path/", wantOK: true},
		{name: "adds slash to bare host", url: "https://example.com", want: "https://example.com/", wantOK: true},
		{name: "resolves relative", url: "../other", base: "https://example.com/a/b/", want: "https://example.com/a/other", wantOK: true},
		{name: "rejects mailto", url: "mailto:a@b.com", wantOK: false},
		{name: "rejects garbage", url: "http://[::1", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := normalizeURL(tc.url, tc.base)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tc.wantOK, got)
			}
			if ok && got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCanonicalURLKey(t *testing.T) {
	cases := map[string]string{
		"http://www.example.com/services": "https://example.com/services",
		"https://example.com/services/":   "https://example.com/services/",
		"https://EXAMPLE.com/?b=2&a=1#x":  "https://example.com/?a=1&b=2",
	}
	for input, want := range cases {
		if got := canonicalURLKey(input); got != want {
			t.Errorf("canonicalURLKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestIsSameOrigin(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		origin string
		want   bool
	}{
		{"exact", "https://example.com/a", "https://example.com", true},
		{"www equivalence", "https://www.example.com/a", "https://example.com", true},
		{"http to https upgrade", "https://example.com/a", "http://example.com", true},
		{"different host", "https://other.com/a", "https://example.com", false},
		{"port mismatch", "https://example.com:8443/a", "https://example.com", false},
		{"https to http downgrade", "http://example.com/a", "https://example.com", false},
		{"subdomain is not same origin", "https://blog.example.com/a", "https://example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSameOrigin(tc.url, tc.origin); got != tc.want {
				t.Fatalf("isSameOrigin(%q, %q) = %v, want %v", tc.url, tc.origin, got, tc.want)
			}
		})
	}
}

func TestDetectURLTemplate(t *testing.T) {
	cases := map[string]string{
		"/blog/my-great-post":                     "/blog/:slug",
		"/products/12345":                         "/products/:id",
		"/u/550e8400-e29b-41d4-a716-446655440000": "/u/:uuid",
		"/archive/2024-01-15":                     "/archive/:date",
		"/my-account":                             "/my-account",
		"/about":                                  "/about",
	}
	for input, want := range cases {
		if got := detectURLTemplate(input); got != want {
			t.Errorf("detectURLTemplate(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGetOrigin(t *testing.T) {
	if got := getOrigin("https://www.example.com:8443/a/b"); got != "https://www.example.com:8443" {
		t.Fatalf("got %q", got)
	}
}
