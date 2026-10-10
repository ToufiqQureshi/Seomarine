package ranktracking

import (
	"context"
	"fmt"
)

// ListConfigSummaries returns active project configs with the list-card data.
func (s Store) ListConfigSummaries(ctx context.Context, projectID string) ([]ConfigSummary, error) {
	rows, err := s.DB.Query(ctx, `SELECT c.id,c.project_id,c.domain,c.location_code,c.language_code,c.location_name,
		c.devices,c.serp_depth,c.schedule_interval,c.is_active,c.last_checked_at,c.next_check_at,c.last_skip_reason,c.created_at,
		(SELECT count(*)::int FROM go_rank_tracking_keywords k WHERE k.config_id=c.id),
		r.status,r.completed_at
		FROM go_rank_tracking_configs c
		LEFT JOIN LATERAL (
			SELECT status,completed_at FROM go_rank_check_runs WHERE config_id=c.id
			ORDER BY started_at DESC,id DESC LIMIT 1
		) r ON true
		WHERE c.project_id=$1 AND c.is_active
		ORDER BY c.created_at,c.id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list rank tracking config summaries: %w", err)
	}
	defer rows.Close()
	summaries := make([]ConfigSummary, 0)
	for rows.Next() {
		var item ConfigSummary
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.Domain, &item.LocationCode, &item.LanguageCode,
			&item.LocationName, &item.Devices, &item.SerpDepth, &item.ScheduleInterval, &item.IsActive,
			&item.LastCheckedAt, &item.NextCheckAt, &item.LastSkipReason, &item.CreatedAt,
			&item.KeywordCount, &item.LastRunStatus, &item.LastRunCompletedAt); err != nil {
			return nil, fmt.Errorf("read rank tracking config summary: %w", err)
		}
		summaries = append(summaries, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rank tracking config summaries: %w", err)
	}
	return summaries, nil
}

var _ interface {
	ListConfigSummaries(context.Context, string) ([]ConfigSummary, error)
} = Store{}
