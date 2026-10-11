package domain

import (
	"context"
	"fmt"
	"strings"
)

// MCPRankedKeywordsInput contains the bounded filters exposed by the MCP tool.
type MCPRankedKeywordsInput struct {
	Target            string
	Scope             Scope
	LocationCode      int
	LanguageCode      string
	Limit             int
	Offset            int
	SortBy            string
	MinSearchVolume   *int
	MaxRank           *int
	ExcludeBrandTerms []string
	ResultTypes       []string
}

// MCPRankedKeyword is the compact row returned by get_ranked_keywords.
type MCPRankedKeyword struct {
	Keyword string   `json:"keyword"`
	Rank    *float64 `json:"rank,omitempty"`
	Volume  *float64 `json:"volume,omitempty"`
	CPC     *float64 `json:"cpc,omitempty"`
	URL     *string  `json:"url,omitempty"`
}

// MCPRankedKeywordsResult is one bounded page of ranked keywords.
type MCPRankedKeywordsResult struct {
	Items      []MCPRankedKeyword `json:"keywords"`
	TotalCount *int               `json:"totalCount"`
}

// RankedKeywordsForMCP returns scoped, filtered ranked keyword rows for MCP.
// Validation is completed before the provider is called because requests are billed.
func (s *Service) RankedKeywordsForMCP(ctx context.Context, organizationID, projectID string, input MCPRankedKeywordsInput) (MCPRankedKeywordsResult, error) {
	target, err := parseResearchTarget(input.Target, input.Scope)
	if err != nil {
		return MCPRankedKeywordsResult{}, badRequest(err.Error())
	}
	limit := input.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return MCPRankedKeywordsResult{}, badRequest("limit must be between 1 and 100.")
	}
	if input.Offset < 0 || input.Offset > 1000 {
		return MCPRankedKeywordsResult{}, badRequest("offset must be between 0 and 1000.")
	}
	orderBy := map[string]string{
		"rank": "ranked_serp_element.serp_item.rank_absolute,asc",
		"traffic_estimate": "ranked_serp_element.serp_item.etv,desc",
		"cpc": "keyword_data.keyword_info.cpc,desc",
		"search_volume": "keyword_data.keyword_info.search_volume,desc",
	}[input.SortBy]
	if input.SortBy == "" {
		orderBy = "keyword_data.keyword_info.search_volume,desc"
	}
	if orderBy == "" {
		return MCPRankedKeywordsResult{}, badRequest("sortBy is invalid.")
	}
	for _, resultType := range input.ResultTypes {
		switch resultType {
		case "organic", "paid", "featured_snippet", "local_pack", "ai_overview_reference":
		default:
			return MCPRankedKeywordsResult{}, badRequest("resultTypes contains an unsupported value.")
		}
	}
	if len(input.ResultTypes) > 5 {
		return MCPRankedKeywordsResult{}, badRequest("resultTypes must contain at most 5 values.")
	}
	if input.MinSearchVolume != nil && *input.MinSearchVolume < 0 {
		return MCPRankedKeywordsResult{}, badRequest("minSearchVolume must be non-negative.")
	}
	if input.MaxRank != nil && (*input.MaxRank < 1 || *input.MaxRank > 100) {
		return MCPRankedKeywordsResult{}, badRequest("maxRank must be between 1 and 100.")
	}
	if len(input.ExcludeBrandTerms) > 10 {
		return MCPRankedKeywordsResult{}, badRequest("excludeBrandTerms must contain at most 10 values.")
	}

	scope := buildRankedKeywordsScopeFilter(target)
	filters := append([]any(nil), scope.clauses...)
	conditions := scope.count
	if input.MinSearchVolume != nil {
		filters = append(filters, []any{"keyword_data.keyword_info.search_volume", ">=", *input.MinSearchVolume})
		conditions++
	}
	if input.MaxRank != nil {
		filters = append(filters, []any{"ranked_serp_element.serp_item.rank_absolute", "<=", *input.MaxRank})
		conditions++
	}
	for _, term := range input.ExcludeBrandTerms {
		term = strings.TrimSpace(term)
		if term == "" || len(term) > 80 {
			return MCPRankedKeywordsResult{}, badRequest("excludeBrandTerms entries must contain 1 to 80 characters.")
		}
		filters = append(filters, []any{"keyword_data.keyword", "not_ilike", "%" + escapeLikeTerm(term) + "%"})
		conditions++
	}
	if conditions > maxFilterConditions {
		return MCPRankedKeywordsResult{}, badRequest(fmt.Sprintf("Too many filter conditions (maximum %d).", maxFilterConditions))
	}

	request := map[string]any{
		"target": target.Hostname,
		"location_code": normalizeLocationCode(input.LocationCode),
		"language_code": normalizeLanguageCode(input.LanguageCode),
		"limit": limit,
		"offset": input.Offset,
		"order_by": []string{orderBy},
	}
	if len(filters) > 0 {
		request["filters"] = join(filters, "and")
	}
	if len(input.ResultTypes) > 0 {
		request["item_types"] = input.ResultTypes
	}
	response, err := s.provider.rankedKeywords(ctx, organizationID, request)
	if err != nil {
		return MCPRankedKeywordsResult{}, err
	}
	rows := make([]MCPRankedKeyword, 0, len(response.Items))
	for _, item := range response.Items {
		keyword := item.Keyword
		var volume, cpc *float64
		if item.KeywordData != nil {
			if item.KeywordData.Keyword != nil {
				keyword = item.KeywordData.Keyword
			}
			if item.KeywordData.KeywordInfo != nil {
				volume = item.KeywordData.KeywordInfo.SearchVolume
				cpc = item.KeywordData.KeywordInfo.CPC
			}
		}
		if keyword == nil || *keyword == "" {
			continue
		}
		var rank *float64
		var pageURL *string
		if element := item.RankedSERPElement; element != nil {
			rank, pageURL = element.RankAbsolute, element.URL
			if serp := element.SERPItem; serp != nil {
				if serp.RankAbsolute != nil {
					rank = serp.RankAbsolute
				}
				if serp.URL != nil {
					pageURL = serp.URL
				}
			}
		}
		rows = append(rows, MCPRankedKeyword{Keyword: *keyword, Rank: rank, Volume: volume, CPC: cpc, URL: pageURL})
	}
	return MCPRankedKeywordsResult{Items: rows, TotalCount: response.TotalCount}, nil
}
