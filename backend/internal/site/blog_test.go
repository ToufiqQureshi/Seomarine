package site

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
)

// front builds a post's front matter from key: value lines.
func front(lines ...string) string {
	return "---\n" + strings.Join(lines, "\n") + "\n---\n"
}

const (
	goodTitle       = "title: What is AEO? Answer engine optimisation explained"
	goodDescription = "description: How to get your pages quoted by ChatGPT and Google AI Overviews, in five clear steps."
	goodDate        = "date: 2026-10-20"
)

func getPost(s *Site, slug string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/blog/"+slug, nil)
	req.SetPathValue("slug", slug)
	rec := httptest.NewRecorder()
	s.ServeBlogPost(rec, req)
	return rec
}

func TestParsePostRejectsWhatWouldHurtThePage(t *testing.T) {
	cases := map[string]struct{ slug, source string }{
		"bad file name":         {"What_Is", front(goodTitle, goodDescription, goodDate) + "Hi"},
		"no front matter":       {"post", "Hi"},
		"unclosed front matter": {"post", "---\n" + goodTitle + "\nHi"},
		"line without colon":    {"post", front("title", goodDescription, goodDate) + "Hi"},
		"no title":              {"post", front(goodDescription, goodDate) + "Hi"},
		"title too long":        {"post", front("title: "+strings.Repeat("x", 71), goodDescription, goodDate) + "Hi"},
		"description too short": {"post", front(goodTitle, "description: Short.", goodDate) + "Hi"},
		"description too long":  {"post", front(goodTitle, "description: "+strings.Repeat("x", 161), goodDate) + "Hi"},
		"bad date":              {"post", front(goodTitle, goodDescription, "date: 20/10/2026") + "Hi"},
		"bad updated":           {"post", front(goodTitle, goodDescription, goodDate, "updated: soon") + "Hi"},
		"empty body":            {"post", front(goodTitle, goodDescription, goodDate) + "  \n"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := parsePost(tc.slug, []byte(tc.source)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestParsePost(t *testing.T) {
	body := "Answer first.\n\n## Why it matters\n\n<script>alert(1)</script>\n\n" + strings.Repeat("word ", 450)
	source := strings.ReplaceAll(front(goodTitle, goodDescription, goodDate)+body, "\n", "\r\n")
	post, draft, err := parsePost("what-is-aeo", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if draft || post.Author != "Uplumo team" || post.ReadMinutes != 3 || !post.Updated.Equal(post.Published) {
		t.Fatalf("post = %+v draft %v", post, draft)
	}
	if !strings.Contains(string(post.Body), `<h2 id="why-it-matters">`) {
		t.Error("headings need ids so a section can be linked")
	}
	if strings.Contains(string(post.Body), "<script>") {
		t.Error("raw HTML must be dropped")
	}
}

func TestBlogPages(t *testing.T) {
	files := fstest.MapFS{
		"README.md":      {Data: []byte("# How to write posts")},
		"older.md":       {Data: []byte(front(goodTitle, goodDescription, "date: 2026-10-01") + "Old post.")},
		"what-is-aeo.md": {Data: []byte(front(goodTitle, goodDescription, goodDate, "updated: 2026-10-22", "author: Asha Rao") + "Answer first.")},
		"hidden.md":      {Data: []byte(front(goodTitle, goodDescription, goodDate, "draft: true") + "Not yet.")},
	}
	s, err := build(&url.URL{Scheme: "https", Host: "uplumo.com"}, files)
	if err != nil {
		t.Fatal(err)
	}

	index := get(s.ServeBlog, "/blog", nil).Body.String()
	if strings.Index(index, "/blog/what-is-aeo") > strings.Index(index, "/blog/older") || strings.Contains(index, "/blog/hidden") {
		t.Error("the index lists published posts newest first, without drafts")
	}

	page := getPost(s, "what-is-aeo").Body.String()
	for _, want := range []string{
		`<link rel="canonical" href="https://uplumo.com/blog/what-is-aeo">`,
		`<meta property="og:type" content="article">`,
		`"@type":"BlogPosting"`, `"dateModified":"2026-10-22"`, `"@type":"Person"`,
		"What is AEO? Answer engine optimisation explained | Uplumo",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("post page lacks %s", want)
		}
	}
	for _, slug := range []string{"hidden", "README", "nope"} {
		if code := getPost(s, slug).Code; code != http.StatusNotFound {
			t.Errorf("/blog/%s: status %d, want 404", slug, code)
		}
	}

	var doc struct {
		URLs []struct {
			Loc     string `xml:"loc"`
			LastMod string `xml:"lastmod"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal(get(s.ServeSitemap, "/sitemap.xml", nil).Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	lastmod := map[string]string{}
	for _, u := range doc.URLs {
		lastmod[u.Loc] = u.LastMod
	}
	if lastmod["https://uplumo.com/blog/what-is-aeo"] != "2026-10-22" || lastmod["https://uplumo.com/blog/older"] != "2026-10-01" {
		t.Errorf("sitemap posts = %v", lastmod)
	}
	if _, ok := lastmod["https://uplumo.com/blog/hidden"]; ok {
		t.Error("a draft must not be in the sitemap")
	}
}

func TestABrokenPostStopsTheSite(t *testing.T) {
	files := fstest.MapFS{"bad.md": {Data: []byte(front("title: x") + "Hi")}}
	if _, err := build(&url.URL{Scheme: "https", Host: "uplumo.com"}, files); err == nil || !strings.Contains(err.Error(), "bad.md") {
		t.Fatalf("err = %v, want the broken file named", err)
	}
}
