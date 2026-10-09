package google

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AccountRepository removes a user's Google data grant and dependent mappings.
type AccountRepository struct{ Pool *pgxpool.Pool }

func accountScope(provider string) (table, providerID, accountColumn string, err error) {
	switch provider {
	case "gsc":
		return "gsc_connections", "google-search-console", "gsc_account_id", nil
	case "ga4":
		return "ga4_connections", "google-analytics", "ga4_account_id", nil
	default:
		return "", "", "", errors.New("invalid google provider")
	}
}

// RemovalImpact counts the signed-in user's project mappings removed with a grant.
func (r AccountRepository) RemovalImpact(ctx context.Context, userID, provider, accountID string) (int, error) {
	if r.Pool == nil || userID == "" || accountID == "" {
		return 0, errors.New("invalid google account parameters")
	}
	table, _, column, err := accountScope(provider)
	if err != nil {
		return 0, err
	}
	condition := column + "=$2"
	if provider == "gsc" {
		condition = "(" + condition + " OR " + column + " IS NULL)"
	}
	// Table and column are fixed by accountScope, never user supplied SQL.
	query := "SELECT count(*) FROM " + table + " WHERE connected_by_user_id=$1 AND " + condition
	var count int
	if err := r.Pool.QueryRow(ctx, query, userID, accountID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count google account mappings: %w", err)
	}
	return count, nil
}

// Remove atomically removes the user's mapping rows and matching data grant.
// It is idempotent; a repeated removal affects no additional rows.
func (r AccountRepository) Remove(ctx context.Context, userID, provider, accountID string) error {
	if r.Pool == nil || userID == "" || accountID == "" {
		return errors.New("invalid google account parameters")
	}
	table, providerID, column, err := accountScope(provider)
	if err != nil {
		return err
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin google account removal: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	condition := column + "=$2"
	if provider == "gsc" {
		condition = "(" + condition + " OR " + column + " IS NULL)"
	}
	query := "DELETE FROM " + table + " WHERE connected_by_user_id=$1 AND " + condition
	if _, err := tx.Exec(ctx, query, userID, accountID); err != nil {
		return fmt.Errorf("remove google project mappings: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM account WHERE user_id=$1 AND provider_id=$2 AND account_id=$3`, userID, providerID, accountID); err != nil {
		return fmt.Errorf("remove google data grant: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit google account removal: %w", err)
	}
	return nil
}
