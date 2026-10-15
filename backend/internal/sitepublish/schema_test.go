package sitepublish

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The JSON-LD builders are the whole free-schema feature: they must match
// the visible text exactly, escape out of script tags, and keep Devanagari
// and other Unicode intact.

func mustParse(t *testing.T, payload string) map[string]any {
	t.Helper()
	if !json.Valid([]byte(payload)) {
		t.Fatalf("payload is not valid JSON: %s", payload)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return parsed
}

func TestArticleSchemaCarriesTheDraft(t *testing.T) {
	reviewed := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	published := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	payload, why := articleSchema(schemaInput{
		Title: "टेक्निकल SEO guide", Meta: "A guide", Content: "body",
		Image: "https://site/wp/img.jpg", HasAuthor: true,
		Author:   AuthorInfo{Name: "Aman Sharma", Role: "SEO Writer", Bio: "<script>evil</script>"},
		Reviewed: &reviewed, PostLink: "https://site/2026/post", Published: &published,
	}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if why != "" {
		t.Fatalf("unexpected skip: %s", why)
	}
	parsed := mustParse(t, payload)
	if parsed["@type"] != "BlogPosting" || parsed["headline"] != "टेक्निकल SEO guide" {
		t.Fatalf("unexpected posting: %v", parsed)
	}
	if parsed["dateModified"] != "2026-10-01" || parsed["datePublished"] != "2026-10-10" {
		t.Fatalf("dates: modified=%v published=%v", parsed["dateModified"], parsed["datePublished"])
	}
	author, ok := parsed["author"].(map[string]any)
	if !ok || author["name"] != "Aman Sharma" || author["jobTitle"] != "SEO Writer" {
		t.Fatalf("author node: %v", parsed["author"])
	}
	// The bio's script attempt must be escaped in the serialized payload,
	// never a literal markup-bearing string: the site embeds it in HTML.
	if strings.Contains(payload, "<") {
		t.Fatalf("payload leaked raw HTML: %s", payload)
	}
	page, _ := parsed["mainEntityOfPage"].(map[string]any)
	if !ok || page["@id"] != "https://site/2026/post" {
		t.Fatalf("mainEntityOfPage: %v", parsed["mainEntityOfPage"])
	}
}

func TestExtractFAQ(t *testing.T) {
	withFAQ := "## FAQ\n### How often is technical SEO reviewed?\nReview it quarterly.\n### सुझाव कब देखें?\nहर तीन महीने में।\n"
	answers, why := extractFAQ(withFAQ)
	if why != "" {
		t.Fatalf("unexpected skip: %s", why)
	}
	if len(answers) != 2 {
		t.Fatalf("expected 2 answers, got %d", len(answers))
	}
	if answers[0].Question != "How often is technical SEO reviewed?" || answers[0].Answer != "Review it quarterly." {
		t.Fatalf("first answer: %+v", answers[0])
	}
	if answers[1].Question != "सुझाव कब देखें?" || answers[1].Answer != "हर तीन महीने में।" {
		t.Fatalf("Devanagari answer lost text: %+v", answers[1])
	}
	matches := []string{"FAQ", "Frequently asked questions", "faq", "Frequently Asked Questions"}
	for _, match := range matches {
		if _, why := extractFAQ("## " + match + "\n### Q?\nA.\n"); why != "" {
			t.Fatalf("%q: unexpected skip: %s", match, why)
		}
	}
}

func TestExtractFAQFailsClosed(t *testing.T) {
	tests := map[string]string{
		// No FAQ section at all: nothing to build, no note.
		"no section": "## How do I improve?\nDo things.\n",
		// A question with no answer under it must not become a bare FAQPage.
		"question without answer": "## FAQ\n### Unanswered question?\n",
		//Formatted answers cannot be copied exactly, so Google would flag it.
		"formatted answer": "## FAQ\n### Q?\n- one\n- two\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			answers, why := extractFAQ(content)
			if name == "no section" {
				if len(answers) != 0 || why != "" {
					t.Fatalf("no-section produced %v %q", answers, why)
				}
				return
			}
			if len(answers) != 0 || why == "" {
				t.Fatalf("expected skip, got answers=%v why=%q", answers, why)
			}
		})
	}
}

