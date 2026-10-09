package domain

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

func overviewResult(etv, count float64) func(string) any {
	return func(string) any {
		return map[string]any{"items": []any{map[string]any{"metrics": map[string]any{"organic": map[string]any{"etv": etv, "count": count}}}}}
	}
}

func TestOverviewRoundsMetricsAndCachesPerOrganization(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{result: overviewResult(1234.6, 99.4)}
	cache := &memoryCache{}
	svc := newTestService(t, provider, cache)

	first, err := svc.Overview(context.Background(), "org-a", "proj", "https://www.Example.com/blog", ScopeSubfolder, 2840, "en")
	if err != nil {
		t.Fatal(err)
	}
	if floatValue(t, first.OrganicTraffic) != 1235 || floatValue(t, first.OrganicKeywords) != 99 || !first.HasData {
		t.Fatalf("overview = %+v", first)
	}
	if first.Domain != "example.com" || first.Scope != ScopeSubfolder || first.DisplayTarget == "" || first.FetchedAt != "2026-10-09T11:12:13Z" {
		t.Fatalf("overview identity = %+v", first)
	}
	body := provider.lastBody(t)
	if body["target"] != "example.com" || body["location_code"] != 2840.0 || body["language_code"] != "en" {
		t.Fatalf("provider request = %v", body)
	}

	// A second scope on the same hostname shares the cache entry but must be
	// labelled with the scope that was asked for.
	second, err := svc.Overview(context.Background(), "org-a", "proj", "example.com", ScopeDomain, 2840, "en")
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1 (cache hit)", provider.calls.Load())
	}
	if second.Scope != ScopeDomain || second.DisplayTarget == first.DisplayTarget {
		t.Fatalf("cached overview kept the first lookup's scope or label: first %+v, second %+v", first, second)
	}

	if _, err := svc.Overview(context.Background(), "org-b", "proj", "example.com", ScopeDomain, 2840, "en"); err != nil {
		t.Fatal(err)
	}
	if provider.calls.Load() != 2 {
		t.Fatalf("provider calls = %d, want 2: another organization must not read org-a's cache", provider.calls.Load())
	}
	if _, err := svc.Overview(context.Background(), "org-a", "proj", "example.com", ScopeDomain, 2826, "en"); err != nil {
		t.Fatal(err)
	}
	if provider.calls.Load() != 3 {
		t.Fatalf("provider calls = %d, want 3: another location is another cache entry", provider.calls.Load())
	}
}

func TestOverviewDoesNotCacheEmptyResults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		result func(string) any
	}{
		{"zero keywords", overviewResult(0, 0)},
		{"no metrics", func(string) any { return map[string]any{"items": []any{map[string]any{}}} }},
		{"no items", func(string) any { return map[string]any{"items": []any{}} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := &fakeProvider{result: tc.result}
			cache := &memoryCache{}
			svc := newTestService(t, provider, cache)
			got, err := svc.Overview(context.Background(), "org", "proj", "example.com", ScopeDomain, 0, "")
			if err != nil {
				t.Fatal(err)
			}
			if got.HasData {
				t.Fatalf("HasData = true for %s", tc.name)
			}
			if cache.size() != 0 {
				t.Fatal("an empty result was cached; the next lookup would never retry")
			}
			body := provider.lastBody(t)
			if body["location_code"] != 2840.0 || body["language_code"] != "en" {
				t.Fatalf("defaults not applied: %v", body)
			}
		})
	}
}

