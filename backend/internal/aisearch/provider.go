package aisearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

const (
	mentionsSearchPath  = "/v3/ai_optimization/llm_mentions/search/live"
	aggregatedPath      = "/v3/ai_optimization/llm_mentions/aggregated_metrics/live"
	topPagesPath        = "/v3/ai_optimization/llm_mentions/top_pages/live"
	crossAggregatedPath = "/v3/ai_optimization/llm_mentions/cross_aggregated_metrics/live"

	minCrossGroups = 2
	maxCrossGroups = 10
)

// provider calls DataForSEO's AI optimization endpoints. Every call is billed
// to an organization, so none is retried: a replayed live call could be
// charged twice.
type provider struct {
	client *dataforseo.Client
	models *modelCatalog
}

// llmTarget is the provider's target entry: a domain or a keyword.
type llmTarget map[string]any

// buildLLMTarget builds the single target of a call. includeSubdomains only
// applies to domains; the API has no URL-level targeting, so page scopes are
// applied to the returned rows instead.
func buildLLMTarget(target Target, includeSubdomains bool) llmTarget {
	if target.Type == TargetDomain {
		return llmTarget{
			"domain":             target.Value,
			"include_subdomains": includeSubdomains,
			"search_filter":      "include",
			"search_scope":       []string{"any"},
		}
	}
	return llmTarget{
		"keyword":       target.Value,
		"search_filter": "include",
		"search_scope":  []string{"any", "brand_entities"},
		"match_type":    "word_match",
	}
}

// platformQuery is what every mentions endpoint needs.
type platformQuery struct {
	organizationID string
	target         llmTarget
	platform       string
	locationCode   int
	languageCode   string
}

func (q platformQuery) body(extra map[string]any) []map[string]any {
	body := map[string]any{
		"target":        []llmTarget{q.target},
		"platform":      q.platform,
		"location_code": q.locationCode,
		"language_code": q.languageCode,
	}
	for k, v := range extra {
		body[k] = v
	}
	return []map[string]any{body}
}

func clamp(value, lo, hi int) int { return min(hi, max(lo, value)) }

func (p *provider) post(ctx context.Context, organizationID, path string, body any) ([]json.RawMessage, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode %s request: %w", path, err)
	}
	payload, err := p.client.Do(ctx, organizationID, http.MethodPost, path, raw, false)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", path, err)
	}
	results, err := dataforseo.Results(payload)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", path, err)
	}
	return results, nil
}

// items reads result[0].items.
func items[T any](path string, results []json.RawMessage) ([]T, error) {
	if len(results) == 0 {
		return nil, nil
	}
	var first struct {
		Items []T `json:"items"`
	}
	if err := json.Unmarshal(results[0], &first); err != nil {
		return nil, fmt.Errorf("%s returned an invalid items shape: %w", path, err)
	}
	return first.Items, nil
}

// mentionsSearch returns the LLM answers that mention the target.
func (p *provider) mentionsSearch(ctx context.Context, q platformQuery, limit int) ([]mentionItem, error) {
	results, err := p.post(ctx, q.organizationID, mentionsSearchPath, q.body(map[string]any{"limit": clamp(limit, 1, 1000)}))
	if err != nil {
		return nil, err
	}
	return items[mentionItem](mentionsSearchPath, results)
}

// aggregatedMetrics returns the target's totals per platform.
func (p *provider) aggregatedMetrics(ctx context.Context, q platformQuery, listLimit int) (aggregatedTotal, error) {
	results, err := p.post(ctx, q.organizationID, aggregatedPath, q.body(map[string]any{"internal_list_limit": clamp(listLimit, 1, 20)}))
	if err != nil || len(results) == 0 {
		return aggregatedTotal{}, err
	}
	var first struct {
		Total aggregatedTotal `json:"total"`
	}
	if err := json.Unmarshal(results[0], &first); err != nil {
		return aggregatedTotal{}, fmt.Errorf("%s returned an invalid shape: %w", aggregatedPath, err)
	}
	return first.Total, nil
}

