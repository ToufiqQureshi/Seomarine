package audit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf16"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxBody = 32 << 10

// Deps contains the middleware and service the audit routes need.
type Deps struct {
	Logger            *slog.Logger
	Service           *Service
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// Mount registers the audit routes on the root mux. Every route needs a session
// and membership of the project's organization.
func Mount(mux *http.ServeMux, d Deps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	for _, route := range []struct {
		path string
		fn   func(Deps) http.HandlerFunc
	}{
		{"start", startHandler},
		{"status", statusHandler},
		{"results", resultsHandler},
		{"history", historyHandler},
		{"progress", progressHandler},
		{"delete", deleteHandler},
		{"capabilities", capabilitiesHandler},
	} {
		mux.Handle("POST /api/v1/projects/{projectId}/audit/"+route.path, protect(route.fn(d)))
	}
}

type badRequest string

func (e badRequest) Error() string { return string(e) }

// decode reads a bounded JSON request body, rejecting unknown fields.
func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return badRequest("The request is too large.")
		}
		if errors.Is(err, io.EOF) {
			// An empty body is valid for endpoints with no required fields.
			return nil
		}
		return badRequest("The request is not valid JSON.")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return badRequest("The request must contain exactly one JSON value.")
	}
	return nil
}

// writeError maps a service error onto an HTTP status and error code.
func writeError(ctx context.Context, logger *slog.Logger, w http.ResponseWriter, err error) {
	var invalid badRequest
	if errors.As(err, &invalid) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", invalid.Error())
		return
	}
	switch {
	case err == nil:
		return
	case errors.Is(err, ErrStartURLInvalid):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Enter a valid http(s) site URL.")
	case errors.Is(err, ErrCrawlTargetBlocked):
		httpx.WriteError(w, http.StatusBadRequest, "crawl_target_blocked", "That address is not a public website we can crawl.")
	case errors.Is(err, ErrAuditPageLimitExceeded):
		httpx.WriteError(w, http.StatusBadRequest, "audit_page_limit_exceeded", err.Error())
	case errors.Is(err, ErrAuditNotFound):
		httpx.WriteError(w, http.StatusNotFound, "audit_not_found", "Audit not found in this project.")
	case errors.Is(err, ErrLighthouseNotFound):
		httpx.WriteError(w, http.StatusNotFound, "lighthouse_not_found", "Lighthouse result not found in this project.")
	case errors.Is(err, ErrPaymentRequired):
		httpx.WriteError(w, http.StatusPaymentRequired, "payment_required", "Subscribe to run site audits.")
	case errors.Is(err, ErrRenderingUnavailable):
		httpx.WriteError(w, http.StatusForbidden, "rendering_unavailable", "JavaScript rendering is not available on this deployment.")
	case errors.Is(err, ErrAuditAlreadyRunning):
		httpx.WriteError(w, http.StatusConflict, "audit_already_running", "Another audit is already running. Wait for it to finish.")
	case errors.Is(err, ErrAuditRunning):
		httpx.WriteError(w, http.StatusConflict, "audit_running", "This audit is still running.")
	case errors.Is(err, ErrAuditCapacityReached):
		httpx.WriteError(w, http.StatusTooManyRequests, "audit_capacity_reached", "This workspace has reached its audit capacity.")
	case errors.Is(err, ErrInvalidConfig):
		logger.ErrorContext(ctx, "audit has an invalid stored config", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
	default:
		logger.ErrorContext(ctx, "audit request failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
	}
}

// handle wraps a service call with session/project context and error mapping.
func handle(d Deps, fn func(http.ResponseWriter, *http.Request, string) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "audit_unavailable", "Site audits are not set up on this server.")
			return
		}
		if _, ok := auth.ProjectOrganizationFromContext(r.Context()); !ok {
			d.Logger.ErrorContext(r.Context(), "audit request without project organization")
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		result, err := fn(w, r, r.PathValue("projectId"))
		if err != nil {
			if r.Context().Err() != nil {
				d.Logger.InfoContext(r.Context(), "audit request canceled", "err", err)
				return
			}
			writeError(r.Context(), d.Logger, w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	}
}

type startRequest struct {
	ProjectID          string `json:"projectId,omitempty"`
	StartURL           string `json:"startUrl"`
	MaxPages           *int   `json:"maxPages,omitempty"`
	LighthouseStrategy string `json:"lighthouseStrategy,omitempty"`
	RenderJavaScript   bool   `json:"renderJavaScript,omitempty"`
}

// startHandler starts an audit for the project.
func startHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, projectID string) (any, error) {
		var req startRequest
		if err := decode(w, r, &req); err != nil {
			return nil, err
		}
		if err := validateStartRequest(req); err != nil {
			return nil, err
		}
		organizationID, _ := auth.ProjectOrganizationFromContext(r.Context())
		user, _ := auth.UserFromContext(r.Context())
		tier, err := d.Service.ResolveAuditLimitTier(r.Context(), organizationID)
		if err != nil {
			return nil, err
		}
		maxPages := 0
		if req.MaxPages != nil {
			maxPages = *req.MaxPages
		}
		return d.Service.StartAudit(r.Context(), StartAuditInput{
			ActorUserID: user.ID, OrganizationID: organizationID, ProjectID: projectID, StartURL: req.StartURL,
			MaxPages: maxPages, LighthouseStrategy: LighthouseStrategy(req.LighthouseStrategy),
			RenderJavaScript: req.RenderJavaScript, LimitTier: tier,
		})
	})
}