func TestOverviewIgnoresACachedEmptyEntry(t *testing.T) {
	t.Parallel()
	cache := &memoryCache{}
	key, err := makeCacheKey("overview", "org", "proj", struct {
		Domain       string
		LocationCode int
		LanguageCode string
	}{"example.com", 2840, "en"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Set(context.Background(), key, []byte(`{"domain":"example.com","hasData":false}`), 0); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{result: overviewResult(10, 5)}
	svc := newTestService(t, provider, cache)
	got, err := svc.Overview(context.Background(), "org", "proj", "example.com", ScopeDomain, 2840, "en")
	if err != nil || !got.HasData || provider.calls.Load() != 1 {
		t.Fatalf("overview = %+v, err = %v, provider calls = %d: an entry without data must be refetched", got, err, provider.calls.Load())
	}
}

func TestOverviewErrors(t *testing.T) {
	t.Parallel()
	t.Run("invalid domain never reaches the provider", func(t *testing.T) {
		t.Parallel()
		provider := &fakeProvider{}
		svc := newTestService(t, provider, nil)
		_, err := svc.Overview(context.Background(), "org", "proj", "not a domain", ScopeDomain, 2840, "en")
		if _, ok := errors.AsType[badRequest](err); !ok {
			t.Fatalf("error = %v, want badRequest", err)
		}
		if provider.calls.Load() != 0 {
			t.Fatal("provider called for an invalid domain")
		}
	})
	t.Run("billing issue is surfaced and not cached", func(t *testing.T) {
		t.Parallel()
		provider := &fakeProvider{taskStatus: 40210}
		cache := &memoryCache{}
		svc := newTestService(t, provider, cache)
		_, err := svc.Overview(context.Background(), "org", "proj", "example.com", ScopeDomain, 2840, "en")
		if !errors.Is(err, dataforseo.ErrBillingIssue) {
			t.Fatalf("error = %v, want billing issue", err)
		}
		if cache.size() != 0 || provider.calls.Load() != 1 {
			t.Fatalf("cache size %d, provider calls %d", cache.size(), provider.calls.Load())
		}
	})
	t.Run("provider task failure", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t, &fakeProvider{taskStatus: 50000}, nil)
		if _, err := svc.Overview(context.Background(), "org", "proj", "example.com", ScopeDomain, 2840, "en"); err == nil {
			t.Fatal("task failure returned success")
		}
	})
	t.Run("cancelled context", func(t *testing.T) {
		t.Parallel()
		provider := &fakeProvider{result: overviewResult(1, 1)}
		svc := newTestService(t, provider, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := svc.Overview(ctx, "org", "proj", "example.com", ScopeDomain, 2840, "en"); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	})
}

func TestOverviewWorksWhenCacheIsDown(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{result: overviewResult(10, 5)}
	svc := newTestService(t, provider, &memoryCache{fail: true})
	for range 2 {
		got, err := svc.Overview(context.Background(), "org", "proj", "example.com", ScopeDomain, 2840, "en")
		if err != nil || !got.HasData {
			t.Fatalf("overview = %+v, err = %v", got, err)
		}
	}
	if provider.calls.Load() != 2 {
		t.Fatalf("provider calls = %d, want 2 with the cache down", provider.calls.Load())
	}
}

func TestKeywordSuggestionsRequestAndMapping(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{result: rankedKeywordsResult(2,
		rankedItem("seo audit", 3.4, 120.5, 880.2, 1.25, 41.6, "https://example.com/audit", "/audit"),
		map[string]any{"keyword": "fallback keyword", "ranked_serp_element": map[string]any{"rank_absolute": 9.0, "etv": 2.0}},
		map[string]any{"keyword_data": map[string]any{"keyword": ""}},
	)}
	cache := &memoryCache{}
	svc := newTestService(t, provider, cache)

	got, err := svc.KeywordSuggestions(context.Background(), "org", "proj", "example.com", ScopeDomain, 2840, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("suggestions = %+v, want 2 (the row without a keyword is dropped)", got)
	}
	first := got[0]
	if first.Keyword != "seo audit" || floatValue(t, first.Position) != 3 || floatValue(t, first.SearchVolume) != 880 ||
		floatValue(t, first.Traffic) != 120.5 || floatValue(t, first.CPC) != 1.25 || floatValue(t, first.KeywordDifficulty) != 42 {
		t.Fatalf("mapped suggestion = %+v", first)
	}
	if got[1].Keyword != "fallback keyword" || floatValue(t, got[1].Position) != 9 {
		t.Fatalf("fallback suggestion = %+v", got[1])
	}
	body := provider.lastBody(t)
	if body["limit"] != 100.0 || toJSON(t, body["order_by"]) != `["ranked_serp_element.serp_item.etv,desc"]` {
		t.Fatalf("provider request = %v", body)
	}
	if _, ok := body["offset"]; ok {
		t.Fatalf("suggestions must not page: %v", body)
	}

	if _, err := svc.KeywordSuggestions(context.Background(), "org", "proj", "example.com", ScopeDomain, 2840, "en"); err != nil {
		t.Fatal(err)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1 (cached)", provider.calls.Load())
	}
}

func TestKeywordSuggestionsEmptyResultIsNotCached(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{result: rankedKeywordsResult(0)}
	cache := &memoryCache{}
	svc := newTestService(t, provider, cache)
	got, err := svc.KeywordSuggestions(context.Background(), "org", "proj", "example.com", ScopeSubdomains, 2840, "en")
	if err != nil || len(got) != 0 || got == nil {
		t.Fatalf("suggestions = %#v, err = %v, want an empty non-nil slice", got, err)
	}
	if cache.size() != 0 {
		t.Fatal("empty suggestions were cached")
	}
	if _, has := provider.lastBody(t)["filters"]; has {
		t.Fatal("subdomains scope must not send a scope filter")
	}
}

func TestKeywordSuggestionsErrors(t *testing.T) {
	t.Parallel()
	svc := newTestService(t, &fakeProvider{taskStatus: 40210}, nil)
	if _, err := svc.KeywordSuggestions(context.Background(), "org", "proj", "example.com", ScopeDomain, 2840, "en"); !errors.Is(err, dataforseo.ErrBillingIssue) {
		t.Fatalf("error = %v, want billing issue", err)
	}
	if _, err := svc.KeywordSuggestions(context.Background(), "org", "proj", "192.0.2.1", ScopeDomain, 2840, "en"); err == nil {
		t.Fatal("IP address accepted as a domain")
	}
}

func TestKeywordsPageSendsPagingSortAndFilters(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{result: rankedKeywordsResult(120,
		rankedItem("alpha", 1, 10, 100, 0.5, 20, "https://example.com/a?x=1", ""),
		rankedItem("beta", 2, 5, 50, 0.2, 30, "", "/b"),
		rankedItem("gamma", 3, 1, 10, 0.1, 40, "ftp-not-absolute", ""),
	)}
	svc := newTestService(t, provider, &memoryCache{})
	input := KeywordsPageInput{
		ProjectID: "proj", Domain: "example.com", Scope: ScopeSubfolder, LocationCode: 2840, LanguageCode: "en",
		Page: 2, PageSize: 50, SortMode: "volume", SortOrder: "asc", Filters: keywordFilters{Include: "alp", MinVol: f64(10)},
	}
	got, err := svc.KeywordsPage(context.Background(), "org", withPath(input))
	if err != nil {
		t.Fatal(err)
	}
	body := provider.lastBody(t)
	if body["limit"] != 50.0 || body["offset"] != 50.0 {
		t.Fatalf("paging = limit %v offset %v, want 50/50", body["limit"], body["offset"])
	}
	if toJSON(t, body["order_by"]) != `["keyword_data.keyword_info.search_volume,asc"]` {
		t.Fatalf("order_by = %v", body["order_by"])
	}
	filters := toJSON(t, body["filters"])
	for _, want := range []string{"%alp%", `"keyword_data.keyword_info.search_volume",">=",10`, "ranked_serp_element.serp_item.domain", "/blog/%"} {
		if !strings.Contains(filters, want) {
			t.Errorf("filters %s lack %s", filters, want)
		}
	}

	if got.Page != 2 || got.PageSize != 50 || got.TotalCount == nil || *got.TotalCount != 120 || !got.HasMore {
		t.Fatalf("page metadata = %+v", got)
	}
	if len(got.Keywords) != 3 {
		t.Fatalf("keywords = %+v", got.Keywords)
	}
	alpha, beta, gamma := got.Keywords[0], got.Keywords[1], got.Keywords[2]
	if alpha.URL == nil || *alpha.URL != "https://example.com/a?x=1" || alpha.RelativeURL == nil || *alpha.RelativeURL != "/a?x=1" {
		t.Fatalf("relative URL must be derived from the ranking URL: %+v", alpha)
	}
	if beta.URL != nil || beta.RelativeURL == nil || *beta.RelativeURL != "/b" {
		t.Fatalf("provider relative_url must be kept: %+v", beta)
	}
	if gamma.RelativeURL != nil {
		t.Fatalf("a non-absolute URL has no relative path: %+v", gamma)
	}
	out := toJSON(t, got.Keywords[0])
	if !strings.Contains(out, `"url":"https://example.com/a?x=1"`) || !strings.Contains(out, `"relativeUrl":"/a?x=1"`) || !strings.Contains(out, `"keyword":"alpha"`) {
		t.Fatalf("JSON contract for the keywords table: %s", out)
	}
}

func withPath(in KeywordsPageInput) KeywordsPageInput {
	in.Domain = "example.com/blog"
	return in
}

func TestKeywordsPageDefaultsAndHasMore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		total    any
		items    int
		wantMore bool
	}{
		{"total says more", 51, 50, true},
		{"last page", 50, 50, false},
		{"no total and full page", nil, 50, true},
		{"no total and short page", nil, 3, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			items := make([]any, 0, tc.items)
			for i := range tc.items {
				items = append(items, rankedItem("kw"+strings.Repeat("x", i), float64(i+1), 1, 1, 1, 1, "", ""))
			}
			provider := &fakeProvider{result: func(string) any { return map[string]any{"total_count": tc.total, "items": items} }}
			svc := newTestService(t, provider, nil)
			got, err := svc.KeywordsPage(context.Background(), "org", KeywordsPageInput{ProjectID: "proj", Domain: "example.com", Scope: ScopeDomain})
			if err != nil {
				t.Fatal(err)
			}
			if got.HasMore != tc.wantMore {
				t.Fatalf("HasMore = %v, want %v", got.HasMore, tc.wantMore)
			}
			if got.Page != 1 || got.PageSize != 50 {
				t.Fatalf("defaults = page %d size %d", got.Page, got.PageSize)
			}
			if toJSON(t, provider.lastBody(t)["order_by"]) != `["ranked_serp_element.serp_item.etv,desc"]` {
				t.Fatalf("default sort = %v", provider.lastBody(t)["order_by"])
			}
		})
	}
}

