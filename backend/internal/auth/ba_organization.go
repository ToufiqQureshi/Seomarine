package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/ids"
)

// The routes below keep Better Auth's organization plugin contract
// (/api/auth/organization/...) so the React client works unchanged. Creating an
// organization, inviting and deleting one are not here on purpose: every user
// gets exactly one workspace at sign-in, invitations are created and emailed by
// the team API, and deletion stays a support action.

type orgAPIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *orgAPIError) Error() string { return e.Code + ": " + e.Message }

var (
	errInvitationNotFound = &orgAPIError{http.StatusBadRequest, "INVITATION_NOT_FOUND", "Invitation not found"}
	errNotRecipient       = &orgAPIError{http.StatusForbidden, "YOU_ARE_NOT_THE_RECIPIENT_OF_THE_INVITATION", "You are not the recipient of the invitation"}
	errAlreadyMember      = &orgAPIError{http.StatusBadRequest, "BAD_REQUEST", "You are already a member of this organization."}
	errMemberNotFound     = &orgAPIError{http.StatusBadRequest, "MEMBER_NOT_FOUND", "Member not found"}
	errNotAllowed         = &orgAPIError{http.StatusForbidden, "FORBIDDEN", "You are not allowed to do this"}
	errCannotRemoveOwner  = &orgAPIError{http.StatusForbidden, "YOU_ARE_NOT_ALLOWED_TO_DELETE_THIS_MEMBER", "You are not allowed to delete this member"}
	errNotAMember         = &orgAPIError{http.StatusForbidden, "USER_IS_NOT_A_MEMBER_OF_THE_ORGANIZATION", "User is not a member of the organization"}
	errBadBody            = &orgAPIError{http.StatusBadRequest, "BAD_REQUEST", "Invalid request body"}
	errNoSession          = &orgAPIError{http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized"}
)

const (
	maxOrgBody  = 4 << 10
	stampLayout = "2006-01-02T15:04:05.000Z"
)

type invitationBody struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	Email          string  `json:"email"`
	Role           *string `json:"role"`
	Status         string  `json:"status"`
	ExpiresAt      string  `json:"expiresAt"`
	InviterID      string  `json:"inviterId"`
}

type memberBody struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	UserID         string `json:"userId"`
	Role           string `json:"role"`
	CreatedAt      string `json:"createdAt"`
}

// MountBetterAuthOrganization registers the organization routes the client uses.
func MountBetterAuthOrganization(mux *http.ServeMux, s *Service, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	handle := func(pattern string, h func(http.ResponseWriter, *http.Request, User) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			user, err := s.Authenticate(r)
			if err != nil {
				if errors.Is(err, ErrUnauthenticated) {
					writeOrgError(w, errNoSession)
					return
				}
				logger.ErrorContext(r.Context(), "authenticate organization request", "err", err)
				writeOrgError(w, &orgAPIError{http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Internal server error"})
				return
			}
			err = h(w, r, user)
			var apiErr *orgAPIError
			switch {
			case err == nil:
			case errors.As(err, &apiErr):
				writeOrgError(w, apiErr)
			case r.Context().Err() != nil:
			default:
				logger.ErrorContext(r.Context(), "organization request failed", "path", r.URL.Path, "err", err)
				writeOrgError(w, &orgAPIError{http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Internal server error"})
			}
		})
	}
	handle("GET /api/auth/organization/get-invitation", s.getInvitation)
	handle("POST /api/auth/organization/accept-invitation", s.acceptInvitation)
	handle("POST /api/auth/organization/reject-invitation", s.rejectInvitation)
	handle("POST /api/auth/organization/cancel-invitation", s.cancelInvitation)
	handle("POST /api/auth/organization/remove-member", s.removeMember)
	handle("POST /api/auth/organization/set-active", s.setActiveOrganization)
}

func writeOrgError(w http.ResponseWriter, e *orgAPIError) {
	writeOrgJSON(w, e.Status, e)
}

func writeOrgJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func decodeOrgBody(r *http.Request, dst any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxOrgBody+1))
	if err != nil || len(body) > maxOrgBody || json.Unmarshal(body, dst) != nil {
		return errBadBody
	}
	return nil
}

