package reports

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxRequestBody = 4 << 20

type Deps struct {
	Logger            *slog.Logger
	Service           *Service
	PublicURL         *url.URL
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

func Mount(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("GET /s/{token}/raw", d.sharedRaw)
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	base := "POST /api/v1/projects/{projectId}/reports/"
	mux.Handle(base+"list", protect(http.HandlerFunc(d.listReports)))
	mux.Handle(base+"get", protect(http.HandlerFunc(d.getReport)))
	mux.Handle(base+"save", protect(http.HandlerFunc(d.saveReport)))
	mux.Handle(base+"delete", protect(http.HandlerFunc(d.deleteReport)))
	mux.Handle(base+"sharing", protect(http.HandlerFunc(d.setSharing)))
	mux.Handle(base+"templates/list", protect(http.HandlerFunc(d.listTemplates)))
	mux.Handle(base+"templates/get", protect(http.HandlerFunc(d.getTemplate)))
	mux.Handle(base+"templates/save", protect(http.HandlerFunc(d.saveTemplate)))
	mux.Handle(base+"templates/delete", protect(http.HandlerFunc(d.deleteTemplate)))
}

// sharedRaw serves only the sandboxed public document; the share wrapper page
// and its social image remain on the existing frontend route.
func (d Deps) sharedRaw(w http.ResponseWriter, r *http.Request) {
	if d.Service == nil || !d.Service.Hosted {
		http.NotFound(w, r)
		return
	}
	token := r.PathValue("token")
	if r.URL.RawQuery != "" {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, "/s/"+token+"/raw", http.StatusFound)
		return
	}
	if r.Header.Get("Sec-Fetch-Dest") == "document" {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, "/s/"+token, http.StatusFound)
		return
	}
	report, err := d.Service.GetShared(r.Context(), token)
	if err != nil {
		var typed *Error
		if errors.As(err, &typed) && typed.Code == "NOT_FOUND" {
			http.NotFound(w, r)
			return
		}
		if r.Context().Err() != nil {
			return
		}
		if d.Logger != nil {
			d.Logger.ErrorContext(r.Context(), "load shared report", "err", err)
		}
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	if report.Archived {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "sandbox allow-popups allow-popups-to-escape-sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; form-action 'none'; base-uri 'none'; object-src 'none'; frame-ancestors 'self'")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Cache-Control", "public, max-age=0, s-maxage=60")
	if _, err := io.WriteString(w, report.HTML); err != nil && d.Logger != nil {
		d.Logger.ErrorContext(r.Context(), "write shared report document", "err", err)
	}
}

func (d Deps) service(w http.ResponseWriter) bool {
	if d.Service == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "reports_unavailable", "Reports are not available on this server.")
		return false
	}
	return true
}
func (d Deps) listReports(w http.ResponseWriter, r *http.Request) {
	if !d.service(w) {
		return
	}
	var in struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	result, err := d.Service.List(r.Context(), r.PathValue("projectId"), in.Limit, in.Offset)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}