func TestKeywordsPageRejectsBadInputBeforeSpending(t *testing.T) {
	t.Parallel()
	base := KeywordsPageInput{ProjectID: "proj", Domain: "example.com", Scope: ScopeDomain}
	tests := []struct {
		name   string
		mutate func(*KeywordsPageInput)
	}{
		{"page too large", func(in *KeywordsPageInput) { in.Page = maxPage + 1 }},
		{"negative page", func(in *KeywordsPageInput) { in.Page = -1 }},
		{"page size 75", func(in *KeywordsPageInput) { in.PageSize = 75 }},
		{"unknown sort mode", func(in *KeywordsPageInput) { in.SortMode = "keywords" }},
		{"unknown sort order", func(in *KeywordsPageInput) { in.SortOrder = "sideways" }},
		{"invalid domain", func(in *KeywordsPageInput) { in.Domain = "localhost" }},
		{"too many filters", func(in *KeywordsPageInput) { in.Filters = keywordFilters{Include: "a,b,c,d,e,f,g,h,i"} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := &fakeProvider{result: rankedKeywordsResult(0)}
			svc := newTestService(t, provider, nil)
			in := base
			tc.mutate(&in)
			_, err := svc.KeywordsPage(context.Background(), "org", in)
			if _, ok := errors.AsType[badRequest](err); !ok {
				t.Fatalf("error = %v, want badRequest", err)
			}
			if provider.calls.Load() != 0 {
				t.Fatal("invalid request reached the provider")
			}
		})
	}
}

