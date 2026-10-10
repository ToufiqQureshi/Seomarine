package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/audit"
	"github.com/toufiqqureshi/seomarine/backend/internal/keywords"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

func searchSerpLocationsTool() *tool {
	return &tool{Name: "search_serp_locations", Title: "Search SERP locations", Description: "Find canonical DataForSEO city, county, or region names for local rank tracking. Uses no credits.", InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","minLength":1,"maxLength":100},"countryCode":{"type":"string","pattern":"^[a-zA-Z]{2}$"}},"required":["query","countryCode"]}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"locations":{"type":"array","items":{"type":"object"}}},"additionalProperties":true}`), Annotations: readOnlyAnnotations(), Handler: handleSearchSerpLocations}
}
func handleSearchSerpLocations(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var a struct {
		Query       string `json:"query"`
		CountryCode string `json:"countryCode"`
	}
	if err := json.Unmarshal(raw, &a); err != nil || len(a.Query) == 0 || len(a.Query) > 100 || len(a.CountryCode) != 2 {
		return nil, newAppErrorf("VALIDATION_ERROR", "query and a two-letter countryCode are required")
	}
	rows, err := env.deps.Locations.Search(ctx, env.auth.OrganizationID, strings.ToLower(a.CountryCode), a.Query)
	if err != nil {
		return nil, err
	}
	locations := make([]map[string]any, 0, len(rows))
	lines := make([]string, 0, len(rows))
	for _, v := range rows {
		locations = append(locations, map[string]any{"locationName": v.LocationName, "locationCode": v.LocationCode, "locationType": v.LocationType})
		lines = append(lines, fmt.Sprintf("%s (%s, %d)", v.LocationName, v.LocationType, v.LocationCode))
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		text = fmt.Sprintf("No Google locations match %q in %s. Try the city name alone.", a.Query, a.CountryCode)
	}
	return mcpResponse(text, map[string]any{"locations": locations}, metaFields{}), nil
}

var auditProjectSchema = json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1}},"required":["projectId"]}`)
var auditLookupSchema = json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"auditId":{"type":"string","minLength":1}},"required":["projectId"]}`)

