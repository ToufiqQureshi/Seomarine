package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/audit"
)

var auditIssuesInputSchema = json.RawMessage("{\"type\":\"object\",\"properties\":{\"projectId\":{\"type\":\"string\",\"minLength\":1},\"auditId\":{\"type\":\"string\"},\"severity\":{\"type\":\"string\",\"enum\":[\"critical\",\"warning\",\"info\"]},\"issueType\":{\"type\":\"string\",\"minLength\":1},\"limit\":{\"type\":\"integer\",\"minimum\":1,\"maximum\":1000}},\"required\":[\"projectId\"]}")

func auditIssuesTool() *tool {
	return &tool{
		Name: "get_audit_issues", Title: "Get site audit issues",
		Description: "Read prioritized SEO issues for the latest or a selected site audit. Supports severity and issue-type filters. Each issue includes its title, details, URL, and remediation guidance. Free; reads Seomarine state.",
		InputSchema: auditIssuesInputSchema,
		OutputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"auditId\":{\"type\":\"string\"},\"issues\":{\"type\":\"array\",\"items\":{\"type\":\"object\"}},\"summary\":{\"type\":\"array\",\"items\":{\"type\":\"object\"}},\"totalCount\":{\"type\":\"integer\"},\"returnedCount\":{\"type\":\"integer\"}},\"additionalProperties\":true}"),
		Annotations: readOnlyAnnotations(),
		Handler: handleAuditIssues,
	}
}

func handleAuditIssues(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string  `json:"projectId"`
		AuditID   *string `json:"auditId"`
		Severity  string  `json:"severity"`
		IssueType string  `json:"issueType"`
		Limit     int     `json:"limit"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId is required")
	}
	if args.Severity != "" && args.Severity != "critical" && args.Severity != "warning" && args.Severity != "info" {
		return nil, newAppErrorf("VALIDATION_ERROR", "severity must be critical, warning, or info")
	}
	if len(args.IssueType) > 200 { return nil, newAppErrorf("VALIDATION_ERROR", "issueType must be at most 200 characters") }
	limit := args.Limit
	if limit == 0 { limit = 200 }
	if limit < 1 || limit > 1000 { return nil, newAppErrorf("VALIDATION_ERROR", "limit must be between 1 and 1000") }
	if env.deps.Audit == nil { return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Site audits are not configured on this server.") }
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil { return nil, err }
	auditID, err := getAuditID(ctx, env, access.Project.ID, args.AuditID)
	if err != nil { return nil, err }
	payload, err := env.deps.Audit.GetResults(ctx, auditID, access.Project.ID)
	if err != nil { return nil, err }

	rows := make([]map[string]any, 0, len(payload.Issues))
	counts, severities, titles := map[string]int{}, map[string]string{}, map[string]string{}
	for _, issue := range payload.Issues {
		severity, title, howToFix := string(issue.Severity), issue.IssueType, any(nil)
		if descriptor, ok := audit.DescribeIssueType(issue.IssueType); ok {
			severity, title, howToFix = string(descriptor.Severity), descriptor.Title, descriptor.HowToFix
		} else { severity = "info" }
		if args.Severity != "" && severity != args.Severity { continue }
		if args.IssueType != "" && issue.IssueType != args.IssueType { continue }
		counts[issue.IssueType]++
		severities[issue.IssueType], titles[issue.IssueType] = severity, title
		rows = append(rows, map[string]any{"severity": severity, "issueType": issue.IssueType, "title": title, "url": issue.PageURL, "details": issue.Details, "howToFix": howToFix})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		left, right := rows[i], rows[j]
		li, ri := severityOrder(left["severity"].(string)), severityOrder(right["severity"].(string))
		if li != ri { return li < ri }; return left["issueType"].(string) < right["issueType"].(string)
	})
	total := len(rows); limited := rows
	if len(limited) > limit { limited = limited[:limit] }
	summary := make([]map[string]any, 0, len(counts))
	for issueType, count := range counts { summary = append(summary, map[string]any{"issueType": issueType, "title": titles[issueType], "severity": severities[issueType], "count": count}) }
	sort.Slice(summary, func(i, j int) bool {
		left, right := summary[i], summary[j]
		li, ri := severityOrder(left["severity"].(string)), severityOrder(right["severity"].(string))
		if li != ri { return li < ri }
		lc, rc := left["count"].(int), right["count"].(int)
		if lc != rc { return lc > rc }; return left["issueType"].(string) < right["issueType"].(string)
	})
	lines := make([]string, 0, len(summary))
	for _, item := range summary { lines = append(lines, fmt.Sprintf("- %s [%s]: %d", item["title"], item["severity"], item["count"])) }
	text := fmt.Sprintf("Audit %s issues: %d total, %d returned.\n%s", auditID, total, len(limited), strings.Join(lines, "\n"))
	return mcpResponse(text, map[string]any{"auditId": auditID, "issues": limited, "summary": summary, "totalCount": total, "returnedCount": len(limited)}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/audit/"+auditID, nil)}), nil
}

func severityOrder(severity string) int {
	switch severity { case "critical": return 0; case "warning": return 1; default: return 2 }
}