func TestKeywordsPageCacheKeyCoversEveryInput(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{result: rankedKeywordsResult(1, rankedItem("kw", 1, 1, 1, 1, 1, "", ""))}
	svc := newTestService(t, provider, &memoryCache{})
	base := KeywordsPageInput{ProjectID: "proj", Domain: "example.com", Scope: ScopeDomain, LocationCode: 2840, LanguageCode: "en", Page: 1, PageSize: 50, SortMode: "traffic", SortOrder: "desc"}
	variants := []func(*KeywordsPageInput){
		func(*KeywordsPageInput) {},
		func(in *KeywordsPageInput) { in.Page = 2 },
		func(in *KeywordsPageInput) { in.PageSize = 100 },
		func(in *KeywordsPageInput) { in.SortMode = "cpc" },
		func(in *KeywordsPageInput) { in.SortOrder = "asc" },
		func(in *KeywordsPageInput) { in.Filters.Include = "seo" },
		func(in *KeywordsPageInput) { in.Search = "audit" },
		func(in *KeywordsPageInput) { in.LocationCode = 2826 },
		func(in *KeywordsPageInput) { in.LanguageCode = "fr" },
		func(in *KeywordsPageInput) { in.Scope = ScopeSubdomains },
		func(in *KeywordsPageInput) { in.ProjectID = "other-project" },
	}
	for i, mutate := range variants {
		in := base
		mutate(&in)
		if _, err := svc.KeywordsPage(context.Background(), "org", in); err != nil {
			t.Fatal(err)
		}
		if got := int(provider.calls.Load()); got != i+1 {
			t.Fatalf("variant %d: provider calls = %d, want %d (the cache key ignored this input)", i, got, i+1)
		}
	}
	if _, err := svc.KeywordsPage(context.Background(), "other-org", base); err != nil {
		t.Fatal(err)
	}
	if got := int(provider.calls.Load()); got != len(variants)+1 {
		t.Fatalf("provider calls = %d, want %d: another organization must not share a cache entry", got, len(variants)+1)
	}
	if _, err := svc.KeywordsPage(context.Background(), "org", base); err != nil {
		t.Fatal(err)
	}
	if got := int(provider.calls.Load()); got != len(variants)+1 {
		t.Fatalf("repeating the base request called the provider again (calls = %d)", got)
	}
}

