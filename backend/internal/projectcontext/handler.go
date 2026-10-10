package projectcontext

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxRequestBytes = 1 << 20

type HandlerDeps struct {
	Logger            *slog.Logger
	Service           APIService
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// APIService is the project-scoped read and update surface.
type APIService interface {
	Get(context.Context, string) (ProjectContext, error)
	Apply(context.Context, string, []json.RawMessage, string) (ProjectContext, error)
}

func Mount(mux *http.ServeMux, d HandlerDeps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	mux.Handle("POST /api/v1/projects/{projectId}/context/get", protect(http.HandlerFunc(d.get)))
	mux.Handle("POST /api/v1/projects/{projectId}/context/update", protect(http.HandlerFunc(d.update)))
}

type contextRequest struct {
	ProjectID string            `json:"projectId"`
	Updates   []json.RawMessage `json:"updates"`
}

func (d HandlerDeps) get(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ProjectID string `json:"projectId"`
	}
	if !decode(w, r, &in) || !sameProject(w, r, in.ProjectID) {
		return
	}
	result, err := d.Service.Get(r.Context(), in.ProjectID)
	d.respond(w, r, result, err)
}
func (d HandlerDeps) update(w http.ResponseWriter, r *http.Request) {
	var in contextRequest
	if !decode(w, r, &in) || !sameProject(w, r, in.ProjectID) {
		return
	}
	result, err := d.Service.Apply(r.Context(), in.ProjectID, in.Updates, "user")
	d.respond(w, r, result, err)
}
func sameProject(w http.ResponseWriter, r *http.Request, bodyID string) bool {
	if bodyID == "" || bodyID != r.PathValue("projectId") {
		invalidRequest(w)
		return false
	}
	return true
}
func (d HandlerDeps) respond(w http.ResponseWriter, r *http.Request, value any, err error) {
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
		httpx.WriteError(w, status, strings.ToLower(typed.Code), typed.Message)
		return
	}
	if r.Context().Err() != nil {
		return
	}
	if d.Logger != nil {
		d.Logger.ErrorContext(r.Context(), "project context request failed", "err", err)
	}
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		invalidRequest(w)
		return false
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		invalidRequest(w)
		return false
	}
	return true
}
func invalidRequest(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid project context request.")
}
