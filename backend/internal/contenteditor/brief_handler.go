package contenteditor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/httpx"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
)

// A brief fetch makes one SERP call and up to eight page parses in sequence.
const briefRequestTimeout = 90 * time.Second

// BriefGetter returns a brief for an organization; BriefService implements it.
type BriefGetter interface {
	Get(ctx context.Context, organizationID string, input BriefInput, refresh bool) (ContentBrief, error)
}

// ProjectMarket reads the search market and own domain of a project in an
// organization. The brief uses them so competitors are local to the project
// and the customer's own pages are left out.
type ProjectMarket interface {
	ProjectMarket(ctx context.Context, organizationID, projectID string) (domain string, locationCode int, languageCode string, err error)
}

// BriefPlans reports whether a workspace's plan includes search briefs.
type BriefPlans interface {
	HasPaidPlan(ctx context.Context, organizationID string) (bool, error)
}

// BriefDeps wires the brief route.
type BriefDeps struct {
	Logger            *slog.Logger
	Briefs            BriefGetter
	Projects          ProjectMarket
	Plans             BriefPlans
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// MountBrief registers POST /api/v1/projects/{projectId}/content/brief with
// {"keyword": "...", "refresh": false}.
func MountBrief(mux *http.ServeMux, d BriefDeps) {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	mux.Handle("POST /api/v1/projects/{projectId}/content/brief", d.WithSession(d.WithProjectAccess(http.HandlerFunc(d.brief))))
}

func (d BriefDeps) brief(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Keyword string `json:"keyword"`
		Refresh bool   `json:"refresh"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil || dec.Decode(new(any)) != io.EOF {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Send a keyword.")
		return
	}
	if d.Briefs == nil || d.Projects == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "brief_unavailable", "Search briefs are not available yet.")
		return
	}
	org, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
		return
	}
	// Fail closed: with no plan service, or when it errors, nothing is fetched.
	if d.Plans == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "brief_unavailable", "Search briefs are not available yet.")
		return
	}
	allowed, err := d.Plans.HasPaidPlan(r.Context(), org)
	if err != nil {
		d.Logger.ErrorContext(r.Context(), "check brief plan", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
		return
	}
	if !allowed {
		httpx.WriteError(w, http.StatusPaymentRequired, "plan_required", "Search briefs are included in Basic and higher plans.")
		return
	}
	domain, location, language, err := d.Projects.ProjectMarket(r.Context(), org, r.PathValue("projectId"))
	if err != nil {
		d.Logger.ErrorContext(r.Context(), "read project market for brief", "err", err)
		httpx.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), briefRequestTimeout)
	defer cancel()
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(briefRequestTimeout + 10*time.Second)); err != nil {
		d.Logger.WarnContext(r.Context(), "extend brief write deadline", "err", err)
	}
	brief, err := d.Briefs.Get(ctx, org, BriefInput{Keyword: body.Keyword, LocationCode: location, LanguageCode: language, OwnDomain: domain}, body.Refresh)
	if err != nil {
		var validation *ValidationError
		switch {
		case errors.As(err, &validation):
			httpx.WriteError(w, http.StatusBadRequest, "validation_error", validation.Message)
		case spend.WriteHTTPError(w, err):
		case errors.Is(err, ErrBriefUnavailable):
			d.Logger.WarnContext(r.Context(), "brief unavailable", "err", err)
			httpx.WriteError(w, http.StatusBadGateway, "provider_error", "Search data is temporarily unavailable. No credits were used. Try again later.")
		case r.Context().Err() != nil:
		default:
			d.Logger.ErrorContext(r.Context(), "build brief", "err", err)
			httpx.WriteError(w, http.StatusBadGateway, "provider_error", "Search data is temporarily unavailable. No credits were used. Try again later.")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, brief)
}