func TestPagesPage(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{result: func(string) any {
		return map[string]any{"total_count": 3, "items": []any{
			map[string]any{"page_address": "https://example.com/blog/post?a=b", "metrics": map[string]any{"organic": map[string]any{"etv": 55.5, "count": 7.2}}},
			map[string]any{"page_address": "https://example.com", "metrics": map[string]any{}},
			map[string]any{"page_address": "relative/only"},
			map[string]any{"page_address": ""},
			map[string]any{},
		}}
	}}
	svc := newTestService(t, provider, &memoryCache{})
	got, err := svc.PagesPage(context.Background(), "org", PagesPageInput{
		ProjectID: "proj", Domain: "example.com", Scope: ScopeDomain, Page: 1, PageSize: 100, SortMode: "keywords", SortOrder: "asc",
		Filters: keywordFilters{MinTraffic: f64(5)}, Search: "blog",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := provider.lastBody(t)
	if body["limit"] != 100.0 || body["offset"] != 0.0 || toJSON(t, body["order_by"]) != `["metrics.organic.count,asc"]` {
		t.Fatalf("provider request = %v", body)
	}
	filters := toJSON(t, body["filters"])
	for _, want := range []string{"page_address", "metrics.organic.etv", "%blog%", "%://www.example.com/%"} {
		if !strings.Contains(filters, want) {
			t.Errorf("filters %s lack %s", filters, want)
		}
	}
	if len(got.Pages) != 3 {
		t.Fatalf("pages = %+v, want 3 (rows without an address are dropped)", got.Pages)
	}
	first := got.Pages[0]
	if first.Page != "https://example.com/blog/post?a=b" || first.RelativePath == nil || *first.RelativePath != "/blog/post?a=b" ||
		floatValue(t, first.OrganicTraffic) != 56 || floatValue(t, first.Keywords) != 7 {
		t.Fatalf("first page = %+v", first)
	}
	if got.Pages[1].RelativePath == nil || *got.Pages[1].RelativePath != "/" || got.Pages[1].OrganicTraffic != nil {
		t.Fatalf("root page = %+v", got.Pages[1])
	}
	if got.Pages[2].RelativePath != nil {
		t.Fatalf("a row with a non-absolute address must stay, with no relative path: %+v", got.Pages[2])
	}
	if got.TotalCount == nil || *got.TotalCount != 3 || got.HasMore {
		t.Fatalf("total %v, HasMore %v: all 3 rows fit on the first page", got.TotalCount, got.HasMore)
	}
}

func TestPagesPageErrors(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{result: rankedKeywordsResult(0)}
	svc := newTestService(t, provider, nil)
	for name, in := range map[string]PagesPageInput{
		"invalid domain":   {Domain: "x", Scope: ScopeDomain},
		"bad sort":         {Domain: "example.com", Scope: ScopeDomain, SortMode: "rank"},
		"bad page size":    {Domain: "example.com", Scope: ScopeDomain, PageSize: 7},
		"too many filters": {Domain: "example.com", Scope: ScopeSubfolder, Filters: keywordFilters{Include: "a,b,c,d,e,f"}},
	} {
		if _, err := svc.PagesPage(context.Background(), "org", in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if provider.calls.Load() != 0 {
		t.Fatal("invalid requests reached the provider")
	}
	billing := newTestService(t, &fakeProvider{taskStatus: 40210}, nil)
	if _, err := billing.PagesPage(context.Background(), "org", PagesPageInput{ProjectID: "p", Domain: "example.com", Scope: ScopeDomain}); !errors.Is(err, dataforseo.ErrBillingIssue) {
		t.Fatalf("error = %v, want billing issue", err)
	}
}

func TestToRelativePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"https://example.com/a/b?x=1&y=2", "/a/b?x=1&y=2", true},
		{"https://example.com", "/", true},
		{"https://example.com/?q=1", "/?q=1", true},
		{"https://example.com/ü ber", "/%C3%BC%20ber", true},
		{"example.com/a", "", false},
		{"/just/a/path", "", false},
		{"", "", false},
		{"https://exa mple.com/a", "", false},
	}
	for _, tc := range tests {
		got := toRelativePath(tc.in)
		if (got != nil) != tc.ok || (got != nil && *got != tc.want) {
			t.Errorf("toRelativePath(%q) = %v, want %q (ok=%v)", tc.in, got, tc.want, tc.ok)
		}
	}
}

func TestConcurrentOrganizationsAreRaceSafe(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{result: overviewResult(10, 5)}
	svc := newTestService(t, provider, &memoryCache{})
	done := make(chan error, 16)
	for i := range 16 {
		go func() {
			org := "org-a"
			if i%2 == 1 {
				org = "org-b"
			}
			_, err := svc.Overview(context.Background(), org, "proj", "example.com", ScopeDomain, 2840, "en")
			done <- err
		}()
	}
	for range 16 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