func auditHistoryTool() *tool {
	return &tool{Name: "list_site_audits", Title: "List site audits", Description: "List recent audits for a project. Free; reads Seomarine state.", InputSchema: auditProjectSchema, OutputSchema: json.RawMessage(`{"type":"object","properties":{"audits":{"type":"array","items":{"type":"object"}}},"additionalProperties":true}`), Annotations: readOnlyAnnotations(), Handler: handleAuditHistory}
}
func auditStatusTool() *tool {
	return &tool{Name: "get_audit_status", Title: "Get site audit status", Description: "Check audit progress. Omit auditId for the most recent audit. Free; reads Seomarine state and may reconcile a dead workflow.", InputSchema: auditLookupSchema, OutputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"object"}},"additionalProperties":true}`), Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": false, "destructiveHint": false}, Handler: handleAuditStatus}
}
func auditPagesTool() *tool {
	return &tool{Name: "get_audit_pages", Title: "Get site audit pages", Description: "List crawled pages from an audit with per-page SEO data. Free; reads Seomarine state.", InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"auditId":{"type":"string"},"fetchClass":{"type":"string"},"statusCode":{"type":"integer"},"urlContains":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":1000}},"required":["projectId"]}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"pages":{"type":"array","items":{"type":"object"}},"total":{"type":"integer"}},"additionalProperties":true}`), Annotations: readOnlyAnnotations(), Handler: handleAuditPages}
}
func deleteAuditTool() *tool {
	return &tool{Name: "delete_site_audit", Title: "Delete site audit", Description: "Permanently deletes one site audit and its crawled pages, issues, and Lighthouse results. Uses no credits. Requires an owner or admin.", InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"auditId":{"type":"string","minLength":1}},"required":["projectId","auditId"]}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"auditId":{"type":"string"},"deleted":{"type":"boolean","const":true}},"required":["auditId","deleted"],"additionalProperties":true}`), Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": false, "destructiveHint": true}, Handler: handleDeleteAudit}
}
func getAuditID(ctx context.Context, env *callEnv, projectID string, requested *string) (string, error) {
	if requested != nil && strings.TrimSpace(*requested) != "" {
		return *requested, nil
	}
	history, err := env.deps.Audit.GetHistory(ctx, projectID)
	if err != nil {
		return "", err
	}
	if len(history) == 0 {
		return "", newAppErrorf("NOT_FOUND", "No site audit found for this project.")
	}
	return history[0].ID, nil
}
func handleAuditHistory(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var a struct {
		ProjectID string `json:"projectId"`
	}
	if err := json.Unmarshal(raw, &a); err != nil || a.ProjectID == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId is required")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, a.ProjectID)
	if err != nil {
		return nil, err
	}
	rows, err := env.deps.Audit.GetHistory(ctx, access.Project.ID)
	if err != nil {
		return nil, err
	}
	return mcpResponse(fmt.Sprintf("Site audits: %d", len(rows)), map[string]any{"audits": rows}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/audit", nil)}), nil
}
func handleAuditStatus(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var a struct {
		ProjectID string  `json:"projectId"`
		AuditID   *string `json:"auditId"`
	}
	if err := json.Unmarshal(raw, &a); err != nil || a.ProjectID == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId is required")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, a.ProjectID)
	if err != nil {
		return nil, err
	}
	id, err := getAuditID(ctx, env, access.Project.ID, a.AuditID)
	if err != nil {
		return nil, err
	}
	status, err := env.deps.Audit.GetStatus(ctx, id, access.Project.ID)
	if err != nil {
		return nil, err
	}
	return mcpResponse(fmt.Sprintf("Audit %s (%s): %s â€” %d/%d pages", status.ID, status.StartURL, status.Status, status.PagesCrawled, status.PagesTotal), map[string]any{"status": status}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/audit/"+id, nil)}), nil
}
func handleAuditPages(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var a struct {
		ProjectID   string  `json:"projectId"`
		AuditID     *string `json:"auditId"`
		FetchClass  string  `json:"fetchClass"`
		StatusCode  *int    `json:"statusCode"`
		URLContains string  `json:"urlContains"`
		Limit       int     `json:"limit"`
	}
	if err := json.Unmarshal(raw, &a); err != nil || a.ProjectID == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId is required")
	}
	if a.Limit < 0 || a.Limit > 1000 {
		return nil, newAppErrorf("VALIDATION_ERROR", "limit must be between 1 and 1000")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, a.ProjectID)
	if err != nil {
		return nil, err
	}
	id, err := getAuditID(ctx, env, access.Project.ID, a.AuditID)
	if err != nil {
		return nil, err
	}
	payload, err := env.deps.Audit.GetResults(ctx, id, access.Project.ID)
	if err != nil {
		return nil, err
	}
	filtered := make([]audit.PageRecord, 0, len(payload.Pages))
	for _, p := range payload.Pages {
		if a.FetchClass != "" && string(p.FetchClass) != a.FetchClass {
			continue
		}
		if a.StatusCode != nil && (p.StatusCode == nil || *p.StatusCode != *a.StatusCode) {
			continue
		}
		if a.URLContains != "" && !strings.Contains(p.URL, a.URLContains) {
			continue
		}
		filtered = append(filtered, p)
	}
	limit := a.Limit
	if limit == 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	total := len(filtered)
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return mcpResponse(fmt.Sprintf("Audit %s: %d matching pages (showing %d)", id, total, len(filtered)), map[string]any{"pages": filtered, "total": total}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/audit/"+id, nil)}), nil
}
func handleDeleteAudit(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var a struct {
		ProjectID string `json:"projectId"`
		AuditID   string `json:"auditId"`
	}
	if err := json.Unmarshal(raw, &a); err != nil || a.ProjectID == "" || a.AuditID == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and auditId are required")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, a.ProjectID)
	if err != nil {
		return nil, err
	}
	if !hasProjectCreatePermission(access.Auth.Role) {
		return nil, newAppErrorf("FORBIDDEN", "Your organization role does not allow deleting site audits.")
	}
	if err := env.deps.Audit.Remove(ctx, a.AuditID, access.Project.ID); err != nil {
		return nil, err
	}
	return mcpResponse("Site audit deleted.", map[string]any{"auditId": a.AuditID, "deleted": true}, metaFields{ProjectID: access.Project.ID}), nil
}

func listSavedKeywordsTool() *tool {
	return &tool{Name: "list_saved_keywords", Title: "List saved keywords", Description: "Lists saved keywords and cached metrics for a project. Uses no credits.", InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"search":{"type":"string","minLength":1,"maxLength":200},"tags":{"type":"array","items":{"type":"string","minLength":1,"maxLength":64},"maxItems":20},"limit":{"type":"integer","enum":[50,100,250]}},"required":["projectId"]}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"rows":{"type":"array","items":{"type":"object"}},"totalCount":{"type":"integer"},"tags":{"type":"array","items":{"type":"object"}}},"additionalProperties":true}`), Annotations: readOnlyAnnotations(), Handler: handleListSavedKeywords}
}
func saveKeywordsTool() *tool {
	return &tool{Name: "save_keywords", Title: "Save keywords", Description: "Save keywords and optional cached metrics to a project. Uses no credits; tags append by default and may be replaced when tagMode is replace.", InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"keywords":{"type":"array","items":{"type":"string","minLength":1},"minItems":1,"maxItems":100},"metrics":{"type":"array","items":{"type":"object"},"maxItems":100},"tags":{"type":"array","items":{"type":"string","minLength":1,"maxLength":64},"maxItems":20},"tagMode":{"type":"string","enum":["append","replace"]},"locationCode":{"type":"integer"},"languageCode":{"type":"string"}},"required":["projectId","keywords"]}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string"},"savedCount":{"type":"integer"},"keywords":{"type":"array","items":{"type":"string"}},"tags":{"type":"array","items":{"type":"string"}},"tagMode":{"type":"string"},"locationCode":{"type":"integer"},"languageCode":{"type":"string"}},"additionalProperties":true}`), Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": false, "destructiveHint": true}, Handler: handleSaveKeywords}
}
func removeSavedKeywordsTool() *tool {
	return &tool{Name: "remove_saved_keywords", Title: "Remove saved keywords", Description: "Permanently deletes saved keywords by row ID. Uses no credits.", InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"savedKeywordIds":{"type":"array","items":{"type":"string","minLength":1},"minItems":1,"maxItems":2000}},"required":["projectId","savedKeywordIds"]}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string"},"requested":{"type":"integer"},"deletedCount":{"type":"integer"}},"additionalProperties":true}`), Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": false, "destructiveHint": true}, Handler: handleRemoveSavedKeywords}
}

