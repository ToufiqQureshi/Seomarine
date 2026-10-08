package aisearch

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

func lookupInput() BrandLookupInput {
	return BrandLookupInput{Query: "acme.com", LocationCode: 2826, LanguageCode: "en"}
}

func TestBrandLookupFetchesBothPlatformsAndShapesTheResult(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)

	result, err := svc.BrandLookup(context.Background(), newOrg(), "project-1", lookupInput())
	if err != nil {
		t.Fatalf("BrandLookup() error = %v", err)
	}

	// 3 calls per platform, 2 platforms; no competitors means no cross call.
	if got := api.total(); got != 6 {
		t.Errorf("provider calls = %d, want 6", got)
	}
	if got := len(api.requestsTo(crossAggregatedPath)); got != 0 {
		t.Errorf("cross-aggregated calls = %d, want 0 without competitors", got)
	}
	if got := api.recorder.count(); got != 6 {
		t.Errorf("recorded costs = %d, want one per billed call (6)", got)
	}
	if !result.HasData || *result.TotalMentions != 5 || *result.TotalAISearchVolume != 50 {
		// Location 2826 is not US, so ChatGPT's 10 mentions are left out.
		t.Errorf("result = hasData %v, mentions %v, volume %v, want true, 5, 50", result.HasData, result.TotalMentions, result.TotalAISearchVolume)
	}
	if result.ResolvedTarget != "acme.com" || *result.Scope != ScopeSubdomains || result.DetectedTargetType != "domain" {
		t.Errorf("target = %q %v %q, want acme.com subdomains domain", result.ResolvedTarget, *result.Scope, result.DetectedTargetType)
	}
	if len(result.TopPages) != 2 || len(result.TopQueries) != 2 {
		t.Errorf("topPages = %d, topQueries = %d, want 2 and 2 (one per platform)", len(result.TopPages), len(result.TopQueries))
	}
}

func TestBrandLookupAsksChatGPTForUSEnglishOnly(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	in := lookupInput()
	in.LocationCode, in.LanguageCode = 2356, "hi"

	if _, err := svc.BrandLookup(context.Background(), newOrg(), "p", in); err != nil {
		t.Fatal(err)
	}

	var sent []map[string]any
	for _, r := range api.requestsTo(aggregatedPath) {
		var body []map[string]any
		if err := json.Unmarshal([]byte(r.body), &body); err != nil {
			t.Fatal(err)
		}
		sent = append(sent, body[0])
	}
	want := map[string][2]any{platformChatGPT: {2840.0, "en"}, platformGoogle: {2356.0, "hi"}}
	for _, body := range sent {
		platform := body["platform"].(string)
		if body["location_code"] != want[platform][0] || body["language_code"] != want[platform][1] {
			t.Errorf("%s asked for %v/%v, want %v", platform, body["location_code"], body["language_code"], want[platform])
		}
	}
	if len(sent) != 2 {
		t.Fatalf("aggregated calls = %d, want 2", len(sent))
	}
}

func TestBrandLookupScopesTheProviderTarget(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		scope          Scope
		wantSubdomains any
		wantKeyword    string
	}{
		{name: "root domain includes subdomains", query: "acme.com", wantSubdomains: true},
		{name: "domain scope excludes subdomains", query: "acme.com", scope: ScopeDomain, wantSubdomains: false},
		{name: "subfolder is filtered after the call, not scoped by it", query: "acme.com/blog", wantSubdomains: false},
		{name: "keyword has no subdomain flag", query: "Acme Brand", wantKeyword: "Acme Brand"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t)
			svc := newTestService(t, api)
			in := lookupInput()
			in.Query, in.Scope = tt.query, tt.scope

			if _, err := svc.BrandLookup(context.Background(), newOrg(), "p", in); err != nil {
				t.Fatal(err)
			}

			var body []struct {
				Target []map[string]any `json:"target"`
			}
			if err := json.Unmarshal([]byte(api.requestsTo(aggregatedPath)[0].body), &body); err != nil {
				t.Fatal(err)
			}
			target := body[0].Target[0]
			if tt.wantKeyword != "" {
				if target["keyword"] != tt.wantKeyword || target["match_type"] != "word_match" {
					t.Errorf("target = %v, want keyword %q with word_match", target, tt.wantKeyword)
				}
				return
			}
			if target["include_subdomains"] != tt.wantSubdomains {
				t.Errorf("include_subdomains = %v, want %v", target["include_subdomains"], tt.wantSubdomains)
			}
		})
	}
}

