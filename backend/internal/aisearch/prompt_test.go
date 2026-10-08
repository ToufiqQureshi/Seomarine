package aisearch

import (
	"reflect"
	"testing"
)

func TestMentionsBrand(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		brand string
		want  bool
	}{
		{"whole word", "I like Acme a lot", "acme", true},
		{"case is ignored", "ACME wins", "Acme", true},
		{"at the start and end", "acme", "acme", true},
		{"inside a longer word", "Acmeware is different", "acme", false},
		{"prefixed by letters", "the macme tool", "acme", false},
		{"next to punctuation", "(acme), really", "acme", true},
		{"digits count as word characters", "acme2 is a product", "acme", false},
		{"underscore counts as a word character", "acme_corp", "acme", false},
		{"brand ending in a symbol", "I write C++ daily", "C++", true},
		{"symbol brand must not match a longer run", "C+++ is not a language", "C++", false},
		{"ampersand brand", "AT&T announced", "AT&T", true},
		{"ampersand brand inside a longer run", "AT&&T announced", "AT&T", false},
		{"later occurrence is found after an earlier bad one", "Acmeware and Acme", "acme", true},
		{"phrase", "the best Seo Tool around", "seo tool", true},
		{"unicode neighbor is not a word character", "Acmé is not it but acme—is", "acme", true},
		{"no occurrence", "nothing here", "acme", false},
		{"empty text", "", "acme", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mentionsBrand(tt.text, tt.brand); got != tt.want {
				t.Errorf("mentionsBrand(%q, %q) = %v, want %v", tt.text, tt.brand, got, tt.want)
			}
		})
	}
}

func message(annotations ...responseAnnotation) llmResponse {
	return llmResponse{Items: []responseItem{
		// Annotations outside message items are never citations.
		{Type: ptr("reasoning"), Sections: []responseSection{{Text: ptr("thinking"), Annotations: []responseAnnotation{{Title: ptr("x"), URL: ptr("https://x.test/1")}}}}},
		{Type: ptr("message"), Sections: []responseSection{{Text: ptr("answer"), Annotations: annotations}}},
	}}
}

func TestExtractCitations(t *testing.T) {
	t.Run("keeps untyped annotations and derives the domain from the URL", func(t *testing.T) {
		got := extractCitations(message(
			responseAnnotation{Title: ptr("Town & Country"), URL: ptr("https://www.townandcountrymag.com/x")},
			responseAnnotation{Title: ptr("Stylevana"), URL: ptr("https://www.stylevana.com/y")},
		))
		if len(got) != 2 || got[0].URL != "https://www.townandcountrymag.com/x" || *got[0].Domain != "townandcountrymag.com" || *got[0].Title != "Town & Country" {
			t.Errorf("citations = %+v", got)
		}
	})
	t.Run("dedupes repeated URLs and drops unsafe ones", func(t *testing.T) {
		got := extractCitations(message(
			responseAnnotation{Title: ptr("A"), URL: ptr("https://example.com/a")},
			responseAnnotation{Title: ptr("A dup"), URL: ptr("https://example.com/a")},
			responseAnnotation{Title: ptr("evil"), URL: ptr("javascript:alert(1)")},
			responseAnnotation{Title: ptr("no url")},
		))
		if len(got) != 1 || got[0].URL != "https://example.com/a" || *got[0].Title != "A" {
			t.Errorf("citations = %+v, want only the first https://example.com/a", got)
		}
	})
	t.Run("caps the list", func(t *testing.T) {
		var annotations []responseAnnotation
		for i := range 40 {
			annotations = append(annotations, responseAnnotation{URL: ptr("https://example.com/" + string(rune('a'+i%26)) + string(rune('a'+i/26)))})
		}
		if got := extractCitations(message(annotations...)); len(got) != maxCitations {
			t.Errorf("citations = %d, want %d", len(got), maxCitations)
		}
	})
}

