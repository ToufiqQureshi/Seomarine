package ranktracking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const runColumns = `id, config_id, project_id, status, keywords_total, keywords_checked, is_subset_run,
	error_message, started_at, completed_at`

func (s Store) queryRun(ctx context.Context, where, configID string) (*Run, error) {
	var r Run
	err := s.DB.QueryRow(ctx, `SELECT `+runColumns+` FROM go_rank_check_runs WHERE config_id = $1 `+where+
		` ORDER BY started_at DESC, id DESC LIMIT 1`, configID).Scan(&r.ID, &r.ConfigID, &r.ProjectID, &r.Status,
		&r.KeywordsTotal, &r.KeywordsChecked, &r.IsSubsetRun, &r.ErrorMessage, &r.StartedAt, &r.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rank check run: %w", err)
	}
	return &r, nil
}

// LatestRun returns the newest run of a config, or nil when it never ran.
func (s Store) LatestRun(ctx context.Context, configID string) (*Run, error) {
	return s.queryRun(ctx, "", configID)
}

// ActiveRun returns the pending or running run of a config, or nil. The
// partial unique index allows at most one.
func (s Store) ActiveRun(ctx context.Context, configID string) (*Run, error) {
	return s.queryRun(ctx, "AND status IN ('pending', 'running')", configID)
}

// Snapshots returns one snapshot per keyword and device from completed runs:
// the newest, or the oldest when q.Earliest is set. Ties on time go to the
// later-written row, so the answer does not depend on row order.
func (s Store) Snapshots(ctx context.Context, q SnapshotQuery) ([]Snapshot, error) {
	direction := "DESC"
	if q.Earliest {
		direction = "ASC"
	}
	rows, err := s.DB.Query(ctx, `SELECT DISTINCT ON (s.tracking_keyword_id, s.device)
			s.id, s.run_id, s.tracking_keyword_id, s.keyword, s.device, s.position, s.url, s.serp_features, s.checked_at
		FROM go_rank_snapshots s JOIN go_rank_check_runs r ON r.id = s.run_id
		WHERE r.config_id = $1 AND r.status = 'completed'
			AND ($2::timestamptz IS NULL OR s.checked_at <= $2)
			AND ($3::text[] IS NULL OR s.tracking_keyword_id = ANY($3))
		ORDER BY s.tracking_keyword_id, s.device, s.checked_at `+direction+`, s.id `+direction,
		q.ConfigID, q.AtOrBefore, q.KeywordIDs)
	if err != nil {
		return nil, fmt.Errorf("query rank snapshots: %w", err)
	}
	snapshots, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Snapshot, error) {
		var snap Snapshot
		err := r.Scan(&snap.ID, &snap.RunID, &snap.TrackingKeywordID, &snap.Keyword, &snap.Device, &snap.Position,
			&snap.URL, &snap.SerpFeatures, &snap.CheckedAt)
		return snap, err
	})
	if err != nil {
		return nil, fmt.Errorf("read rank snapshots: %w", err)
	}
	return snapshots, nil
}

// KeywordHistory returns a keyword's positions from completed runs since a time.
func (s Store) KeywordHistory(ctx context.Context, configID, keywordID string, since time.Time) ([]HistoryPoint, error) {
	rows, err := s.DB.Query(ctx, `SELECT s.device, s.checked_at, s.position
		FROM go_rank_snapshots s JOIN go_rank_check_runs r ON r.id = s.run_id
		WHERE r.config_id = $1 AND r.status = 'completed' AND s.tracking_keyword_id = $2 AND s.checked_at >= $3
		ORDER BY s.checked_at, s.id`, configID, keywordID, since)
	if err != nil {
		return nil, fmt.Errorf("query keyword history: %w", err)
	}
	points, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (HistoryPoint, error) {
		var p HistoryPoint
		err := r.Scan(&p.Device, &p.CheckedAt, &p.Position)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("read keyword history: %w", err)
	}
	return points, nil
}

// Trend counts keywords per position bucket for each full completed run.
func (s Store) Trend(ctx context.Context, configID, device string, since time.Time) ([]TrendPoint, error) {
	rows, err := s.DB.Query(ctx, `SELECT s.run_id, r.started_at, count(*)::int,
			(count(*) FILTER (WHERE s.position BETWEEN 1 AND 3))::int,
			(count(*) FILTER (WHERE s.position BETWEEN 4 AND 10))::int,
			(count(*) FILTER (WHERE s.position BETWEEN 11 AND 20))::int
		FROM go_rank_snapshots s JOIN go_rank_check_runs r ON r.id = s.run_id
		WHERE r.config_id = $1 AND r.status = 'completed' AND NOT r.is_subset_run AND s.device = $2 AND s.checked_at >= $3
		GROUP BY s.run_id, r.started_at ORDER BY r.started_at, s.run_id`, configID, device, since)
	if err != nil {
		return nil, fmt.Errorf("query rank trend: %w", err)
	}
	points, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (TrendPoint, error) {
		var p TrendPoint
		err := r.Scan(&p.RunID, &p.CheckedAt, &p.Total, &p.Top3, &p.Top4to10, &p.Top11to20)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("read rank trend: %w", err)
	}
	return points, nil
}

// Matrix returns positions from the newest runLimit full completed runs.
func (s Store) Matrix(ctx context.Context, configID, device string, runLimit int) ([]MatrixPoint, error) {
	rows, err := s.DB.Query(ctx, `SELECT s.run_id, r.started_at, s.tracking_keyword_id, s.position
		FROM go_rank_snapshots s JOIN go_rank_check_runs r ON r.id = s.run_id
		WHERE s.device = $2 AND s.run_id IN (
			SELECT id FROM go_rank_check_runs
			WHERE config_id = $1 AND status = 'completed' AND NOT is_subset_run
			ORDER BY started_at DESC, id DESC LIMIT $3)
		ORDER BY r.started_at, s.run_id, s.tracking_keyword_id`, configID, device, runLimit)
	if err != nil {
		return nil, fmt.Errorf("query position matrix: %w", err)
	}
	points, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (MatrixPoint, error) {
		var p MatrixPoint
		err := r.Scan(&p.RunID, &p.CheckedAt, &p.TrackingKeywordID, &p.Position)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("read position matrix: %w", err)
	}
	return points, nil
}
