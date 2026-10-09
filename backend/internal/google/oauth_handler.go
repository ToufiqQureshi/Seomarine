package google

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

// OAuthDeps holds the authenticated route dependencies for Google consent.
type OAuthDeps struct {
	Service     *OAuthService
	Auth        *auth.Service
	Logger      *slog.Logger
	WithSession func(http.Handler) http.Handler
}

type startRequest struct {
	CallbackURL string `json:"callbackURL"`
}

// MountOAuth registers consent-start and Google callback routes.
func MountOAuth(mux *http.ServeMux, deps OAuthDeps) {
	mux.Handle("POST /api/v1/google/oauth/{provider}/start", deps.WithSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := integrationFor(r.PathValue("provider")); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid Google integration.")
			return
		}
		var input startRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || input.CallbackURL == "" || len(input.CallbackURL) > 2048 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid return page.")
			return
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid return page.")
			return
		}
		if deps.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "Google connection is not configured.")
			return
		}
		user, _ := auth.UserFromContext(r.Context())
		authorizationURL, err := deps.Service.Start(r.Context(), user.ID, r.PathValue("provider"), input.CallbackURL)
		if err != nil {
			deps.Logger.ErrorContext(r.Context(), "start google oauth", "provider", r.PathValue("provider"), "err", err)
			httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "Could not start Google connection.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"url": authorizationURL})
	})))
	for _, provider := range []string{"gsc", "ga4"} {
		in, _ := integrationFor(provider)
		mux.HandleFunc("GET "+in.callbackPath, func(w http.ResponseWriter, r *http.Request) {
			if deps.Service == nil || deps.Auth == nil {
				http.Redirect(w, r, "/auth-error?error=auth_config_missing", http.StatusSeeOther)
				return
			}
			user, err := deps.Auth.Authenticate(r)
			if err != nil {
				http.Redirect(w, r, "/auth-error?error=unauthenticated", http.StatusSeeOther)
				return
			}
			stateValue, code, googleError := r.URL.Query().Get("state"), r.URL.Query().Get("code"), r.URL.Query().Get("error")
			if len(stateValue) > 256 || len(googleError) > 64 {
				http.Redirect(w, r, "/auth-error?error=state_mismatch", http.StatusSeeOther)
				return
			}
			path, resultError := deps.Service.Complete(r.Context(), user.ID, provider, stateValue, code, googleError)
			if resultError != "" {
				path = appendLinkError(path, provider, resultError, deps.Service.PublicOrigin)
			}
			http.Redirect(w, r, path, http.StatusSeeOther)
		})
	}
}

func appendLinkError(path, provider, code, origin string) string {
	safePath := SafeCallbackPath(path, origin)
	parsed, err := url.Parse(safePath)
	if err != nil {
		return "/auth-error?error=state_mismatch"
	}
	values := parsed.Query()
	values.Set("google_link_error", provider)
	values.Set("error", strings.TrimSpace(code))
	parsed.RawQuery = values.Encode()
	return parsed.String()
}
