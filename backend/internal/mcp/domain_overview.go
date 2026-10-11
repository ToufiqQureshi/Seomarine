package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/backlinks"
	"github.com/toufiqqureshi/seomarine/backend/internal/domain"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

func getDomainOverviewTool() *tool {
	return &tool{
		Name: "get_domain_overview", Title: "Get domain overview",
		Description: "Returns organic traffic and keyword estimates plus backlinks and referring domains. Use this first for domain research; use get_domain_keyword_suggestions for ranked keywords. DataForSEO usage is charged and cached for 12 hours. Overview organic metrics cover the hostname and subdomains; narrower scopes are labeled for use with ranked keyword tools.",
		InputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"projectId\":{\"type\":\"string\",\"minLength\":1},\"domain\":{\"type\":\"string\",\"minLength\":1,\"maxLength\":2048},\"scope\":{\"type\":\"string\",\"enum\":[\"exact_url\",\"subfolder\",\"domain\",\"subdomains\"]},\"includeSubdomains\":{\"type\":\"boolean\",\"deprecated\":true},\"locationCode\":{\"type\":\"integer\",\"minimum\":1},\"languageCode\":{\"type\":\"string\",\"minLength\":2,\"maxLength\":8}},\"required\":[\"projectId\",\"domain\"]}"),
		OutputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"domain\":{\"type\":\"string\"},\"scope\":{\"type\":\"string\"},\"displayTarget\":{\"type\":\"string\"},\"organicTraffic\":{\"type\":[\"number\",\"null\"]},\"organicKeywords\":{\"type\":[\"number\",\"null\"]},\"backlinks\":{\"type\":[\"number\",\"null\"]},\"referringDomains\":{\"type\":[\"number\",\"null\"]}},\"additionalProperties\":true}"),
		Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": true, "destructiveHint": false},
		Handler: handleDomainOverview,
	}
}

func handleDomainOverview(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string
		Domain string
		Scope string
		IncludeSubdomains *bool
		LocationCode *int
		LanguageCode *string
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" || strings.TrimSpace(args.Domain) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and domain are required")
	}
	if len(args.Domain) > 2048 { return nil, newAppErrorf("VALIDATION_ERROR", "domain must be at most 2048 characters") }
	scope := args.Scope
	if scope == "" && args.IncludeSubdomains != nil {
		if *args.IncludeSubdomains { scope = string(domain.ScopeSubdomains) } else { scope = string(domain.ScopeDomain) }
	}
	switch scope {
	case "", string(domain.ScopeExactURL), string(domain.ScopeSubfolder), string(domain.ScopeDomain), string(domain.ScopeSubdomains):
	default:
		return nil, newAppErrorf("VALIDATION_ERROR", "scope is invalid")
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
	projectMarket := market.Pair{LocationCode: access.Project.LocationCode, LanguageCode: access.Project.LanguageCode}
	resolved := market.ResolveLabs(requested, projectMarket)
	if !market.IsLabsLocationCode(resolved.LocationCode) {
		return nil, newAppErrorf("VALIDATION_ERROR", "Domain analytics is not available for this country.")
	}
	if !market.IsLanguageServedForLocation(resolved.LocationCode, resolved.LanguageCode) {
		return nil, newAppErrorf("VALIDATION_ERROR", "Language is not available for this location.")
	}
	result, err := env.deps.Domain.Overview(ctx, access.Auth.OrganizationID, access.Project.ID, args.Domain, domain.Scope(scope), resolved.LocationCode, resolved.LanguageCode)
	if err != nil { return nil, err }
	if env.deps.Backlinks != nil {
		linkSummary, linkErr := env.deps.Backlinks.MCPOverview(ctx, access.Auth.OrganizationID, args.Domain, string(backlinks.ScopeSubdomains))
		if linkErr != nil { return nil, linkErr }
		result.Backlinks = linkSummary.Summary.Backlinks
		result.ReferringDomains = linkSummary.Summary.ReferringDomains
	}
	lines := []string{
		"Target: "+result.DisplayTarget+" (scope: "+string(result.Scope)+")",
		"Organic traffic: "+formatBacklinkMetric(result.OrganicTraffic),
		"Organic keywords: "+formatBacklinkMetric(result.OrganicKeywords),
		"Backlinks: "+formatBacklinkMetric(result.Backlinks),
		"Referring domains: "+formatBacklinkMetric(result.ReferringDomains),
	}
	if result.Scope != domain.ScopeSubdomains {
		lines = append(lines, "Note: overview metrics cover the whole domain including subdomains; use get_ranked_keywords with this scope for scoped keyword data.")
	}
	return mcpResponse(strings.Join(lines, "\n"), result, metaFields{
		ProjectID: access.Project.ID,
		URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/domain", map[string]string{"domain": args.Domain}),
	}), nil
}
