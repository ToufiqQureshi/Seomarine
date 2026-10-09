package keywords

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

func TestIsAdsKeyword(t *testing.T) {
	tests := []struct {
		keyword string
		want    bool
	}{
		{"best seo tool", true},
		{"日本語 キーワード", true},
		{"café près", true},
		{"c++ tutorial", true},
		{"100% free", false},
		{"what is seo?", false},
		{"seo, tools", false},
		{"rocket 🚀 seo", false},
		{"star ⭐ seo", false},
		{"a b c d e f g h i j k", false},
		{strings.Repeat("a", 81), false},
		{strings.Repeat("a", 80), true},
	}
	for _, tt := range tests {
		if got := IsAdsKeyword(tt.keyword); got != tt.want {
			t.Errorf("IsAdsKeyword(%q) = %v, want %v", tt.keyword, got, tt.want)
		}
	}
}

type providerServer struct {
	mu     sync.Mutex
	paths  []string
	bodies []map[string]any
	reply  func(path string) map[string]any
}

func newProviderServer(t *testing.T, reply func(path string) map[string]any) (*providerServer, DataForSEOProvider) {
	t.Helper()
	ps := &providerServer{reply: reply}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		ps.mu.Lock()
		ps.paths = append(ps.paths, r.URL.Path)
		if len(body) > 0 {
			ps.bodies = append(ps.bodies, body[0])
		}
		ps.mu.Unlock()
		task := ps.reply(r.URL.Path)
		task["path"] = []string{"v3"}
		task["cost"] = 0.01
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "tasks": []any{task}})
	}))
	t.Cleanup(server.Close)
	client, err := dataforseo.NewClient(dataforseo.Options{BaseURL: server.URL, APIKey: base64.StdEncoding.EncodeToString([]byte("fixture-user:fixture-password")), Recorder: noopRecorder{}})
	if err != nil {
		t.Fatal(err)
	}
	return ps, DataForSEOProvider{Client: client}
}

func okTask(result ...any) map[string]any {
	return map[string]any{"status_code": 20000, "status_message": "Ok.", "result": result}
}

func TestLabsKeywordsRequestsAndParsing(t *testing.T) {
	item := func(keyword string, volume int) map[string]any {
		return map[string]any{"keyword": keyword,
			"keyword_info": map[string]any{"search_volume": volume, "cpc": 1.5, "competition": 0.2, "monthly_searches": []any{map[string]any{"year": 2026, "month": 9, "search_volume": volume}, map[string]any{"year": nil}}},
			"keyword_info_normalized_with_clickstream": map[string]any{"search_volume": volume * 2},
			"keyword_properties":                       map[string]any{"keyword_difficulty": 33},
			"search_intent_info":                       map[string]any{"main_intent": "commercial"}}
	}
	ps, p := newProviderServer(t, func(path string) map[string]any {
		if strings.HasSuffix(path, "related_keywords/live") {
			return okTask(map[string]any{"items": []any{map[string]any{"keyword_data": item("wrapped", 7)}, map[string]any{"keyword_data": nil}}})
		}
		return okTask(map[string]any{"items": []any{item("alpha", 10), map[string]any{"keyword": nil}, map[string]any{"keyword": ""}}})
	})
	ctx := context.Background()
	req := LabsRequest{Seed: "seed", LocationCode: 2840, LanguageCode: "en", Limit: 150, Clickstream: true, IgnoreSynonyms: true}

	for _, c := range []struct {
		source LabsSource
		path   string
	}{{SourceSuggestions, labsSuggestionsPath}, {SourceIdeas, labsIdeasPath}} {
		source, wantPath := c.source, c.path
		req.Source = source
		items, err := p.LabsKeywords(ctx, "org", req)
		if err != nil || len(items) != 1 || items[0].Keyword != "alpha" {
			t.Fatalf("%s = %+v, %v", source, items, err)
		}
		got := items[0]
		if *got.Info.SearchVolume != 10 || *got.Clickstream.SearchVolume != 20 || *got.Difficulty != 33 || *got.MainIntent != "commercial" ||
			len(got.Info.Monthly) != 2 || got.Info.Monthly[1].Year != 0 {
			t.Errorf("%s item = %+v", source, got)
		}
		if last := ps.paths[len(ps.paths)-1]; last != wantPath {
			t.Errorf("%s path = %s, want %s", source, last, wantPath)
		}
	}
	suggestions, ideas := ps.bodies[0], ps.bodies[1]
	if suggestions["keyword"] != "seed" || suggestions["include_seed_keyword"] != true || suggestions["exact_match"] != false ||
		suggestions["include_clickstream_data"] != true || suggestions["ignore_synonyms"] != true || suggestions["limit"] != float64(150) {
		t.Errorf("suggestions request = %v", suggestions)
	}
	if kws, _ := ideas["keywords"].([]any); len(kws) != 1 || kws[0] != "seed" || ideas["closely_variants"] != false {
		t.Errorf("ideas request = %v", ideas)
	}

	req.Source = SourceRelated
	related, err := p.LabsKeywords(ctx, "org", req)
	if err != nil || len(related) != 1 || related[0].Keyword != "wrapped" {
		t.Fatalf("related = %+v, %v; want the nested keyword_data unwrapped and empty wrappers skipped", related, err)
	}
	if ps.bodies[2]["depth"] != float64(3) {
		t.Errorf("related request = %v", ps.bodies[2])
	}
	if _, err := p.LabsKeywords(ctx, "org", LabsRequest{Source: "bogus"}); err == nil {
		t.Error("an unknown source was accepted")
	}

	overview, err := p.LabsOverview(ctx, "org", []string{"alpha"}, 2840, "en", false)
	if err != nil || len(overview) != 1 || ps.paths[len(ps.paths)-1] != labsOverviewPath {
		t.Errorf("overview = %+v, %v, path %s", overview, err, ps.paths[len(ps.paths)-1])
	}
}

