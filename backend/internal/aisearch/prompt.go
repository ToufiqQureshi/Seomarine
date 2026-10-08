package aisearch

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

const (
	maxCitations     = 25
	maxFanOutQueries = 20
)

// modelLabels are the names the UI shows.
var modelLabels = map[string]string{
	modelChatGPT:    "ChatGPT",
	modelClaude:     "Claude",
	modelGemini:     "Gemini",
	modelPerplexity: "Perplexity",
}

// countryLabel is the English name of an ISO 3166-1 country code.
func countryLabel(code string) string {
	region, err := language.ParseRegion(code)
	if err != nil {
		return code
	}
	if name := display.English.Regions().Name(region); name != "" {
		return name
	}
	return code
}

// unsupportedCountryResult explains why a model cannot take the country.
func unsupportedCountryResult(model, country string) ModelError {
	label := modelLabels[model]
	limitation := fmt.Sprintf("%s doesn’t support %s as a search country.", label, countryLabel(country))
	if model == modelGemini {
		limitation = label + " doesn’t support country selection."
	}
	return ModelError{
		Status:    "error",
		Model:     model,
		ErrorCode: "UNSUPPORTED_COUNTRY",
		Message:   fmt.Sprintf("%s Select “No country preference” above, then run again to include %s.", limitation, label),
	}
}

func upstreamErrorResult(model string) ModelError {
	return ModelError{
		Status:    "error",
		Model:     model,
		ErrorCode: "UPSTREAM_ERROR",
		Message:   "This model is temporarily unavailable. Please try again.",
	}
}

// extractText joins the text sections of the answer's message items.
func extractText(r llmResponse) string {
	var parts []string
	for _, item := range r.Items {
		if derefOr(item.Type, "") != "message" {
			continue
		}
		for _, section := range item.Sections {
			if text := derefOr(section.Text, ""); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

// extractCitations lists the distinct pages the answer cites. Annotations are
// untyped title and URL pairs, so the only filter is URL safety: models can be
// coaxed into emitting javascript: payloads, and the UI renders these as links.
func extractCitations(r llmResponse) []Citation {
	citations := []Citation{}
	seen := map[string]bool{}
	for _, item := range r.Items {
		if derefOr(item.Type, "") != "message" {
			continue
		}
		for _, section := range item.Sections {
			for _, annotation := range section.Annotations {
				pageURL, ok := safeHTTPURL(derefOr(annotation.URL, ""))
				if !ok || seen[pageURL] {
					continue
				}
				seen[pageURL] = true
				citations = append(citations, Citation{URL: pageURL, Domain: safeHostname(pageURL), Title: annotation.Title})
			}
		}
	}
	return citations[:min(len(citations), maxCitations)]
}

// shapeSuccess turns a raw answer into the brand-agnostic payload that is
// cached. Brand fields are set by withHighlightBrand on every read, so one
// cache entry serves requests with different brands.
func shapeSuccess(model string, r llmResponse) ModelSuccess {
	fanOut := r.FanOutQueries
	if fanOut == nil {
		fanOut = []string{}
	}
	return ModelSuccess{
		Status:        "success",
		Model:         model,
		ModelName:     r.ModelName,
		Text:          extractText(r),
		Citations:     extractCitations(r),
		FanOutQueries: fanOut[:min(len(fanOut), maxFanOutQueries)],
		OutputTokens:  roundOrNil(r.OutputTokens),
		WebSearch:     r.WebSearch != nil && *r.WebSearch,
	}
}

// withHighlightBrand marks the citations that match brand and whether the
// answer mentions it. An empty brand leaves the answer unmarked.
func (s ModelSuccess) withHighlightBrand(brand string) ModelSuccess {
	citations := make([]Citation, len(s.Citations))
	copy(citations, s.Citations)
	s.BrandMentioned = nil
	if brand == "" {
		for i := range citations {
			citations[i].MatchedBrand = false
		}
		s.Citations = citations
		return s
	}
	needle := strings.ToLower(brand)
	matched := false
	for i, c := range citations {
		haystack := strings.ToLower(c.URL + " " + derefOr(c.Title, ""))
		citations[i].MatchedBrand = strings.Contains(haystack, needle)
		matched = matched || citations[i].MatchedBrand
	}
	mentioned := matched || mentionsBrand(s.Text, brand)
	s.Citations = citations
	s.BrandMentioned = &mentioned
	return s
}

// mentionsBrand reports whether text contains brand, ignoring case, as a whole
// word. A word boundary is only required on a side where the brand ends in a
// word character; for brands like "C++" or "AT&T" the side instead must not
// repeat the brand's own edge character, so "C++" does not match "C+++".
func mentionsBrand(text, brand string) bool {
	text, brand = strings.ToLower(text), strings.ToLower(brand)
	if brand == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(brand)
	last, _ := utf8.DecodeLastRuneInString(brand)
	for from := 0; from < len(text); {
		i := strings.Index(text[from:], brand)
		if i < 0 {
			return false
		}
		start := from + i
		end := start + len(brand)
		before, beforeSize := utf8.DecodeLastRuneInString(text[:start])
		after, afterSize := utf8.DecodeRuneInString(text[end:])
		if edgeOK(first, before, beforeSize > 0) && edgeOK(last, after, afterSize > 0) {
			return true
		}
		_, width := utf8.DecodeRuneInString(text[start:])
		from = start + width
	}
	return false
}

// edgeOK checks one side of a match: edge is the brand's character on that
// side and neighbor the text's character beside it.
func edgeOK(edge, neighbor rune, hasNeighbor bool) bool {
	if !hasNeighbor {
		return true
	}
	if isWordRune(edge) {
		return !isWordRune(neighbor)
	}
	return neighbor != edge
}

func isWordRune(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
