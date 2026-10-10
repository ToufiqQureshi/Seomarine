package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// repository reads the legacy better-auth and project tables, which the
// legacy app owns and migrates.
type repository struct {
	db *pgxpool.Pool
}

// OrganizationMembership is the authenticated user's workspace membership.
type OrganizationMembership struct {
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName"`
	Role             string `json:"role"`
}

type OrganizationMember struct {
	ID     string `json:"id"`
	UserID string `json:"userId"`
	Role   string `json:"role"`
	User   struct {
		Name  *string `json:"name"`
		Email string  `json:"email"`
	} `json:"user"`
}

type OrganizationInvitation struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      *string   `json:"role"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type OrganizationTeam struct {
	Members            []OrganizationMember     `json:"members"`
	PendingInvitations []OrganizationInvitation `json:"pendingInvitations"`
}

// userBySessionToken returns the user of the unexpired session with token,
// with the session's active organization when the user is still a member of
// it, or ErrUnauthenticated when there is no such session.
func (r repository) userBySessionToken(ctx context.Context, token string) (User, error) {
	var u User
	err := r.db.QueryRow(ctx, `
		SELECT u.id, u.name, u.email, coalesce(m.organization_id, ''), coalesce(m.role, '')
		FROM session s
		JOIN "user" u ON u.id = s.user_id
		LEFT JOIN member m ON m.organization_id = s.active_organization_id AND m.user_id = s.user_id
		WHERE s.token = $1 AND s.expires_at > now()`,
		token,
	).Scan(&u.ID, &u.Name, &u.Email, &u.OrganizationID, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, fmt.Errorf("load session: %w", err)
	}
	return u, nil
}

// projectOrganization returns the organization that owns the unarchived
// project projectID when userID is a member of it, and false otherwise.
func (r repository) projectOrganization(ctx context.Context, userID, projectID string) (string, bool, error) {
	var orgID string
	err := r.db.QueryRow(ctx, `
		SELECT p.organization_id
		FROM projects p
		JOIN member m ON m.organization_id = p.organization_id
		WHERE p.id = $1 AND p.archived_at IS NULL AND m.user_id = $2`,
		projectID, userID,
	).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("check project membership: %w", err)
	}
	return orgID, true, nil
}

func (r repository) memberships(ctx context.Context, userID string) ([]OrganizationMembership, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.organization_id, o.name, m.role
		FROM member m JOIN organization o ON o.id = m.organization_id
		WHERE m.user_id = $1
		ORDER BY m.created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("list organization memberships: %w", err)
	}
	defer rows.Close()
	out := []OrganizationMembership{}
	for rows.Next() {
		var row OrganizationMembership
		if err := rows.Scan(&row.OrganizationID, &row.OrganizationName, &row.Role); err != nil {
			return nil, fmt.Errorf("scan organization membership: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organization memberships: %w", err)
	}
	return out, nil
}

func (r repository) team(ctx context.Context, organizationID string, includeInvitations bool) (OrganizationTeam, error) {
	rows, err := r.db.Query(ctx, `SELECT m.id,m.user_id,m.role,u.name,u.email FROM member m JOIN "user" u ON u.id=m.user_id WHERE m.organization_id=$1 ORDER BY m.created_at`, organizationID)
	if err != nil {
		return OrganizationTeam{}, fmt.Errorf("list organization members: %w", err)
	}
	out := OrganizationTeam{Members: []OrganizationMember{}, PendingInvitations: []OrganizationInvitation{}}
	for rows.Next() {
		var item OrganizationMember
		if err := rows.Scan(&item.ID, &item.UserID, &item.Role, &item.User.Name, &item.User.Email); err != nil {
			rows.Close()
			return OrganizationTeam{}, fmt.Errorf("scan organization member: %w", err)
		}
		out.Members = append(out.Members, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return OrganizationTeam{}, fmt.Errorf("iterate organization members: %w", err)
	}
	rows.Close()
	if !includeInvitations {
		return out, nil
	}
	invites, err := r.db.Query(ctx, `SELECT id,email,role,expires_at FROM invitation WHERE organization_id=$1 AND status='pending' AND expires_at>now() ORDER BY created_at`, organizationID)
	if err != nil {
		return OrganizationTeam{}, fmt.Errorf("list organization invitations: %w", err)
	}
	defer invites.Close()
	for invites.Next() {
		var item OrganizationInvitation
		if err := invites.Scan(&item.ID, &item.Email, &item.Role, &item.ExpiresAt); err != nil {
			return OrganizationTeam{}, fmt.Errorf("scan organization invitation: %w", err)
		}
		out.PendingInvitations = append(out.PendingInvitations, item)
	}
	if err := invites.Err(); err != nil {
		return OrganizationTeam{}, fmt.Errorf("iterate organization invitations: %w", err)
	}
	return out, nil
}