// canManage reports whether a role may manage members and invitations. Better
// Auth stores several roles as one comma-separated string and ORs the checks;
// an unknown role grants nothing.
func canManage(role string) bool {
	for _, name := range strings.Split(role, ",") {
		if name = strings.TrimSpace(name); name == "owner" || name == "admin" {
			return true
		}
	}
	return false
}

func isOwner(role string) bool {
	for _, name := range strings.Split(role, ",") {
		if strings.TrimSpace(name) == "owner" {
			return true
		}
	}
	return false
}

type invitationRow struct {
	invitationBody
	expires      time.Time
	orgName      string
	orgSlug      string
	inviterEmail string
}

type orgQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadInvitation(ctx context.Context, q orgQuerier, id string, lock bool) (invitationRow, bool, error) {
	var row invitationRow
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF i"
	}
	err := q.QueryRow(ctx, `SELECT i.id, i.organization_id, i.email, i.role, i.status, i.expires_at, i.inviter_id, o.name, o.slug, u.email
		FROM invitation i
		JOIN organization o ON o.id = i.organization_id
		JOIN "user" u ON u.id = i.inviter_id
		WHERE i.id = $1`+suffix, id).Scan(&row.ID, &row.OrganizationID, &row.Email, &row.Role, &row.Status, &row.expires, &row.InviterID, &row.orgName, &row.orgSlug, &row.inviterEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return invitationRow{}, false, nil
	}
	if err != nil {
		return invitationRow{}, false, fmt.Errorf("load invitation: %w", err)
	}
	row.ExpiresAt = row.expires.UTC().Format(stampLayout)
	return row, true, nil
}

// usable is a pending, unexpired invitation addressed to this user.
func (row invitationRow) usable(user User, now time.Time) error {
	if row.Status != "pending" || !row.expires.After(now) {
		return errInvitationNotFound
	}
	if !strings.EqualFold(strings.TrimSpace(row.Email), strings.TrimSpace(user.Email)) {
		return errNotRecipient
	}
	return nil
}

func (s *Service) getInvitation(w http.ResponseWriter, r *http.Request, user User) error {
	row, ok, err := loadInvitation(r.Context(), s.repo.db, r.URL.Query().Get("id"), false)
	if err != nil {
		return err
	}
	if !ok {
		return errInvitationNotFound
	}
	if err := row.usable(user, time.Now()); err != nil {
		return err
	}
	writeOrgJSON(w, http.StatusOK, map[string]any{
		"id": row.ID, "email": row.Email, "role": row.Role, "status": row.Status, "expiresAt": row.ExpiresAt,
		"organizationId": row.OrganizationID, "inviterId": row.InviterID,
		"organizationName": row.orgName, "organizationSlug": row.orgSlug, "inviterEmail": row.inviterEmail,
	})
	return nil
}

