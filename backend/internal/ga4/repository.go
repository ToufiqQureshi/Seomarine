package ga4

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrConnectionNotFound marks a project without a configured GA4 property.
var ErrConnectionNotFound = errors.New("GA4 connection not found")

// RowQuerier is the pgx row-query method required by the repository.
type RowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// ConnectionRepository reads the legacy-owned GA4 connection without changing
// its schema while the setup flow is still served by TypeScript.
type ConnectionRepository struct{ DB RowQuerier }

// GetByProjectID returns the configured GA4 property for a project.
func (r ConnectionRepository) GetByProjectID(ctx context.Context, projectID string) (Connection, error) {
	var connection Connection
	err := r.DB.QueryRow(ctx, `SELECT property_id, property_display_name, property_time_zone, property_currency_code, connected_by_user_id, ga4_account_id FROM ga4_connections WHERE project_id=$1`, projectID).
		Scan(&connection.PropertyID, &connection.PropertyDisplayName, &connection.PropertyTimeZone, &connection.PropertyCurrencyCode, &connection.ConnectedByUserID, &connection.GA4AccountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, ErrConnectionNotFound
	}
	if err != nil {
		return Connection{}, err
	}
	return connection, nil
}
