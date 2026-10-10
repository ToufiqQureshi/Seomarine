package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/domain"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

func getRankedKeywordsTool() *tool {
	return &tool{
		Name: "get_ranked_keywords", Title: "Get ranked keywords",
		Description: "Returns market-specific keyword, URL, rank, search volume, and CPC rows for a domain or page. Use for strategy evidence; use get_domain_overview for an aggregate domain footprint. DataForSEO usage is charged.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"target":{"type":"string","minLength":1,"maxLength":2048},"scope":{"type":"string","enum":["exact_url","subfolder","domain","subdomains"]},"market":{"type":"object","properties":{"country":{"type":"string","enum":["US","USA","United States","United States of America"]}}},"locationCode":{"type":"integer","minimum":1},"languageCode":{"type":"string","minLength":2,"maxLength":8},"resultTypes":{"type":"array","items":{"type":"string","enum":["organic","paid","featured_snippet","local_pack","ai_overview_reference"]},"minItems":1,"maxItems":5},"includeSubdomains":{"type":"boolean"},"minSearchVolume":{"type":"integer","minimum":0},"maxRank":{"type":"integer","minimum":1,"maximum":100},"excludeBrandTerms":{"type":"array","items":{"type":"string","minLength":1,"maxLength":80},"minItems":1,"maxItems":10},"sortBy":{"type":"string","enum":["rank","search_volume","traffic_estimate","cpc"]},"limit":{"type":"integer","minimum":1,"maximum":100},"offset":{"type":"integer","minimum":0,"maximum":1000}},"required":["projectId","target"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"keywords":{"type":"array","items":{"type":"object"}},"totalCount":{"type":["integer","null"]},"target":{"type":"string"},"scope":{"type":"string"}},"additionalProperties":true}`),
		Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": true, "destructiveHint": false},
		Handler: handleRankedKeywords,
	}
}

func handleRankedKeywords(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string
		Target string
		Scope string
		Market *struct { Country *string }
		LocationCode *int
		LanguageCode *string
		ResultTypes []string
		IncludeSubdomains *bool
		MinSearchVolume *int
		MaxRank *int
		ExcludeBrandTerms []string
		SortBy string
		Limit *int
		Offset *int
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" || strings.TrimSpace(args.Target) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and target are required")
	}
	if len(args.Target) > 2048 { return nil, newAppErrorf("VALIDATION_ERROR", "target must be at most 2048 characters") }
	switch args.Scope {
	case "", string(domain.ScopeExactURL), string(domain.ScopeSubfolder), string(domain.ScopeDomain), string(domain.ScopeSubdomains):
	default: return nil, newAppErrorf("VALIDATION_ERROR", "scope is invalid")
	}
	if args.ResultTypes != nil && len(args.ResultTypes) == 0 { return nil, newAppErrorf("VALIDATION_ERROR", "resultTypes must contain at least 1 value") }
	if args.ExcludeBrandTerms != nil && len(args.ExcludeBrandTerms) == 0 { return nil, newAppErrorf("VALIDATION_ERROR", "excludeBrandTerms must contain at least 1 value") }
	if args.LocationCode != nil && *args.LocationCode < 1 { return nil, newAppErrorf("VALIDATION_ERROR", "locationCode must be positive") }
	if args.LanguageCode != nil && (len(strings.TrimSpace(*args.LanguageCode)) < 2 || len(strings.TrimSpace(*args.LanguageCode)) > 8) { return nil, newAppErrorf("VALIDATION_ERROR", "languageCode must be 2 to 8 characters") }
	if args.Market != nil && args.Market.Country != nil {
		switch *args.Market.Country {
		case "US", "USA", "United States", "United States of America":
		default: return nil, newAppErrorf("VALIDATION_ERROR", "Only the United States can be selected explicitly.")
		}
	}
	if env.deps.Domain == nil { return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Domain analysis is not configured on this server.") }
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil { return nil, err }

	targetDefault, err := domain.ResolveTarget(args.Target, "")
	if err != nil { return nil, newAppErrorf("VALIDATION_ERROR", err.Error()) }
	scope := domain.Scope(args.Scope)
	if args.Scope == "" && args.IncludeSubdomains != nil {
		if targetDefault.Path == "" {
			if *args.IncludeSubdomains { scope = domain.ScopeSubdomains } else { scope = domain.ScopeDomain }
		} else {
			scope = domain.ScopeExactURL
		}
	}
	requested := market.Pair{}
	if args.LocationCode != nil { requested.LocationCode = *args.LocationCode }
	if args.LanguageCode != nil { requested.LanguageCode = strings.TrimSpace(*args.LanguageCode) }
	if args.LocationCode == nil && args.LanguageCode == nil && args.Market != nil && args.Market.Country != nil {
		requested.LocationCode = 2840
		requested.LanguageCode = "en"
	}
	resolved := market.ResolveLabs(requested, market.Pair{LocationCode: access.Project.LocationCode, LanguageCode: access.Project.LanguageCode})
	if !market.IsLabsLocationCode(resolved.LocationCode) { return nil, newAppErrorf("VALIDATION_ERROR", "Domain analytics is not available for this country.") }
	if !market.IsLanguageServedForLocation(resolved.LocationCode, resolved.LanguageCode) { return nil, newAppErrorf("VALIDATION_ERROR", "Language is not available for this location.") }

	input := domain.MCPRankedKeywordsInput{
		Target: args.Target, Scope: scope, LocationCode: resolved.LocationCode, LanguageCode: resolved.LanguageCode,
		SortBy: args.SortBy, MinSearchVolume: args.MinSearchVolume, MaxRank: args.MaxRank,
		ExcludeBrandTerms: args.ExcludeBrandTerms, ResultTypes: args.ResultTypes,
	}
	if args.Limit != nil { input.Limit = *args.Limit }
	if args.Offset != nil { input.Offset = *args.Offset }
	result, err := env.deps.Domain.RankedKeywordsForMCP(ctx, access.Auth.OrganizationID, access.Project.ID, input)
	if err != nil { return nil, err }
	target, err := domain.ResolveTarget(args.Target, scope)
	if err != nil { return nil, newAppErrorf("VALIDATION_ERROR", err.Error()) }
	label := fmt.Sprintf("%s (scope: %s)", target.Display, target.Scope)
	lines := make([]string, 0, len(result.Items)+1)
	if len(result.Items) == 0 {
		lines = append(lines, "No ranked keyword rows for "+label+".")
	} else {
		header := fmt.Sprintf("Found %d ranked keyword rows for %s", len(result.Items), label)
		if result.TotalCount != nil { header += fmt.Sprintf(" (of %d total)", *result.TotalCount) }
		lines = append(lines, header+":", "keyword | rank | volume | CPC | url")
		for _, row := range result.Items {
			lines = append(lines, fmt.Sprintf("%s | %s | %s | %s | %s", row.Keyword, formatBacklinkMetric(row.Rank), formatBacklinkMetric(row.Volume), formatBacklinkMetric(row.CPC), stringValue(row.URL)))
		}
	}
	return mcpResponse(strings.Join(lines, "\n"), map[string]any{
		"keywords": result.Items, "totalCount": result.TotalCount, "target": target.Display, "scope": target.Scope,
	}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/domain", map[string]string{"domain": target.Display, "scope": string(target.Scope)})}), nil
}

func stringValue(value *string) string {
	if value == nil { return "" }
	return *value
}
