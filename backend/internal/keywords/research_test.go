package keywords

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
)

type fakeData struct {
	mu        sync.Mutex
	labs      []LabsRequest
	overviews [][]string
	ideas     int
	volumes   []AdsVolumeRequest
	serps     []SerpRequest

	labsFn     func(LabsRequest) ([]LabsItem, error)
	overviewFn func(keywords []string) ([]LabsItem, error)
	ideasFn    func() ([]AdsItem, error)
	volumeFn   func(AdsVolumeRequest) ([]AdsItem, error)
	serpFn     func(SerpRequest) ([]SerpItem, error)
}

func (f *fakeData) LabsKeywords(_ context.Context, _ string, req LabsRequest) ([]LabsItem, error) {
	f.mu.Lock()
	f.labs = append(f.labs, req)
	f.mu.Unlock()
	if f.labsFn != nil {
		return f.labsFn(req)
	}
	return nil, nil
}

func (f *fakeData) LabsOverview(_ context.Context, _ string, keywords []string, _ int, _ string, _ bool) ([]LabsItem, error) {
	f.mu.Lock()
	f.overviews = append(f.overviews, keywords)
	f.mu.Unlock()
	if f.overviewFn != nil {
		return f.overviewFn(keywords)
	}
	return nil, nil
}

func (f *fakeData) AdsIdeas(context.Context, string, string, int, string, int) ([]AdsItem, error) {
	f.mu.Lock()
	f.ideas++
	f.mu.Unlock()
	if f.ideasFn != nil {
		return f.ideasFn()
	}
	return nil, nil
}

func (f *fakeData) AdsVolume(_ context.Context, _ string, req AdsVolumeRequest) ([]AdsItem, error) {
	f.mu.Lock()
	f.volumes = append(f.volumes, req)
	f.mu.Unlock()
	if f.volumeFn != nil {
		return f.volumeFn(req)
	}
	return nil, nil
}

func (f *fakeData) SerpLive(_ context.Context, _ string, req SerpRequest) ([]SerpItem, error) {
	f.mu.Lock()
	f.serps = append(f.serps, req)
	f.mu.Unlock()
	if f.serpFn != nil {
		return f.serpFn(req)
	}
	return nil, nil
}

func (f *fakeData) labsCalls(source LabsSource) []LabsRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []LabsRequest
	for _, r := range f.labs {
		if r.Source == source {
			out = append(out, r)
		}
	}
	return out
}

type fakeRegistry struct {
	names []string
	err   error
	calls int
}

func (r *fakeRegistry) Locations(_ context.Context, _, _ string) ([]SerpLocation, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	out := make([]SerpLocation, len(r.names))
	for i, n := range r.names {
		out[i] = SerpLocation{LocationName: n}
	}
	return out, nil
}

type researchFixture struct {
	savedFixture
	svc      *ResearchService
	data     *fakeData
	cache    *memoryLocationCache
	registry *fakeRegistry
}

func newResearchFixture(t *testing.T) researchFixture {
	t.Helper()
	f := newSavedFixture(t)
	data, cache, registry := &fakeData{}, &memoryLocationCache{}, &fakeRegistry{names: []string{"Pune,Maharashtra,India"}}
	svc := &ResearchService{Data: data, Cache: cache, Registry: registry, Metrics: SavedRepository{DB: f.pool}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return researchFixture{savedFixture: f, svc: svc, data: data, cache: cache, registry: registry}
}

func (f researchFixture) research(t *testing.T, mutate func(*ResearchInput)) (ResearchResult, error) {
	t.Helper()
	in := ResearchInput{OrganizationID: "org", ProjectID: f.project, Keywords: []string{"Seed"}, LocationCode: 2840, LanguageCode: "en", ResultLimit: 150, Mode: "auto"}
	if mutate != nil {
		mutate(&in)
	}
	return f.svc.Research(context.Background(), in)
}

func labsItem(keyword string, volume int) LabsItem {
	d, intent := 30, "Commercial"
	return LabsItem{Keyword: keyword, Info: Info{SearchVolume: &volume, CPC: floatp(1.1), Competition: floatp(0.5), Monthly: []MonthlySearch{{Year: 2026, Month: 9, SearchVolume: volume}}},
		Difficulty: &d, MainIntent: &intent}
}

func items(prefix string, n int) []LabsItem {
	out := make([]LabsItem, n)
	for i := range out {
		out[i] = labsItem(fmt.Sprintf("%s %d", prefix, i), 100-i)
	}
	return out
}

func rowWords(rows []ResearchRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Keyword
	}
	return out
}

