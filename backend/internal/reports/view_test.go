package reports

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/toufiqqureshi/uplumo/backend/internal/branding"
)

func TestPrintScriptHashMatchesTheCSP(t *testing.T) {
	sum := sha256.Sum256([]byte(printScript))
	if got := base64.StdEncoding.EncodeToString(sum[:]); got != printScriptSHA256 {
		t.Fatalf("hash of printScript = %s, CSP names %s: printing would be blocked", got, printScriptSHA256)
	}
	if !strings.Contains(ReportCSP(true), "'sha256-"+printScriptSHA256+"'") {
		t.Fatal("print mode CSP does not name the script hash")
	}
	if strings.Contains(ReportCSP(false), "allow-scripts") {
		t.Fatal("scripts are allowed outside print mode")
	}
}

func TestWithBrandBar(t *testing.T) {
	website := "https://acme.example/path"
	b := &branding.Branding{BrandName: `A<b>"&`, AccentColor: "#112233", WebsiteURL: &website}
	tests := []struct {
		name, doc string
		bar       *branding.Branding
		want      func(string) bool
	}{
		{"no branding leaves the document alone", "<body>x</body>", nil, func(s string) bool { return s == "<body>x</body>" }},
		{"no body tag is served unbranded", "<!doctype html><p>x</p>", b, func(s string) bool { return s == "<!doctype html><p>x</p>" }},
		{"bar goes right after the body tag", "<html><BODY class=a><p>x</p></body></html>", b, func(s string) bool {
			return strings.Contains(s, "<BODY class=a><div data-brand-bar") && strings.HasSuffix(s, "<p>x</p></body></html>")
		}},
		{"the name is escaped", "<body></body>", b, func(s string) bool {
			return strings.Contains(s, "A&lt;b&gt;&#34;&amp;") && !strings.Contains(s, "A<b>")
		}},
		{"a bad accent falls back", "<body></body>", &branding.Branding{BrandName: "A", AccentColor: "red;}</style>"}, func(s string) bool {
			return strings.Contains(s, branding.DefaultAccent) && !strings.Contains(s, "</style>")
		}},
		{"a javascript website is dropped", "<body></body>", func() *branding.Branding {
			js := "javascript:alert(1)"
			return &branding.Branding{BrandName: "A", AccentColor: "#112233", WebsiteURL: &js}
		}(), func(s string) bool { return !strings.Contains(s, "javascript:") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := withBrandBar(tt.doc, tt.bar); !tt.want(got) {
				t.Errorf("withBrandBar() = %q", got)
			}
		})
	}
}

func TestVisitURLAllowsOnlyCredentialFreeHTTPURLs(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "https", url: "https://example.test/path", want: "https://example.test/path"},
		{name: "http", url: "http://example.test/path", want: "http://example.test/path"},
		{name: "script scheme", url: "javascript:alert(1)"},
		{name: "relative URL", url: "//example.test/path"},
		{name: "credentials", url: (&url.URL{Scheme: "https", User: url.UserPassword("username", strings.Repeat("x", 8)), Host: "example.test"}).String()},
		{name: "malformed", url: "https://%zz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(visitURL(branding.Branding{WebsiteURL: &tt.url}))
			if got != tt.want {
				t.Errorf("visitURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWithPrintScriptGoesLast(t *testing.T) {
	for doc, want := range map[string]string{
		"<body>a</body></html>": "<body>a<script>" + printScript + "</script></body></html>",
		"<p>a</p></html>":       "<p>a</p><script>" + printScript + "</script></html>",
		"<p>a</p>":              "<p>a</p><script>" + printScript + "</script>",
	} {
		if got := withPrintScript(doc); got != want {
			t.Errorf("withPrintScript(%q) = %q, want %q", doc, got, want)
		}
	}
}

func TestShareDescription(t *testing.T) {
	long := strings.Repeat("é", 300)
	if got := []rune(shareDescription("\n\n  first line  \nsecond")); string(got) != "first line" {
		t.Errorf("first non-empty line = %q", string(got))
	}
	if got := shareDescription(long); len([]rune(got)) != maxDescriptionChars || !strings.HasSuffix(got, "…") {
		t.Errorf("long line = %d runes, %q", len([]rune(got)), got[len(got)-3:])
	}
	if got := shareDescription(" \n "); got != "" {
		t.Errorf("blank summary = %q", got)
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	for ago, want := range map[time.Duration]string{
		10 * time.Second: "just now", time.Minute: "1 minute ago", 5 * time.Hour: "5 hours ago",
		48 * time.Hour: "2 days ago", 90 * 24 * time.Hour: "13 Jul 2026",
	} {
		if got := relativeTime(now.Add(-ago), now); got != want {
			t.Errorf("%v ago = %q, want %q", ago, got, want)
		}
	}
}
