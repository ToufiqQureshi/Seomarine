package aisearch

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func group(platform string, mentions, volume float64) groupElement {
	return groupElement{Key: ptr(platform), Mentions: ptr(mentions), AISearchVolume: ptr(volume)}
}

func platformOK(platform string, mentions, volume float64) platformOutcome {
	return platformOutcome{platform: platform, ok: true, bundle: platformBundle{
		aggregated: aggregatedTotal{Platform: []groupElement{group(platform, mentions, volume)}},
		complete:   true,
	}}
}

func crossRow(key, platform string, mentions *float64) crossItem {
	return crossItem{Key: ptr(key), Platform: []groupElement{{Key: ptr(platform), Mentions: mentions}}}
}

func baseArgs() shapeArgs {
	return shapeArgs{
		query:        "acme",
		detected:     Target{Type: TargetKeyword, Value: "acme"},
		platforms:    []platformOutcome{platformOK(platformChatGPT, 10, 100), platformOK(platformGoogle, 5, 50)},
		locationCode: 2840,
		languageCode: "en",
		now:          time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC),
	}
}

func TestShapeResultLeavesChatGPTOutOutsideUSEnglish(t *testing.T) {
	args := baseArgs()
	args.locationCode = 2826
	args.competitorKeys = []string{"rival"}
	args.cross = []crossOutcome{
		{platform: platformChatGPT, ok: true, items: []crossItem{
			crossRow("acme", platformChatGPT, ptr(90.0)), crossRow("rival", platformChatGPT, ptr(10.0)),
		}},
		{platform: platformGoogle, ok: true, items: []crossItem{
			crossRow("acme", platformGoogle, ptr(10.0)), crossRow("rival", platformGoogle, ptr(30.0)),
		}},
	}

	result := shapeResult(args)

	if got := valueOr(result.TotalMentions, -1); got != 5 {
		t.Errorf("TotalMentions = %d, want 5 (Google only)", got)
	}
	if !reflect.DeepEqual(result.ShareOfVoice.Platforms, []string{platformGoogle}) {
		t.Errorf("ShareOfVoice.Platforms = %v, want [google]", result.ShareOfVoice.Platforms)
	}
	if first := result.ShareOfVoice.Entries[0]; first.Label != "rival" || *first.SharePct != 75 {
		t.Errorf("first entry = %+v, want rival with 75%%", first)
	}
}

func TestShapeResultKeepsChatGPTForUSEnglishWithRegionalLanguageTag(t *testing.T) {
	args := baseArgs()
	args.languageCode = "EN_us"
	if got := valueOr(shapeResult(args).TotalMentions, -1); got != 15 {
		t.Errorf("TotalMentions = %d, want 15 (both platforms)", got)
	}
}

func TestShapeResultCapsLongTextAndDerivesDomainFromURL(t *testing.T) {
	args := baseArgs()
	args.platforms = []platformOutcome{{platform: platformGoogle, ok: true, bundle: platformBundle{
		mentions: []mentionItem{{
			Question:       ptr(strings.Repeat("q", 600)),
			AISearchVolume: ptr(100.0),
			// The provider's own domain field is ignored: it can disagree with the URL.
			Sources:       []mentionSource{{URL: ptr("https://evil.example/path"), Title: ptr(strings.Repeat("x", 400))}},
			BrandEntities: []brandEntity{{Title: ptr(strings.Repeat("b", 300))}},
		}},
		complete: true,
	}}}

	query := shapeResult(args).TopQueries[0]

	if n := utf16Len(query.Question); n != 500 {
		t.Errorf("question length = %d, want 500", n)
	}
	source := query.CitedSources[0]
	if source.URL != "https://evil.example/path" || *source.Domain != "evil.example" || len(*source.Title) != 300 {
		t.Errorf("cited source = %+v, want evil.example with a 300 character title", source)
	}
	if n := len(query.BrandsMentioned[0]); n != 200 {
		t.Errorf("brand length = %d, want 200", n)
	}
}

