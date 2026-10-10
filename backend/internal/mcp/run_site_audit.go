package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/toufiqqureshi/seomarine/backend/internal/audit"
)

var runSiteAuditInputSchema = json.RawMessage("{\"type\":\"object\",\"properties\":{\"projectId\":{\"type\":\"string\",\"minLength\":1,\"description\":\"Project to audit.\"},\"url\":{\"type\":\"string\",\"minLength\":1,\"maxLength\":2048,\"description\":\"Start URL to crawl.\"},\"maxPages\":{\"type\":\"integer\",\"minimum\":10,\"maximum\":10000,\"description\":\"Page budget for the crawl (default 50).\"},\"renderJavaScript\":{\"type\":\"boolean\",\"description\":\"Render JavaScript before auditing. Slower and limited to 1000 pages. Default false.\"},\"runLighthouse\":{\"type\":\"boolean\",\"description\":\"Run Lighthouse on representative pages. Adds time and usage. Default false.\"}},\"required\":[\"projectId\",\"url\"]}")

func runSiteAuditTool() *tool {
	return &tool{
		Name: "run_site_audit", Title: "Run site audit",
		Description: "Start a same-origin, robots-aware site crawl and SEO audit. It runs in the background; poll get_audit_status and then read get_audit_issues. Lighthouse and JavaScript rendering are optional and use additional capacity.",
		InputSchema: runSiteAuditInputSchema,
		OutputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"auditId\":{\"type\":\"string\"},\"meta\":{\"type\":\"object\",\"additionalProperties\":true}},\"required\":[\"auditId\"],\"additionalProperties\":true}"),
		Annotations: mutatingAnnotations(),
		Handler: handleRunSiteAudit,
	}
}

func handleRunSiteAudit(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID        string `json:"projectId"`
		URL              string `json:"url"`
		MaxPages         *int   `json:"maxPages"`
		RenderJavaScript bool   `json:"renderJavaScript"`
		RunLighthouse    bool   `json:"runLighthouse"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || args.ProjectID == "" || args.URL == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and url are required")
	}
	if len(args.URL) > 2048 {
		return nil, newAppErrorf("VALIDATION_ERROR", "url must be at most 2048 characters")
	}
	if args.MaxPages != nil && (*args.MaxPages < audit.MinAuditPages || *args.MaxPages > audit.PaidMaxAuditPages) {
		return nil, newAppErrorf("VALIDATION_ERROR", "maxPages must be between 10 and 10000")
	}
	if env.deps.Audit == nil {
		return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Site audits are not configured on this server.")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil {
		return nil, err
	}
	tier, err := env.deps.Audit.ResolveAuditLimitTier(ctx, access.Auth.OrganizationID)
	if err != nil {
		return nil, auditToolError(ctx, env, err)
	}
	maxPages := audit.DefaultAuditPages
	if args.MaxPages != nil {
		maxPages = *args.MaxPages
	}
	strategy := audit.LighthouseNone
	if args.RunLighthouse {
		strategy = audit.LighthouseAuto
	}
	started, err := env.deps.Audit.StartAudit(ctx, audit.StartAuditInput{
		ActorUserID: access.Auth.UserID, OrganizationID: access.Auth.OrganizationID,
		ProjectID: access.Project.ID, StartURL: args.URL, MaxPages: maxPages,
		LighthouseStrategy: strategy, RenderJavaScript: args.RenderJavaScript, LimitTier: tier,
	})
	if err != nil {
		return nil, auditToolError(ctx, env, err)
	}
	return mcpResponse(
		fmt.Sprintf("Audit %s started for %s. Poll get_audit_status until it finishes, then call get_audit_issues for the prioritized report.", started.AuditID, args.URL),
		map[string]any{"auditId": started.AuditID},
		metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/audit", map[string]string{"auditId": started.AuditID})},
	), nil
}

func auditToolError(ctx context.Context, env *callEnv, err error) error {
	switch {
	case errors.Is(err, audit.ErrPaymentRequired):
		return newAppErrorf("PAYMENT_REQUIRED", "This organization does not have managed access for site audits.")
	case errors.Is(err, audit.ErrAuditCapacityReached):
		return newAppErrorf("AUDIT_CAPACITY_REACHED", "Audit capacity reached. Delete older audits in the dashboard to free capacity.")
	case errors.Is(err, audit.ErrAuditAlreadyRunning):
		return newAppErrorf("AUDIT_ALREADY_RUNNING", "This organization is at its concurrent audit limit. Poll get_audit_status until one finishes.")
	case errors.Is(err, audit.ErrAuditPageLimitExceeded):
		return newAppErrorf("AUDIT_PAGE_LIMIT_EXCEEDED", err.Error())
	case errors.Is(err, audit.ErrRenderingUnavailable):
		return newAppErrorf("FORBIDDEN", err.Error())
	case errors.Is(err, audit.ErrStartURLInvalid), errors.Is(err, audit.ErrCrawlTargetBlocked):
		return newAppErrorf("INVALID_URL", "The audit URL is invalid or points to an address the crawler cannot access.")
	default:
		if env.deps.Logger != nil {
			env.deps.Logger.ErrorContext(ctx, "start MCP site audit", "err", err)
		}
		return newAppErrorf("INTERNAL_ERROR", "Could not start the site audit. Please try again.")
	}
}
