package domain

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

// ErrProjectNotFound means the project is absent, archived, or owned by another organization.
var ErrProjectNotFound = errors.New("project not found")

// ErrProjectMarketUnavailable marks a storage failure separate from provider failures.
var ErrProjectMarketUnavailable = errors.New("project market unavailable")

// ProjectMarkets reads the project's market while enforcing organization ownership.
type ProjectMarkets interface {
	Get(context.Context, string, string) (market.Pair, error)
}

// ProjectMarketRepository reads the legacy projects table until projects move to Go.
type ProjectMarketRepository struct{ DB *pgxpool.Pool }

// Get returns the market for an active project within the specified organization.
func (r ProjectMarketRepository) Get(ctx context.Context, organizationID, projectID string) (market.Pair, error) {
	var pair market.Pair
	err := r.DB.QueryRow(ctx, `SELECT location_code, language_code FROM projects
		WHERE id = $1 AND organization_id = $2 AND archived_at IS NULL`, projectID, organizationID).Scan(&pair.LocationCode, &pair.LanguageCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return market.Pair{}, ErrProjectNotFound
	}
	if err != nil {
		return market.Pair{}, fmt.Errorf("%w: %w", ErrProjectMarketUnavailable, err)
	}
	return pair, nil
}