func (d Deps) getReport(w http.ResponseWriter, r *http.Request) {
	if !d.service(w) {
		return
	}
	var in struct {
		ReportID    string `json:"reportId"`
		IncludeHTML bool   `json:"includeHtml"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.ReportID) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "reportId is required.")
		return
	}
	if in.IncludeHTML {
		metadata, html, err := d.Service.GetWithHTML(r.Context(), r.PathValue("projectId"), in.ReportID)
		if err != nil {
			d.writeError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": metadata, "html": html})
		return
	}
	metadata, err := d.Service.Get(r.Context(), r.PathValue("projectId"), in.ReportID)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": metadata})
}
func (d Deps) saveReport(w http.ResponseWriter, r *http.Request) {
	if !d.service(w) {
		return
	}
	var in struct {
		ReportID   string  `json:"reportId"`
		Title      string  `json:"title"`
		Summary    string  `json:"summary"`
		HTML       string  `json:"html"`
		Skill      *string `json:"skill"`
		TemplateID *string `json:"templateId"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	orgID, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
		return
	}
	label := strings.TrimSpace(user.Name)
	if label == "" {
		label = "Workspace member"
	}
	result, err := d.Service.Save(r.Context(), SaveInput{ProjectID: r.PathValue("projectId"), OrganizationID: orgID, ReportID: in.ReportID, Title: in.Title, Summary: in.Summary, HTML: in.HTML, Skill: in.Skill, TemplateID: in.TemplateID, CreatedBy: label, CreatedByUserID: user.ID})
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}
func (d Deps) deleteReport(w http.ResponseWriter, r *http.Request) {
	if !d.service(w) {
		return
	}
	var in struct {
		ReportID string `json:"reportId"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	if err := d.Service.Delete(r.Context(), r.PathValue("projectId"), in.ReportID); err != nil {
		d.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"reportId": in.ReportID, "deleted": true})
}
func (d Deps) setSharing(w http.ResponseWriter, r *http.Request) {
	if !d.service(w) {
		return
	}
	var in struct {
		ReportID string `json:"reportId"`
		Public   *bool  `json:"public"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	if in.ReportID == "" || in.Public == nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "reportId and public are required.")
		return
	}
	var report Metadata
	var err error
	if *in.Public {
		report, err = d.Service.Share(r.Context(), r.PathValue("projectId"), in.ReportID)
	} else {
		report, err = d.Service.Unshare(r.Context(), r.PathValue("projectId"), in.ReportID)
	}
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	reportURL := d.projectURL(r, "/p/"+r.PathValue("projectId")+"/reports/"+report.ID)
	var shareURL *string
	if report.ShareToken != nil {
		value := d.projectURL(r, "/s/"+*report.ShareToken)
		shareURL = &value
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"reportId": report.ID, "public": shareURL != nil, "url": reportURL, "shareUrl": shareURL})
}
func (d Deps) listTemplates(w http.ResponseWriter, r *http.Request) {
	if !d.service(w) {
		return
	}
	if !decodeBody(w, r, &struct{}{}) {
		return
	}
	result, err := d.Service.ListTemplates(r.Context(), r.PathValue("projectId"))
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}
func (d Deps) getTemplate(w http.ResponseWriter, r *http.Request) {
	if !d.service(w) {
		return
	}
	var in struct {
		TemplateID string `json:"templateId"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	result, err := d.Service.GetTemplate(r.Context(), r.PathValue("projectId"), in.TemplateID)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"template": result})
}
func (d Deps) saveTemplate(w http.ResponseWriter, r *http.Request) {
	if !d.service(w) {
		return
	}
	var in struct {
		TemplateID   string `json:"templateId"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		Instructions string `json:"instructions"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	label := strings.TrimSpace(user.Name)
	if label == "" {
		label = "Workspace member"
	}
	result, err := d.Service.SaveTemplate(r.Context(), SaveTemplateInput{ProjectID: r.PathValue("projectId"), TemplateID: in.TemplateID, Name: in.Name, Description: in.Description, Instructions: in.Instructions, CreatedBy: label, CreatedByUserID: user.ID})
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}
func (d Deps) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	if !d.service(w) {
		return
	}
	var in struct {
		TemplateID string `json:"templateId"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	if err := d.Service.DeleteTemplate(r.Context(), r.PathValue("projectId"), in.TemplateID); err != nil {
		d.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"templateId": in.TemplateID, "deleted": true})
}

func (d Deps) projectURL(r *http.Request, path string) string {
	if d.PublicURL != nil {
		copy := *d.PublicURL
		copy.Path = strings.TrimRight(copy.Path, "/") + path
		copy.RawQuery = ""
		copy.Fragment = ""
		return copy.String()
	}
	return "https://" + r.Host + path
}
func (d Deps) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var typed *Error
	if errors.As(err, &typed) {
		status := http.StatusBadRequest
		if typed.Code == "NOT_FOUND" {
			status = http.StatusNotFound
		}
		httpx.WriteError(w, status, strings.ToLower(typed.Code), typed.Message)
		return
	}
	if r.Context().Err() != nil {
		return
	}
	if d.Logger != nil {
		d.Logger.ErrorContext(r.Context(), "report request failed", "err", err)
	}
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "request_too_large", "The request is too large.")
		} else {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must be a single JSON object with valid fields.")
		}
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
		return false
	}
	return true
}
