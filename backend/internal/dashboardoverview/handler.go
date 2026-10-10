package dashboardoverview

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

type Deps struct {
	Logger            *slog.Logger
	Service           *Service
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

func Mount(mux *http.ServeMux, d Deps) {
	h := http.Handler(http.HandlerFunc(d.get))
	h = d.WithProjectAccess(h)
	h = d.WithSession(h)
	mux.Handle("POST /api/v1/projects/{projectId}/dashboard/overview", h)
	refresh := http.Handler(http.HandlerFunc(d.refresh))
	refresh = d.WithProjectAccess(refresh)
	refresh = d.WithSession(refresh)
	mux.Handle("POST /api/v1/projects/{projectId}/dashboard/backlinks/refresh", refresh)
}
func (d Deps) get(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil || body == nil || len(body) != 0 || dec.Decode(new(any)) != io.EOF {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid dashboard request.")
		return
	}
	if d.Service == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "dashboard_unavailable", "Dashboard data is not available.")
		return
	}
	org, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
		return
	}
	out, err := d.Service.Get(r.Context(), org, r.PathValue("projectId"))
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found.")
			return
		}
		if r.Context().Err() != nil {
			return
		}
		if d.Logger != nil {
			d.Logger.ErrorContext(r.Context(), "dashboard overview failed", "err", err)
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (d Deps) refresh(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil || body == nil || len(body) != 0 || dec.Decode(new(any)) != io.EOF {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid dashboard request.")
		return
	}
	if d.Service == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "dashboard_unavailable", "Dashboard data is not available.")
		return
	}
	org, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
		return
	}
	if err := d.Service.RefreshBacklinkSnapshot(r.Context(), org, r.PathValue("projectId")); err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found.")
			return
		}
		if r.Context().Err() != nil {
			return
		}
		if d.Logger != nil {
			d.Logger.ErrorContext(r.Context(), "dashboard backlink snapshot refresh failed", "err", err)
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