func TestShapeResultKeepsOnlyInScopePagesUnderSubfolderScope(t *testing.T) {
	target, err := parseResearchTarget("acme.com/blog", ScopeSubfolder)
	if err != nil {
		t.Fatal(err)
	}
	page := func(u string) topPagesItem {
		return topPagesItem{Key: ptr(u), Platform: []groupElement{group(platformGoogle, 2, 20)}}
	}
	mention := func(question, source string) mentionItem {
		return mentionItem{Question: ptr(question), AISearchVolume: ptr(10.0), Sources: []mentionSource{{URL: ptr(source)}}}
	}
	args := baseArgs()
	args.detected = Target{Type: TargetDomain, Value: "acme.com"}
	args.research = &target
	args.platforms = []platformOutcome{{platform: platformGoogle, ok: true, bundle: platformBundle{
		aggregated: aggregatedTotal{Platform: []groupElement{group(platformGoogle, 12, 120)}},
		topPages: []topPagesItem{
			page("https://acme.com/blog/post"),
			page("https://acme.com/blogging"), // look-alike sibling
			page("https://other.example/review"),
		},
		mentions: []mentionItem{
			mention("in scope", "https://acme.com/blog/post"),
			mention("out of scope", "https://acme.com/pricing"),
		},
		complete: true,
	}}}

	result := shapeResult(args)

	if len(result.TopPages) != 1 || result.TopPages[0].URL != "https://acme.com/blog/post" {
		t.Errorf("TopPages = %+v, want only /blog/post", result.TopPages)
	}
	if len(result.TopQueries) != 1 || result.TopQueries[0].Question != "in scope" {
		t.Errorf("TopQueries = %+v, want only the in-scope prompt", result.TopQueries)
	}
	// Mentions cannot be narrowed upstream, so they stay and are flagged.
	if valueOr(result.TotalMentions, -1) != 12 || !result.AggregatesAreDomainLevel {
		t.Errorf("TotalMentions = %v, AggregatesAreDomainLevel = %v, want 12 and true", result.TotalMentions, result.AggregatesAreDomainLevel)
	}
	if result.ResolvedTarget != "acme.com/blog" || *result.Scope != ScopeSubfolder {
		t.Errorf("ResolvedTarget = %q, Scope = %v, want acme.com/blog and subfolder", result.ResolvedTarget, *result.Scope)
	}
}

func TestShapeResultFailedPlatformIsReportedNotSummed(t *testing.T) {
	args := baseArgs()
	args.platforms = []platformOutcome{{platform: platformChatGPT}, platformOK(platformGoogle, 5, 50)}

	result := shapeResult(args)

	if got := result.PerPlatform[0]; got.Status != "error" || got.Mentions != nil {
		t.Errorf("failed platform = %+v, want status error and no mentions", got)
	}
	if valueOr(result.TotalMentions, -1) != 5 {
		t.Errorf("TotalMentions = %v, want 5", result.TotalMentions)
	}
}

func TestShapeResultMonthlyVolumeKeepsLatestTwelveMonthsInOrder(t *testing.T) {
	var months []monthlySearch
	for m := 12; m >= 1; m-- { // newest first, to prove it is sorted
		months = append(months, monthlySearch{Year: 2026, Month: m, SearchVolume: ptr(10.4)})
	}
	months = append(months, monthlySearch{Year: 2025, Month: 12, SearchVolume: ptr(1.0)}, monthlySearch{Year: 2025, Month: 11})
	args := baseArgs()
	args.platforms = []platformOutcome{{platform: platformGoogle, ok: true, bundle: platformBundle{
		mentions: []mentionItem{{Question: ptr("q"), MonthlySearches: months}, {Question: ptr("q2"), MonthlySearches: months[:1]}},
		complete: true,
	}}}

	got := shapeResult(args).MonthlyVolume

	if len(got) != 12 || got[0] != (MonthlyVolume{Year: 2026, Month: 1, Volume: 10}) {
		t.Fatalf("MonthlyVolume = %+v, want 12 months starting 2026-01", got)
	}
	if last := got[11]; last != (MonthlyVolume{Year: 2026, Month: 12, Volume: 21}) {
		t.Errorf("last month = %+v, want 2026-12 with volume 21 (two answers summed)", last)
	}
}