func TestRowMapping(t *testing.T) {
	zero, five := 0, 5
	rows := labsRows([]LabsItem{
		{Keyword: "  Alpha  ", Info: Info{SearchVolume: &five, CPC: floatp(2)}, Clickstream: &Info{SearchVolume: intp(50), Monthly: []MonthlySearch{{Year: 2026, Month: 1, SearchVolume: 50}}}, MainIntent: strp("Informational query")},
		{Keyword: "alpha", Info: Info{SearchVolume: intp(999)}},
		{Keyword: "beta", Info: Info{SearchVolume: &five}, Clickstream: &Info{SearchVolume: &zero}},
		{Keyword: "  "},
		{Keyword: "gamma", MainIntent: strp("navigational")},
	})
	if len(rows) != 3 || rows[0].Keyword != "alpha" {
		t.Fatalf("rows = %+v, want deduped and normalized", rows)
	}
	if *rows[0].SearchVolume != 50 || len(rows[0].Trend) != 1 || *rows[0].CPC != 2 || rows[0].Intent != "informational" {
		t.Errorf("alpha = %+v, want the clickstream volume and trend with the plain CPC", rows[0])
	}
	if *rows[1].SearchVolume != 5 || rows[1].Trend == nil {
		t.Errorf("beta = %+v, want a zero clickstream volume ignored", rows[1])
	}
	if rows[2].Intent != "navigational" || rows[2].SearchVolume != nil || rows[2].Trend == nil {
		t.Errorf("gamma = %+v", rows[2])
	}

	ads := adsRows([]AdsItem{{Keyword: "Delta", SearchVolume: intp(70), CompetitionIndex: floatp(55)}, {Keyword: "delta"}, {Keyword: "eps"}})
	if len(ads) != 2 || *ads[0].Competition != 0.55 || ads[0].KeywordDifficulty != nil || ads[0].Intent != "unknown" || ads[1].Competition != nil {
		t.Errorf("ads rows = %+v, want the 0-100 index scaled to a ratio, no difficulty", ads)
	}
	for in, want := range map[string]string{"commercial": "commercial", "TRANSACTIONAL": "transactional", "x": "unknown"} {
		if got := normalizeIntent(strp(in)); got != want {
			t.Errorf("normalizeIntent(%q) = %q, want %q", in, got, want)
		}
	}
	if normalizeIntent(nil) != "unknown" {
		t.Error("a missing intent must be unknown")
	}
}

func TestInterleave(t *testing.T) {
	row := func(k string) ResearchRow { return ResearchRow{Keyword: k} }
	a := []ResearchRow{row("a1"), row("a2"), row("shared"), row("a4")}
	b := []ResearchRow{row("b1"), row("shared"), row("b3")}
	if got := rowWords(interleave(a, b, 10)); !slices.Equal(got, []string{"a1", "b1", "a2", "shared", "b3", "a4"}) {
		t.Errorf("interleave = %q, want alternating rows without duplicates", got)
	}
	if got := rowWords(interleave(a, b, 3)); !slices.Equal(got, []string{"a1", "b1", "a2"}) {
		t.Errorf("limited = %q", got)
	}
	if got := interleave(nil, nil, 5); got == nil || len(got) != 0 {
		t.Errorf("empty = %v, want an empty non-nil list", got)
	}
}

func TestResearchAutoBlendsSuggestionsAndIdeas(t *testing.T) {
	f := newResearchFixture(t)
	f.data.labsFn = func(req LabsRequest) ([]LabsItem, error) {
		if req.Source == SourceSuggestions {
			return items("sug", 4), nil
		}
		return items("idea", 4), nil
	}
	got, err := f.research(t, func(in *ResearchInput) { in.ResultLimit = 300 })
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "blended" || got.UsedFallback || len(got.Rows) != 8 || got.Rows[0].Keyword != "sug 0" || got.Rows[1].Keyword != "idea 0" {
		t.Errorf("result = %v source %s fallback %v", rowWords(got.Rows), got.Source, got.UsedFallback)
	}
	for _, source := range []LabsSource{SourceSuggestions, SourceIdeas} {
		calls := f.data.labsCalls(source)
		if len(calls) != 1 || calls[0].Limit != 150 || calls[0].Seed != "seed" || !calls[0].IgnoreSynonyms {
			t.Errorf("%s calls = %+v, want one call with half the limit, the normalized seed and synonyms ignored", source, calls)
		}
	}
	if len(f.data.labsCalls(SourceRelated)) != 0 {
		t.Error("related ran although the blend had enough rows")
	}

	if _, err := f.research(t, func(in *ResearchInput) { in.ResultLimit = 150; in.Keywords = []string{"other"} }); err != nil {
		t.Fatal(err)
	}
	if got := f.data.labsCalls(SourceSuggestions); got[len(got)-1].Limit != 75 {
		t.Errorf("limit for 150 = %d, want 75", got[len(got)-1].Limit)
	}
}

