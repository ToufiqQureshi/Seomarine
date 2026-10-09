package ranktracking

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// DueConfig is a config whose next check time has passed, with its owner.
type DueConfig struct {
	Config
	OrganizationID string
}

// ClaimInput moves a due config's anchor if nobody changed it meanwhile.
type ClaimInput struct {
	ConfigID  string
	ProjectID string
	Observed  time.Time // the anchor the caller read
	Next      time.Time // the anchor to write
	// SetSkipReason also writes SkipReason (nil clears it).
	SetSkipReason bool
	SkipReason    *string
}

// DueConfigs returns active, scheduled configs whose time has come, oldest
// first so a backlog drains in order. The owner comes from the projects table.
func (s Store) DueConfigs(ctx context.Context, now time.Time, limit int) ([]DueConfig, error) {
	rows, err := s.DB.Query(ctx, `SELECT c.id, c.project_id, c.domain, c.location_code, c.language_code, c.location_name, c.devices,
			c.serp_depth, c.schedule_interval, c.is_active, c.last_checked_at, c.next_check_at, c.last_skip_reason, c.created_at,
			p.organization_id
		FROM go_rank_tracking_configs c JOIN projects p ON p.id = c.project_id
		WHERE c.is_active AND c.schedule_interval <> 'manual' AND c.next_check_at <= $1 AND p.archived_at IS NULL
		ORDER BY c.next_check_at, c.id LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list due rank tracking configs: %w", err)
	}
	due, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (DueConfig, error) {
		var d DueConfig
		err := r.Scan(&d.ID, &d.ProjectID, &d.Domain, &d.LocationCode, &d.LanguageCode, &d.LocationName, &d.Devices,
			&d.SerpDepth, &d.ScheduleInterval, &d.IsActive, &d.LastCheckedAt, &d.NextCheckAt, &d.LastSkipReason,
			&d.CreatedAt, &d.OrganizationID)
		return d, err
	})
	if err != nil {
		return nil, fmt.Errorf("read due rank tracking configs: %w", err)
	}
	return due, nil
}

// KeywordCounts returns the number of keywords per config.
func (s Store) KeywordCounts(ctx context.Context, configIDs []string) (map[string]int, error) {
	rows, err := s.DB.Query(ctx, `SELECT config_id, count(*)::int FROM go_rank_tracking_keywords
		WHERE config_id = ANY($1) GROUP BY config_id`, configIDs)
	if err != nil {
		return nil, fmt.Errorf("count rank tracking keywords: %w", err)
	}
	defer rows.Close()
	counts := make(map[string]int, len(configIDs))
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("read rank tracking keyword counts: %w", err)
		}
		counts[id] = n
	}
	return counts, rows.Err()
}

// ClaimDueConfig advances a due config's anchor only if it still holds the
// anchor the caller read. It reports false when another worker or a manual edit
// got there first, which is how two schedulers never start the same check.
func (s Store) ClaimDueConfig(ctx context.Context, in ClaimInput) (bool, error) {
	tag, err := s.DB.Exec(ctx, `UPDATE go_rank_tracking_configs
		SET next_check_at = $4, last_skip_reason = CASE WHEN $5 THEN $6 ELSE last_skip_reason END
		WHERE id = $1 AND project_id = $2 AND is_active AND next_check_at = $3`,
		in.ConfigID, in.ProjectID, in.Observed, in.Next, in.SetSkipReason, in.SkipReason)
	if err != nil {
		return false, fmt.Errorf("claim due rank tracking config: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