type savedKeywordsArgs struct {
	ProjectID string   `json:"projectId"`
	Search    string   `json:"search"`
	Tags      []string `json:"tags"`
	Limit     int      `json:"limit"`
}

func handleListSavedKeywords(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var a savedKeywordsArgs
	if err := json.Unmarshal(raw, &a); err != nil || a.ProjectID == "" || len(a.Search) > 200 || len(a.Tags) > 20 {
		return nil, newAppErrorf("VALIDATION_ERROR", "Invalid saved keyword filters")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, a.ProjectID)
	if err != nil {
		return nil, err
	}
	size := a.Limit
	if size == 0 {
		size = 100
	}
	if size != 50 && size != 100 && size != 250 {
		return nil, newAppErrorf("VALIDATION_ERROR", "limit must be 50, 100, or 250")
	}
	result, err := env.deps.SavedKeywords.List(ctx, keywords.ListQuery{ProjectID: access.Project.ID, Search: a.Search, TagNames: a.Tags, Page: 1, PageSize: size, Sort: "createdAt", Descending: true})
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(result.Rows))
	lines := make([]string, 0, len(result.Rows))
	for _, r := range result.Rows {
		tags := make([]string, 0, len(r.Tags))
		for _, tag := range r.Tags {
			tags = append(tags, tag.Name)
		}
		rows = append(rows, map[string]any{"id": r.ID, "keyword": r.Keyword, "searchVolume": r.SearchVolume, "keywordDifficulty": r.KeywordDifficulty, "cpc": r.CPC, "competition": r.Competition, "intent": r.Intent, "tags": tags})
		lines = append(lines, fmt.Sprintf("- %s id:%s", r.Keyword, r.ID))
	}
	tags := make([]map[string]any, 0, len(result.Tags))
	for _, tag := range result.Tags {
		tags = append(tags, map[string]any{"name": tag.Name, "keywordCount": tag.KeywordCount})
	}
	return mcpResponse(fmt.Sprintf("Saved keywords (%d of %d):\n%s", len(rows), result.TotalCount, strings.Join(lines, "\n")), map[string]any{"rows": rows, "totalCount": result.TotalCount, "tags": tags}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/saved", nil)}), nil
}