func TestBrandLookupCachesCompleteResultsPerScope(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	org := newOrg()
	ctx := context.Background()

	if _, err := svc.BrandLookup(ctx, org, "p", lookupInput()); err != nil {
		t.Fatal(err)
	}
	calls := api.total()

	in := lookupInput()
	in.Query = "https://www.acme.com/" // same target, different spelling
	cached, err := svc.BrandLookup(ctx, org, "p", in)
	if err != nil {
		t.Fatal(err)
	}
	if api.total() != calls {
		t.Errorf("a repeated lookup made %d provider calls, want none", api.total()-calls)
	}
	if cached.Query != in.Query {
		t.Errorf("cached Query = %q, want the new spelling %q", cached.Query, in.Query)
	}

	in.Scope = ScopeDomain
	if _, err := svc.BrandLookup(ctx, org, "p", in); err != nil {
		t.Fatal(err)
	}
	if api.total() == calls {
		t.Error("a different scope was served from the cache, want a new lookup")
	}

	if _, err := svc.BrandLookup(ctx, newOrg(), "p", lookupInput()); err != nil {
		t.Fatal(err)
	}
	if api.total() <= calls*2 {
		t.Error("another organization was served from the cache, want its own paid lookup")
	}
}

func TestBrandLookupDoesNotCachePartialResults(t *testing.T) {
	api := newFakeAPI(t)
	api.on(topPagesPath, func(int, string) taskReply { return failReply(40501, "Invalid Field: 'target'.") })
	svc := newTestService(t, api)
	org := newOrg()

	first, err := svc.BrandLookup(context.Background(), org, "p", lookupInput())
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasData || len(first.TopPages) != 0 {
		t.Errorf("partial result = hasData %v, topPages %d, want data without pages", first.HasData, len(first.TopPages))
	}
	calls := api.total()

	if _, err := svc.BrandLookup(context.Background(), org, "p", lookupInput()); err != nil {
		t.Fatal(err)
	}
	if api.total() == calls {
		t.Error("a result with a failed sub-call was cached, want it fetched again")
	}
}

func TestBrandLookupReportsAFailedPlatformWithoutLosingTheOther(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	failGoogle := func(_ int, body string) taskReply {
		if strings.Contains(body, `"platform":"google"`) {
			return failReply(50000, "Internal Error.")
		}
		return defaultReply(aggregatedPath)
	}
	api.on(aggregatedPath, failGoogle)
	api.on(topPagesPath, func(_ int, body string) taskReply {
		if strings.Contains(body, `"platform":"google"`) {
			return failReply(50000, "Internal Error.")
		}
		return defaultReply(topPagesPath)
	})
	api.on(mentionsSearchPath, func(_ int, body string) taskReply {
		if strings.Contains(body, `"platform":"google"`) {
			return failReply(50000, "Internal Error.")
		}
		return defaultReply(mentionsSearchPath)
	})

	result, err := svc.BrandLookup(context.Background(), newOrg(), "p", BrandLookupInput{Query: "acme.com", LocationCode: 2840, LanguageCode: "en"})
	if err != nil {
		t.Fatalf("BrandLookup() error = %v, want the surviving platform's data", err)
	}

	if got := result.PerPlatform[1]; got.Platform != platformGoogle || got.Status != "error" {
		t.Errorf("google row = %+v, want status error", got)
	}
	if got := result.PerPlatform[0]; got.Status != "success" || *got.Mentions != 10 {
		t.Errorf("chat_gpt row = %+v, want success with 10 mentions", got)
	}
}

func TestBrandLookupBillingIssueFailsTheLookup(t *testing.T) {
	api := newFakeAPI(t)
	api.on(aggregatedPath, func(int, string) taskReply { return failReply(40200, "Payment Required.") })
	svc := newTestService(t, api)

	_, err := svc.BrandLookup(context.Background(), newOrg(), "p", lookupInput())

	if !errors.Is(err, dataforseo.ErrBillingIssue) {
		t.Fatalf("BrandLookup() error = %v, want ErrBillingIssue", err)
	}
	if got := api.total(); got != 1 {
		t.Errorf("provider calls after a billing failure = %d, want 1 (stop at the first)", got)
	}
}