func TestSchemaScriptEscapesScriptClose(t *testing.T) {
	payload, err := marshalSchema(map[string]any{"headline": strings.Repeat("</script>", 2)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	tag := schemaScriptTag(payload)
	// The literal </script> inside the JSON must be escaped, or a hostile
	// draft title could terminate the tag early and inject markup.
	mid := tag[len(`<script type="application/ld+json">`):strings.Index(tag, `</script>`)]
	if strings.Contains(strings.ToLower(mid), "</script") {
		t.Fatalf("payload holds an unescaped script close: %s", payload)
	}
}

func TestSchemaBuildFAQOnlyWithSection(t *testing.T) {
	build := buildSchemas(schemaInput{Title: "G", Content: "No questions here"}, SchemaOptions{Article: true, FAQ: true})
	// Without an FAQ section the only block is the Article one; nothing is
	// implied about a section the article does not have.
	if len(build.Blocks) != 1 || !strings.Contains(build.Blocks[0], "BlogPosting") {
		t.Fatalf("unexpected blocks: %v", build.Blocks)
	}
	if build.Skipped != "" {
		t.Fatalf("no-section is not a skip worth a note: %q", build.Skipped)
	}
}

func TestBylineMarkdownEscapesAuthorText(t *testing.T) {
	reviewed := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	byline := bylineMarkdown(schemaInput{
		HasAuthor: true, Author: AuthorInfo{Name: "A [link](https://x) *and* `code`", Role: "|admin|"},
		Reviewed: &reviewed,
	}, reviewed)
	// The byline renders as italic text only; every Markdown control
	// character is escaped so the name cannot inject markup.
	if !strings.HasPrefix(byline, "\n\n_Written by A \\[link\\]") {
		t.Fatalf("unexpected byline: %q", byline)
	}
	if strings.Contains(byline, "*and*") || strings.Contains(byline, "`code`") || strings.Contains(byline, "|admin|") {
		t.Fatalf("byline carried unescaped markup: %q", byline)
	}
	if !strings.Contains(byline, "Updated 5 Jan 2026") {
		t.Fatalf("byline missing date: %q", byline)
	}
}

func TestPublishAddsSchemaWhenWordPressAllows(t *testing.T) {
	t.Parallel()
	wp := newFakeWordPress(t)
	wp.unfiltered = true
	drafts := memoryDrafts{"draft-1": {
		ID: "draft-1", ProjectID: "project-1", Title: "Technical SEO guide",
		MetaDescription: "A guide.", Content: "# Intro\n\n## FAQ\n### How often?\nReview it quarterly.\n",
		TargetKeyword: "technical SEO",
	}}
	service, _ := newTestService(t, wp, drafts)
	connect(t, service, wp, "project-1")
	_, info, err := service.Publish(t.Context(), "project-1", "draft-1", StatusDraft, SchemaOptions{Article: true, FAQ: true})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !info.Added {
		t.Fatalf("schema not added: %+v", info)
	}
	posts, _, _ := wp.snapshot()
	schemas := 0
	for _, post := range posts {
		schemas += strings.Count(post.Content, `type="application/ld+json"`)
	}
	// Article + FAQPage, sent in one follow-up save.
	if schemas == 0 {
		t.Fatal("no JSON-LD script in the saved post")
	}
}

func TestPublishFallsBackWhenWordPressStripsScripts(t *testing.T) {
	t.Parallel()
	wp := newFakeWordPress(t)
	wp.unfiltered = false
	drafts := memoryDrafts{"draft-1": {
		ID: "draft-1", ProjectID: "project-1", Title: "Technical SEO guide",
		Content: "# Intro", TargetKeyword: "technical SEO",
	}}
	service, _ := newTestService(t, wp, drafts)
	connect(t, service, wp, "project-1")
	_, info, err := service.Publish(t.Context(), "project-1", "draft-1", StatusDraft, SchemaOptions{Article: true})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if info.Added {
		t.Fatal("schema added although the user lacks unfiltered_html")
	}
	if !strings.Contains(info.Note, "cannot keep scripts") {
		t.Fatalf("note should say the user cannot keep scripts: %q", info.Note)
	}
	posts, _, _ := wp.snapshot()
	for _, post := range posts {
		if strings.Contains(post.Content, "application/ld+json") {
			t.Fatal("a post without the capability got a script tag")
		}
	}
}
