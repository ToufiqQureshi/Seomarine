package gsc

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
)

var ErrProjectPropertyNotFound = errors.New("Search Console connection not found")

type propertyRowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type connectionQuerier interface {
	propertyRowQuerier
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Connection is the property and grant selected for a project.
type ProjectProperty struct {
	SiteURL               string
	ConnectedByUserID     string
	GSCAccountID          *string
	ConnectedAccountEmail *string
	CreatedAt             string
}

// Grant identifies one linked Search Console Google grant.
type Grant struct{ AccountID string }

// PropertyRepository reads and updates existing project-to-property rows.
type PropertyRepository struct{ DB connectionQuerier }

// GetByProjectID reads the mapping only when both project and organization match.
func (r PropertyRepository) GetByProjectID(ctx context.Context, organizationID, projectID string) (ProjectProperty, error) {
	var c ProjectProperty
	err := r.DB.QueryRow(ctx, `SELECT g.site_url, g.connected_by_user_id, g.gsc_account_id, g.connected_account_email, g.created_at
		FROM gsc_connections g JOIN projects p ON p.id=g.project_id AND p.organization_id=g.organization_id
		WHERE g.project_id=$1 AND g.organization_id=$2`, projectID, organizationID).
		Scan(&c.SiteURL, &c.ConnectedByUserID, &c.GSCAccountID, &c.ConnectedAccountEmail, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProjectProperty{}, ErrProjectPropertyNotFound
	}
	if err != nil {
		return ProjectProperty{}, fmt.Errorf("get Search Console connection: %w", err)
	}
	return c, nil
}

// HasGrant reports whether a user has linked any Search Console grant.
func (r PropertyRepository) HasGrant(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := r.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account WHERE user_id=$1 AND provider_id='google-search-console')`, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check Search Console grant: %w", err)
	}
	return exists, nil
}

// ListGrants returns the Google account IDs linked to a user.
func (r PropertyRepository) ListGrants(ctx context.Context, userID string) ([]Grant, error) {
	rows, err := r.DB.Query(ctx, `SELECT account_id FROM account WHERE user_id=$1 AND provider_id='google-search-console'`, userID)
	if err != nil {
		return nil, fmt.Errorf("list Search Console grants: %w", err)
	}
	grants, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Grant, error) {
		var grant Grant
		err := row.Scan(&grant.AccountID)
		return grant, err
	})
	if err != nil {
		return nil, fmt.Errorf("read Search Console grants: %w", err)
	}
	return grants, nil
}

// CanManage checks the project membership role. Only owners and admins can
// change a shared integration, matching the TypeScript organization policy.
func (r PropertyRepository) CanManage(ctx context.Context, userID, organizationID, projectID string) (bool, error) {
	var allowed bool
	err := r.DB.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM projects p JOIN member m ON m.organization_id=p.organization_id
		WHERE p.id=$1 AND p.organization_id=$2 AND m.user_id=$3 AND m.role IN ('owner','admin')
	)`, projectID, organizationID, userID).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check Search Console integration permission: %w", err)
	}
	return allowed, nil
}

// Upsert replaces a project's selected property and retains the saved email
// only when the same user and Google grant are selected again.
func (r PropertyRepository) Upsert(ctx context.Context, organizationID, projectID string, c ProjectProperty) (ProjectProperty, error) {
	if c.GSCAccountID == nil {
		return ProjectProperty{}, errors.New("selected Search Console grant is required")
	}
	id, err := ids.New()
	if err != nil {
		return ProjectProperty{}, err
	}
	var saved ProjectProperty
	err = r.DB.QueryRow(ctx, `INSERT INTO gsc_connections
		(id, project_id, organization_id, site_url, connected_by_user_id, gsc_account_id, connected_account_email)
		SELECT $1, p.id, p.organization_id, $4, $5, $6, $7 FROM projects p
		WHERE p.id=$2 AND p.organization_id=$3
		ON CONFLICT (project_id) DO UPDATE SET
			site_url=EXCLUDED.site_url, organization_id=EXCLUDED.organization_id,
			connected_by_user_id=EXCLUDED.connected_by_user_id, gsc_account_id=EXCLUDED.gsc_account_id,
			connected_account_email=CASE
				WHEN gsc_connections.connected_by_user_id=EXCLUDED.connected_by_user_id
				 AND gsc_connections.gsc_account_id=EXCLUDED.gsc_account_id
				THEN COALESCE(EXCLUDED.connected_account_email, gsc_connections.connected_account_email)
				ELSE EXCLUDED.connected_account_email END,
			updated_at=to_char(current_timestamp AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		RETURNING site_url, connected_by_user_id, gsc_account_id, connected_account_email, created_at`,
		id, projectID, organizationID, c.SiteURL, c.ConnectedByUserID, *c.GSCAccountID, c.ConnectedAccountEmail).
		Scan(&saved.SiteURL, &saved.ConnectedByUserID, &saved.GSCAccountID, &saved.ConnectedAccountEmail, &saved.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProjectProperty{}, ErrProjectPropertyNotFound
	}
	if err != nil {
		return ProjectProperty{}, fmt.Errorf("save Search Console connection: %w", err)
	}
	return saved, nil
}

// Delete removes the selected property from a project within its organization.
func (r PropertyRepository) Delete(ctx context.Context, organizationID, projectID string) error {
	if _, err := r.DB.Exec(ctx, `DELETE FROM gsc_connections WHERE project_id=$1 AND organization_id=$2`, projectID, organizationID); err != nil {
		return fmt.Errorf("delete Search Console connection: %w", err)
	}
	return nil
}
