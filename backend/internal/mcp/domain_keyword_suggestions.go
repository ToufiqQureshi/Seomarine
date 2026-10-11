package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/domain"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

func getDomainKeywordSuggestionsTool() *tool {
	return &tool{
		Name: "get_domain_keyword_suggestions", Title: "Get domain keyword opportunities",
		Description: "Returns the organic keywords a domain ranks for, with positions and available metrics. Use after get_domain_overview for a detailed keyword opportunity list. DataForSEO usage is charged and cached for 12 hours.",
		InputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"projectId\":{\"type\":\"string\",\"minLength\":1},\"domain\":{\"type\":\"string\",\"minLength\":1,\"maxLength\":2048},\"scope\":{\"type\":\"string\",\"enum\":[\"exact_url\",\"subfolder\",\"domain\",\"subdomains\"]},\"locationCode\":{\"type\":\"integer\",\"minimum\":1},\"languageCode\":{\"type\":\"string\",\"minLength\":2,\"maxLength\":8}},\"required\":[\"projectId\",\"domain\"]}"),
		OutputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"keywords\":{\"type\":\"array\",\"items\":{\"type\":\"object\"}},\"target\":{\"type\":\"string\"},\"scope\":{\"type\":\"string\"}},\"additionalProperties\":true}"),
		Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": true, "destructiveHint": false},
		Handler: handleDomainKeywordSuggestions,
	}
}

func handleDomainKeywordSuggestions(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string
		Domain string
		Scope string
		LocationCode *int
		LanguageCode *string
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" || strings.TrimSpace(args.Domain) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and domain are required")
	}
	if len(args.Domain) > 2048 { return nil, newAppErrorf("VALIDATION_ERROR", "domain must be at most 2048 characters") }
	switch args.Scope {
	case "", string(domain.ScopeExactURL), string(domain.ScopeSubfolder), string(domain.ScopeDomain), string(domain.ScopeSubdomains):
	default: return nil, newAppErrorf("VALIDATION_ERROR", "scope is invalid")
	}
	if args.LocationCode != nil && *args.LocationCode <= 0 {
		return nil, newAppErrorf("VALIDATION_ERROR", "locationCode must be positive")
	}
	if args.LanguageCode != nil && (len(strings.TrimSpace(*args.LanguageCode)) < 2 || len(strings.TrimSpace(*args.LanguageCode)) > 8) {
		return nil, newAppErrorf("VALIDATION_ERROR", "languageCode must be 2 to 8 characters")
	}
	if env.deps.Domain == nil { return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Domain analysis is not configured on this server.") }
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil { return nil, err }
	requested := market.Pair{}
	if args.LocationCode != nil { requested.LocationCode = *args.LocationCode }
	if args.LanguageCode != nil { requested.LanguageCode = strings.TrimSpace(*args.LanguageCode) }
	resolved := market.ResolveLabs(requested, market.Pair{LocationCode: access.Project.LocationCode, LanguageCode: access.Project.LanguageCode})
	if !market.IsLabsLocationCode(resolved.LocationCode) {
		return nil, newAppErrorf("VALIDATION_ERROR", "Domain analytics is not available for this country.")
	}
	if !market.IsLanguageServedForLocation(resolved.LocationCode, resolved.LanguageCode) {
		return nil, newAppErrorf("VALIDATION_ERROR", "Language is not available for this location.")
	}
	scope := domain.Scope(args.Scope)
	target, err := domain.ResolveTarget(args.Domain, scope)
	if err != nil { return nil, newAppErrorf("VALIDATION_ERROR", err.Error()) }
	rows, err := env.deps.Domain.KeywordSuggestions(ctx, access.Auth.OrganizationID, access.Project.ID, args.Domain, scope, resolved.LocationCode, resolved.LanguageCode)
	if err != nil { return nil, err }
	lines := make([]string, 0, len(rows)+1)
	if len(rows) == 0 {
		lines = append(lines, fmt.Sprintf("No ranked keywords found for %s (scope: %s).", target.Display, target.Scope))
	} else {
		lines = append(lines, fmt.Sprintf("Keywords for %s (scope: %s) (%d):", target.Display, target.Scope, len(rows)))
		lines = append(lines, "keyword | position | volume | KD")
		for _, row := range rows {
			lines = append(lines, fmt.Sprintf("%s | %s | %s | %s", row.Keyword, formatBacklinkMetric(row.Position), formatBacklinkMetric(row.SearchVolume), formatBacklinkMetric(row.KeywordDifficulty)))
		}
	}
	return mcpResponse(strings.Join(lines, "\n"), map[string]any{
		"keywords": rows, "target": target.Display, "scope": target.Scope,
	}, metaFields{
		ProjectID: access.Project.ID,
		URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/domain", map[string]string{"domain": target.Display, "scope": string(target.Scope)}),
	}), nil
}