func TestAdsRequestsAndParsing(t *testing.T) {
	ps, p := newProviderServer(t, func(_ string) map[string]any {
		rows := []any{}
		for i := range 5 {
			rows = append(rows, map[string]any{"keyword": fmt.Sprintf("idea %d", i), "search_volume": 100 - i, "competition_index": 40.0, "cpc": 0.5})
		}
		return okTask(rows...)
	})
	ctx := context.Background()
	ideas, err := p.AdsIdeas(ctx, "org", "seed", 2356, "hi", 3)
	if err != nil || len(ideas) != 3 || *ideas[0].SearchVolume != 100 || *ideas[0].CompetitionIndex != 40 {
		t.Fatalf("AdsIdeas() = %+v, %v; want the first 3 of the 5 returned", ideas, err)
	}
	if ps.paths[0] != adsIdeasPath || ps.bodies[0]["sort_by"] != "search_volume" {
		t.Errorf("ideas request = %s %v", ps.paths[0], ps.bodies[0])
	}

	city := "Pune,Maharashtra,India"
	if _, err := p.AdsVolume(ctx, "org", AdsVolumeRequest{Keywords: []string{"ok one", "bad, one", "ok two"}, LocationCode: 2356, LocationName: &city, LanguageCode: "en"}); err != nil {
		t.Fatal(err)
	}
	body := ps.bodies[1]
	if kws := body["keywords"].([]any); len(kws) != 2 || body["location_name"] != city || body["location_code"] != nil {
		t.Errorf("volume request = %v, want the rejected keyword left out and the city used", body)
	}
	before := len(ps.paths)
	if items, err := p.AdsVolume(ctx, "org", AdsVolumeRequest{Keywords: []string{"bad, one"}, LocationCode: 2356, LanguageCode: "en"}); err != nil || items != nil || len(ps.paths) != before {
		t.Errorf("only rejected keywords: items %v, err %v, calls %d -> %d; want no provider call", items, err, before, len(ps.paths))
	}
	if _, err := p.AdsVolume(ctx, "org", AdsVolumeRequest{Keywords: []string{"fine"}, LocationCode: 2356, LanguageCode: "en"}); err != nil {
		t.Fatal(err)
	}
	if ps.bodies[2]["location_code"] != float64(2356) || ps.bodies[2]["location_name"] != nil {
		t.Errorf("national volume request = %v", ps.bodies[2])
	}
}

func TestSerpLive(t *testing.T) {
	status := 20000
	ps, p := newProviderServer(t, func(string) map[string]any {
		task := okTask(map[string]any{"items": []any{
			map[string]any{"type": "organic", "rank_group": 1, "rank_absolute": 2, "domain": "a.com", "title": "A", "url": "https://a.com", "description": "d", "etv": 12.5,
				"estimated_paid_traffic_cost": 3.0, "backlinks_info": map[string]any{"referring_domains": 10, "backlinks": 40}},
			map[string]any{"type": "people_also_ask"},
		}})
		task["status_code"] = status
		return task
	})
	ctx := context.Background()
	city := "Pune,Maharashtra,India"
	items, err := p.SerpLive(ctx, "org", SerpRequest{Keyword: "seo", LocationCode: 2356, LanguageCode: "en", LocationName: &city, Depth: 20})
	if err != nil || len(items) != 2 {
		t.Fatalf("SerpLive() = %+v, %v", items, err)
	}
	organic := items[0]
	if organic.Type != "organic" || *organic.RankGroup != 1 || organic.Domain != "a.com" || *organic.Etv != 12.5 || *organic.Backlinks != 40 || *organic.ReferringDomains != 10 {
		t.Errorf("organic item = %+v", organic)
	}
	if b := ps.bodies[0]; b["location_name"] != city || b["location_code"] != nil || b["depth"] != float64(20) || b["device"] != "desktop" {
		t.Errorf("request = %v", b)
	}
	status = 40501
	if items, err := p.SerpLive(ctx, "org", SerpRequest{Keyword: "seo", LocationCode: 2840, LanguageCode: "en", Depth: 20}); err != nil || len(items) != 0 {
		t.Errorf("no-results SERP = %+v, %v; want an empty list, not an error", items, err)
	}
	before := len(ps.paths)
	for _, depth := range []int{0, 15, 110} {
		if _, err := p.SerpLive(ctx, "org", SerpRequest{Keyword: "seo", LocationCode: 2840, LanguageCode: "en", Depth: depth}); err == nil {
			t.Errorf("depth %d was accepted", depth)
		}
	}
	if len(ps.paths) != before {
		t.Error("an invalid depth reached the provider, where it would be billed")
	}
}
