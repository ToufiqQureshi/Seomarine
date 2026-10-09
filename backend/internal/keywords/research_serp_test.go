package keywords

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func organic(rank int, domain string) SerpItem {
	g := rank
	return SerpItem{Type: "organic", RankGroup: &g, Title: "T" + domain, URL: "https://" + domain, Domain: domain, Description: "d", Etv: floatp(1.5), ReferringDomains: floatp(3)}
}

func (f researchFixture) serp(t *testing.T, mutate func(*SerpAnalysisInput)) (SerpAnalysis, error) {
	t.Helper()
	in := SerpAnalysisInput{OrganizationID: "org", ProjectID: f.project, Keyword: " Best SEO ", LocationCode: 2840, LanguageCode: "en", Depth: SerpShallowDepth}
	if mutate != nil {
		mutate(&in)
	}
	return f.svc.SerpAnalysis(context.Background(), in)
}

func TestSerpAnalysisMapsOrganicResults(t *testing.T) {
	f := newResearchFixture(t)
	absolute := 7
	f.data.serpFn = func(SerpRequest) ([]SerpItem, error) {
		return []SerpItem{organic(1, "a.com"), {Type: "people_also_ask"}, {Type: "organic", RankAbsolute: &absolute, Domain: "b.com"}, {Type: "organic"}}, nil
	}
	got, err := f.serp(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequestedKeyword != "best seo" || got.Depth != 20 || got.Reason != "" || len(got.Items) != 3 {
		t.Fatalf("result = %+v", got)
	}
	if first := got.Items[0]; first.Rank != 1 || first.Domain != "a.com" || *first.Etv != 1.5 || *first.ReferringDomains != 3 || first.Backlinks != nil || first.IsNew || first.RankChange != nil {
		t.Errorf("first = %+v", first)
	}
	if got.Items[1].Rank != 7 || got.Items[2].Rank != 0 {
		t.Errorf("ranks = %d, %d; want rank_absolute as the fallback and 0 when neither exists", got.Items[1].Rank, got.Items[2].Rank)
	}
	if req := f.data.serps[0]; req.Keyword != "best seo" || req.Depth != 20 || req.LocationCode != 2840 {
		t.Errorf("request = %+v", req)
	}
	if _, err := f.serp(t, func(in *SerpAnalysisInput) { in.Keyword = "  " }); !isValidation(err) {
		t.Errorf("blank keyword error = %v, want ValidationError", err)
	}
}

func TestSerpAnalysisCacheAndDepth(t *testing.T) {
	f := newResearchFixture(t)
	f.data.serpFn = func(req SerpRequest) ([]SerpItem, error) {
		out := make([]SerpItem, 0, req.Depth)
		for i := 1; i <= req.Depth; i++ {
			out = append(out, organic(i, fmt.Sprintf("site%d.com", i)))
		}
		return out, nil
	}
	calls := func() int { return len(f.data.serps) }
	if got, _ := f.serp(t, nil); len(got.Items) != 20 {
		t.Fatalf("shallow = %d items", len(got.Items))
	}
	if _, err := f.serp(t, func(in *SerpAnalysisInput) { in.Keyword = "BEST SEO" }); err != nil || calls() != 1 {
		t.Errorf("an equivalent shallow request called the provider again (%d calls)", calls())
	}
	deep, err := f.serp(t, func(in *SerpAnalysisInput) { in.Depth = SerpDeepDepth })
	if err != nil || len(deep.Items) != 100 || deep.Depth != 100 || calls() != 2 {
		t.Fatalf("deep = %d items depth %d (%v), calls %d; want a second, deeper crawl", len(deep.Items), deep.Depth, err, calls())
	}
	shallow, err := f.serp(t, nil)
	if err != nil || shallow.Depth != 100 || len(shallow.Items) != 100 || calls() != 2 {
		t.Errorf("shallow after deep = depth %d, %d items, calls %d; want the deeper snapshot to answer without a call", shallow.Depth, len(shallow.Items), calls())
	}
	for name, mutate := range map[string]func(*SerpAnalysisInput){
		"organization": func(in *SerpAnalysisInput) { in.OrganizationID = "another" },
		"project":      func(in *SerpAnalysisInput) { in.ProjectID = "another" },
		"keyword":      func(in *SerpAnalysisInput) { in.Keyword = "other" },
		"country":      func(in *SerpAnalysisInput) { in.LocationCode = 2356 },
		"language":     func(in *SerpAnalysisInput) { in.LanguageCode = "hi" },
	} {
		before := calls()
		if _, err := f.serp(t, mutate); err != nil || calls() == before {
			t.Errorf("a different %s was answered from the cache (err %v)", name, err)
		}
	}
}

func TestSerpAnalysisEmptyResults(t *testing.T) {
	f := newResearchFixture(t)
	empty := false
	f.data.serpFn = func(SerpRequest) ([]SerpItem, error) {
		if empty {
			return nil, nil
		}
		return []SerpItem{organic(1, "a.com")}, nil
	}
	got, _ := f.serp(t, nil)
	if len(got.Items) != 1 {
		t.Fatal("setup failed")
	}
	empty = true
	deeper, err := f.serp(t, func(in *SerpAnalysisInput) { in.Depth = SerpDeepDepth })
	if err != nil || len(deeper.Items) != 0 || deeper.Reason != "no_organic_results" {
		t.Fatalf("empty deeper = %+v, %v; want the empty answer with a reason", deeper, err)
	}
	// The transient empty crawl must not evict the good snapshot.
	again, err := f.serp(t, nil)
	if err != nil || len(again.Items) != 1 || again.Depth != 20 {
		t.Errorf("after an empty re-crawl = %+v, %v; want the good shallow snapshot kept", again, err)
	}

	fresh := newResearchFixture(t)
	fresh.data.serpFn = func(SerpRequest) ([]SerpItem, error) { return []SerpItem{{Type: "ads"}}, nil }
	first, _ := fresh.serp(t, nil)
	if first.Reason != "no_organic_results" || first.Items == nil || len(first.Items) != 0 {
		t.Errorf("no organic results = %+v, want an empty non-nil list with a reason", first)
	}
	if _, err := fresh.serp(t, nil); err != nil {
		t.Fatal(err)
	}
	if len(fresh.data.serps) != 1 {
		t.Error("an empty first result was not cached")
	}

	boom := errors.New("provider down")
	failing := newResearchFixture(t)
	failing.data.serpFn = func(SerpRequest) ([]SerpItem, error) { return nil, boom }
	if _, err := failing.serp(t, nil); !errors.Is(err, boom) || failing.cache.size() != 0 {
		t.Errorf("provider failure: err %v, cached entries %d", err, failing.cache.size())
	}
}

func TestSerpAnalysisLocalNeedsAKnownPlace(t *testing.T) {
	f := newResearchFixture(t)
	f.data.serpFn = func(SerpRequest) ([]SerpItem, error) { return []SerpItem{organic(1, "a.com")}, nil }
	city, bad := "Pune,Maharashtra,India", "Nowhere"
	if _, err := f.serp(t, func(in *SerpAnalysisInput) { in.LocationCode, in.LocationName = 2356, &bad }); !isUnknownLocation(err) || len(f.data.serps) != 0 {
		t.Errorf("unknown place: err %v, calls %d; want a refusal before any spend", err, len(f.data.serps))
	}
	if _, err := f.serp(t, func(in *SerpAnalysisInput) { in.LocationCode, in.LocationName = 2356, &city }); err != nil {
		t.Fatal(err)
	}
	if got := f.data.serps[0].LocationName; got == nil || *got != city {
		t.Errorf("request location = %v", got)
	}
	if _, err := f.serp(t, func(in *SerpAnalysisInput) { in.LocationCode = 2356 }); err != nil {
		t.Fatal(err)
	}
	if len(f.data.serps) != 2 {
		t.Error("a national SERP was answered from the city's cache entry")
	}
}

func TestMetricsForList(t *testing.T) {
	f := newResearchFixture(t)
	ctx := context.Background()
	overview := func(keywords []string) ([]LabsItem, error) {
		var out []LabsItem
		for _, k := range keywords {
			out = append(out, labsItem(k, 10))
		}
		return out, nil
	}
	f.data.overviewFn = overview

	t.Run("national Labs batches at 700", func(t *testing.T) {
		keywords := make([]string, 1500)
		for i := range keywords {
			keywords[i] = fmt.Sprintf("kw %d", i)
		}
		rows, err := f.svc.metricsForList(ctx, "org", keywords, 2840, "en", nil)
		if err != nil || len(rows) != 1500 || len(f.data.overviews) != 3 || len(f.data.overviews[0]) != 700 || len(f.data.overviews[2]) != 100 {
			t.Errorf("rows %d (%v), batches %d; want 3 provider calls of 700, 700 and 100", len(rows), err, len(f.data.overviews))
		}
		if rows[0].Intent == nil || *rows[0].Intent != "Commercial" || *rows[0].KeywordDifficulty != 30 {
			t.Errorf("row = %+v, want the raw intent and difficulty kept", rows[0])
		}
	})
	t.Run("clickstream block counts when it has a volume", func(t *testing.T) {
		if got := overviewRow(LabsItem{Keyword: "a", Info: Info{SearchVolume: intp(5)}, Clickstream: &Info{SearchVolume: intp(0)}}); *got.SearchVolume != 0 {
			t.Errorf("volume = %d, want the clickstream block used whenever it carries a volume, even zero", *got.SearchVolume)
		}
	})
	t.Run("Google Ads countries", func(t *testing.T) {
		g := newResearchFixture(t) //nolint:contextcheck // The fixture opens its own test database context.
		g.data.volumeFn = func(_ AdsVolumeRequest) ([]AdsItem, error) {
			return []AdsItem{{Keyword: "Covered", SearchVolume: intp(9), CompetitionIndex: floatp(50)}}, nil
		}
		rows, err := g.svc.metricsForList(ctx, "org", []string{"covered", "collapsed", "bad, text"}, 2020, "ca", nil)
		if err != nil {
			t.Fatal(err)
		}
		words := map[string]metricRow{}
		for _, r := range rows {
			words[r.Keyword] = r
		}
		if *words["Covered"].Competition != 0.5 {
			t.Errorf("covered = %+v", words["Covered"])
		}
		if _, ok := words["collapsed"]; ok {
			t.Error("a national refresh wrote a null row for a keyword Ads collapsed; it may simply be missing")
		}
		if r, ok := words["bad, text"]; !ok || r.SearchVolume != nil {
			t.Errorf("rejected keyword row = %+v (%v), want an explicit empty row", r, ok)
		}
		city := "Andorra la Vella"
		local, _ := g.svc.metricsForList(ctx, "org", []string{"covered", "collapsed"}, 2020, "ca", &city)
		if len(local) != 2 {
			t.Errorf("local rows = %d, want every keyword to get a row so stale numbers are overwritten", len(local))
		}
	})
	t.Run("a city merges local volume with national difficulty", func(t *testing.T) {
		g := newResearchFixture(t) //nolint:contextcheck // The fixture opens its own test database context.
		g.data.volumeFn = func(AdsVolumeRequest) ([]AdsItem, error) {
			return []AdsItem{{Keyword: "alpha", SearchVolume: intp(12), CompetitionIndex: floatp(20)}}, nil
		}
		g.data.overviewFn = overview
		city := "Pune,Maharashtra,India"
		rows, err := g.svc.metricsForList(ctx, "org", []string{"alpha", "beta"}, 2356, "en", &city)
		if err != nil || len(rows) != 2 {
			t.Fatalf("rows = %+v, %v", rows, err)
		}
		alpha, beta := rows[0], rows[1]
		if *alpha.SearchVolume != 12 || *alpha.KeywordDifficulty != 30 || alpha.Intent == nil {
			t.Errorf("alpha = %+v", alpha)
		}
		if beta.Keyword != "beta" || beta.SearchVolume != nil || *beta.KeywordDifficulty != 30 {
			t.Errorf("beta = %+v, want difficulty kept and no national volume", beta)
		}
		boom := errors.New("ads down")
		g.data.volumeFn = func(AdsVolumeRequest) ([]AdsItem, error) { return nil, boom }
		if _, err := g.svc.metricsForList(ctx, "org", []string{"alpha"}, 2356, "en", &city); !errors.Is(err, boom) {
			t.Errorf("error = %v, want the Ads error", err)
		}
	})
}

func TestRefreshSavedMetrics(t *testing.T) {
	f := newResearchFixture(t)
	ctx := context.Background()
	f.save(t, func(in *SaveInput) { in.Keywords = []string{"alpha", "beta"} })
	f.save(t, func(in *SaveInput) { in.Keywords = []string{"alpha"}; in.LocationCode, in.LanguageCode = 2020, "ca" })
	f.data.overviewFn = func(keywords []string) ([]LabsItem, error) {
		var out []LabsItem
		for _, k := range keywords {
			if k != "beta" { // the provider has nothing for beta
				out = append(out, labsItem(k, 77))
			}
		}
		return out, nil
	}
	f.data.volumeFn = func(AdsVolumeRequest) ([]AdsItem, error) {
		return []AdsItem{{Keyword: "alpha", SearchVolume: intp(5), CompetitionIndex: floatp(80)}}, nil
	}
	updated, err := f.svc.RefreshSavedMetrics(ctx, "org", f.project, f.savedFixture.svc)
	if err != nil || updated != 2 {
		t.Fatalf("RefreshSavedMetrics() = %d, %v; want 2 keywords answered (alpha in each market)", updated, err)
	}
	if len(f.data.overviews) != 1 || len(f.data.volumes) != 1 {
		t.Errorf("provider calls: overview %d, ads volume %d; want one per market", len(f.data.overviews), len(f.data.volumes))
	}
	rows := f.list(t, func(q *ListQuery) { q.Sort, q.Descending = "keyword", false }).Rows
	byMarket := map[string]SavedKeyword{}
	for _, r := range rows {
		byMarket[fmt.Sprintf("%s:%d", r.Keyword, r.LocationCode)] = r
	}
	us, ad, beta := byMarket["alpha:2840"], byMarket["alpha:2020"], byMarket["beta:2840"]
	if *us.SearchVolume != 77 || *us.KeywordDifficulty != 30 || *us.Intent != "commercial" {
		t.Errorf("US alpha = %+v, want the refreshed Labs metrics with a normalized intent", us)
	}
	if *ad.SearchVolume != 5 || *ad.Competition != 0.8 || ad.KeywordDifficulty != nil || *ad.Intent != "unknown" {
		t.Errorf("Andorra alpha = %+v, want Google Ads metrics without difficulty", ad)
	}
	if beta.SearchVolume != nil || beta.FetchedAt != nil {
		t.Errorf("beta = %+v, want it left alone when the provider returned nothing", beta)
	}

	empty := newResearchFixture(t)
	if n, err := empty.svc.RefreshSavedMetrics(ctx, "org", empty.project, empty.savedFixture.svc); err != nil || n != 0 || len(empty.data.overviews) != 0 {
		t.Errorf("empty project: %d, %v, provider calls %d; want zero and no spend", n, err, len(empty.data.overviews))
	}
	boom := errors.New("labs down")
	f.data.overviewFn = func([]string) ([]LabsItem, error) { return nil, boom }
	if _, err := f.svc.RefreshSavedMetrics(ctx, "org", f.project, f.savedFixture.svc); !errors.Is(err, boom) {
		t.Errorf("provider failure error = %v", err)
	}
}

func TestSerpAnalysisCacheEntryWithoutDepthIsTheShallowFloor(t *testing.T) {
	f := newResearchFixture(t)
	f.data.serpFn = func(_ SerpRequest) ([]SerpItem, error) { return []SerpItem{organic(1, "a.com")}, nil }
	if _, err := f.serp(t, nil); err != nil {
		t.Fatal(err)
	}
	for key := range f.cache.values {
		f.cache.values[key] = []byte(`{"requestedKeyword":"best seo","items":[{"rank":1,"title":"t","url":"u","domain":"d","description":"","etv":null,"estimatedPaidTrafficCost":null,"referringDomains":null,"backlinks":null,"isNew":false,"rankChange":null}]}`)
	}
	got, err := f.serp(t, nil)
	if err != nil || got.Depth != SerpShallowDepth || len(f.data.serps) != 1 {
		t.Errorf("shallow request = depth %d (%v), calls %d; want the old entry to answer it", got.Depth, err, len(f.data.serps))
	}
	if _, err := f.serp(t, func(in *SerpAnalysisInput) { in.Depth = SerpDeepDepth }); err != nil || len(f.data.serps) != 2 {
		t.Errorf("deep request: err %v, calls %d; want a re-crawl rather than 20 results shown as 100", err, len(f.data.serps))
	}
}
