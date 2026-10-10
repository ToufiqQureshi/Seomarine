package onboarding

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
	Logger      *slog.Logger
	Service     *Service
	WithSession func(http.Handler) http.Handler
}

func Mount(mux *http.ServeMux, d Deps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(h) }
	mux.Handle("POST /api/v1/onboarding/answers/get", protect(http.HandlerFunc(d.get)))
	mux.Handle("POST /api/v1/onboarding/answers/save", protect(http.HandlerFunc(d.save)))
	mux.Handle("POST /api/v1/onboarding/gsc-nudge/dismiss", protect(http.HandlerFunc(d.dismissNudge)))
}

func (d Deps) get(w http.ResponseWriter, r *http.Request) {
	if !empty(w, r) {
		invalid(w)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	if d.Service == nil {
		unavailable(w)
		return
	}
	out, err := d.Service.Get(r.Context(), user.ID)
	if err != nil {
		d.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (d Deps) save(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InterestedFeatures *[]string `json:"interestedFeatures"`
		WorkFor            *string   `json:"workFor"`
		ClientWebsiteCount *string   `json:"clientWebsiteCount"`
		FoundVia           *string   `json:"foundVia"`
		Completed          bool      `json:"completed"`
	}
	if !decode(w, r, &body) {
		invalid(w)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	if d.Service == nil {
		unavailable(w)
		return
	}
	if err := d.Service.Save(r.Context(), user.ID, user.OrganizationID, SaveInput{
		InterestedFeatures: body.InterestedFeatures, WorkFor: body.WorkFor,
		ClientWebsiteCount: body.ClientWebsiteCount, FoundVia: body.FoundVia, Completed: body.Completed,
	}); err != nil {
		d.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func (d Deps) dismissNudge(w http.ResponseWriter, r *http.Request) {
	if !empty(w, r) {
		invalid(w)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	if d.Service == nil {
		unavailable(w)
		return
	}
	if err := d.Service.DismissGSCNudge(r.Context(), user.ID, user.OrganizationID); err != nil {
		d.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func (d Deps) fail(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil {
		return
	}
	if d.Logger != nil {
		d.Logger.ErrorContext(r.Context(), "onboarding request failed", "err", err)
	}
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(dst) != nil {
		return false
	}
	return errors.Is(dec.Decode(new(any)), io.EOF)
}
func empty(w http.ResponseWriter, r *http.Request) bool {
	var body struct{}
	return decode(w, r, &body)
}
func invalid(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose valid onboarding answers.")
}
func unavailable(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusServiceUnavailable, "onboarding_unavailable", "Onboarding is not available on this server.")
}
