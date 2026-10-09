package audit

import (
	"strings"
	"testing"
)

const sampleHTML = `<!doctype html>
<html>
<head>
  <title>  Hello World  </title>
  <meta name="description" content=" A description. ">
  <meta name="robots" content="noindex, follow">
  <meta property="og:title" content="OG Title">
  <meta property="og:description" content="OG Desc">
  <meta property="og:image" content="https://cdn.test/a.png">
  <link rel="canonical" href="https://example.com/canonical">
  <link rel="alternate" hreflang="en" href="https://example.com/en">
  <script type="application/ld+json">{"@context":"https://schema.org"}</script>
  <style>body{color:red}</style>
</head>
<body>
  <h1>Main Heading</h1>
  <h3>Skipped</h3>
  <img src="/a.png" alt="A">
  <img src="/b.png">
  <a href="/internal" rel="nofollow">Internal</a>
  <a href="https://other.com/ext">External</a>
  <a href="mailto:x@y.com">Mail</a>
  <p>Hello   world this is text.</p>
  <noscript><img src="/no.png" alt="no"><a href="/noscript">x</a></noscript>
</body>
</html>`

func TestAnalyzeHTMLExtractsEverything(t *testing.T) {
	got := AnalyzeHTML(sampleHTML, "https://example.com/page", 200, 120, "")
	if got.Title != "Hello World" {
		t.Errorf("title = %q", got.Title)
	}
	if got.MetaDescription != "A description." {
		t.Errorf("metaDescription = %q", got.MetaDescription)
	}
	if got.RobotsMeta != "noindex, follow" {
		t.Errorf("robotsMeta = %q", got.RobotsMeta)
	}
	if got.Canonical != "https://example.com/canonical" {
		t.Errorf("canonical = %q", got.Canonical)
	}
	if got.OgTitle != "OG Title" || got.OgDescription != "OG Desc" || got.OgImage != "https://cdn.test/a.png" {
		t.Errorf("og tags = %q/%q/%q", got.OgTitle, got.OgDescription, got.OgImage)
	}
	if len(got.HreflangTags) != 1 || got.HreflangTags[0] != "en" {
		t.Errorf("hreflang = %v", got.HreflangTags)
	}
	if !got.HasStructuredData {
		t.Error("expected structured data")
	}
	if len(got.H1s) != 1 || got.H1s[0] != "Main Heading" {
		t.Errorf("h1s = %v", got.H1s)
	}
	if len(got.HeadingOrder) != 2 || got.HeadingOrder[0] != 1 || got.HeadingOrder[1] != 3 {
		t.Errorf("headingOrder = %v", got.HeadingOrder)
	}
	// The <noscript> image and anchor are skipped as raw text.
	if len(got.Images) != 2 {
		t.Fatalf("images = %d, want 2", len(got.Images))
	}
	if got.Images[0].Alt != "A" || got.Images[1].Alt != "" {
		t.Errorf("image alts = %q/%q", got.Images[0].Alt, got.Images[1].Alt)
	}
	if len(got.Links) != 2 {
		t.Fatalf("links = %v, want 2", got.Links)
	}
	internal := findLink(got.Links, "https://example.com/internal")
	if internal == nil || !internal.IsInternal || !internal.IsNofollow || internal.Anchor != "Internal" {
		t.Errorf("internal link = %+v", internal)
	}
	external := findLink(got.Links, "https://other.com/ext")
	if external == nil || external.IsInternal {
		t.Errorf("external link = %+v", external)
	}
	if !strings.Contains(got.BodyText, "Hello world this is text.") {
		t.Errorf("bodyText = %q", got.BodyText)
	}
	if got.WordCount < 5 {
		t.Errorf("wordCount = %d", got.WordCount)
	}
	if got.JavaScriptShell {
		t.Error("expected no javascript shell")
	}
}

func findLink(links []PageLink, target string) *PageLink {
	for i := range links {
		if links[i].TargetURL == target {
			return &links[i]
		}
	}
	return nil
}

func TestAnalyzeHTMLDetectsJavaScriptShell(t *testing.T) {
	document := `<html><body><div id="root"></div><script src="/app.js"></script></body></html>`
	got := AnalyzeHTML(document, "https://example.com/", 200, 50, "")
	if !got.JavaScriptShell {
		t.Fatalf("expected a javascript shell, got %+v", got)
	}
	if got.WordCount >= 20 {
		t.Fatalf("wordCount = %d", got.WordCount)
	}
}

func TestAnalyzeHTMLIgnoresSVGTitle(t *testing.T) {
	document := `<html><body><svg><title>icon</title></svg><p>Visible</p></body></html>`
	got := AnalyzeHTML(document, "https://example.com/", 200, 10, "")
	if got.Title != "" {
		t.Fatalf("title = %q, want empty", got.Title)
	}
}

func TestAnalyzeHTMLDedupesLinksAndCapsCollections(t *testing.T) {
	var builder strings.Builder
	builder.WriteString(`<html><body>`)
	for i := 0; i < 1100; i++ {
		builder.WriteString(`<a href="/dup">dup</a>`)
	}
	builder.WriteString(`</body></html>`)
	got := AnalyzeHTML(builder.String(), "https://example.com/", 200, 10, "")
	if len(got.Links) != 1 {
		t.Fatalf("links = %d, want 1 after dedupe", len(got.Links))
	}

	builder.Reset()
	builder.WriteString(`<html><body>`)
	for i := 0; i < 1100; i++ {
		builder.WriteString(`<img src="/a.png" alt="x">`)
	}
	builder.WriteString(`</body></html>`)
	got = AnalyzeHTML(builder.String(), "https://example.com/", 200, 10, "")
	if len(got.Images) != maxExtractedImages {
		t.Fatalf("images = %d, want %d", len(got.Images), maxExtractedImages)
	}
}

func TestAnalyzeHTMLTruncatesAnchorText(t *testing.T) {
	longAnchor := strings.Repeat("word ", 100)
	document := `<html><body><a href="/x">` + longAnchor + `</a></body></html>`
	got := AnalyzeHTML(document, "https://example.com/", 200, 10, "")
	if len(got.Links) != 1 {
		t.Fatalf("links = %d", len(got.Links))
	}
	if len(got.Links[0].Anchor) > maxAnchorChars {
		t.Fatalf("anchor not truncated: %d chars", len(got.Links[0].Anchor))
	}
}

func TestAnalyzeHTMLUnicodeAndEmpty(t *testing.T) {
	got := AnalyzeHTML("", "https://example.com/", 200, 0, "")
	if got.WordCount != 0 || got.Title != "" || len(got.Links) != 0 {
		t.Fatalf("empty document produced %+v", got)
	}
	emoji := `<html><head><title>🎉 café</title></head><body><h1>Ünïcode</h1><p>naïve résumé</p></body></html>`
	result := AnalyzeHTML(emoji, "https://example.com/", 200, 0, "")
	if result.Title != "🎉 café" {
		t.Fatalf("title = %q", result.Title)
	}
	if len(result.H1s) != 1 || result.H1s[0] != "Ünïcode" {
		t.Fatalf("h1s = %v", result.H1s)
	}
}