func TestExtractTextJoinsMessageSections(t *testing.T) {
	r := llmResponse{Items: []responseItem{
		{Type: ptr("message"), Sections: []responseSection{{Text: ptr("  first")}, {Text: ptr("")}, {Text: ptr("second  ")}}},
		{Type: ptr("reasoning"), Sections: []responseSection{{Text: ptr("hidden")}}},
		{Type: ptr("message"), Sections: []responseSection{{Text: nil}, {Text: ptr("third")}}},
	}}
	if got, want := extractText(r), "first\n\nsecond  \n\nthird"; got != want {
		t.Errorf("extractText() = %q, want %q", got, want)
	}
}

func TestWithHighlightBrand(t *testing.T) {
	answer := ModelSuccess{
		Status: "success", Model: modelChatGPT, Text: "Many tools exist.",
		Citations: []Citation{
			{URL: "https://acme.com/review", Title: ptr("Review")},
			{URL: "https://other.example/x", Title: ptr("Acme alternatives")},
			{URL: "https://third.example/y"},
		},
	}
	tests := []struct {
		name          string
		brand         string
		text          string
		wantMatched   []bool
		wantMentioned *bool
	}{
		{"no brand leaves the answer unmarked", "", "", []bool{false, false, false}, nil},
		{"a matching URL or title counts", "acme", "", []bool{true, true, false}, ptr(true)},
		{"a mention in the text counts without a citation", "tools", "", []bool{false, false, false}, ptr(true)},
		{"no mention anywhere", "zzz", "", []bool{false, false, false}, ptr(false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := answer.withHighlightBrand(tt.brand)
			var matched []bool
			for _, c := range got.Citations {
				matched = append(matched, c.MatchedBrand)
			}
			if !reflect.DeepEqual(matched, tt.wantMatched) || !reflect.DeepEqual(got.BrandMentioned, tt.wantMentioned) {
				t.Errorf("matched = %v, mentioned = %v, want %v, %v", matched, derefBool(got.BrandMentioned), tt.wantMatched, derefBool(tt.wantMentioned))
			}
		})
	}

	// The cached answer is shared between requests, so marking must not leak.
	answer.withHighlightBrand("acme")
	if answer.Citations[0].MatchedBrand || answer.BrandMentioned != nil {
		t.Error("withHighlightBrand changed the receiver's citations")
	}
}

func derefBool(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

func TestUnsupportedCountryResultNamesTheModelAndCountry(t *testing.T) {
	claude := unsupportedCountryResult(modelClaude, "VN")
	if claude.ErrorCode != "UNSUPPORTED_COUNTRY" || claude.Message !=
		"Claude doesn’t support Vietnam as a search country. Select “No country preference” above, then run again to include Claude." {
		t.Errorf("claude message = %q", claude.Message)
	}
	gemini := unsupportedCountryResult(modelGemini, "US")
	if gemini.Message != "Gemini doesn’t support country selection. Select “No country preference” above, then run again to include Gemini." {
		t.Errorf("gemini message = %q", gemini.Message)
	}
}

func TestSupportsWebSearchCountry(t *testing.T) {
	tests := []struct {
		model, country string
		want           bool
	}{
		{modelChatGPT, "BG", true},
		{modelPerplexity, "ZW", true},
		{modelChatGPT, "XX", false},
		{modelClaude, "US", true},
		{modelClaude, "BG", false},
		{modelGemini, "US", false},
		{"unknown", "US", false},
	}
	for _, tt := range tests {
		if got := supportsWebSearchCountry(tt.model, tt.country); got != tt.want {
			t.Errorf("supportsWebSearchCountry(%q, %q) = %v, want %v", tt.model, tt.country, got, tt.want)
		}
	}
	if len(webSearchCountries) != 249 {
		t.Errorf("webSearchCountries has %d entries, want 249", len(webSearchCountries))
	}
}
