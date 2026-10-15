package contenteditor

import (
	"slices"
	"strings"
	"testing"
)

func TestBriefTermsRankByPagesNotByOneLongPage(t *testing.T) {
	competitors := []BriefCompetitor{
		{Text: "Channel manager software. Channel manager pricing. " + strings.Repeat("Spa resort spa resort ", 50)},
		{Text: "A channel manager syncs rates. Use a channel manager daily."},
		{Text: "Pick a channel manager. The channel manager updates OTAs."},
		{Text: "Revenue tips without the term."},
	}
	terms := briefTerms(competitors)
	index := slices.IndexFunc(terms, func(term BriefTerm) bool { return term.Term == "channel manager" })
	if index < 0 || terms[index].Domains != 3 {
		t.Fatalf("terms = %+v, want channel manager on 3 pages", terms)
	}
	if slices.ContainsFunc(terms, func(term BriefTerm) bool { return term.Term == "spa resort" }) {
		t.Fatalf("a phrase on one page must not become a term: %+v", terms)
	}
	if slices.ContainsFunc(terms, func(term BriefTerm) bool { return term.Term == "channel" || term.Term == "manager" }) {
		t.Fatalf("single words inside a chosen two-word term must be skipped: %+v", terms)
	}
	got := terms[index]
	if got.Min != 2 || got.Max != 2 || got.Min > got.Max {
		t.Fatalf("channel manager range = %d–%d, want 2–2", got.Min, got.Max)
	}
}

func TestBriefTermsSkipStopWordsNumbersAndHinglishFillers(t *testing.T) {
	competitors := []BriefCompetitor{
		{Text: "the and 2026 hai aur booking engine"},
		{Text: "the and 2026 hai aur booking engine"},
	}
	for _, term := range briefTerms(competitors) {
		if term.Term != "booking engine" && term.Term != "booking" && term.Term != "engine" {
			t.Fatalf("unexpected term %q", term.Term)
		}
	}
}

func TestBriefSubtopicsComeFromHeadingsSeveralPagesShare(t *testing.T) {
	competitors := []BriefCompetitor{
		{Headings: []string{"What is crawl budget?", "Site speed tips"}},
		{Headings: []string{"Crawl budget explained", "Our story"}},
		{Headings: []string{"Improve site speed"}},
	}
	got := briefSubtopics(competitors)
	if !slices.Contains(got, "crawl budget") || !slices.Contains(got, "site speed") || slices.Contains(got, "story") {
		t.Fatalf("subtopics = %v", got)
	}
}

func TestBriefTermsAreEmptyForNoPages(t *testing.T) {
	if got := briefTerms(nil); len(got) != 0 {
		t.Fatalf("terms = %v", got)
	}
}
