package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// repository reads the legacy better-auth and project tables, which the
// legacy app owns and migrates.
type repository struct {
	db *pgxpool.Pool
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

// isProjectMember reports whether userID is a member of the organization
// that owns the unarchived project projectID.
func (r repository) isProjectMember(ctx context.Context, userID, projectID string) (bool, error) {
	var ok bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM projects p
			JOIN member m ON m.organization_id = p.organization_id
			WHERE p.id = $1 AND p.archived_at IS NULL AND m.user_id = $2
		)`,
		projectID, userID,
	).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check project membership: %w", err)
	}
	return ok, nil
}
