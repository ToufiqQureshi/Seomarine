package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/backlinks"
)

const backlinksOverviewDescription = "Returns a backlinks profile summary, trends, and top referring domains. Charges DataForSEO usage (typically about 50 credits for a domain and 25 for a page). Bare domains default to subdomains; use scope domain to exclude subdomains. Trend data always includes subdomains due to a provider limitation. Subfolder scope has filtered counts but no rank, trends, or referring-domain breakdown."

func getBacklinksOverviewTool() *tool {
	return &tool{
		Name: "get_backlinks_overview", Title: "Get backlinks overview",
		Description: backlinksOverviewDescription,
		InputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"projectId\":{\"type\":\"string\",\"minLength\":1},\"target\":{\"type\":\"string\",\"minLength\":1,\"maxLength\":2048},\"scope\":{\"type\":\"string\",\"enum\":[\"exact_url\",\"subfolder\",\"domain\",\"subdomains\",\"page\"]},\"hideSpam\":{\"type\":\"boolean\"}},\"required\":[\"projectId\",\"target\"]}"),
		OutputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"target\":{\"type\":\"string\"},\"scope\":{\"type\":\"string\"},\"scopeNote\":{\"type\":\"string\"},\"overview\":{\"type\":\"object\"},\"referringDomains\":{\"type\":\"object\"}},\"additionalProperties\":true}"),
		Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": true, "destructiveHint": false},
		Handler: handleBacklinksOverview,
	}
}

func handleBacklinksOverview(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string `json:"projectId"`
		Target string `json:"target"`
		Scope string `json:"scope"`
		HideSpam *bool `json:"hideSpam"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" || strings.TrimSpace(args.Target) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and target are required")
	}
	if len(args.Target) > 2048 {
		return nil, newAppErrorf("VALIDATION_ERROR", "target must be at most 2048 characters")
	}
	switch args.Scope {
	case "", "exact_url", "subfolder", "domain", "subdomains", "page":
	default:
		return nil, newAppErrorf("VALIDATION_ERROR", "scope must be exact_url, subfolder, domain, or subdomains")
	}
	scope := args.Scope
	if scope == "page" { scope = string(backlinks.ScopeExactURL) }
	if env.deps.Backlinks == nil {
		return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Backlinks are not configured on this server.")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil { return nil, err }
	hideSpam := true
	if args.HideSpam != nil { hideSpam = *args.HideSpam }
	overview, err := env.deps.Backlinks.MCPOverview(ctx, access.Auth.OrganizationID, args.Target, scope)
	if err != nil { return nil, err }
	var referring *backlinks.Page[backlinks.ReferringDomainRow]
	if overview.Scope != backlinks.ScopeSubfolder {
		page, pageErr := env.deps.Backlinks.MCPReferringDomains(ctx, access.Auth.OrganizationID, args.Target, string(overview.Scope), hideSpam)
		if pageErr != nil { return nil, pageErr }
		referring = &page
	}
	scopeNote := ""
	switch overview.Scope {
	case backlinks.ScopeDomain:
		scopeNote = "Summary excludes subdomains; trend data includes subdomains (provider limitation)."
	case backlinks.ScopeSubfolder:
		scopeNote = "Counts are computed from filtered backlink totals; rank, trends, and the referring-domains breakdown aren't available for subfolders."
	}
	domains := []backlinks.ReferringDomainRow{}
	if referring != nil { domains = referring.Rows }
	lines := []string{
		fmt.Sprintf("Backlinks profile for %s (scope: %s):", overview.DisplayTarget, overview.Scope),
	}
	if scopeNote != "" { lines = append(lines, "Note: "+scopeNote) }
	lines = append(lines,
		"- backlinks: "+formatBacklinkMetric(overview.Summary.Backlinks),
		"- referring domains: "+formatBacklinkMetric(overview.Summary.ReferringDomains),
		"- referring pages: "+formatBacklinkMetric(overview.Summary.ReferringPages),
		"- rank: "+formatBacklinkMetric(overview.Summary.Rank),
		"",
	)
	if referring == nil {
		lines = append(lines, "Referring-domains breakdown unavailable for subfolder scope.")
	} else if len(domains) == 0 {
		lines = append(lines, "No referring domains found.")
	} else {
		lines = append(lines, fmt.Sprintf("Referring domains (%d):", len(domains)))
		for _, row := range domains {
			lines = append(lines, fmt.Sprintf("- %s | backlinks: %s | referring pages: %s | rank: %s",
				formatBacklinkText(row.Domain), formatBacklinkMetric(row.Backlinks), formatBacklinkMetric(row.ReferringPages), formatBacklinkMetric(row.Rank)))
		}
	}
	payload := map[string]any{"target": overview.DisplayTarget, "scope": overview.Scope, "overview": overview}
	if scopeNote != "" { payload["scopeNote"] = scopeNote }
	if referring != nil { payload["referringDomains"] = referring }
	return mcpResponse(strings.Join(lines, "\n"), payload, metaFields{
		ProjectID: access.Project.ID,
		URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/backlinks", map[string]string{"target": args.Target, "scope": string(overview.Scope)}),
	}), nil
}

func formatBacklinkMetric(value *float64) string {
	if value == nil { return "?" }
	return fmt.Sprint(*value)
}

func formatBacklinkText(value *string) string {
	if value == nil { return "?" }
	return *value
}
