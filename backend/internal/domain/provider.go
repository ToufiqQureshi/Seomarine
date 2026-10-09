package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

const (
	domainRankOverviewPath = "/v3/dataforseo_labs/google/domain_rank_overview/live"
	rankedKeywordsPath     = "/v3/dataforseo_labs/google/ranked_keywords/live"
	relevantPagesPath      = "/v3/dataforseo_labs/google/relevant_pages/live"
)

type provider struct {
	client *dataforseo.Client
}

type organicMetrics struct {
	ETV   *float64 `json:"etv"`
	Count *float64 `json:"count"`
}

type providerMetrics struct {
	Organic *organicMetrics `json:"organic"`
}

type rankedKeywordItem struct {
	KeywordData *struct {
		Keyword     *string `json:"keyword"`
		KeywordInfo *struct {
			SearchVolume      *float64 `json:"search_volume"`
			CPC               *float64 `json:"cpc"`
			KeywordDifficulty *float64 `json:"keyword_difficulty"`
		} `json:"keyword_info"`
		KeywordProperties *struct {
			KeywordDifficulty *float64 `json:"keyword_difficulty"`
		} `json:"keyword_properties"`
	} `json:"keyword_data"`
	Keyword           *string `json:"keyword"`
	RankedSERPElement *struct {
		SERPItem *struct {
			URL          *string  `json:"url"`
			RelativeURL  *string  `json:"relative_url"`
			RankAbsolute *float64 `json:"rank_absolute"`
			ETV          *float64 `json:"etv"`
		} `json:"serp_item"`
		URL          *string  `json:"url"`
		RelativeURL  *string  `json:"relative_url"`
		RankAbsolute *float64 `json:"rank_absolute"`
		ETV          *float64 `json:"etv"`
	} `json:"ranked_serp_element"`
}

type relevantPageItem struct {
	PageAddress *string          `json:"page_address"`
	Metrics     *providerMetrics `json:"metrics"`
}

type rankedKeywordsResponse struct {
	Items      []rankedKeywordItem `json:"items"`
	TotalCount *int                `json:"total_count"`
}

type relevantPagesResponse struct {
	Items      []relevantPageItem `json:"items"`
	TotalCount *int               `json:"total_count"`
}

func (p *provider) overview(ctx context.Context, organizationID, target string, locationCode int, languageCode string) (providerMetrics, error) {
	results, err := p.post(ctx, organizationID, domainRankOverviewPath, []map[string]any{{
		"target": target, "location_code": locationCode, "language_code": languageCode, "limit": 1,
	}})
	if err != nil || len(results) == 0 {
		return providerMetrics{}, err
	}
	var result struct {
		Items []struct {
			Metrics *providerMetrics `json:"metrics"`
		} `json:"items"`
	}
	if err := json.Unmarshal(results[0], &result); err != nil {
		return providerMetrics{}, fmt.Errorf("decode domain rank overview: %w", err)
	}
	if len(result.Items) == 0 || result.Items[0].Metrics == nil {
		return providerMetrics{}, nil
	}
	return *result.Items[0].Metrics, nil
}

func (p *provider) rankedKeywords(ctx context.Context, organizationID string, request map[string]any) (rankedKeywordsResponse, error) {
	results, err := p.post(ctx, organizationID, rankedKeywordsPath, []map[string]any{request})
	if err != nil || len(results) == 0 {
		return rankedKeywordsResponse{Items: []rankedKeywordItem{}}, err
	}
	var result rankedKeywordsResponse
	if err := json.Unmarshal(results[0], &result); err != nil {
		return rankedKeywordsResponse{}, fmt.Errorf("decode ranked keywords: %w", err)
	}
	if result.Items == nil {
		result.Items = []rankedKeywordItem{}
	}
	return result, nil
}

func (p *provider) relevantPages(ctx context.Context, organizationID string, request map[string]any) (relevantPagesResponse, error) {
	results, err := p.post(ctx, organizationID, relevantPagesPath, []map[string]any{request})
	if err != nil || len(results) == 0 {
		return relevantPagesResponse{Items: []relevantPageItem{}}, err
	}
	var result relevantPagesResponse
	if err := json.Unmarshal(results[0], &result); err != nil {
		return relevantPagesResponse{}, fmt.Errorf("decode relevant pages: %w", err)
	}
	if result.Items == nil {
		result.Items = []relevantPageItem{}
	}
	return result, nil
}

func (p *provider) post(ctx context.Context, organizationID, path string, body any) ([]json.RawMessage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode %s request: %w", path, err)
	}
	response, err := p.client.Do(ctx, organizationID, http.MethodPost, path, payload, false)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", path, err)
	}
	results, err := dataforseo.Results(response)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", path, err)
	}
	return results, nil
}
