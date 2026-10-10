package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/keywords"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

type serpToolItem struct {
	Type        *string `json:"type,omitempty"`
	Rank        *int    `json:"rank"`
	Title       *string `json:"title"`
	URL         *string `json:"url"`
	Domain      *string `json:"domain"`
	Description *string `json:"description"`
}

type serpToolQueryResult struct {
	Keyword string         `json:"keyword"`
	OK      bool           `json:"ok"`
	Items   []serpToolItem  `json:"items,omitempty"`
	Error   string         `json:"error,omitempty"`
}

func getSerpResultsTool() *tool {
	return &tool{
		Name: "get_serp_results", Title: "Get Google SERP results",
		Description: "Fetch live Google organic search results for 1-10 keywords. Returns up to the requested depth per keyword; individual query errors do not fail the batch. DataForSEO usage is charged per keyword.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"queries":{"type":"array","minItems":1,"maxItems":10,"items":{"type":"object","properties":{"keyword":{"type":"string","minLength":1},"locationCode":{"type":"integer","minimum":1},"languageCode":{"type":"string","minLength":2,"maxLength":8},"locationName":{"type":"string","minLength":1}},"required":["keyword"],"additionalProperties":false}},"depth":{"type":"integer","minimum":10,"maximum":100,"multipleOf":10}},"required":["projectId","queries"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"results":{"type":"array","items":{"type":"object"}}},"additionalProperties":true}`),
		Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": true, "destructiveHint": false},
		Handler: handleGetSerpResults,
	}
}

func handleGetSerpResults(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string `json:"projectId"`
		Queries []struct {
			Keyword string `json:"keyword"`
			LocationCode *int `json:"locationCode"`
			LanguageCode string `json:"languageCode"`
			LocationName *string `json:"locationName"`
		} `json:"queries"`
		Depth int `json:"depth"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and queries are required")
	}
	if len(args.Queries) < 1 || len(args.Queries) > 10 {
		return nil, newAppErrorf("VALIDATION_ERROR", "queries must contain 1 to 10 items")
	}
	depth := args.Depth
	if depth == 0 { depth = keywords.SerpShallowDepth }
	if depth < 10 || depth > 100 || depth%10 != 0 {
		return nil, newAppErrorf("VALIDATION_ERROR", "depth must be a multiple of 10 from 10 to 100")
	}
	for _, q := range args.Queries {
		if strings.TrimSpace(q.Keyword) == "" {
			return nil, newAppErrorf("VALIDATION_ERROR", "each query keyword is required")
		}
		if (q.LocationCode != nil && *q.LocationCode < 1) || (q.LanguageCode != "" && (len(strings.TrimSpace(q.LanguageCode)) < 2 || len(strings.TrimSpace(q.LanguageCode)) > 8)) {
			return nil, newAppErrorf("VALIDATION_ERROR", "query location or language is not valid")
		}
		if q.LocationName != nil && strings.TrimSpace(*q.LocationName) == "" {
			return nil, newAppErrorf("VALIDATION_ERROR", "locationName must not be empty")
		}
	}
	if env.deps.KeywordResearch == nil {
		return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Keyword research is not configured on this server.")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil { return nil, err }

	results := make([]serpToolQueryResult, len(args.Queries))
	okCount := 0
	for i, q := range args.Queries {
		results[i] = serpToolQueryResult{Keyword: q.Keyword}
		locationCode := 0
		if q.LocationCode != nil {
			locationCode = *q.LocationCode
		}
		pair, err := keywords.ResolveMarket(locationCode, strings.TrimSpace(q.LanguageCode), market.Pair{
			LocationCode: access.Project.LocationCode, LanguageCode: access.Project.LanguageCode,
		})
		if err != nil {
			results[i].Error = err.Error()
			continue
		}
		items, err := env.deps.KeywordResearch.SerpResultsLive(ctx, keywords.SerpAnalysisInput{
			OrganizationID: access.Auth.OrganizationID, ProjectID: access.Project.ID,
			Keyword: q.Keyword, LocationCode: pair.LocationCode, LanguageCode: pair.LanguageCode,
			LocationName: q.LocationName, Depth: depth,
		})
		if err != nil {
			results[i].Error = serpToolErrorMessage(err)
			continue
		}
		trimmed := make([]serpToolItem, 0, min(len(items), depth))
		for _, item := range items[:min(len(items), depth)] {
			rank := item.RankAbsolute
			if rank == nil { rank = item.RankGroup }
			trimmed = append(trimmed, serpToolItem{
				Type: nonemptyString(item.Type), Rank: rank, Title: nullableText(item.Title),
				URL: nullableText(item.URL), Domain: nullableText(item.Domain), Description: nullableText(item.Description),
			})
		}
		results[i].OK, results[i].Items = true, trimmed
		okCount++
	}
	sections := make([]string, 0, len(results)+1)
	for _, result := range results {
		if !result.OK {
			sections = append(sections, fmt.Sprintf("%q: FAILED — %s", result.Keyword, result.Error))
		} else if len(result.Items) == 0 {
			sections = append(sections, fmt.Sprintf("%q (0 results)", result.Keyword))
		} else {
			lines := []string{fmt.Sprintf("%q (%d results):", result.Keyword, len(result.Items)), "rank | domain | title | url"}
			for _, item := range result.Items {
				lines = append(lines, fmt.Sprintf("%s | %s | %s | %s", rankText(item.Rank), textValue(item.Domain), textValue(item.Title), textValue(item.URL)))
			}
			sections = append(sections, strings.Join(lines, "\n"))
		}
	}
	sections = append(sections, fmt.Sprintf("%d of %d queries succeeded.", okCount, len(results)))
	return mcpResponse(strings.Join(sections, "\n\n"), map[string]any{"results": results},
		metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/keywords", nil)}), nil
}

func serpToolErrorMessage(err error) string {
	if _, ok := errors.AsType[keywords.UnknownLocationError](err); ok {
		return err.Error() + " Call search_serp_locations and pass the returned locationName exactly."
	}
	return err.Error()
}

func nonemptyString(value string) *string {
	if value == "" { return nil }
	return &value
}

func nullableText(value string) *string { return &value }

func textValue(value *string) string {
	if value == nil { return "" }
	return *value
}

func rankText(value *int) string {
	if value == nil { return "" }
	return fmt.Sprint(*value)
}
