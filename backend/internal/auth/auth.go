// Package auth authenticates requests with the legacy app's better-auth
// session cookie and authorizes access to projects.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrUnauthenticated means the request has no valid, unexpired session.
	ErrUnauthenticated = errors.New("unauthenticated")
	// ErrProjectNotFound means the project does not exist, is archived, or
	// belongs to an organization the user is not a member of. The cases are
	// indistinguishable so project ids from other organizations don't leak.
	ErrProjectNotFound = errors.New("project not found")
)

// better-auth names the cookie with a __Secure- prefix when its base URL is
// https, and without it on plain-http localhost.
const (
	sessionCookie       = "better-auth.session_token"
	secureSessionCookie = "__Secure-" + sessionCookie
)

// User is the signed-in user behind a session.
type User struct {
	ID    string
	Name  string
	Email string
	// OrganizationID is the session's active organization, and Role the
	// user's role in it ("owner", "admin" or "member"). Both are empty when
	// the session has no active organization or the user is no longer a
	// member of it: the session only hints at the organization, the member
	// row is the authorization fact.
	OrganizationID string
	Role           string
}

// Service authenticates sessions and authorizes project access.
type Service struct {
	repo   repository
	secret []byte
}

// NewService returns a Service that reads the legacy auth tables through db
// and verifies cookies signed with the legacy BETTER_AUTH_SECRET.
func NewService(db *pgxpool.Pool, secret string) *Service {
	return &Service{repo: repository{db: db}, secret: []byte(secret)}
}

// Authenticate returns the user whose session cookie r carries. It returns
// ErrUnauthenticated when the cookie is missing, its signature is wrong, or
// the session is unknown or expired.
//
// It always reads the session table and ignores better-auth's cached
// session_data cookie, so a revoked session stops working immediately.
func (s *Service) Authenticate(r *http.Request) (User, error) {
	cookie, err := r.Cookie(secureSessionCookie)
	if errors.Is(err, http.ErrNoCookie) {
		cookie, err = r.Cookie(sessionCookie)
	}
	if err != nil {
		return User{}, ErrUnauthenticated
	}
	token, ok := verifySignedValue(cookie.Value, s.secret)
	if !ok {
		return User{}, ErrUnauthenticated
	}
	user, err := s.repo.userBySessionToken(r.Context(), token)
	if err != nil {
		return User{}, fmt.Errorf("authenticate session: %w", err)
	}
	return user, nil
}

// AuthorizeProject returns nil when userID is a member of the organization
// that owns the active project projectID, and ErrProjectNotFound otherwise.
func (s *Service) AuthorizeProject(ctx context.Context, userID, projectID string) error {
	ok, err := s.repo.isProjectMember(ctx, userID, projectID)
	if err != nil {
		return fmt.Errorf("authorize project %s: %w", projectID, err)
	}
	if !ok {
		return ErrProjectNotFound
	}
	return nil
}

// verifySignedValue checks a better-call signed cookie value and returns the
// token it carries. better-call signs as
// encodeURIComponent(token + "." + base64(HMAC-SHA256(secret, token))).
func verifySignedValue(raw string, secret []byte) (string, bool) {
	value, err := url.PathUnescape(raw)
	if err != nil {
		return "", false
	}
	dot := strings.LastIndexByte(value, '.')
	if dot < 1 {
		return "", false
	}
	token, signature := value[:dot], value[dot+1:]

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(token))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(want)) {
		return "", false
	}
	return token, true
}
