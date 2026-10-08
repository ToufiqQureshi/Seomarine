package dataforseo

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UsageRecorder stores the cost of every billed task in go_dataforseo_usage.
type UsageRecorder struct {
	db *pgxpool.Pool
}

// NewUsageRecorder returns a CostRecorder backed by db.
func NewUsageRecorder(db *pgxpool.Pool) *UsageRecorder {
	return &UsageRecorder{db: db}
}

// RecordDataForSEO implements CostRecorder.
func (r *UsageRecorder) RecordDataForSEO(ctx context.Context, organizationID string, cost Cost) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO go_dataforseo_usage (organization_id, path, cost_usd) VALUES ($1, $2, $3)`,
		organizationID, "/"+strings.Join(cost.Path, "/"), cost.USD)
	if err != nil {
		return fmt.Errorf("insert DataForSEO usage: %w", err)
	}
	return nil
}
