package sam

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxBodyBytes = 8 << 10

// Deps supplies the session service and authentication/project middleware.
type Deps struct {
	Logger            *slog.Logger
	Service           *Service
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// Mount registers SAM session list, create and archive endpoints.
func Mount(mux *http.ServeMux, d Deps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	base := "POST /api/v1/projects/{projectId}/sam/sessions/"
	mux.Handle(base+"list", protect(http.HandlerFunc(d.list)))
	mux.Handle(base+"create", protect(http.HandlerFunc(d.create)))
	mux.Handle(base+"archive", protect(http.HandlerFunc(d.archive)))
}

func (d Deps) list(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyObject(w, r) {
		writeInvalid(w)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	if d.Service == nil {
		d.unavailable(w)
		return
	}
	sessions, err := d.Service.List(r.Context(), r.PathValue("projectId"), user.ID)
	if err != nil {
		d.fail(w, r, "list", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sessions)
}

func (d Deps) create(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyObject(w, r) {
		writeInvalid(w)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	if d.Service == nil {
		d.unavailable(w)
		return
	}
	session, err := d.Service.Create(r.Context(), r.PathValue("projectId"), user.ID)
	if err != nil {
		d.fail(w, r, "create", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"id": session.ID})
}

type archiveRequest struct {
	SessionID string `json:"sessionId"`
}

func (d Deps) archive(w http.ResponseWriter, r *http.Request) {
	var input archiveRequest
	if !decodeJSON(w, r, &input) || input.SessionID == "" || len(input.SessionID) > 128 {
		writeInvalid(w)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	if d.Service == nil {
		d.unavailable(w)
		return
	}
	if err := d.Service.Archive(r.Context(), r.PathValue("projectId"), user.ID, input.SessionID); err != nil {
		d.fail(w, r, "archive", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func decodeEmptyObject(w http.ResponseWriter, r *http.Request) bool {
	var input map[string]json.RawMessage
	return decodeJSON(w, r, &input) && input != nil && len(input) == 0
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return false
	}
	return true
}

func (d Deps) fail(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if errors.Is(err, ErrSessionNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "sam_session_not_found", "Chat session not found.")
		return
	}
	if r.Context().Err() != nil {
		d.Logger.InfoContext(r.Context(), "SAM session request canceled", "operation", operation, "err", err)
		return
	}
	d.Logger.ErrorContext(r.Context(), "SAM session request failed", "operation", operation, "err", err)
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
}

func (d Deps) unavailable(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusServiceUnavailable, "sam_unavailable", "SAM sessions are not available on this server.")
}

func writeInvalid(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid SAM session request.")
}
