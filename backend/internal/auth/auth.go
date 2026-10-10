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
	ErrProjectNotFound       = errors.New("project not found")
	ErrOrganizationForbidden = errors.New("organization operation forbidden")
	ErrOrganizationConflict  = errors.New("organization operation conflicts with current state")
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

// AuthorizeProject returns the id of the organization that owns the active
// project projectID when userID is a member of it, and ErrProjectNotFound
// otherwise.
func (s *Service) AuthorizeProject(ctx context.Context, userID, projectID string) (string, error) {
	orgID, ok, err := s.repo.projectOrganization(ctx, userID, projectID)
	if err != nil {
		return "", fmt.Errorf("authorize project %s: %w", projectID, err)
	}
	if !ok {
		return "", ErrProjectNotFound
	}
	return orgID, nil
}

// OrganizationMemberships returns the organizations the user may switch to.
func (s *Service) OrganizationMemberships(ctx context.Context, userID string) ([]OrganizationMembership, error) {
	return s.repo.memberships(ctx, userID)
}

// OrganizationTeam returns members and, for workspace admins, current invitations.
func (s *Service) OrganizationTeam(ctx context.Context, organizationID, role string) (OrganizationTeam, error) {
	return s.repo.team(ctx, organizationID, hasRole(role, "owner") || hasRole(role, "admin"))
}

func hasRole(roles, target string) bool {
	for _, role := range strings.Split(roles, ",") {
		if strings.TrimSpace(role) == target {
			return true
		}
	}
	return false
}

// TransferOrganizationOwnership promotes a current member and demotes the
// current owner in one transaction.
func (s *Service) TransferOrganizationOwnership(ctx context.Context, organizationID, ownerUserID, newOwnerMemberID string) error {
	tx, err := s.repo.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin ownership transfer: %w", err)
	}
	defer tx.Rollback(ctx)
	var ownerRole string
	if err := tx.QueryRow(ctx, `SELECT role FROM member WHERE organization_id=$1 AND user_id=$2 FOR UPDATE`, organizationID, ownerUserID).Scan(&ownerRole); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("lock current owner membership: %w", err)
	}
	if ownerRole != "owner" {
		return ErrOrganizationForbidden
	}
	var nextUserID string
	if err := tx.QueryRow(ctx, `SELECT user_id FROM member WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, newOwnerMemberID).Scan(&nextUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("lock new owner membership: %w", err)
	}
	if nextUserID == ownerUserID {
		return ErrOrganizationConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE member SET role='owner' WHERE organization_id=$1 AND id=$2`, organizationID, newOwnerMemberID); err != nil {
		return fmt.Errorf("promote new organization owner: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE member SET role='admin' WHERE organization_id=$1 AND user_id=$2 AND role='owner'`, organizationID, ownerUserID); err != nil {
		return fmt.Errorf("demote previous organization owner: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit ownership transfer: %w", err)
	}
	return nil
}

// SwitchOrganization changes the active organization only when the user has
// a current membership in it.
func (s *Service) SwitchOrganization(r *http.Request, userID, organizationID string) error {
	cookie, err := r.Cookie(secureSessionCookie)
	if errors.Is(err, http.ErrNoCookie) {
		cookie, err = r.Cookie(sessionCookie)
	}
	if err != nil {
		return ErrUnauthenticated
	}
	token, ok := verifySignedValue(cookie.Value, s.secret)
	if !ok {
		return ErrUnauthenticated
	}
	tx, err := s.repo.db.Begin(r.Context())
	if err != nil {
		return fmt.Errorf("begin organization switch: %w", err)
	}
	defer tx.Rollback(r.Context())
	var exists bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM member WHERE user_id = $1 AND organization_id = $2)`, userID, organizationID).Scan(&exists); err != nil {
		return fmt.Errorf("check organization membership: %w", err)
	}
	if !exists {
		return ErrProjectNotFound
	}
	tag, err := tx.Exec(r.Context(), `UPDATE session SET active_organization_id = $1 WHERE token = $2 AND user_id = $3 AND expires_at > now()`, organizationID, token, userID)
	if err != nil {
		return fmt.Errorf("update active session organization: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrUnauthenticated
	}
	if _, err := tx.Exec(r.Context(), `UPDATE "user" SET last_active_organization_id = $1 WHERE id = $2`, organizationID, userID); err != nil {
		return fmt.Errorf("save last active organization: %w", err)
	}
	if err := tx.Commit(r.Context()); err != nil {
		return fmt.Errorf("commit organization switch: %w", err)
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
