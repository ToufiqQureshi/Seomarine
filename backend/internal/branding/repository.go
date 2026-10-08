package branding

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// repository reads and writes the legacy organization_branding table through
// plain SQL. The schema is unchanged from the TypeScript app.
type repository struct {
	db *pgxpool.Pool
}

// get returns organizationID's branding row. found is false when the
// organization has never saved branding.
func (r repository) get(ctx context.Context, organizationID string) (Branding, bool, error) {
	var b Branding
	err := r.db.QueryRow(ctx, `
		SELECT brand_name, accent_color, logo_data_url, website_url, updated_at
		FROM organization_branding
		WHERE organization_id = $1`,
		organizationID,
	).Scan(&b.BrandName, &b.AccentColor, &b.LogoDataURL, &b.WebsiteURL, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Branding{}, false, nil
	}
	if err != nil {
		return Branding{}, false, fmt.Errorf("load organization branding: %w", err)
	}
	return b, true, nil
}

// upsert writes organizationID's branding row, replacing an existing one.
func (r repository) upsert(ctx context.Context, organizationID string, in Input, now time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO organization_branding
			(organization_id, brand_name, accent_color, logo_data_url, website_url, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (organization_id) DO UPDATE SET
			brand_name = EXCLUDED.brand_name,
			accent_color = EXCLUDED.accent_color,
			logo_data_url = EXCLUDED.logo_data_url,
			website_url = EXCLUDED.website_url,
			updated_at = EXCLUDED.updated_at`,
		organizationID, in.BrandName, in.AccentColor, in.LogoDataURL, in.WebsiteURL, formatTimestamp(now),
	)
	if err != nil {
		return fmt.Errorf("upsert organization branding: %w", err)
	}
	return nil
}

// delete returns organizationID to the default product branding.
func (r repository) delete(ctx context.Context, organizationID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM organization_branding WHERE organization_id = $1`, organizationID)
	if err != nil {
		return fmt.Errorf("delete organization branding: %w", err)
	}
	return nil
}
