package activation

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
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	mux.Handle("POST /api/v1/projects/{projectId}/activation/click", protect(http.HandlerFunc(d.click)))
	mux.Handle("POST /api/v1/projects/{projectId}/activation/ga4-dismiss", protect(http.HandlerFunc(d.ga4Dismiss)))
	mux.Handle("POST /api/v1/projects/{projectId}/activation/dismiss", protect(http.HandlerFunc(d.dismiss)))
}
func (d Deps) click(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Step string `json:"step"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := d.Service.MarkClicked(r.Context(), r.PathValue("projectId"), in.Step); err != nil {
		invalid(w)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func (d Deps) ga4Dismiss(w http.ResponseWriter, r *http.Request) {
	if !empty(w, r) {
		return
	}
	if err := d.Service.DismissGA4(r.Context(), r.PathValue("projectId")); err != nil {
		d.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func (d Deps) dismiss(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Step      string `json:"step"`
		Dismissed bool   `json:"dismissed"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	if err := d.Service.SetDismissed(r.Context(), u.ID, r.PathValue("projectId"), in.Step, in.Dismissed); err != nil {
		invalid(w)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func (d Deps) fail(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil {
		return
	}
	if d.Logger != nil {
		d.Logger.ErrorContext(r.Context(), "activation update failed", "err", err)
	}
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
}
func empty(w http.ResponseWriter, r *http.Request) bool { var in struct{}; return decode(w, r, &in) }
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
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
func invalid(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid activation update.")
}
