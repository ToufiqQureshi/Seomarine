package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

// SessionSnapshot is the Better Auth get-session response shape used by the
// hosted React client.
type SessionSnapshot struct {
	ID                   string    `json:"id"`
	ExpiresAt            time.Time `json:"expiresAt"`
	Token                string    `json:"token"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
	IPAddress            *string   `json:"ipAddress"`
	UserAgent            *string   `json:"userAgent"`
	UserID               string    `json:"userId"`
	ActiveOrganizationID *string   `json:"activeOrganizationId"`
}

// AuthenticatedUser is the public user object returned by Better Auth.
type AuthenticatedUser struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"emailVerified"`
	Image         *string   `json:"image"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// CurrentSession is the payload returned for an authenticated browser session.
type CurrentSession struct {
	Session SessionSnapshot    `json:"session"`
	User    AuthenticatedUser `json:"user"`
}

// GetCurrentSession reads the current valid Better Auth session. A missing,
// invalid, revoked or expired cookie returns ErrUnauthenticated.
func (s *Service) GetCurrentSession(r *http.Request) (CurrentSession, error) {
	cookie, err := r.Cookie(secureSessionCookie)
	if errors.Is(err, http.ErrNoCookie) {
		cookie, err = r.Cookie(sessionCookie)
	}
	if err != nil {
		return CurrentSession{}, ErrUnauthenticated
	}
	token, ok := verifySignedValue(cookie.Value, s.secret)
	if !ok {
		return CurrentSession{}, ErrUnauthenticated
	}

	var result CurrentSession
	var ipAddress, userAgent, activeOrganizationID, image string
	err = s.repo.db.QueryRow(r.Context(), `
		SELECT s.id, s.expires_at, s.token, s.created_at, s.updated_at,
		       coalesce(s.ip_address, ''), coalesce(s.user_agent, ''),
		       s.user_id, coalesce(s.active_organization_id, ''),
		       u.id, u.name, u.email, u.email_verified, coalesce(u.image, ''),
		       u.created_at, u.updated_at
		FROM session s
		JOIN "user" u ON u.id = s.user_id
		WHERE s.token = $1 AND s.expires_at > now()`, token,
	).Scan(
		&result.Session.ID, &result.Session.ExpiresAt, &result.Session.Token,
		&result.Session.CreatedAt, &result.Session.UpdatedAt,
		&ipAddress, &userAgent, &result.Session.UserID, &activeOrganizationID,
		&result.User.ID, &result.User.Name, &result.User.Email, &result.User.EmailVerified,
		&image, &result.User.CreatedAt, &result.User.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return CurrentSession{}, ErrUnauthenticated
	}
	if err != nil {
		return CurrentSession{}, fmt.Errorf("load current auth session: %w", err)
	}
	result.Session.IPAddress = optionalString(ipAddress)
	result.Session.UserAgent = optionalString(userAgent)
	result.Session.ActiveOrganizationID = optionalString(activeOrganizationID)
	result.User.Image = optionalString(image)
	return result, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// MountCurrentSession registers the hosted Better Auth get-session endpoint.
func MountCurrentSession(mux *http.ServeMux, service *Service, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	mux.HandleFunc("GET /api/auth/get-session", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "auth_unavailable", "Session lookup is temporarily unavailable.")
			return
		}
		session, err := service.GetCurrentSession(r)
		if errors.Is(err, ErrUnauthenticated) {
			httpx.WriteJSON(w, http.StatusOK, nil)
			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "load current auth session", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "auth_session_failed", "Could not load the current session.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, session)
	})
}