func TestResearchAutoFallsBackToRelated(t *testing.T) {
	f := newResearchFixture(t)
	f.data.labsFn = func(req LabsRequest) ([]LabsItem, error) {
		switch req.Source {
		case SourceSuggestions:
			return []LabsItem{labsItem("seed", 10), labsItem("only one", 5)}, nil
		case SourceIdeas:
			return []LabsItem{labsItem("idea", 3)}, nil
		}
		return items("rel", 6), nil
	}
	got, err := f.research(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.UsedFallback || got.Source != "blended" || len(got.Rows) != 2+1+6 {
		t.Errorf("result = %v fallback %v, want related blended in", rowWords(got.Rows), got.UsedFallback)
	}
	if related := f.data.labsCalls(SourceRelated); len(related) != 1 || related[0].Limit != 150 {
		t.Errorf("related calls = %+v, want one at the full limit", related)
	}
}

func TestResearchAutoLegErrors(t *testing.T) {
	boom, boom2 := errors.New("suggestions down"), errors.New("ideas down")
	f := newResearchFixture(t)
	f.data.labsFn = func(req LabsRequest) ([]LabsItem, error) {
		if req.Source == SourceSuggestions {
			return nil, boom
		}
		return nil, boom2
	}
	if _, err := f.research(t, nil); !errors.Is(err, boom) {
		t.Errorf("both legs failing returned %v, want the suggestions error", err)
	}
	if len(f.data.labsCalls(SourceIdeas)) != 1 {
		t.Error("the ideas leg did not run to completion; its spend would be unaccounted for")
	}
	g := newResearchFixture(t)
	g.data.labsFn = func(req LabsRequest) ([]LabsItem, error) {
		if req.Source == SourceIdeas {
			return nil, boom2
		}
		return items("sug", 6), nil
	}
	if _, err := g.research(t, nil); !errors.Is(err, boom2) {
		t.Errorf("ideas failing returned %v, want the ideas error", err)
	}
	if g.cache.size() != 0 {
		t.Error("a failed research was cached")
	}
}

func TestResearchManualModesAndGoogleAds(t *testing.T) {
	f := newResearchFixture(t)
	f.data.labsFn = func(LabsRequest) ([]LabsItem, error) { return items("kw", 3), nil }
	for _, mode := range []string{"related", "suggestions", "ideas"} {
		got, err := f.research(t, func(in *ResearchInput) { in.Mode = mode; in.Clickstream = true; in.GroupKeywords = true })
		if err != nil || got.Source != mode || len(got.Rows) != 3 {
			t.Fatalf("%s = %+v, %v", mode, got, err)
		}
		call := f.data.labsCalls(LabsSource(mode))[0]
		if call.Limit != 150 || !call.Clickstream || call.IgnoreSynonyms {
			t.Errorf("%s request = %+v, want the full limit, clickstream on, synonyms kept", mode, call)
		}
	}

	// Andorra is served by Google Ads only: modes and Labs-only flags do not apply.
	ads := newResearchFixture(t)
	ads.data.ideasFn = func() ([]AdsItem, error) {
		return []AdsItem{{Keyword: "idea", SearchVolume: intp(40), CompetitionIndex: floatp(20)}}, nil
	}
	for _, mutate := range []func(*ResearchInput){
		func(in *ResearchInput) { in.Mode = "related"; in.Clickstream = true },
		func(in *ResearchInput) { in.Mode = "ideas"; in.GroupKeywords = true },
		nil,
	} {
		got, err := ads.research(t, func(in *ResearchInput) {
			in.LocationCode, in.LanguageCode = 2020, "ca"
			if mutate != nil {
				mutate(in)
			}
		})
		if err != nil || got.Source != "google_ads" || len(got.Rows) != 1 || *got.Rows[0].Competition != 0.2 {
			t.Fatalf("google ads research = %+v, %v", got, err)
		}
	}
	if ads.data.ideas != 1 || len(ads.data.labs) != 0 {
		t.Errorf("provider calls: ideas %d, labs %d; want the three equivalent requests to share one cached call", ads.data.ideas, len(ads.data.labs))
	}
}

func TestResearchCache(t *testing.T) {
	f := newResearchFixture(t)
	f.data.labsFn = func(LabsRequest) ([]LabsItem, error) { return items("kw", 6), nil }
	calls := func() int { return len(f.data.labs) }
	if _, err := f.research(t, nil); err != nil {
		t.Fatal(err)
	}
	first := calls()
	if _, err := f.research(t, func(in *ResearchInput) { in.Keywords = []string{"  SEED "} }); err != nil || calls() != first {
		t.Errorf("an equivalent request called the provider again (%d -> %d)", first, calls())
	}
	for name, mutate := range map[string]func(*ResearchInput){
		"result limit": func(in *ResearchInput) { in.ResultLimit = 300 },
		"mode":         func(in *ResearchInput) { in.Mode = "ideas" },
		"organization": func(in *ResearchInput) { in.OrganizationID = "another-org" },
		"project":      func(in *ResearchInput) { in.ProjectID = "another-project" },
		"clickstream":  func(in *ResearchInput) { in.Clickstream = true },
		"group":        func(in *ResearchInput) { in.GroupKeywords = true },
		"another seed": func(in *ResearchInput) { in.Keywords = []string{"other"} },
		"seed order":   func(in *ResearchInput) { in.Keywords = []string{"seed", "second"} },
		"language":     func(in *ResearchInput) { in.LanguageCode = "hi" },
		"country":      func(in *ResearchInput) { in.LocationCode = 2356 },
	} {
		before := calls()
		if _, err := f.research(t, mutate); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if calls() == before {
			t.Errorf("a different %s was answered from the cache", name)
		}
	}

	// A cache entry with no rows, or one that does not decode, is not an answer.
	empty := newResearchFixture(t)
	empty.data.labsFn = func(LabsRequest) ([]LabsItem, error) { return nil, nil }
	for range 2 {
		if _, err := empty.research(t, nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(empty.data.labsCalls(SourceSuggestions)) != 2 {
		t.Error("an empty result was served from the cache; the provider's emptiness may be transient")
	}
	for key := range f.cache.values {
		f.cache.values[key] = []byte(`{"rows":"nope"}`)
	}
	before := calls()
	if _, err := f.research(t, nil); err != nil || calls() == before {
		t.Errorf("a corrupt cache entry was trusted (err %v)", err)
	}
	f.cache.failed = true
	if _, err := f.research(t, nil); err != nil {
		t.Errorf("a cache outage failed the research: %v", err)
	}
}

func TestResearchSeedsAndLanguage(t *testing.T) {
	f := newResearchFixture(t)
	f.data.labsFn = func(LabsRequest) ([]LabsItem, error) { return items("kw", 6), nil }
	if _, err := f.research(t, func(in *ResearchInput) { in.Keywords = []string{" ", ""} }); !isValidation(err) {
		t.Errorf("blank seeds error = %v, want ValidationError", err)
	}
	if _, err := f.research(t, func(in *ResearchInput) { in.Keywords = []string{" Second Seed ", "ignored"} }); err != nil {
		t.Fatal(err)
	}
	if call := f.data.labsCalls(SourceSuggestions)[0]; call.Seed != "second seed" {
		t.Errorf("seed = %q, want the first keyword, normalized", call.Seed)
	}
	// Labs serves English in the US; a Hindi request would be billed and fail.
	if _, err := f.research(t, func(in *ResearchInput) { in.LanguageCode = "hi" }); err != nil {
		t.Fatal(err)
	}
	last := f.data.labs[len(f.data.labs)-1]
	if last.LanguageCode != "en" {
		t.Errorf("language sent to Labs = %q, want the country's served language", last.LanguageCode)
	}
	var stored int
	_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM go_keyword_metrics WHERE project_id = $1 AND language_code = 'hi'`, f.project).Scan(&stored)
	if stored == 0 {
		t.Error("metrics were stored under the Labs language; they must use the requested language so they join the saved keywords")
	}
}

func TestResearchPersistsMetricsForSavedKeywords(t *testing.T) {
	f := newResearchFixture(t)
	f.data.labsFn = func(_ LabsRequest) ([]LabsItem, error) {
		return []LabsItem{labsItem("alpha", 90), labsItem("beta", 40)}, nil
	}
	if _, err := f.research(t, func(in *ResearchInput) { in.ResultLimit = 150 }); err != nil {
		t.Fatal(err)
	}
	f.save(t, func(in *SaveInput) { in.Keywords = []string{"alpha"} })
	got := f.list(t, nil).Rows[0]
	if got.SearchVolume == nil || *got.SearchVolume != 90 || got.KeywordDifficulty == nil || *got.KeywordDifficulty != 30 || got.Intent == nil || *got.Intent != "commercial" {
		t.Errorf("saved keyword = %+v, want the researched metrics", got)
	}
	// A metrics write that fails must not fail the research the user paid for.
	broken := *f.svc
	broken.Metrics = failingMetrics{}
	broken.Cache = &memoryLocationCache{}
	if _, err := broken.Research(context.Background(), ResearchInput{OrganizationID: "org", ProjectID: f.project, Keywords: []string{"seed"}, LocationCode: 2840, LanguageCode: "en", ResultLimit: 150, Mode: "auto"}); err != nil {
		t.Errorf("research failed because storing metrics did: %v", err)
	}
}

type failingMetrics struct{}

func (failingMetrics) UpsertMetrics(context.Context, string, int, string, []Metric) error {
	return errors.New("database is down")
}

func TestResearchLocal(t *testing.T) {
	f := newResearchFixture(t)
	f.data.labsFn = func(_ LabsRequest) ([]LabsItem, error) {
		return []LabsItem{labsItem("alpha", 1000), labsItem("beta", 500), labsItem("gamma", 100)}, nil
	}
	f.data.volumeFn = func(_ AdsVolumeRequest) ([]AdsItem, error) {
		return []AdsItem{{Keyword: "alpha", SearchVolume: intp(12), CPC: floatp(0.4), CompetitionIndex: floatp(10), Monthly: []MonthlySearch{{Year: 2026, Month: 9, SearchVolume: 12}}}}, nil
	}
	city := "Pune,Maharashtra,India"
	got, err := f.research(t, func(in *ResearchInput) {
		in.LocationCode, in.LanguageCode, in.LocationName, in.Clickstream = 2356, "en", &city, true
	})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]ResearchRow{}
	for _, r := range got.Rows {
		by[r.Keyword] = r
	}
	if *by["alpha"].SearchVolume != 12 || *by["alpha"].Competition != 0.1 || len(by["alpha"].Trend) != 1 || *by["alpha"].KeywordDifficulty != 30 || by["alpha"].Intent != "commercial" {
		t.Errorf("alpha = %+v, want local volume with national difficulty and intent", by["alpha"])
	}
	if b := by["beta"]; b.SearchVolume != nil || b.CPC != nil || b.Competition != nil || len(b.Trend) != 0 || *b.KeywordDifficulty != 30 {
		t.Errorf("beta = %+v, want empty local metrics but the national difficulty, never national volume", b)
	}
	if call := f.data.labs[0]; call.Clickstream {
		t.Error("a local request paid for clickstream volumes that local volume replaces")
	}
	vol := f.data.volumes[0]
	if vol.LocationName == nil || *vol.LocationName != city || len(vol.Keywords) != 3 {
		t.Errorf("volume request = %+v", vol)
	}
	// National metrics are stored so a save from local results still has them; the local ones are not.
	var volume *int
	if err := f.pool.QueryRow(context.Background(), `SELECT search_volume FROM go_keyword_metrics WHERE project_id = $1 AND keyword = 'alpha' AND location_code = 2356`, f.project).Scan(&volume); err != nil || volume == nil || *volume != 1000 {
		t.Errorf("stored alpha volume = %v (%v), want the national 1000", volume, err)
	}

	f.data.mu.Lock()
	f.data.labs, f.data.volumes = nil, nil
	f.data.mu.Unlock()
	if _, err := f.research(t, func(in *ResearchInput) { in.LocationCode, in.LanguageCode, in.LocationName = 2356, "en", &city }); err != nil {
		t.Fatal(err)
	}
	if len(f.data.volumes) != 0 || len(f.data.labs) != 0 {
		t.Error("a repeated local research called the provider; both layers should be cached")
	}
}

func TestResearchLocalRefusesUnknownPlaces(t *testing.T) {
	f := newResearchFixture(t)
	f.data.labsFn = func(LabsRequest) ([]LabsItem, error) { return items("kw", 6), nil }
	for _, name := range []string{"Catonsville, MD", "560001", "pune,maharashtra,india"} {
		_, err := f.research(t, func(in *ResearchInput) { in.LocationCode, in.LocationName = 2356, &name })
		var unknown UnknownLocationError
		if !errors.As(err, &unknown) || !strings.Contains(err.Error(), "India") {
			t.Errorf("%q error = %v, want UnknownLocationError naming the country", name, err)
		}
	}
	if len(f.data.labs) != 0 || len(f.data.volumes) != 0 {
		t.Error("an unknown place reached the paid provider")
	}
	f.registry.err = errors.New("registry down")
	city := "Pune,Maharashtra,India"
	if _, err := f.research(t, func(in *ResearchInput) { in.LocationCode, in.LocationName = 2356, &city }); err == nil || len(f.data.labs) != 0 {
		t.Errorf("a registry outage error = %v, provider calls %d; want a refusal before any spend", err, len(f.data.labs))
	}
}

func TestResearchAutoNeedsFiveNonSeedRows(t *testing.T) {
	f := newResearchFixture(t)
	f.data.labsFn = func(req LabsRequest) ([]LabsItem, error) {
		if req.Source == SourceSuggestions {
			return append([]LabsItem{labsItem("seed", 9)}, items("sug", 3)...), nil
		}
		return items("idea", 2), nil
	}
	got, err := f.research(t, nil)
	if err != nil || got.UsedFallback || len(f.data.labsCalls(SourceRelated)) != 0 {
		t.Errorf("exactly 5 non-seed rows: fallback %v, related calls %d; want the blend kept (5 is enough)", got.UsedFallback, len(f.data.labsCalls(SourceRelated)))
	}
}

func TestResearchCacheEntryWithAnUnknownSourceIsIgnored(t *testing.T) {
	f := newResearchFixture(t)
	f.data.labsFn = func(LabsRequest) ([]LabsItem, error) { return items("kw", 6), nil }
	if _, err := f.research(t, nil); err != nil {
		t.Fatal(err)
	}
	for key := range f.cache.values {
		f.cache.values[key] = []byte(`{"rows":[{"keyword":"stale","trend":[],"intent":"unknown"}],"source":"from_the_future","usedFallback":false}`)
	}
	before := len(f.data.labs)
	got, err := f.research(t, nil)
	if err != nil || len(f.data.labs) == before || slices.Contains(rowWords(got.Rows), "stale") {
		t.Errorf("an entry with an unknown source was served (err %v, rows %v)", err, rowWords(got.Rows))
	}
}

func TestResearchLocalStoresNationalMetricsEvenFromCache(t *testing.T) {
	f := newResearchFixture(t)
	f.data.labsFn = func(LabsRequest) ([]LabsItem, error) { return []LabsItem{labsItem("alpha", 1000)}, nil }
	if _, err := f.research(t, func(in *ResearchInput) { in.LocationCode = 2356 }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM go_keyword_metrics WHERE project_id = $1`, f.project); err != nil {
		t.Fatal(err)
	}
	f.data.volumeFn = func(AdsVolumeRequest) ([]AdsItem, error) {
		return []AdsItem{{Keyword: "alpha", SearchVolume: intp(3)}}, nil
	}
	city := "Pune,Maharashtra,India"
	if _, err := f.research(t, func(in *ResearchInput) { in.LocationCode, in.LocationName = 2356, &city }); err != nil {
		t.Fatal(err)
	}
	var volume *int
	if err := f.pool.QueryRow(context.Background(), `SELECT search_volume FROM go_keyword_metrics WHERE project_id = $1 AND keyword = 'alpha'`, f.project).Scan(&volume); err != nil || volume == nil || *volume != 1000 {
		t.Errorf("stored volume = %v (%v), want the national 1000 re-stored from the cached national result, not the city's 3", volume, err)
	}
}

func TestResearchAutoDoesNotCountTheSeed(t *testing.T) {
	f := newResearchFixture(t)
	f.data.labsFn = func(req LabsRequest) ([]LabsItem, error) {
		switch req.Source {
		case SourceSuggestions:
			return append([]LabsItem{labsItem("seed", 9)}, items("sug", 3)...), nil
		case SourceIdeas:
			return items("idea", 1), nil
		}
		return items("rel", 4), nil
	}
	got, err := f.research(t, nil)
	if err != nil || !got.UsedFallback {
		t.Errorf("seed plus 4 other rows: fallback %v (%v); want the fallback, because the seed does not count", got.UsedFallback, err)
	}
}
