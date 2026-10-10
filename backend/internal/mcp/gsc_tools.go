package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/gsc"
)

func getSearchConsolePerformanceTool() *tool {
	return &tool{
		Name: "get_search_console_performance", Title: "Get Google Search Console performance",
		Description:  "Query the connected Search Console property's clicks, impressions, CTR, and average position by query/page/country/device/date. Google sorts by clicks and cannot filter by position, so position and impression thresholds are applied locally over up to 1000 rows. Reads only the property connected to the authorized project; uses no credits.",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"dimensions":{"type":"array","items":{"type":"string","enum":["query","page","country","device","date","searchAppearance"]},"minItems":1,"maxItems":4},"dateRange":{"type":"string","enum":["last_7_days","last_28_days","last_3_months","last_6_months","last_12_months","last_16_months"]},"startDate":{"type":"string","pattern":"^\\d{4}-\\d{2}-\\d{2}$"},"endDate":{"type":"string","pattern":"^\\d{4}-\\d{2}-\\d{2}$"},"filters":{"type":"array","items":{"type":"object","properties":{"dimension":{"type":"string","enum":["query","page","country","device","date","searchAppearance"]},"operator":{"type":"string","enum":["equals","notEquals","contains","notContains"],"default":"equals"},"expression":{"type":"string","minLength":1}},"required":["dimension","expression"],"additionalProperties":false},"maxItems":5},"rowLimit":{"type":"integer","minimum":1,"maximum":1000},"startRow":{"type":"integer","minimum":0},"minPosition":{"type":"number","minimum":1},"maxPosition":{"type":"number","minimum":1},"minImpressions":{"type":"integer","minimum":0},"type":{"type":"string","enum":["web","image","video","news","googleNews","discover"]},"dataState":{"type":"string","enum":["all","final"]}},"required":["projectId"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"},"connected":{"type":"boolean"},"reason":{"type":"string"},"connectUrl":{"type":"string"},"siteUrl":{"type":"string"},"startDate":{"type":"string"},"endDate":{"type":"string"},"dimensions":{"type":"array","items":{"type":"string"}},"rowCount":{"type":"integer"},"rows":{"type":"array","items":{"type":"object","properties":{"keys":{"type":"array","items":{"type":"string"}},"clicks":{"type":"number"},"impressions":{"type":"number"},"ctr":{"type":"number"},"position":{"type":"number"}},"required":["clicks","impressions","ctr"],"additionalProperties":false}},"hasMore":{"type":"boolean"},"nextStartRow":{"type":"integer"},"meta":{"type":"object","additionalProperties":true}},"required":["ok"],"additionalProperties":true}`),
		Annotations:  readOnlyAnnotations(), Handler: handleSearchConsolePerformance,
	}
}

func handleSearchConsolePerformance(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var input gsc.MCPPerformanceInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, newAppErrorf("VALIDATION_ERROR", "Search Console filters are not valid JSON")
	}
	var project struct {
		ProjectID string `json:"projectId"`
	}
	if err := json.Unmarshal(raw, &project); err != nil || strings.TrimSpace(project.ProjectID) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId is required")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, project.ProjectID)
	if err != nil {
		return nil, err
	}
	if env.deps.GSC == nil {
		return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Search Console is not available on this server.")
	}
	result, err := env.deps.GSC.GetMCPPerformance(ctx, access.Project.OrganizationID, access.Project.ID, input)
	if err != nil {
		if gsc.IsValidationError(err) {
			return nil, newAppErrorf("INVALID_REQUEST", err.Error())
		}
		if code, message, ok := gsc.PublicError(err); ok {
			return mcpResponse(message, map[string]any{"ok": false, "connected": false, "reason": code, "connectUrl": connectSearchConsoleURL(env.auth.BaseURL, access.Project.ID)}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/search-performance", nil)}), nil
		}
		env.deps.Logger.ErrorContext(ctx, "MCP Search Console performance", "project_id", access.Project.ID, "err", err)
		return nil, newAppErrorf("INTERNAL_ERROR", "Search Console performance is temporarily unavailable.")
	}
	meta := metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/search-performance", nil)}
	if connected, ok := result["connected"].(bool); ok && !connected {
		result["ok"] = false
		result["reason"] = "gsc_not_connected"
		result["connectUrl"] = connectSearchConsoleURL(env.auth.BaseURL, access.Project.ID)
		return mcpResponse("Search Console is not connected for this project.", result, meta), nil
	}
	result["ok"] = true
	rows, _ := result["rows"].([]map[string]any)
	text := formatSearchConsoleRows(rows)
	return mcpResponse(text, result, meta), nil
}

func connectSearchConsoleURL(baseURL, projectID string) string {
	return buildDashboardURL(baseURL, "/p/"+projectID+"/search-performance", nil)
}

func formatSearchConsoleRows(rows []map[string]any) string {
	if len(rows) == 0 {
		return "No Search Console rows matched the requested range and filters."
	}
	lines := make([]string, 0, min(len(rows), 16))
	for i, row := range rows {
		if i == 15 {
			lines = append(lines, fmt.Sprintf("… and %d more row(s) in structured content", len(rows)-15))
			break
		}
		keys, _ := row["keys"].([]string)
		key := "(total)"
		if len(keys) > 0 {
			key = strings.Join(keys, " / ")
		}
		position := "—"
		if value, ok := row["position"].(float64); ok {
			position = fmt.Sprintf("%.1f", value)
		}
		lines = append(lines, fmt.Sprintf("- %s | clicks %.0f | impressions %.0f | CTR %.1f%% | position %s", key, number(row["clicks"]), number(row["impressions"]), number(row["ctr"])*100, position))
	}
	return strings.Join(lines, "\n")
}

func number(value any) float64 {
	result, _ := value.(float64)
	return result
}
