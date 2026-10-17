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
		{"live serp default depth is 10", "/v3/serp/google/organic/live/advanced", `[{"keyword":"shoes"}]`, 0.002},
		{"live serp with operator", "/v3/serp/google/organic/live/advanced", `[{"keyword":"site:example.com shoes","depth":10}]`, 0.01},
		{"queued serp per task", "/v3/serp/google/organic/task_post", `[{"keyword":"a","depth":20},{"keyword":"b","depth":20}]`, 2 * (0.0006 + 0.0006)},
		{"queued serp priority", "/v3/serp/google/organic/task_post", `[{"keyword":"a","depth":10,"priority":2}]`, 0.0012},
		{"labs with limit at July 2026 rates", "/v3/dataforseo_labs/google/keyword_ideas/live", `[{"limit":100}]`, 0.024},
		{"labs without limit holds the maximum rows at July 2026 rates", "/v3/dataforseo_labs/google/keyword_overview/live", `[{}]`, 0.132},
		{"backlinks at July 2026 rates", "/v3/backlinks/referring_domains/live", `[{"limit":1000}]`, 0.06},
		{"google ads live", "/v3/keywords_data/google_ads/search_volume/live", `[{}]`, 0.09},
		{"ai visibility", "/v3/ai_optimization/llm_mentions/search/live", `[{}]`, 0.5},
		{"lighthouse", "/v3/on_page/lighthouse/live/json", `[{}]`, 0.005},
		{"business info live", "/v3/business_data/google/my_business_info/live", `[{}]`, 0.0054},
		{"business reviews default priority", "/v3/business_data/google/reviews/task_post", `[{"depth":100,"priority":2}]`, 0.015},
		{"business reviews normal priority", "/v3/business_data/google/reviews/task_post", `[{"depth":20,"priority":1}]`, 0.0015},
		{"extended reviews keyword at priority", "/v3/business_data/google/extended_reviews/task_post", `[{"depth":40,"priority":2,"keyword":"venue"}]`, 0.009},
		{"business questions live", "/v3/business_data/google/questions_and_answers/live", `[{"depth":40}]`, 0.005},
		{"business updates at priority", "/v3/business_data/google/my_business_updates/task_post", `[{"depth":20,"priority":2}]`, 0.006},
		{"business listings paid rows", "/v3/business_data/business_listings/search/live", `[{"limit":100}]`, 0.048},
		{"unknown endpoint is held high", "/v3/content_analysis/search/live", `[{}]`, 1},
		{"unknown business endpoint is held high", "/v3/business_data/unknown/live", `[{}]`, 1},
		{"body that is not a task list is one task", "/v3/on_page/lighthouse/live/json", `{"url":"x"}`, 0.005},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := EstimateUSD(tc.path, []byte(tc.body)); math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("EstimateUSD() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEstimateUSDForApprovedBriefCalls(t *testing.T) {
	serp := EstimateUSD("/v3/serp/google/organic/live/advanced", []byte(`[{"depth":30}]`))
	parsing := EstimateUSD("/v3/on_page/content_parsing/live", []byte(`[{},{},{},{},{},{},{},{}]`))
	if math.Abs(serp-0.006) > 1e-9 || math.Abs(parsing-0.0012) > 1e-9 || math.Abs(serp+parsing-0.0072) > 1e-9 {
		t.Fatalf("brief ceilings SERP=%v parse=%v total=%v, want 0.006, 0.0012, 0.0072", serp, parsing, serp+parsing)
	}
}