func TestBrandLookupComparesCompetitors(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	in := lookupInput()
	in.Competitors = []string{"rival.com", "RIVAL.com", "acme.com"} // a duplicate and the target itself

	result, err := svc.BrandLookup(context.Background(), newOrg(), "p", in)
	if err != nil {
		t.Fatal(err)
	}

	requests := api.requestsTo(crossAggregatedPath)
	if len(requests) != 2 {
		t.Fatalf("cross-aggregated calls = %d, want one per platform", len(requests))
	}
	var body []struct {
		Targets []struct {
			Key string `json:"aggregation_key"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(requests[0].body), &body); err != nil {
		t.Fatal(err)
	}
	if keys := body[0].Targets; len(keys) != 2 || keys[0].Key != "acme.com" || keys[1].Key != "rival.com" {
		t.Errorf("compared keys = %+v, want acme.com and rival.com once each", keys)
	}
	if result.ShareOfVoice == nil || len(result.ShareOfVoice.Entries) != 2 {
		t.Fatalf("ShareOfVoice = %+v, want 2 entries", result.ShareOfVoice)
	}
}

func TestBrandLookupRejectsAScopeThatDoesNotFitTheQuery(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	in := lookupInput()
	in.Scope = ScopeSubfolder // acme.com has no path

	_, err := svc.BrandLookup(context.Background(), newOrg(), "p", in)

	var input inputError
	if !errors.As(err, &input) {
		t.Fatalf("BrandLookup() error = %v, want an inputError", err)
	}
	if api.total() != 0 {
		t.Errorf("provider calls = %d, want none for an invalid request", api.total())
	}
}

func TestBrandLookupStopsWhenTheCallerGoesAway(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := svc.BrandLookup(ctx, newOrg(), "p", lookupInput()); !errors.Is(err, context.Canceled) {
		t.Fatalf("BrandLookup() error = %v, want context.Canceled", err)
	}
}

func TestBrandLookupDoesNotCacheAnEmptyResult(t *testing.T) {
	api := newFakeAPI(t)
	empty := func(path string) func(int, string) taskReply {
		return func(int, string) taskReply {
			switch path {
			case aggregatedPath:
				return okReply(map[string]any{"total": map[string]any{"platform": []any{}}})
			default:
				return okReply(map[string]any{"items": []any{}})
			}
		}
	}
	for _, path := range []string{aggregatedPath, topPagesPath, mentionsSearchPath} {
		api.on(path, empty(path))
	}
	svc := newTestService(t, api)
	org := newOrg()

	first, err := svc.BrandLookup(context.Background(), org, "p", lookupInput())
	if err != nil {
		t.Fatal(err)
	}
	if first.HasData {
		t.Fatal("HasData = true for an empty provider answer")
	}
	calls := api.total()

	// The provider may simply not have data yet, so the user can look again later.
	if _, err := svc.BrandLookup(context.Background(), org, "p", lookupInput()); err != nil {
		t.Fatal(err)
	}
	if api.total() == calls {
		t.Error("an empty result was cached, want it fetched again")
	}
}

func TestBrandLookupCompetitorOrderAndCaseShareOneCacheEntry(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	org := newOrg()
	first := lookupInput()
	first.Competitors = []string{"rival.com", "other.com"}
	if _, err := svc.BrandLookup(context.Background(), org, "p", first); err != nil {
		t.Fatal(err)
	}
	calls := api.total()

	second := lookupInput()
	second.Competitors = []string{"OTHER.com", "Rival.com"}
	if _, err := svc.BrandLookup(context.Background(), org, "p", second); err != nil {
		t.Fatal(err)
	}
	if api.total() != calls {
		t.Errorf("a reordered competitor list made %d new provider calls, want none", api.total()-calls)
	}
}

func TestCacheKeys(t *testing.T) {
	key := cacheKey("brand-lookup", "org-1", "a", "b")
	if !strings.HasPrefix(key, "ai-search:brand-lookup:org-1:") {
		t.Errorf("key = %q, want the kind and organization in the clear, for purging by prefix", key)
	}
	for name, other := range map[string]string{
		"another organization":         cacheKey("brand-lookup", "org-2", "a", "b"),
		"another kind":                 cacheKey("prompt-response", "org-1", "a", "b"),
		"a part moved across boundary": cacheKey("brand-lookup", "org-1", "ab", ""),
		"a part moved the other way":   cacheKey("brand-lookup", "org-1", "", "ab"),
		"a part appended":              cacheKey("brand-lookup", "org-1", "a", "b", ""),
	} {
		if other == key {
			t.Errorf("%s gives the same key %q", name, key)
		}
	}
	if again := cacheKey("brand-lookup", "org-1", "a", "b"); again != key {
		t.Errorf("same input gives %q then %q, want a stable key", key, again)
	}
}

func TestBrandLookupTreatsACorruptCacheEntryAsAMiss(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	org := newOrg()
	ctx := context.Background()
	if _, err := svc.BrandLookup(ctx, org, "p", lookupInput()); err != nil {
		t.Fatal(err)
	}
	keys, err := svc.redis.Keys(ctx, "ai-search:brand-lookup:"+org+":*").Result()
	if err != nil || len(keys) != 1 {
		t.Fatalf("cache keys = %v, %v; want exactly one for the organization", keys, err)
	}
	if err := svc.redis.Set(ctx, keys[0], "{not json", 0).Err(); err != nil {
		t.Fatal(err)
	}
	calls := api.total()

	result, err := svc.BrandLookup(ctx, org, "p", lookupInput())

	if err != nil || !result.HasData {
		t.Fatalf("BrandLookup() = hasData %v, error %v; want a fresh lookup", result.HasData, err)
	}
	if api.total() == calls {
		t.Error("a corrupt cache entry was served, want it ignored and fetched again")
	}
}

func TestBrandLookupWorksWhenRedisIsDown(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	down := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	t.Cleanup(func() {
		if err := down.Close(); err != nil {
			t.Errorf("close redis: %v", err)
		}
	})
	svc.redis = down

	result, err := svc.BrandLookup(context.Background(), newOrg(), "p", lookupInput())

	if err != nil || !result.HasData {
		t.Fatalf("BrandLookup() = hasData %v, error %v; want data without the cache", result.HasData, err)
	}
}

func TestBrandLookupShareOfVoiceSurvivesOnePlatformFailing(t *testing.T) {
	api := newFakeAPI(t)
	api.on(crossAggregatedPath, func(_ int, body string) taskReply {
		if strings.Contains(body, `"platform":"chat_gpt"`) {
			return failReply(50000, "Internal Error.")
		}
		return defaultReply(crossAggregatedPath)
	})
	svc := newTestService(t, api)
	org := newOrg()
	in := lookupInput()
	in.LocationCode = 2840
	in.Competitors = []string{"rival.com"}

	result, err := svc.BrandLookup(context.Background(), org, "p", in)
	if err != nil {
		t.Fatal(err)
	}

	if sov := result.ShareOfVoice; sov == nil || len(sov.Platforms) != 1 || sov.Platforms[0] != platformGoogle {
		t.Errorf("ShareOfVoice = %+v, want google only", sov)
	}
	calls := api.total()
	if _, err := svc.BrandLookup(context.Background(), org, "p", in); err != nil {
		t.Fatal(err)
	}
	if api.total() == calls {
		t.Error("a result with a failed share-of-voice call was cached, want it fetched again")
	}
}

func TestBrandLookupBillingIssueDuringShareOfVoiceFailsTheLookup(t *testing.T) {
	api := newFakeAPI(t)
	api.on(crossAggregatedPath, func(int, string) taskReply { return failReply(40200, "Payment Required.") })
	svc := newTestService(t, api)
	in := lookupInput()
	in.Competitors = []string{"rival.com"}

	if _, err := svc.BrandLookup(context.Background(), newOrg(), "p", in); !errors.Is(err, dataforseo.ErrBillingIssue) {
		t.Fatalf("BrandLookup() error = %v, want ErrBillingIssue", err)
	}
}

func TestBrandLookupAllSubCallsFailingMarksThePlatformFailed(t *testing.T) {
	api := newFakeAPI(t)
	for _, path := range []string{aggregatedPath, topPagesPath, mentionsSearchPath} {
		api.on(path, func(_ int, body string) taskReply {
			if strings.Contains(body, `"platform":"chat_gpt"`) {
				return failReply(50000, "Internal Error.")
			}
			return defaultReply(path)
		})
	}
	svc := newTestService(t, api)
	in := lookupInput()
	in.LocationCode = 2840

	result, err := svc.BrandLookup(context.Background(), newOrg(), "p", in)
	if err != nil {
		t.Fatal(err)
	}

	if got := result.PerPlatform[0]; got.Platform != platformChatGPT || got.Status != "error" {
		t.Errorf("chat_gpt row = %+v, want status error", got)
	}
	if got := result.PerPlatform[1]; got.Status != "success" {
		t.Errorf("google row = %+v, want success", got)
	}
}
