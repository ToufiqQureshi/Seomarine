package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"

	"github.com/toufiqqureshi/seomarine/backend/internal/reports"
)

var reportToolAnnotations = map[string]any{"readOnlyHint": true, "openWorldHint": false, "destructiveHint": false}
var reportMutationAnnotations = map[string]any{"readOnlyHint": false, "openWorldHint": true, "destructiveHint": true}
var templateMutationAnnotations = map[string]any{"readOnlyHint": false, "openWorldHint": false, "destructiveHint": true}

func reportTools() []*tool {
	return []*tool{
		{Name: "save_report", Title: "Save report", Description: "Saves a finished HTML report to this project. Uses no credits. New reports are private; replacing preserves the existing sharing setting. Only enable public sharing when explicitly requested.", InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"title":{"type":"string","minLength":1},"summary":{"type":"string","minLength":1},"html":{"type":"string","minLength":1},"reportId":{"type":"string","minLength":1},"skill":{"type":"string"},"templateId":{"type":"string","minLength":1}},"required":["projectId","title","summary","html"],"additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"reportId":{"type":"string"},"title":{"type":"string"},"created":{"type":"boolean"},"htmlBytes":{"type":"integer"},"url":{"type":"string"},"meta":{"type":"object"}},"required":["reportId","title","created","htmlBytes","url"],"additionalProperties":true}`), Annotations: reportMutationAnnotations, Handler: handleSaveReport},
		{Name: "list_reports", Title: "List reports", Description: "Lists saved reports in this project, newest first. Uses no credits. Call before saving to find a report to replace.", InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"limit":{"type":"integer","minimum":1,"maximum":50},"offset":{"type":"integer","minimum":0}},"required":["projectId"],"additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"reports":{"type":"array"},"totalCount":{"type":"integer"},"rowCount":{"type":"integer"},"remaining":{"type":"integer"},"meta":{"type":"object"}},"required":["reports","totalCount","rowCount","remaining"],"additionalProperties":true}`), Annotations: reportToolAnnotations, Handler: handleListReports},
		{Name: "get_report", Title: "Get report", Description: "Reads one saved report and its summary. Include HTML only when you need to edit the document. Reading does not publish it.", InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"reportId":{"type":"string","minLength":1},"includeHtml":{"type":"boolean"}},"required":["projectId","reportId"],"additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"report":{"type":"object"},"meta":{"type":"object"}},"required":["report"],"additionalProperties":true}`), Annotations: reportToolAnnotations, Handler: handleGetReport},
		{Name: "delete_report", Title: "Delete report", Description: "Permanently deletes one saved report and disables its public link. Uses no credits.", InputSchema: reportIDSchema(), OutputSchema: json.RawMessage(`{"type":"object","properties":{"reportId":{"type":"string"},"deleted":{"const":true}},"required":["reportId","deleted"],"additionalProperties":true}`), Annotations: reportMutationAnnotations, Handler: handleDeleteReport},
		{Name: "set_report_sharing", Title: "Set report sharing", Description: "Enables or revokes public sharing for one report. Only enable sharing when the user explicitly asks.", InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"reportId":{"type":"string","minLength":1},"public":{"type":"boolean"}},"required":["projectId","reportId","public"],"additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"reportId":{"type":"string"},"public":{"type":"boolean"},"url":{"type":"string"},"shareUrl":{"type":["string","null"]}},"required":["reportId","public","url","shareUrl"],"additionalProperties":true}`), Annotations: reportMutationAnnotations, Handler: handleReportSharing},
		{Name: "list_report_templates", Title: "List report templates", Description: "Lists this project's reusable report briefs. Uses no credits.", InputSchema: projectOnlySchema(), OutputSchema: json.RawMessage(`{"type":"object","properties":{"templates":{"type":"array"},"remaining":{"type":"integer"}},"required":["templates","remaining"],"additionalProperties":true}`), Annotations: reportToolAnnotations, Handler: handleListReportTemplates},
		{Name: "save_report_template", Title: "Save report template", Description: "Saves or replaces a reusable report brief. Only do this when the user asks for a template.", InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"templateId":{"type":"string","minLength":1},"name":{"type":"string","minLength":1},"description":{"type":"string","minLength":1},"instructions":{"type":"string","minLength":1}},"required":["projectId","name","description","instructions"],"additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"templateId":{"type":"string"},"name":{"type":"string"},"created":{"type":"boolean"},"url":{"type":"string"}},"required":["templateId","name","created","url"],"additionalProperties":true}`), Annotations: templateMutationAnnotations, Handler: handleSaveReportTemplate},
		{Name: "delete_report_template", Title: "Delete report template", Description: "Permanently deletes one report template. Existing saved reports are kept.", InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"templateId":{"type":"string","minLength":1}},"required":["projectId","templateId"],"additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"templateId":{"type":"string"},"deleted":{"const":true}},"required":["templateId","deleted"],"additionalProperties":true}`), Annotations: templateMutationAnnotations, Handler: handleDeleteReportTemplate},
	}
}

func projectOnlySchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1}},"required":["projectId"],"additionalProperties":false}`)
}
func reportIDSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"reportId":{"type":"string","minLength":1}},"required":["projectId","reportId"],"additionalProperties":false}`)
}

type reportArgs struct {
	ProjectID    string  `json:"projectId"`
	ReportID     string  `json:"reportId"`
	Title        string  `json:"title"`
	Summary      string  `json:"summary"`
	HTML         string  `json:"html"`
	Skill        *string `json:"skill"`
	TemplateID   *string `json:"templateId"`
	IncludeHTML  bool    `json:"includeHtml"`
	Limit        int     `json:"limit"`
	Offset       int     `json:"offset"`
	Public       *bool   `json:"public"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Instructions string  `json:"instructions"`
}

func decodeReportArgs(raw json.RawMessage, dst *reportArgs) error {
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return newAppErrorf("VALIDATION_ERROR", "Report tool arguments are invalid.")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return newAppErrorf("VALIDATION_ERROR", "Report tool arguments must be one JSON object.")
	}
	if dst.ProjectID == "" || len(utf16.Encode([]rune(dst.ProjectID))) > 128 {
		return newAppErrorf("VALIDATION_ERROR", "projectId is required.")
	}
	return nil
}
func reportProject(ctx context.Context, raw json.RawMessage, env *callEnv) (reportArgs, projectAccess, error) {
	if env == nil || env.deps.Reports == nil {
		return reportArgs{}, projectAccess{}, newAppErrorf("SERVICE_UNAVAILABLE", "Reports are not available on this server.")
	}
	var a reportArgs
	if err := decodeReportArgs(raw, &a); err != nil {
		return a, projectAccess{}, err
	}
	access, err := env.h.authorizeProject(ctx, env.auth, a.ProjectID)
	return a, access, err
}
func reportFailure(err error) error {
	var typed *reports.Error
	if errors.As(err, &typed) {
		return newAppErrorf(typed.Code, typed.Message)
	}
	return fmt.Errorf("reports: %w", err)
}
func reportURL(env *callEnv, projectID, reportID string) string {
	return buildDashboardURL(env.auth.BaseURL, "/p/"+projectID+"/reports/"+reportID, nil)
}
func reportTemplateURL(env *callEnv, projectID string) string {
	return buildDashboardURL(env.auth.BaseURL, "/p/"+projectID+"/reports/templates", nil)
}
func preview(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 240 {
		return string(r[:237]) + "..."
	}
	return s
}
func reportMeta(r reports.Metadata) map[string]any {
	return map[string]any{"id": r.ID, "projectId": r.ProjectID, "title": r.Title, "summary": r.Summary, "skill": r.Skill, "templateId": r.TemplateID, "createdBy": r.CreatedBy, "sizeBytes": r.SizeBytes, "createdAt": r.CreatedAt, "updatedAt": r.UpdatedAt}
}

func handleSaveReport(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	a, access, err := reportProject(ctx, raw, env)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Title) == "" || strings.TrimSpace(a.Summary) == "" || a.HTML == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "title, summary, and html are required.")
	}
	saved, err := env.deps.Reports.Save(ctx, reports.SaveInput{ProjectID: access.Project.ID, OrganizationID: access.Auth.OrganizationID, ReportID: a.ReportID, Title: a.Title, Summary: a.Summary, HTML: a.HTML, Skill: a.Skill, TemplateID: a.TemplateID, CreatedBy: access.Auth.ClientLabel, CreatedByUserID: access.Auth.UserID})
	if err != nil {
		return nil, reportFailure(err)
	}
	url := reportURL(env, access.Project.ID, saved.ReportID)
	word := "Saved"
	if !saved.Created {
		word = "Replaced"
	}
	return mcpResponse(fmt.Sprintf("%s report %q (%d bytes). Open it at %s.", word, saved.Title, saved.HTMLBytes, url), map[string]any{"reportId": saved.ReportID, "title": saved.Title, "created": saved.Created, "htmlBytes": saved.HTMLBytes, "url": url}, metaFields{ProjectID: access.Project.ID, URL: url}), nil
}
func handleListReports(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	a, access, err := reportProject(ctx, raw, env)
	if err != nil {
		return nil, err
	}
	result, err := env.deps.Reports.List(ctx, access.Project.ID, a.Limit, a.Offset)
	if err != nil {
		return nil, reportFailure(err)
	}
	rows := make([]map[string]any, 0, len(result.Reports))
	blocks := make([]string, 0, len(result.Reports))
	for _, r := range result.Reports {
		item := reportMeta(r)
		item["summary"] = preview(r.Summary)
		rows = append(rows, item)
		blocks = append(blocks, fmt.Sprintf("%s  %s\n  %s | %s | updated %s\n  %s", r.ID, r.Title, valueOr(r.Skill, "no skill"), r.CreatedBy, dateOnly(r.UpdatedAt), preview(r.Summary)))
	}
	if len(blocks) == 0 {
		blocks = append(blocks, "No reports saved for this project yet.")
	}
	blocks = append(blocks, fmt.Sprintf("%d reports. %d shown.", result.TotalCount, len(rows)))
	return mcpResponse(strings.Join(blocks, "\n\n"), map[string]any{"reports": rows, "totalCount": result.TotalCount, "rowCount": result.RowCount, "remaining": result.Remaining}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/reports", nil)}), nil
}
func valueOr(v *string, fallback string) string {
	if v == nil || *v == "" {
		return fallback
	}
	return *v
}
func dateOnly(value string) string {
	if len(value) >= 10 {
		return value[:10]
	}
	return value
}
func handleGetReport(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	a, access, err := reportProject(ctx, raw, env)
	if err != nil {
		return nil, err
	}
	var item reports.Metadata
	html := ""
	if a.IncludeHTML {
		item, html, err = env.deps.Reports.GetWithHTML(ctx, access.Project.ID, a.ReportID)
	} else {
		item, err = env.deps.Reports.Get(ctx, access.Project.ID, a.ReportID)
	}
	if err != nil {
		return nil, reportFailure(err)
	}
	url := reportURL(env, access.Project.ID, item.ID)
	var shareURL any
	if env.deps.Reports.Hosted && item.ShareToken != nil {
		shareURL = buildDashboardURL(env.auth.BaseURL, "/s/"+*item.ShareToken, nil)
	}
	text := fmt.Sprintf("%s (%s)\n%s\n%s\n", item.Title, item.ID, item.Summary, url)
	if shareURL == nil {
		text += "No public link available."
	} else {
		text += "Public link: " + shareURL.(string)
	}
	if a.IncludeHTML {
		text += "\n\nHTML:\n" + html
	}
	out := reportMeta(item)
	out["url"] = url
	out["shareUrl"] = shareURL
	return mcpResponse(text, map[string]any{"report": out}, metaFields{ProjectID: access.Project.ID, URL: url}), nil
}
func handleDeleteReport(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	a, access, err := reportProject(ctx, raw, env)
	if err != nil {
		return nil, err
	}
	if a.ReportID == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "reportId is required.")
	}
	if err = env.deps.Reports.Delete(ctx, access.Project.ID, a.ReportID); err != nil {
		return nil, reportFailure(err)
	}
	return mcpResponse("Deleted report "+a.ReportID+".", map[string]any{"reportId": a.ReportID, "deleted": true}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/reports", nil)}), nil
}
func handleReportSharing(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	a, access, err := reportProject(ctx, raw, env)
	if err != nil {
		return nil, err
	}
	if a.ReportID == "" || a.Public == nil {
		return nil, newAppErrorf("VALIDATION_ERROR", "reportId and public are required.")
	}
	var item reports.Metadata
	if *a.Public {
		item, err = env.deps.Reports.Share(ctx, access.Project.ID, a.ReportID)
	} else {
		item, err = env.deps.Reports.Unshare(ctx, access.Project.ID, a.ReportID)
	}
	if err != nil {
		return nil, reportFailure(err)
	}
	url := reportURL(env, access.Project.ID, item.ID)
	var shareURL any
	if item.ShareToken != nil {
		shareURL = buildDashboardURL(env.auth.BaseURL, "/s/"+*item.ShareToken, nil)
	}
	pub := shareURL != nil
	text := "Public sharing disabled. The report is private."
	if pub {
		text = "Public sharing enabled. Anyone with this link can read the latest saved report: " + shareURL.(string)
	}
	return mcpResponse(text, map[string]any{"reportId": item.ID, "public": pub, "url": url, "shareUrl": shareURL}, metaFields{ProjectID: access.Project.ID, URL: url}), nil
}
func handleListReportTemplates(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	_, access, err := reportProject(ctx, raw, env)
	if err != nil {
		return nil, err
	}
	result, err := env.deps.Reports.ListTemplates(ctx, access.Project.ID)
	if err != nil {
		return nil, reportFailure(err)
	}
	rows := make([]map[string]any, 0, len(result.Templates))
	blocks := make([]string, 0, len(result.Templates))
	for _, t := range result.Templates {
		rows = append(rows, map[string]any{"id": t.ID, "name": t.Name, "description": t.Description, "instructionsPreview": preview(t.Instructions), "updatedAt": t.UpdatedAt})
		blocks = append(blocks, fmt.Sprintf("%s  %s\n%s\n\n%s", t.ID, t.Name, t.Description, t.Instructions))
	}
	if len(blocks) == 0 {
		blocks = append(blocks, "This project has no report templates.")
	}
	return mcpResponse(strings.Join(blocks, "\n\n---\n\n"), map[string]any{"templates": rows, "remaining": result.Remaining}, metaFields{ProjectID: access.Project.ID, URL: reportTemplateURL(env, access.Project.ID)}), nil
}
func handleSaveReportTemplate(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	a, access, err := reportProject(ctx, raw, env)
	if err != nil {
		return nil, err
	}
	result, err := env.deps.Reports.SaveTemplate(ctx, reports.SaveTemplateInput{ProjectID: access.Project.ID, TemplateID: a.TemplateIDOrEmpty(), Name: a.Name, Description: a.Description, Instructions: a.Instructions, CreatedBy: access.Auth.ClientLabel, CreatedByUserID: access.Auth.UserID})
	if err != nil {
		return nil, reportFailure(err)
	}
	url := reportTemplateURL(env, access.Project.ID)
	verb := "Saved"
	if !result.Created {
		verb = "Updated"
	}
	return mcpResponse(fmt.Sprintf("%s report template %q (id %s) in this project. Manage templates at %s.", verb, result.Name, result.TemplateID, url), map[string]any{"templateId": result.TemplateID, "name": result.Name, "created": result.Created, "url": url}, metaFields{ProjectID: access.Project.ID, URL: url}), nil
}
func (a reportArgs) TemplateIDOrEmpty() string {
	if a.TemplateID == nil {
		return ""
	}
	return *a.TemplateID
}
func handleDeleteReportTemplate(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	a, access, err := reportProject(ctx, raw, env)
	if err != nil {
		return nil, err
	}
	if a.TemplateID == nil || *a.TemplateID == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "templateId is required.")
	}
	if err = env.deps.Reports.DeleteTemplate(ctx, access.Project.ID, *a.TemplateID); err != nil {
		return nil, reportFailure(err)
	}
	return mcpResponse("Deleted report template "+*a.TemplateID+". Existing reports are kept.", map[string]any{"templateId": *a.TemplateID, "deleted": true}, metaFields{ProjectID: access.Project.ID, URL: reportTemplateURL(env, access.Project.ID)}), nil
}