func (s *Service) acceptInvitation(w http.ResponseWriter, r *http.Request, user User) error {
	var in struct {
		InvitationID string `json:"invitationId"`
	}
	if err := decodeOrgBody(r, &in); err != nil {
		return err
	}
	token, _ := s.sessionToken(r)
	now := time.Now()
	var invitation invitationRow
	var member memberBody
	tx, err := s.repo.db.Begin(r.Context())
	if err != nil {
		return fmt.Errorf("begin accept invitation: %w", err)
	}
	defer tx.Rollback(r.Context())

	invitation, ok, err := loadInvitation(r.Context(), tx, in.InvitationID, true)
	if err != nil {
		return err
	}
	if !ok {
		return errInvitationNotFound
	}
	if err := invitation.usable(user, now); err != nil {
		return err
	}
	var already bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM member WHERE user_id = $1 AND organization_id = $2)`, user.ID, invitation.OrganizationID).Scan(&already); err != nil {
		return fmt.Errorf("check existing membership: %w", err)
	}
	if already {
		return errAlreadyMember
	}
	role := "member"
	if invitation.Role != nil && strings.TrimSpace(*invitation.Role) != "" {
		role = *invitation.Role
	}
	memberID, err := ids.New()
	if err != nil {
		return err
	}
	member = memberBody{ID: memberID, OrganizationID: invitation.OrganizationID, UserID: user.ID, Role: role, CreatedAt: now.UTC().Format(stampLayout)}
	if _, err := tx.Exec(r.Context(), `INSERT INTO member (id, organization_id, user_id, role, created_at) VALUES ($1, $2, $3, $4, $5)`,
		member.ID, member.OrganizationID, member.UserID, member.Role, now); err != nil {
		return fmt.Errorf("create member: %w", err)
	}
	if _, err := tx.Exec(r.Context(), `UPDATE invitation SET status = 'accepted' WHERE id = $1`, invitation.ID); err != nil {
		return fmt.Errorf("accept invitation: %w", err)
	}
	// Land the user's next sign-in in the workspace they just joined, and skip
	// first-run onboarding: the wizard configures a project and spends credits,
	// which must not be re-run against an already-configured workspace.
	if _, err := tx.Exec(r.Context(), `UPDATE "user" SET last_active_organization_id = $1 WHERE id = $2`, invitation.OrganizationID, user.ID); err != nil {
		return fmt.Errorf("save last active organization: %w", err)
	}
	if token != "" {
		if _, err := tx.Exec(r.Context(), `UPDATE session SET active_organization_id = $1 WHERE token = $2 AND user_id = $3`, invitation.OrganizationID, token, user.ID); err != nil {
			return fmt.Errorf("activate joined organization: %w", err)
		}
	}
	stamp := now.UTC().Format(stampLayout)
	if _, err := tx.Exec(r.Context(), `INSERT INTO user_onboarding_answers (user_id, organization_id, completed_at, gsc_nudge_dismissed_at, updated_at)
		VALUES ($1, $2, $3, $3, $3)
		ON CONFLICT (user_id) DO UPDATE SET completed_at = EXCLUDED.completed_at, gsc_nudge_dismissed_at = EXCLUDED.gsc_nudge_dismissed_at, updated_at = EXCLUDED.updated_at`,
		user.ID, invitation.OrganizationID, stamp); err != nil {
		return fmt.Errorf("skip onboarding: %w", err)
	}
	if err := tx.Commit(r.Context()); err != nil {
		return fmt.Errorf("commit accept invitation: %w", err)
	}
	invitation.Status = "accepted"
	writeOrgJSON(w, http.StatusOK, map[string]any{"invitation": invitation.invitationBody, "member": member})
	return nil
}

func (s *Service) rejectInvitation(w http.ResponseWriter, r *http.Request, user User) error {
	var in struct {
		InvitationID string `json:"invitationId"`
	}
	if err := decodeOrgBody(r, &in); err != nil {
		return err
	}
	row, ok, err := loadInvitation(r.Context(), s.repo.db, in.InvitationID, false)
	if err != nil {
		return err
	}
	if !ok {
		return errInvitationNotFound
	}
	if err := row.usable(user, time.Now()); err != nil {
		return err
	}
	if _, err := s.repo.db.Exec(r.Context(), `UPDATE invitation SET status = 'rejected' WHERE id = $1 AND status = 'pending'`, row.ID); err != nil {
		return fmt.Errorf("reject invitation: %w", err)
	}
	row.Status = "rejected"
	writeOrgJSON(w, http.StatusOK, map[string]any{"invitation": row.invitationBody, "member": nil})
	return nil
}

func (s *Service) callerRole(ctx context.Context, userID, organizationID string) (string, error) {
	var role string
	err := s.repo.db.QueryRow(ctx, `SELECT role FROM member WHERE user_id = $1 AND organization_id = $2 LIMIT 1`, userID, organizationID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errNotAMember
	}
	if err != nil {
		return "", fmt.Errorf("read caller role: %w", err)
	}
	return role, nil
}

func (s *Service) cancelInvitation(w http.ResponseWriter, r *http.Request, user User) error {
	var in struct {
		InvitationID string `json:"invitationId"`
	}
	if err := decodeOrgBody(r, &in); err != nil {
		return err
	}
	row, ok, err := loadInvitation(r.Context(), s.repo.db, in.InvitationID, false)
	if err != nil {
		return err
	}
	if !ok || row.Status != "pending" {
		return errInvitationNotFound
	}
	role, err := s.callerRole(r.Context(), user.ID, row.OrganizationID)
	if err != nil {
		return err
	}
	if !canManage(role) {
		return errNotAllowed
	}
	if _, err := s.repo.db.Exec(r.Context(), `UPDATE invitation SET status = 'canceled' WHERE id = $1 AND status = 'pending'`, row.ID); err != nil {
		return fmt.Errorf("cancel invitation: %w", err)
	}
	row.Status = "canceled"
	writeOrgJSON(w, http.StatusOK, row.invitationBody)
	return nil
}

func (s *Service) removeMember(w http.ResponseWriter, r *http.Request, user User) error {
	var in struct {
		MemberIDOrEmail string `json:"memberIdOrEmail"`
		OrganizationID  string `json:"organizationId"`
	}
	if err := decodeOrgBody(r, &in); err != nil {
		return err
	}
	orgID := in.OrganizationID
	if orgID == "" {
		orgID = user.OrganizationID
	}
	if orgID == "" || in.MemberIDOrEmail == "" {
		return errBadBody
	}
	role, err := s.callerRole(r.Context(), user.ID, orgID)
	if err != nil {
		return err
	}
	var target memberBody
	var created time.Time
	err = s.repo.db.QueryRow(r.Context(), `SELECT m.id, m.organization_id, m.user_id, m.role, m.created_at
		FROM member m JOIN "user" u ON u.id = m.user_id
		WHERE m.organization_id = $1 AND (m.id = $2 OR lower(u.email) = lower($2)) LIMIT 1`, orgID, in.MemberIDOrEmail).
		Scan(&target.ID, &target.OrganizationID, &target.UserID, &target.Role, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return errMemberNotFound
	}
	if err != nil {
		return fmt.Errorf("find member: %w", err)
	}
	target.CreatedAt = created.UTC().Format(stampLayout)
	// Leaving yourself needs no management right; removing someone else does.
	if target.UserID != user.ID && !canManage(role) {
		return errNotAllowed
	}
	// The owner holds billing and the workspace. Ownership moves only through
	// the transfer endpoint, which swaps two roles atomically.
	if isOwner(target.Role) {
		return errCannotRemoveOwner
	}

	tx, err := s.repo.db.Begin(r.Context())
	if err != nil {
		return fmt.Errorf("begin remove member: %w", err)
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `DELETE FROM member WHERE id = $1`, target.ID); err != nil {
		return fmt.Errorf("delete member: %w", err)
	}
	if _, err := tx.Exec(r.Context(), `UPDATE session SET active_organization_id = NULL WHERE user_id = $1 AND active_organization_id = $2`, target.UserID, orgID); err != nil {
		return fmt.Errorf("clear removed member sessions: %w", err)
	}
	if _, err := tx.Exec(r.Context(), `UPDATE "user" SET last_active_organization_id = NULL WHERE id = $1 AND last_active_organization_id = $2`, target.UserID, orgID); err != nil {
		return fmt.Errorf("clear removed member workspace: %w", err)
	}
	if err := tx.Commit(r.Context()); err != nil {
		return fmt.Errorf("commit remove member: %w", err)
	}
	writeOrgJSON(w, http.StatusOK, map[string]any{"member": target})
	return nil
}

func (s *Service) setActiveOrganization(w http.ResponseWriter, r *http.Request, user User) error {
	var in struct {
		OrganizationID string `json:"organizationId"`
	}
	if err := decodeOrgBody(r, &in); err != nil {
		return err
	}
	if in.OrganizationID == "" {
		return errBadBody
	}
	if err := s.SwitchOrganization(r, user.ID, in.OrganizationID); err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return errNotAMember
		}
		return err
	}
	var org struct {
		ID, Name, Slug string
		CreatedAt      time.Time
	}
	if err := s.repo.db.QueryRow(r.Context(), `SELECT id, name, slug, created_at FROM organization WHERE id = $1`, in.OrganizationID).
		Scan(&org.ID, &org.Name, &org.Slug, &org.CreatedAt); err != nil {
		return fmt.Errorf("load organization: %w", err)
	}
	writeOrgJSON(w, http.StatusOK, map[string]any{"id": org.ID, "name": org.Name, "slug": org.Slug, "createdAt": org.CreatedAt.UTC().Format(stampLayout)})
	return nil
}

// sessionToken returns the verified session token carried by r.
func (s *Service) sessionToken(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(secureSessionCookie)
	if errors.Is(err, http.ErrNoCookie) {
		cookie, err = r.Cookie(sessionCookie)
	}
	if err != nil {
		return "", false
	}
	return verifySignedValue(cookie.Value, s.secret)
}
