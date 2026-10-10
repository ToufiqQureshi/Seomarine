package ga4

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
)

type setupDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// SetupRepository implements the legacy-compatible Google grant and property
// mapping operations needed while the TypeScript schema remains authoritative.
type SetupRepository struct{ DB setupDB }

// GetByProjectID reads a property mapping only inside the requested organization.
func (r SetupRepository) GetByProjectID(ctx context.Context, organizationID, projectID string) (ProjectConnection, error) {
	var c ProjectConnection
	err := r.DB.QueryRow(ctx, `SELECT g.property_id, g.property_display_name, g.property_time_zone, g.property_currency_code,
		g.connected_by_user_id, g.ga4_account_id, g.connected_account_email, g.created_at
		FROM ga4_connections g JOIN projects p ON p.id=g.project_id AND p.organization_id=g.organization_id
		WHERE g.project_id=$1 AND g.organization_id=$2`, projectID, organizationID).
		Scan(&c.PropertyID, &c.PropertyDisplayName, &c.PropertyTimeZone, &c.PropertyCurrencyCode,
			&c.ConnectedByUserID, &c.GA4AccountID, &c.ConnectedAccountEmail, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProjectConnection{}, ErrConnectionNotFound
	}
	if err != nil {
		return ProjectConnection{}, fmt.Errorf("get GA4 connection: %w", err)
	}
	return c, nil
}

// HasGrant reports whether the user has connected any Analytics OAuth grant.
func (r SetupRepository) HasGrant(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := r.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account WHERE user_id=$1 AND provider_id='google-analytics')`, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check GA4 grant: %w", err)
	}
	return exists, nil
}

// ListGrants returns the Analytics grants owned by one user.
func (r SetupRepository) ListGrants(ctx context.Context, userID string) ([]PropertyGrant, error) {
	rows, err := r.DB.Query(ctx, `SELECT account_id FROM account WHERE user_id=$1 AND provider_id='google-analytics' ORDER BY account_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list GA4 grants: %w", err)
	}
	grants, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (PropertyGrant, error) {
		var grant PropertyGrant
		return grant, row.Scan(&grant.AccountID)
	})
	if err != nil {
		return nil, fmt.Errorf("read GA4 grants: %w", err)
	}
	return grants, nil
}

// CanManage checks whether the user is an owner or admin of the project organization.
func (r SetupRepository) CanManage(ctx context.Context, userID, organizationID, projectID string) (bool, error) {
	var allowed bool
	err := r.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects p JOIN member m ON m.organization_id=p.organization_id
		WHERE p.id=$1 AND p.organization_id=$2 AND m.user_id=$3 AND m.role IN ('owner','admin'))`, projectID, organizationID, userID).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check GA4 integration permission: %w", err)
	}
	return allowed, nil
}

// Upsert saves selected property metadata while retaining email for an unchanged grant.
func (r SetupRepository) Upsert(ctx context.Context, organizationID, projectID string, c ProjectConnection) (ProjectConnection, error) {
	id, err := ids.New()
	if err != nil {
		return ProjectConnection{}, err
	}
	var saved ProjectConnection
	err = r.DB.QueryRow(ctx, `INSERT INTO ga4_connections
		(id, project_id, organization_id, property_id, property_display_name, property_time_zone, property_currency_code,
		 connected_by_user_id, ga4_account_id, connected_account_email)
		SELECT $1,p.id,p.organization_id,$4,$5,$6,$7,$8,$9,$10 FROM projects p WHERE p.id=$2 AND p.organization_id=$3
		ON CONFLICT (project_id) DO UPDATE SET organization_id=EXCLUDED.organization_id, property_id=EXCLUDED.property_id,
		property_display_name=EXCLUDED.property_display_name, property_time_zone=EXCLUDED.property_time_zone,
		property_currency_code=EXCLUDED.property_currency_code, connected_by_user_id=EXCLUDED.connected_by_user_id,
		ga4_account_id=EXCLUDED.ga4_account_id,
		connected_account_email=CASE WHEN ga4_connections.connected_by_user_id=EXCLUDED.connected_by_user_id AND ga4_connections.ga4_account_id=EXCLUDED.ga4_account_id
		THEN COALESCE(EXCLUDED.connected_account_email,ga4_connections.connected_account_email) ELSE EXCLUDED.connected_account_email END,
		updated_at=to_char(current_timestamp AT TIME ZONE 'utc','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		RETURNING property_id,property_display_name,property_time_zone,property_currency_code,connected_by_user_id,ga4_account_id,
		connected_account_email,created_at`,
		id, projectID, organizationID, c.PropertyID, c.PropertyDisplayName, c.PropertyTimeZone, c.PropertyCurrencyCode, c.ConnectedByUserID, c.GA4AccountID, c.ConnectedAccountEmail).
		Scan(&saved.PropertyID, &saved.PropertyDisplayName, &saved.PropertyTimeZone, &saved.PropertyCurrencyCode, &saved.ConnectedByUserID, &saved.GA4AccountID, &saved.ConnectedAccountEmail, &saved.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProjectConnection{}, ErrConnectionNotFound
	}
	if err != nil {
		return ProjectConnection{}, fmt.Errorf("save GA4 connection: %w", err)
	}
	return saved, nil
}

// Delete removes the GA4 mapping for the project and organization.
func (r SetupRepository) Delete(ctx context.Context, organizationID, projectID string) error {
	if _, err := r.DB.Exec(ctx, `DELETE FROM ga4_connections WHERE project_id=$1 AND organization_id=$2`, projectID, organizationID); err != nil {
		return fmt.Errorf("delete GA4 connection: %w", err)
	}
	return nil
}

var _ PropertyConnectionStore = SetupRepository{}
