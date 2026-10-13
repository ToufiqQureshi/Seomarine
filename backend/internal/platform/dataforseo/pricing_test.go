package dataforseo

import (
	"math"
	"testing"
)

func TestEstimateUSDIsTheMostARequestCanCost(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		want             float64
	}{
		{"free appendix", "/v3/appendix/status", "", 0},
		{"free locations", "/v3/serp/google/locations/us", "", 0},
		{"free ai models", "/v3/ai_optimization/chat_gpt/llm_responses/models", "", 0},
		{"live serp top 10", "/v3/serp/google/organic/live/advanced", `[{"keyword":"shoes","depth":10}]`, 0.002},
		{"live serp default depth is 100", "/v3/serp/google/organic/live/advanced", `[{"keyword":"shoes"}]`, 0.002 + 9*0.0015},
		{"live serp with operator", "/v3/serp/google/organic/live/advanced", `[{"keyword":"site:example.com shoes","depth":10}]`, 0.01},
		{"queued serp per task", "/v3/serp/google/organic/task_post", `[{"keyword":"a","depth":20},{"keyword":"b","depth":20}]`, 2 * (0.0006 + 0.00045)},
		{"queued serp priority", "/v3/serp/google/organic/task_post", `[{"keyword":"a","depth":10,"priority":2}]`, 0.0012},
		{"labs with limit", "/v3/dataforseo_labs/google/keyword_ideas/live", `[{"limit":100}]`, 0.02},
		{"labs without limit holds the maximum rows", "/v3/dataforseo_labs/google/keyword_overview/live", `[{}]`, 0.11},
		{"backlinks", "/v3/backlinks/referring_domains/live", `[{"limit":1000}]`, 0.05},
		{"google ads", "/v3/keywords_data/google_ads/search_volume/live", `[{}]`, 0.1},
		{"ai visibility", "/v3/ai_optimization/llm_mentions/search/live", `[{}]`, 0.5},
		{"lighthouse", "/v3/on_page/lighthouse/live/json", `[{}]`, 0.01},
		{"business reviews depth", "/v3/business_data/google/reviews/task_post", `[{"depth":100}]`, 0.05 + 10*0.0015},
		{"unknown endpoint is held high", "/v3/content_analysis/search/live", `[{}]`, 1},
		{"body that is not a task list is one task", "/v3/on_page/lighthouse/live/json", `{"url":"x"}`, 0.01},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := EstimateUSD(tc.path, []byte(tc.body)); math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("EstimateUSD() = %v, want %v", got, tc.want)
			}
		})
	}
}
