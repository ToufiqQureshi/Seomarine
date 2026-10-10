package projects

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxBody = 16 << 10

type Deps struct {
	Logger            *slog.Logger
	Service           *Service
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

func Mount(mux *http.ServeMux, d Deps) {
	mux.Handle("POST /api/v1/projects/list", d.WithSession(http.HandlerFunc(d.list)))
	mux.Handle("POST /api/v1/projects/create", d.WithSession(http.HandlerFunc(d.create)))
	mux.Handle("POST /api/v1/projects/archived", d.WithSession(http.HandlerFunc(d.archived)))
	mux.Handle("POST /api/v1/projects/restore", d.WithSession(http.HandlerFunc(d.restore)))
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	mux.Handle("POST /api/v1/projects/{projectId}/get", protect(http.HandlerFunc(d.get)))
	mux.Handle("POST /api/v1/projects/{projectId}/update", protect(http.HandlerFunc(d.update)))
	mux.Handle("POST /api/v1/projects/{projectId}/archive", protect(http.HandlerFunc(d.archive)))
}
func (d Deps) list(w http.ResponseWriter, r *http.Request) {
	if !empty(w, r) {
		return
	}
	u, ok := auth.UserFromContext(r.Context())
	if !ok || u.OrganizationID == "" {
		httpx.WriteError(w, http.StatusForbidden, "organization_required", "Select an organization first.")
		return
	}
	rows, err := d.Service.List(r.Context(), u.OrganizationID, false, true)
	d.respond(w, r, rows, err)
}
func (d Deps) create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !decode(w, r, &in) {
		return
	}
	u, ok := auth.UserFromContext(r.Context())
	if !ok || u.OrganizationID == "" {
		httpx.WriteError(w, http.StatusForbidden, "organization_required", "Select an organization first.")
		return
	}
	result, err := d.Service.Create(r.Context(), u.OrganizationID, u.ID, in)
	d.respond(w, r, result, err)
}
func (d Deps) archived(w http.ResponseWriter, r *http.Request) {
	if !empty(w, r) {
		return
	}
	u, ok := auth.UserFromContext(r.Context())
	if !ok || u.OrganizationID == "" {
		httpx.WriteError(w, http.StatusForbidden, "organization_required", "Select an organization first.")
		return
	}
	rows, err := d.Service.List(r.Context(), u.OrganizationID, true, false)
	d.respond(w, r, rows, err)
}
func (d Deps) restore(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ArchivedProjectID string `json:"archivedProjectId"`
	}
	if !decode(w, r, &in) || strings.TrimSpace(in.ArchivedProjectID) == "" {
		invalid(w)
		return
	}
	u, ok := auth.UserFromContext(r.Context())
	if !ok || u.OrganizationID == "" {
		httpx.WriteError(w, http.StatusForbidden, "organization_required", "Select an organization first.")
		return
	}
	err := d.Service.Restore(r.Context(), u.OrganizationID, u.ID, in.ArchivedProjectID)
	if err != nil {
		d.respond(w, r, nil, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}
func (d Deps) get(w http.ResponseWriter, r *http.Request) {
	if !empty(w, r) {
		return
	}
	org, _ := auth.ProjectOrganizationFromContext(r.Context())
	result, err := d.Service.Get(r.Context(), org, r.PathValue("projectId"))
	d.respond(w, r, result, err)
}
func (d Deps) update(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !decode(w, r, &in) {
		return
	}
	org, _ := auth.ProjectOrganizationFromContext(r.Context())
	result, err := d.Service.Update(r.Context(), org, r.PathValue("projectId"), in)
	d.respond(w, r, result, err)
}
func (d Deps) archive(w http.ResponseWriter, r *http.Request) {
	if !empty(w, r) {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	org, _ := auth.ProjectOrganizationFromContext(r.Context())
	if err := d.Service.Archive(r.Context(), org, u.ID, r.PathValue("projectId")); err != nil {
		d.respond(w, r, nil, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}
func (d Deps) respond(w http.ResponseWriter, r *http.Request, value any, err error) {
	if err == nil {
		httpx.WriteJSON(w, http.StatusOK, value)
		return
	}
	var typed *Error
	if errors.As(err, &typed) {
		status := http.StatusBadRequest
		if typed.Code == "NOT_FOUND" {
			status = http.StatusNotFound
		}
		if typed.Code == "FORBIDDEN" {
			status = http.StatusForbidden
		}
		if typed.Code == "CONFLICT" {
			status = http.StatusConflict
		}
		httpx.WriteError(w, status, strings.ToLower(typed.Code), typed.Message)
		return
	}
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Project not found.")
		return
	}
	if r.Context().Err() != nil {
		return
	}
	if d.Logger != nil {
		d.Logger.ErrorContext(r.Context(), "project request failed", "err", err)
	}
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		invalid(w)
		return false
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		invalid(w)
		return false
	}
	return true
}
func empty(w http.ResponseWriter, r *http.Request) bool {
	var in map[string]json.RawMessage
	if !decode(w, r, &in) {
		return false
	}
	if len(in) != 0 {
		invalid(w)
		return false
	}
	return true
}
func invalid(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid project request.")
}