func TestShapeResultHasDataFalseForEmptyProviderData(t *testing.T) {
	args := baseArgs()
	args.platforms = []platformOutcome{platformOK(platformGoogle, 0, 0)}
	if shapeResult(args).HasData {
		t.Error("HasData = true for zero mentions and no rows, want false")
	}
}

func TestShapeResultJSONUsesEmptyArraysAndNulls(t *testing.T) {
	args := baseArgs()
	args.platforms = []platformOutcome{{platform: platformGoogle}}
	raw, err := json.Marshal(shapeResult(args))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"topPages":[]`, `"topQueries":[]`, `"monthlyVolume":[]`, `"shareOfVoice":null`, `"scope":null`, `"totalMentions":null`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("JSON %s lacks %s", raw, want)
		}
	}
}

func TestComputeShareOfVoice(t *testing.T) {
	tests := []struct {
		name        string
		competitors []string
		outcomes    []crossOutcome
		wantNil     bool
		wantLabels  []string
		wantShares  []*float64
	}{
		{
			name:        "sums requested rows, excludes no-data brands and ignores unrequested rows",
			competitors: []string{"rival", "ghost"},
			outcomes: []crossOutcome{{platform: platformGoogle, ok: true, items: []crossItem{
				crossRow("acme", platformGoogle, ptr(30.0)),
				crossRow("rival", platformGoogle, ptr(10.0)),
				crossRow("ghost", platformGoogle, nil),
				crossRow("unexpected", platformGoogle, ptr(60.0)),
			}}},
			wantLabels: []string{"acme", "rival", "ghost"},
			wantShares: []*float64{ptr(75.0), ptr(25.0), nil},
		},
		{
			name:        "echoed keys match case-insensitively",
			competitors: []string{"Rival"},
			outcomes: []crossOutcome{{platform: platformGoogle, ok: true, items: []crossItem{
				crossRow("ACME", platformGoogle, ptr(1.0)), crossRow("rival", platformGoogle, ptr(3.0)),
			}}},
			wantLabels: []string{"Rival", "acme"},
			wantShares: []*float64{ptr(75.0), ptr(25.0)},
		},
		{
			name:        "known zero is counted but has no share when everyone is zero",
			competitors: []string{"rival"},
			outcomes: []crossOutcome{{platform: platformGoogle, ok: true, items: []crossItem{
				crossRow("acme", platformGoogle, ptr(0.0)), crossRow("rival", platformGoogle, ptr(0.0)),
			}}},
			wantLabels: []string{"acme", "rival"},
			wantShares: []*float64{nil, nil},
		},
		{name: "no competitors", outcomes: []crossOutcome{{platform: platformGoogle, ok: true}}, wantNil: true},
		{name: "every call failed", competitors: []string{"rival"}, outcomes: []crossOutcome{{platform: platformGoogle}}, wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeShareOfVoice(tt.outcomes, "acme", tt.competitors)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("computeShareOfVoice() = %+v, want nil", got)
				}
				return
			}
			var labels []string
			var shares []*float64
			for _, e := range got.Entries {
				labels = append(labels, e.Label)
				shares = append(shares, e.SharePct)
			}
			if !reflect.DeepEqual(labels, tt.wantLabels) || !reflect.DeepEqual(shares, tt.wantShares) {
				t.Errorf("entries = %v %v, want %v %v", labels, derefAll(shares), tt.wantLabels, derefAll(tt.wantShares))
			}
		})
	}
}

func derefAll(values []*float64) []any {
	out := make([]any, len(values))
	for i, v := range values {
		if v != nil {
			out[i] = *v
		}
	}
	return out
}

func TestResolveCompetitorGroups(t *testing.T) {
	groups := resolveCompetitorGroups("Nike", []string{"nike", "Adidas", "ADIDAS", "puma.com", "www.PUMA.com"})
	var labels []string
	for _, g := range groups {
		labels = append(labels, g.label)
	}
	if want := []string{"Adidas", "puma.com"}; !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %v, want %v", labels, want)
	}
}

func TestDeriveCitedSourcesUsesTopPagesMetricsAndDedupesExamples(t *testing.T) {
	page := func(u string, mentions, volume float64) topPagesItem {
		return topPagesItem{Key: ptr(u), Platform: []groupElement{group(platformGoogle, mentions, volume)}}
	}
	cited := func(question string, volume float64, urls ...string) mentionItem {
		item := mentionItem{Question: ptr(question), AISearchVolume: ptr(volume)}
		for _, u := range urls {
			item.Sources = append(item.Sources, mentionSource{URL: ptr(u)})
		}
		return item
	}
	sources := deriveCitedSources([]successBundle{{platform: platformGoogle, bundle: platformBundle{
		topPages: []topPagesItem{page("https://a.com/x", 9, 9000), page("https://b.com/y", 2, 1000), page("javascript:alert(1)", 99, 99999)},
		mentions: []mentionItem{
			cited("best seo tools", 1000, "https://a.com/x", "https://b.com/y"),
			cited("cheap seo", 500, "https://a.com/x", "https://a.com/x"), // one example, not two
		},
	}}}, nil)

	if len(sources) != 2 {
		t.Fatalf("got %d sources, want 2 (unsafe URL dropped)", len(sources))
	}
	first := sources[0]
	if *first.Domain != "a.com" || *first.Mentions != 9 || *first.CapturedVolume != 9000 {
		t.Errorf("first source = %+v, want a.com with 9 mentions and 9000 volume", first)
	}
	var questions []string
	for _, k := range first.Keywords {
		questions = append(questions, k.Question)
	}
	if want := []string{"best seo tools", "cheap seo"}; !reflect.DeepEqual(questions, want) {
		t.Errorf("keywords = %v, want %v (by volume, once each)", questions, want)
	}
}

func TestDeriveCitedSourcesCapsEachPlatformSeparately(t *testing.T) {
	var chat, google []topPagesItem
	for i := range 15 {
		chat = append(chat, topPagesItem{Key: ptr("https://chat.example/" + string(rune('a'+i))), Platform: []groupElement{group(platformChatGPT, 1, float64(i))}})
		google = append(google, topPagesItem{Key: ptr("https://google.example/" + string(rune('a'+i))), Platform: []groupElement{group(platformGoogle, 1, float64(1000+i))}})
	}
	sources := deriveCitedSources([]successBundle{
		{platform: platformChatGPT, bundle: platformBundle{topPages: chat}},
		{platform: platformGoogle, bundle: platformBundle{topPages: google}},
	}, nil)

	counts := map[string]int{}
	for _, s := range sources {
		counts[s.Platform]++
	}
	if counts[platformChatGPT] != 10 || counts[platformGoogle] != 10 {
		t.Errorf("per-platform counts = %v, want 10 each", counts)
	}
	if sources[0].Platform != platformGoogle {
		t.Errorf("first source platform = %s, want google (highest volume)", sources[0].Platform)
	}
}

func TestTruncateCountsUTF16UnitsAndKeepsCharactersWhole(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"short text is unchanged", "héllo", 10, "héllo"},
		{"exact fit", "abc", 3, "abc"},
		{"cut", "abcdef", 3, "abc"},
		{"empty", "", 3, ""},
		{"an emoji is two units and is not split", "ab😀cd", 3, "ab"},
		{"an emoji that fits is kept", "ab😀cd", 4, "ab😀"},
		{"multi-byte BMP characters are one unit each", "日本語日本語", 4, "日本語日"},
	}
	for _, tt := range tests {
		if got := truncate(tt.in, tt.limit); got != tt.want {
			t.Errorf("%s: truncate(%q, %d) = %q, want %q", tt.name, tt.in, tt.limit, got, tt.want)
		}
	}
}