// validateStartRequest validates the start body the way the Zod schema did.
func validateStartRequest(req startRequest) error {
	if strings.TrimSpace(req.StartURL) == "" {
		return badRequest("URL is required.")
	}
	if utf16Length(req.StartURL) > 2048 {
		return badRequest("URL must be at most 2048 characters.")
	}
	if req.MaxPages != nil && (*req.MaxPages < MinAuditPages || *req.MaxPages > PaidMaxAuditPages) {
		return badRequest("maxPages must be between 10 and 10000.")
	}
	if req.LighthouseStrategy != "" && req.LighthouseStrategy != string(LighthouseAuto) && req.LighthouseStrategy != string(LighthouseNone) {
		return badRequest("lighthouseStrategy must be auto or none.")
	}
	return nil
}

type auditIDRequest struct {
	ProjectID string `json:"projectId,omitempty"`
	AuditID   string `json:"auditId"`
}

// decodeAuditID decodes and validates an {auditId} body.
func decodeAuditID(w http.ResponseWriter, r *http.Request) (string, error) {
	var req auditIDRequest
	if err := decode(w, r, &req); err != nil {
		return "", err
	}
	if strings.TrimSpace(req.AuditID) == "" {
		return "", badRequest("auditId is required.")
	}
	return req.AuditID, nil
}

// statusHandler returns an audit's status.
func statusHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, projectID string) (any, error) {
		auditID, err := decodeAuditID(w, r)
		if err != nil {
			return nil, err
		}
		return d.Service.GetStatus(r.Context(), auditID, projectID)
	})
}

// resultsHandler returns an audit's results.
func resultsHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, projectID string) (any, error) {
		auditID, err := decodeAuditID(w, r)
		if err != nil {
			return nil, err
		}
		return d.Service.GetResults(r.Context(), auditID, projectID)
	})
}

// progressHandler returns an audit's live crawl feed.
func progressHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, projectID string) (any, error) {
		auditID, err := decodeAuditID(w, r)
		if err != nil {
			return nil, err
		}
		return d.Service.GetCrawlProgress(r.Context(), auditID, projectID)
	})
}

// deleteHandler stops and deletes an audit.
func deleteHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, projectID string) (any, error) {
		auditID, err := decodeAuditID(w, r)
		if err != nil {
			return nil, err
		}
		if err := d.Service.Remove(r.Context(), auditID, projectID); err != nil {
			return nil, err
		}
		return map[string]bool{"success": true}, nil
	})
}

// historyHandler lists a project's audits.
func historyHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, projectID string) (any, error) {
		var req struct {
			ProjectID string `json:"projectId,omitempty"`
		}
		if err := decode(w, r, &req); err != nil {
			return nil, err
		}
		return d.Service.GetHistory(r.Context(), projectID)
	})
}

// capabilitiesHandler reports the deployment's audit features.
func capabilitiesHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, _ *http.Request, _ string) (any, error) {
		return d.Service.Capabilities(), nil
	})
}

// utf16Length returns the length of a string in UTF-16 code units, matching
// JavaScript's string length for the request-size limits.
func utf16Length(value string) int { return len(utf16.Encode([]rune(value))) }

type lighthouseRequest struct {
	ProjectID string             `json:"projectId,omitempty"`
	ResultID  string             `json:"resultId"`
	Mode      ExportMode         `json:"mode,omitempty"`
	Category  LighthouseCategory `json:"category,omitempty"`
}

// decodeLighthouse decodes a {resultId} body, and for exports validates the
// mode and category.
func decodeLighthouse(w http.ResponseWriter, r *http.Request, export bool) (lighthouseRequest, error) {
	var req lighthouseRequest
	if err := decode(w, r, &req); err != nil {
		return req, err
	}
	if strings.TrimSpace(req.ResultID) == "" {
		return req, badRequest("resultId is required.")
	}
	if !export {
		return req, nil
	}
	if req.Mode != ExportFull && req.Mode != ExportIssues && req.Mode != ExportCategory {
		return req, badRequest("mode must be full, issues or category.")
	}
	if req.Category != "" && !req.Category.IsValid() {
		return req, badRequest("category must be performance, accessibility, best-practices or seo.")
	}
	return req, nil
}

// lighthouseIssuesHandler returns the issues of one Lighthouse result.
func lighthouseIssuesHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, projectID string) (any, error) {
		req, err := decodeLighthouse(w, r, false)
		if err != nil {
			return nil, err
		}
		return d.Service.GetLighthouseIssues(r.Context(), req.ResultID, projectID)
	})
}

// lighthouseExportHandler builds an export file for one Lighthouse result.
func lighthouseExportHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, projectID string) (any, error) {
		req, err := decodeLighthouse(w, r, true)
		if err != nil {
			return nil, err
		}
		return d.Service.ExportLighthouse(r.Context(), req.ResultID, projectID, req.Mode, req.Category)
	})
}
