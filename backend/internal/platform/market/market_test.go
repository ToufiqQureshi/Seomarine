package market

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestTableCountsMatchTypeScript(t *testing.T) {
	source, err := os.ReadFile("../../../../src/shared/keyword-locations.ts")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	between := func(start, end string) string {
		after := strings.SplitN(text, start, 2)
		if len(after) != 2 {
			t.Fatalf("missing %q", start)
		}
		before := strings.SplitN(after[1], end, 2)
		if len(before) != 2 {
			t.Fatalf("missing %q", end)
		}
		return before[0]
	}
	cases := []struct {
		name, block, pattern string
		want                 int
	}{
		{"locations", between("export const LOCATION_OPTIONS:", "export const SERP_LANGUAGE_OPTIONS"), `\bcode:\s*\d+`, len(Locations())},
		{"languages", between("export const SERP_LANGUAGE_OPTIONS = [", "] as const;"), `\bcode:\s*"`, len(Languages())},
		{"multilingual", between("const MULTI_LANGUAGE_LOCATIONS:", "export function getLanguageOptions"), `(?m)^\s*\d+:\s*\[`, len(data.Multi)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := len(regexp.MustCompile(tc.pattern).FindAllString(tc.block, -1))
			if got != tc.want {
				t.Fatalf("TypeScript has %d rows, Go has %d", got, tc.want)
			}
		})
	}
}

func TestGeneratedTables(t *testing.T) {
	if got := len(Locations()); got != 143 {
		t.Fatalf("locations = %d, want 143", got)
	}
	if got := len(Languages()); got != 128 {
		t.Fatalf("languages = %d, want 128", got)
	}
	if got := len(data.Multi); got != 20 {
		t.Fatalf("multilingual locations = %d, want 20", got)
	}
	for _, location := range Locations() {
		if !IsSupportedLanguageCode(location.LanguageCode) {
			t.Errorf("unsupported default language for %d: %s", location.Code, location.LanguageCode)
		}
		if location.GoogleAdsOnly == IsLabsLocationCode(location.Code) {
			t.Errorf("incorrect provider for %d", location.Code)
		}
	}
}

func TestResolution(t *testing.T) {
	project := Pair{2356, "hi"}
	cases := []struct {
		name    string
		request Pair
		project Pair
		want    Pair
	}{
		{"project default", Pair{}, project, project},
		{"location override resets language", Pair{2276, ""}, project, Pair{2276, "de"}},
		{"explicit language wins", Pair{2276, "en"}, project, Pair{2276, "en"}},
		{"unsupported labs project", Pair{}, Pair{2044, "en"}, Pair{2840, "en"}},
		{"unsupported language in project", Pair{}, Pair{2356, "ru"}, Pair{2840, "en"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got Pair
			if tc.name == "unsupported labs project" || tc.name == "unsupported language in project" {
				got = ResolveLabs(tc.request, tc.project)
			} else {
				got = Resolve(tc.request, tc.project)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestLanguageAndLabels(t *testing.T) {
	if IsLanguageServedForLocation(2840, "ru") {
		t.Fatal("unsupported Labs pair accepted")
	}
	if !IsLanguageServedForLocation(2840, "es") {
		t.Fatal("supported Labs pair rejected")
	}
	if !IsLanguageServedForLocation(2044, "ru") {
		t.Fatal("Google Ads-only market should defer validation")
	}
	if got := ResolveKeywordDataLanguage(2356, "ru"); got != "en" {
		t.Fatalf("India fallback = %q", got)
	}
	if got := ISOCode(2826); got != "gb" {
		t.Fatalf("UK ISO = %q", got)
	}
	if got := FormatLocationLabel("Portland-Auburn, ME,United States", 2); got != "Portland-Auburn, ME" {
		t.Fatalf("label = %q", got)
	}
	locations := Locations()
	locations[0].Label = "changed"
	if Locations()[0].Label == "changed" {
		t.Fatal("caller changed shared table")
	}
	languages := Languages()
	languages[0].Label = "changed"
	if Languages()[0].Label == "changed" {
		t.Fatal("caller changed shared languages")
	}
	if got := KeywordDataProvider(2044); got != "google_ads" {
		t.Fatalf("provider = %q", got)
	}
	if got := KeywordDataProvider(-1); got != "labs" {
		t.Fatalf("unknown provider = %q", got)
	}
	if got := LanguageCode(-1); got != "en" {
		t.Fatalf("unknown language = %q", got)
	}
	if got := ISOCode(-1); got != "us" {
		t.Fatalf("unknown ISO = %q", got)
	}
	if got := FormatLocationLabel(" A,B ", 0); got != "A, B" {
		t.Fatalf("unlimited label = %q", got)
	}
	if got := ResolveKeywordDataLanguage(2356, "hi"); got != "hi" {
		t.Fatalf("Hindi selection = %q", got)
	}
	if _, ok := Lookup(-1); ok {
		t.Fatal("unknown location found")
	}
	if IsSupportedLanguageCode("not-a-language") {
		t.Fatal("unknown language accepted")
	}
	if got := LanguageOptions(2356); len(got) != 2 || got[0].Code != "en" || got[1].Code != "hi" {
		t.Fatalf("India languages = %+v", got)
	}
}
