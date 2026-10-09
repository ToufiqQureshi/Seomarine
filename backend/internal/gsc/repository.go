package gsc

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrConnectionNotFound = errors.New("Search Console connection not found")

type connectionRowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Connection is the property and grant selected for a project.
type Connection struct {
	SiteURL               string
	ConnectedByUserID     string
	GSCAccountID          *string
	ConnectedAccountEmail *string
}

// ConnectionReader loads only the organization-authorized project's mapping.
type ConnectionReader interface {
	GetByProjectID(context.Context, string, string) (Connection, error)
}

// ConnectionRepository reads the existing GA4-owned GSC mapping table.
type ConnectionRepository struct{ DB connectionRowQuerier }

func (r ConnectionRepository) GetByProjectID(ctx context.Context, organizationID, projectID string) (Connection, error) {
	var c Connection
	err := r.DB.QueryRow(ctx, `SELECT g.site_url, g.connected_by_user_id, g.gsc_account_id, g.connected_account_email
		FROM gsc_connections AS g
		JOIN projects AS p ON p.id = g.project_id AND p.organization_id = g.organization_id
		WHERE g.project_id = $1 AND g.organization_id = $2`, projectID, organizationID).
		Scan(&c.SiteURL, &c.ConnectedByUserID, &c.GSCAccountID, &c.ConnectedAccountEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, ErrConnectionNotFound
	}
	if err != nil {
		return Connection{}, err
	}
	return c, nil
}