type saveKeywordArgs struct {
	ProjectID    string            `json:"projectId"`
	Keywords     []string          `json:"keywords"`
	Metrics      []keywords.Metric `json:"metrics"`
	Tags         []string          `json:"tags"`
	TagMode      string            `json:"tagMode"`
	LocationCode *int              `json:"locationCode"`
	LanguageCode *string           `json:"languageCode"`
}

func handleSaveKeywords(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var a saveKeywordArgs
	if err := json.Unmarshal(raw, &a); err != nil || a.ProjectID == "" || len(a.Keywords) == 0 || len(a.Keywords) > 100 || len(a.Tags) > 20 || len(a.Metrics) > 100 {
		return nil, newAppErrorf("VALIDATION_ERROR", "Invalid save keywords request")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, a.ProjectID)
	if err != nil {
		return nil, err
	}
	if a.TagMode != "" && a.TagMode != "append" && a.TagMode != "replace" {
		return nil, newAppErrorf("VALIDATION_ERROR", "tagMode must be append or replace")
	}
	loc, lang := access.Project.LocationCode, access.Project.LanguageCode
	if a.LocationCode != nil {
		loc = *a.LocationCode
	}
	if a.LanguageCode != nil {
		lang = *a.LanguageCode
	}
	pair, err := keywords.ResolveMarket(loc, lang, market.Pair{LocationCode: access.Project.LocationCode, LanguageCode: access.Project.LanguageCode})
	if err != nil {
		return nil, newAppErrorf("VALIDATION_ERROR", err.Error())
	}
	ids, err := env.deps.SavedKeywords.Save(ctx, keywords.SaveInput{ProjectID: access.Project.ID, LocationCode: pair.LocationCode, LanguageCode: pair.LanguageCode, Keywords: a.Keywords, Tags: a.Tags, ReplaceTags: a.TagMode == "replace", Metrics: a.Metrics})
	if err != nil {
		return nil, err
	}
	tagMode := a.TagMode
	if tagMode == "" {
		tagMode = "append"
	}
	return mcpResponse(fmt.Sprintf("Saved %d keyword(s) to project %s.", len(ids), access.Project.ID), map[string]any{"projectId": access.Project.ID, "savedCount": len(a.Keywords), "keywords": a.Keywords, "tags": a.Tags, "tagMode": tagMode, "locationCode": pair.LocationCode, "languageCode": pair.LanguageCode}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/saved", nil)}), nil
}

type removeSavedArgs struct {
	ProjectID string   `json:"projectId"`
	IDs       []string `json:"savedKeywordIds"`
}

func handleRemoveSavedKeywords(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var a removeSavedArgs
	if err := json.Unmarshal(raw, &a); err != nil || a.ProjectID == "" || len(a.IDs) == 0 || len(a.IDs) > keywords.MaxBatchIDs {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and 1 to 2000 savedKeywordIds are required")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, a.ProjectID)
	if err != nil {
		return nil, err
	}
	count, err := env.deps.SavedKeywords.Remove(ctx, access.Project.ID, a.IDs)
	if err != nil {
		return nil, err
	}
	return mcpResponse(fmt.Sprintf("Deleted %d of %d requested saved keywords.", count, len(a.IDs)), map[string]any{"projectId": access.Project.ID, "requested": len(a.IDs), "deletedCount": count}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/saved", nil)}), nil
}
