package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/backlinks"
)

func getBacklinksProfileTool() *tool {
	return &tool{
		Name: "get_backlinks_profile", Title: "Get backlinks profile",
		Description: "Returns one bounded page of backlink rows with linking URLs, target URLs, anchors, dofollow/nofollow, authority/spam signals, and lost/broken status. Supports filters, sorting, one_per_domain/as_is grouping, and pagination. Typically incurs about 30 credits per page.",
		InputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"projectId\":{\"type\":\"string\",\"minLength\":1},\"target\":{\"type\":\"string\",\"minLength\":1,\"maxLength\":2048},\"scope\":{\"type\":\"string\",\"enum\":[\"exact_url\",\"subfolder\",\"domain\",\"subdomains\",\"page\"]},\"page\":{\"type\":\"integer\",\"minimum\":1},\"pageSize\":{\"type\":\"integer\",\"enum\":[50,100,200]},\"sortField\":{\"type\":\"string\",\"enum\":[\"rank\",\"domainRank\",\"spamScore\",\"firstSeen\"]},\"sortOrder\":{\"type\":\"string\",\"enum\":[\"asc\",\"desc\"]},\"mode\":{\"type\":\"string\",\"enum\":[\"one_per_domain\",\"as_is\"]},\"hideSpam\":{\"type\":\"boolean\"},\"filters\":{\"type\":\"object\",\"properties\":{\"include\":{\"type\":\"string\"},\"exclude\":{\"type\":\"string\"},\"minDomainRank\":{\"type\":\"number\"},\"maxDomainRank\":{\"type\":\"number\"},\"minLinkAuthority\":{\"type\":\"number\"},\"maxLinkAuthority\":{\"type\":\"number\"},\"minSpamScore\":{\"type\":\"number\"},\"maxSpamScore\":{\"type\":\"number\"},\"linkType\":{\"type\":\"string\",\"enum\":[\"dofollow\",\"nofollow\"]},\"hideLost\":{\"type\":\"boolean\"},\"hideBroken\":{\"type\":\"boolean\"},\"domainFrom\":{\"type\":\"string\",\"maxLength\":255}},\"additionalProperties\":false}},\"required\":[\"projectId\",\"target\"],\"additionalProperties\":false}"),
		OutputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"target\":{\"type\":\"string\"},\"scope\":{\"type\":\"string\"},\"backlinks\":{\"type\":\"object\"}},\"additionalProperties\":true}"),
		Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": true, "destructiveHint": false},
		Handler: handleBacklinksProfile,
	}
}

func handleBacklinksProfile(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string `json:"projectId"`
		Target string `json:"target"`
		Scope string `json:"scope"`
		Page int `json:"page"`
		PageSize int `json:"pageSize"`
		SortField string `json:"sortField"`
		SortOrder string `json:"sortOrder"`
		Mode string `json:"mode"`
		HideSpam *bool `json:"hideSpam"`
		Filters backlinks.MCPBacklinkFilters `json:"filters"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" || strings.TrimSpace(args.Target) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and target are required")
	}
	if len(args.Target) > 2048 { return nil, newAppErrorf("VALIDATION_ERROR", "target must be at most 2048 characters") }
	if args.Page != 0 && args.Page < 1 { return nil, newAppErrorf("VALIDATION_ERROR", "page must be positive") }
	if args.PageSize != 0 && args.PageSize != 50 && args.PageSize != 100 && args.PageSize != 200 {
		return nil, newAppErrorf("VALIDATION_ERROR", "pageSize must be 50, 100, or 200")
	}
	scope := args.Scope
	if scope == "page" { scope = string(backlinks.ScopeExactURL) }
	if env.deps.Backlinks == nil { return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Backlinks are not configured on this server.") }
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil { return nil, err }
	target, page, err := env.deps.Backlinks.MCPBacklinkRows(ctx, access.Auth.OrganizationID, backlinks.MCPBacklinkRowsInput{
		Target: args.Target, Scope: scope, Page: args.Page, PageSize: args.PageSize,
		SortField: args.SortField, SortOrder: args.SortOrder, Mode: args.Mode,
		Filters: args.Filters, HideSpam: args.HideSpam,
	})
	if err != nil { return nil, err }
	lines := []string{
		fmt.Sprintf("Backlinks profile for %s (scope: %s):", target.Display, target.Scope),
		fmt.Sprintf("- page: %d", page.Page),
		fmt.Sprintf("- page size: %d", page.PageSize),
		fmt.Sprintf("- rows returned: %d", len(page.Rows)),
		fmt.Sprintf("- total backlinks: %s", formatBacklinkCount(page.TotalCount)),
		fmt.Sprintf("- has more: %s", yesNo(page.HasMore)),
		"",
	}
	if len(page.Rows) == 0 {
		lines = append(lines, "No backlink rows for this page.")
	} else {
		lines = append(lines, "Source | target | anchor | type | rank | domain rank | spam | status")
		for _, row := range page.Rows {
			linkType := "unknown"
			if row.IsDofollow != nil { if *row.IsDofollow { linkType = "dofollow" } else { linkType = "nofollow" } }
			status := "live"
			if row.IsLost || row.IsBroken {
				status = ""
				if row.IsLost { status = "lost" }
				if row.IsBroken { if status != "" { status += ", " }; status += "broken" }
			}
			source := row.URLFrom
			if source == nil { source = row.DomainFrom }
			lines = append(lines, fmt.Sprintf("%s | %s | %s | %s | %s | %s | %s | %s",
				formatBacklinkText(source), formatBacklinkText(row.URLTo), formatBacklinkText(row.Anchor),
				linkType, formatBacklinkMetric(row.Rank), formatBacklinkMetric(row.DomainFromRank),
				formatBacklinkMetric(row.SpamScore), status))
		}
	}
	result := map[string]any{
		"target": target.Display, "scope": target.Scope,
		"backlinks": page,
	}
	return mcpResponse(strings.Join(lines, "\n"), result, metaFields{
		ProjectID: access.Project.ID,
		URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/backlinks", map[string]string{"target": args.Target, "scope": string(target.Scope)}),
	}), nil
}


func formatBacklinkCount(value *int) string {
	if value == nil { return "?" }
	return fmt.Sprint(*value)
}

func yesNo(v bool) string { if v { return "yes" }; return "no" }
