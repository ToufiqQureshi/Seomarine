package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/keywords"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

type keywordMetricToolRow struct {
	Keyword           string           `json:"keyword"`
	SearchVolume      *int             `json:"search_volume"`
	KeywordDifficulty *int             `json:"keyword_difficulty"`
	MainIntent        string           `json:"main_intent"`
	CPC               *float64         `json:"cpc"`
	Competition       *float64         `json:"competition"`
	CompetitionLevel  *string          `json:"competition_level"`
	MonthlySearches   []map[string]any `json:"monthly_searches"`
}

func getKeywordMetricsTool() *tool {
	return &tool{
		Name: "get_keyword_metrics", Title: "Get keyword metrics",
		Description: "Hydrate up to 700 known keywords with search volume, keyword difficulty, search intent, CPC, competition, and monthly trends in one call. Google Ads-only countries do not provide difficulty or intent. Charges credits.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"keywords":{"type":"array","items":{"type":"string","minLength":1,"maxLength":80},"minItems":1,"maxItems":700},"locationCode":{"type":"integer","minimum":1},"languageCode":{"type":"string","minLength":2,"maxLength":8},"includeMonthlyTrends":{"type":"boolean"},"includeClickstreamData":{"type":"boolean"},"sortBy":{"type":"string","enum":["search_volume","keyword_difficulty","cpc","competition"]}},"required":["projectId","keywords"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"keywords":{"type":"array","items":{"type":"object"}}},"additionalProperties":true}`),
		Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": true, "destructiveHint": false},
		Handler: handleGetKeywordMetrics,
	}
}

func handleGetKeywordMetrics(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string   `json:"projectId"`
		Keywords []string  `json:"keywords"`
		LocationCode *int  `json:"locationCode"`
		LanguageCode string `json:"languageCode"`
		IncludeMonthlyTrends *bool `json:"includeMonthlyTrends"`
		IncludeClickstreamData bool `json:"includeClickstreamData"`
		SortBy string `json:"sortBy"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and keywords are required")
	}
	if len(args.Keywords) < 1 || len(args.Keywords) > 700 {
		return nil, newAppErrorf("VALIDATION_ERROR", "keywords must contain 1 to 700 items")
	}
	for _, keyword := range args.Keywords {
		if strings.TrimSpace(keyword) == "" || len([]rune(strings.TrimSpace(keyword))) > 80 {
			return nil, newAppErrorf("VALIDATION_ERROR", "each keyword must contain 1 to 80 characters")
		}
	}
	if args.LocationCode != nil && *args.LocationCode < 1 {
		return nil, newAppErrorf("VALIDATION_ERROR", "locationCode must be positive")
	}
	if args.LanguageCode != "" && (len(strings.TrimSpace(args.LanguageCode)) < 2 || len(strings.TrimSpace(args.LanguageCode)) > 8) {
		return nil, newAppErrorf("VALIDATION_ERROR", "languageCode must be 2 to 8 characters")
	}
	switch args.SortBy {
	case "", "search_volume", "keyword_difficulty", "cpc", "competition":
	default:
		return nil, newAppErrorf("VALIDATION_ERROR", "sortBy is invalid")
	}
	if env.deps.KeywordResearch == nil {
		return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Keyword research is not configured on this server.")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil {
		return nil, err
	}
	locationCode := 0
	if args.LocationCode != nil {
		locationCode = *args.LocationCode
	}
	pair, err := keywords.ResolveMarket(locationCode, strings.TrimSpace(args.LanguageCode), market.Pair{
		LocationCode: access.Project.LocationCode, LanguageCode: access.Project.LanguageCode,
	})
	if err != nil {
		return nil, newAppErrorf("VALIDATION_ERROR", err.Error())
	}
	if !market.IsLanguageServedForLocation(pair.LocationCode, pair.LanguageCode) {
		return nil, newAppErrorf("VALIDATION_ERROR", "Language is not available for this location.")
	}
	rows, err := env.deps.KeywordResearch.MetricLookup(ctx, keywords.MetricLookupInput{
		OrganizationID: access.Auth.OrganizationID, Keywords: args.Keywords, LocationCode: pair.LocationCode,
		LanguageCode: pair.LanguageCode, Clickstream: args.IncludeClickstreamData,
	})
	if err != nil {
		var validation keywords.ValidationError
		if errors.As(err, &validation) {
			return nil, newAppErrorf("VALIDATION_ERROR", validation.Error())
		}
		env.deps.Logger.ErrorContext(ctx, "MCP keyword metrics", "project_id", access.Project.ID, "err", err)
		return nil, newAppErrorf("INTERNAL_ERROR", "Unable to fetch keyword metrics.")
	}

	out := make([]keywordMetricToolRow, 0, len(rows))
	for _, row := range rows {
		trends := make([]map[string]any, 0, len(row.Monthly))
		for _, month := range row.Monthly {
			trends = append(trends, map[string]any{"year": month.Year, "month": month.Month, "search_volume": month.SearchVolume})
		}
		var monthly []map[string]any
		if len(trends) > 0 { monthly = trends }
		out = append(out, keywordMetricToolRow{
			Keyword: row.Keyword, SearchVolume: row.SearchVolume, KeywordDifficulty: row.KeywordDifficulty,
			MainIntent: row.Intent, CPC: row.CPC, Competition: row.Competition, CompetitionLevel: row.CompetitionLevel,
			MonthlySearches: monthly,
		})
	}
	sortBy := args.SortBy
	if sortBy == "" { sortBy = "search_volume" }
	sort.SliceStable(out, func(i, j int) bool {
		return metricSortValue(out[i], sortBy) > metricSortValue(out[j], sortBy)
	})

	// Omit monthly_searches entirely when callers opt out, matching the
	// TypeScript response contract.
	structuredRows := make([]map[string]any, 0, len(out))
	for _, row := range out {
		item := map[string]any{
			"keyword": row.Keyword, "search_volume": row.SearchVolume, "keyword_difficulty": row.KeywordDifficulty,
			"main_intent": row.MainIntent, "cpc": row.CPC, "competition": row.Competition,
			"competition_level": row.CompetitionLevel,
		}
		if args.IncludeMonthlyTrends == nil || *args.IncludeMonthlyTrends {
			item["monthly_searches"] = row.MonthlySearches
		}
		structuredRows = append(structuredRows, item)
	}
	header := fmt.Sprintf("Fetched metrics for %d keywords. Columns: volume = monthly searches, KD = keyword difficulty (0-100), CPC in USD, competition = paid competition (0-1); \"—\" = unavailable.", len(out))
	lines := []string{header}
	for _, row := range out {
		lines = append(lines, fmt.Sprintf("%s | %s | %s | %s | %s | %s",
			row.Keyword, nullableMetric(row.SearchVolume), nullableMetric(row.KeywordDifficulty),
			nullableMetric(row.CPC), nullableMetric(row.Competition), row.MainIntent))
	}
	return mcpResponse(strings.Join(lines, "\n"), map[string]any{"keywords": structuredRows},
		metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/keywords", nil)}), nil
}

func metricSortValue(row keywordMetricToolRow, field string) float64 {
	switch field {
	case "keyword_difficulty":
		if row.KeywordDifficulty != nil { return float64(*row.KeywordDifficulty) }
	case "cpc":
		if row.CPC != nil { return *row.CPC }
	case "competition":
		if row.Competition != nil { return *row.Competition }
	default:
		if row.SearchVolume != nil { return float64(*row.SearchVolume) }
	}
	return 0
}

func nullableMetric[T any](value *T) string {
	if value == nil { return "—" }
	return fmt.Sprint(*value)
}