// topPages returns the pages the platform cites most for the target.
func (p *provider) topPages(ctx context.Context, q platformQuery, limit int) ([]topPagesItem, error) {
	results, err := p.post(ctx, q.organizationID, topPagesPath, q.body(map[string]any{
		"links_scope":         "sources",
		"items_list_limit":    clamp(limit, 1, 10),
		"internal_list_limit": 5,
	}))
	if err != nil {
		return nil, err
	}
	return items[topPagesItem](topPagesPath, results)
}

// crossGroup is one brand of a comparison, keyed by its label.
type crossGroup struct {
	key    string
	target llmTarget
}

// crossAggregatedMetrics compares 2 to 10 brands in one call and returns one
// item per brand, keyed by the label we sent.
func (p *provider) crossAggregatedMetrics(ctx context.Context, organizationID string, groups []crossGroup, platform string, locationCode int, languageCode string) ([]crossItem, error) {
	if len(groups) < minCrossGroups || len(groups) > maxCrossGroups {
		return nil, fmt.Errorf("cross-aggregated metrics need %d to %d target groups, got %d", minCrossGroups, maxCrossGroups, len(groups))
	}
	targets := make([]map[string]any, len(groups))
	for i, g := range groups {
		targets[i] = map[string]any{"aggregation_key": g.key, "target": []llmTarget{g.target}}
	}
	results, err := p.post(ctx, organizationID, crossAggregatedPath, []map[string]any{{
		"targets":             targets,
		"platform":            platform,
		"location_code":       locationCode,
		"language_code":       languageCode,
		"internal_list_limit": 5,
	}})
	if err != nil {
		return nil, err
	}
	return items[crossItem](crossAggregatedPath, results)
}

// responseRequest asks one model one question.
type responseRequest struct {
	organizationID string
	model          string
	modelName      string
	prompt         string
	webSearch      bool
	// countryCode geolocates the web search; empty for no preference.
	countryCode string
	maxTokens   int
}

// errInvalidRequest marks a request refused before it was sent, because the
// provider would reject it and still charge for the task.
var errInvalidRequest = errors.New("invalid LLM response request")

// llmResponse returns one model's answer.
func (p *provider) llmResponse(ctx context.Context, r responseRequest) (llmResponse, error) {
	if !slices.Contains(promptModels, r.model) {
		return llmResponse{}, fmt.Errorf("%w: unknown model %q", errInvalidRequest, r.model)
	}
	if !p.models.known(ctx, r.organizationID, r.model, r.modelName) {
		return llmResponse{}, fmt.Errorf("%w: unsupported model name %q for %s", errInvalidRequest, r.modelName, r.model)
	}
	if r.webSearch && r.countryCode != "" && !supportsWebSearchCountry(r.model, r.countryCode) {
		return llmResponse{}, fmt.Errorf("%w: unsupported web-search country %s for %s", errInvalidRequest, r.countryCode, r.model)
	}

	fields := map[string]any{
		"user_prompt":       r.prompt,
		"model_name":        r.modelName,
		"web_search":        r.webSearch,
		"max_output_tokens": clamp(r.maxTokens, 256, 4096),
	}
	// web_search only permits searching, and models often answer from memory
	// and return no citations. Claude accepts force_web_search to make the
	// search happen; every ChatGPT model rejects the field, and Gemini and
	// Perplexity do not document it, so only Claude gets it.
	if r.webSearch && r.model == modelClaude {
		fields["force_web_search"] = true
	}
	// The country only geolocates the search; every model rejects it when
	// search is off.
	if r.webSearch && r.countryCode != "" {
		fields["web_search_country_iso_code"] = r.countryCode
	}

	path := "/v3/ai_optimization/" + r.model + "/llm_responses/live"
	results, err := p.post(ctx, r.organizationID, path, []map[string]any{fields})
	if err != nil {
		return llmResponse{}, err
	}
	var response llmResponse
	if len(results) > 0 {
		if err := json.Unmarshal(results[0], &response); err != nil {
			return llmResponse{}, fmt.Errorf("%s returned an invalid response shape: %w", path, err)
		}
	}
	return response, nil
}

// supportsWebSearchCountry reports whether model accepts country as a
// web-search location.
func supportsWebSearchCountry(model, country string) bool {
	switch model {
	case modelChatGPT, modelPerplexity:
		return slices.Contains(webSearchCountries, country)
	case modelClaude:
		return slices.Contains(claudeCountries, country)
	}
	return false
}
