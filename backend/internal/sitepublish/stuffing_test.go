package sitepublish

import (
	"errors"
	"strings"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/contenteditor"
)

// The same drafts the editor's scoring tests use: the server must agree with
// the "Publish now" block the editor shows.
func TestKeywordStuffed(t *testing.T) {
	spam := "# SEO Tools\n\n" + strings.Repeat("seo tools are the best seo tools. Buy seo tools now. ", 40)
	natural := "Technical SEO is the work that helps search engines crawl your pages. " +
		strings.Repeat("Crawling and indexing decide whether a page can appear at all. ", 30) +
		"Start technical SEO with a sitemap."
	tests := []struct {
		name, topic, body string
		want              bool
	}{
		{"400-word spam", "seo tools", spam, true},
		{"short spam", "seo tools", strings.Repeat("seo tools ", 10), true},
		{"hyphen and case still count", "seo tools", strings.Repeat("SEO-Tools. ", 10), true},
		{"natural article", "technical SEO", natural, false},
		{"seven uses is never stuffing", "seo", strings.Repeat("seo ", 7), false},
		{"no topic", "", spam, false},
		{"empty body", "seo tools", "", false},
		{"unicode topic", "होटल बुकिंग", strings.Repeat("होटल बुकिंग ", 12), true},
	}
	for _, tt := range tests {
		if got := keywordStuffed(tt.topic, tt.body); got != tt.want {
			t.Errorf("%s: keywordStuffed = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPublishNowRefusesAStuffedDraftButSendAsDraftWorks(t *testing.T) {
	wp := newFakeWordPress(t)
	stuffed := contenteditor.Draft{ID: "d", ProjectID: "p", Title: "SEO tools", TargetKeyword: "seo tools", Content: strings.Repeat("seo tools are the best seo tools. ", 40)}
	service, _ := newTestService(t, wp, memoryDrafts{"d": stuffed})
	connect(t, service, wp, "p")

	if _, _, err := service.Publish(t.Context(), "p", "d", StatusPublish, SchemaOptions{}); !errors.Is(err, ErrKeywordStuffing) {
		t.Fatalf("publish a stuffed draft: err = %v, want ErrKeywordStuffing", err)
	}
	if len(wp.posts) != 0 {
		t.Fatalf("a stuffed draft reached the site: %d posts", len(wp.posts))
	}
	if _, _, err := service.Publish(t.Context(), "p", "d", StatusDraft, SchemaOptions{Article: true, FAQ: true}); err != nil {
		t.Fatalf("send a stuffed draft as a WordPress draft: %v", err)
	}
}
