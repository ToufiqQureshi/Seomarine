package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

// RevokeSession deletes the session carried by r. Missing or invalid cookies
// are treated as signed out so callers can always clear their local cookie.
func (s *Service) RevokeSession(ctx context.Context, r *http.Request) error {
	cookie, err := r.Cookie(secureSessionCookie)
	if errors.Is(err, http.ErrNoCookie) {
		cookie, err = r.Cookie(sessionCookie)
	}
	if errors.Is(err, http.ErrNoCookie) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read session cookie: %w", err)
	}
	token, ok := verifySignedValue(cookie.Value, s.secret)
	if !ok {
		return nil
	}
	if _, err := s.repo.db.Exec(ctx, "DELETE FROM session WHERE token = $1", token); err != nil {
		return fmt.Errorf("revoke auth session: %w", err)
	}
	return nil
}

// MountSession registers the hosted Better Auth sign-out endpoint.
func MountSession(mux *http.ServeMux, service *Service, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	mux.HandleFunc("POST /api/auth/sign-out", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			clearSessionCookies(w)
			httpx.WriteError(w, http.StatusServiceUnavailable, "auth_unavailable", "Sign-out is temporarily unavailable.")
			return
		}
		if err := service.RevokeSession(r.Context(), r); err != nil {
			logger.ErrorContext(r.Context(), "revoke auth session", "err", err)
			clearSessionCookies(w)
			httpx.WriteError(w, http.StatusInternalServerError, "auth_sign_out_failed", "Sign-out could not be completed. Please try again.")
			return
		}
		clearSessionCookies(w)
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
}

func clearSessionCookies(w http.ResponseWriter) {
	expires := time.Unix(1, 0).UTC()
	http.SetCookie(w, &http.Cookie{
		Name: secureSessionCookie, Value: "", Path: "/", Expires: expires,
		MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", Expires: expires,
		MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}
