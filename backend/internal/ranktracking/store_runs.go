package ranktracking

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrRunActive means the config already has a pending or running run.
var ErrRunActive = errors.New("a rank check is already running for this config")

const activeRunIndex = "go_rank_check_runs_one_active_idx"

// SnapshotInput is a snapshot ready to store.
type SnapshotInput struct {
	RunID             string
	TrackingKeywordID string
	Keyword           string
	Device            string
	Position          *int
	URL               *string
	SerpFeatures      []string
}

// InsertRun stores a new pending run. The partial unique index allows one
// active run per config, so a second insert is ErrRunActive.
func (s Store) InsertRun(ctx context.Context, r Run) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO go_rank_check_runs (id, config_id, project_id, status, keywords_total, is_subset_run)
		VALUES ($1, $2, $3, 'pending', $4, $5)`, r.ID, r.ConfigID, r.ProjectID, r.KeywordsTotal, r.IsSubsetRun)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" && pgErr.ConstraintName == activeRunIndex {
		return ErrRunActive
	}
	if err != nil {
		return fmt.Errorf("insert rank check run: %w", err)
	}
	return nil
}

// GetRun returns a run by id, or nil.
func (s Store) GetRun(ctx context.Context, id string) (*Run, error) {
	var r Run
	err := s.DB.QueryRow(ctx, `SELECT `+runColumns+` FROM go_rank_check_runs WHERE id = $1`, id).Scan(&r.ID, &r.ConfigID,
		&r.ProjectID, &r.Status, &r.KeywordsTotal, &r.KeywordsChecked, &r.IsSubsetRun, &r.ErrorMessage, &r.StartedAt, &r.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rank check run %s: %w", id, err)
	}
	return &r, nil
}

// MarkRunning moves a pending run to running and sets its keyword total. It
// reports false when the run is no longer active, for example because stale
// cleanup failed it, so a superseded run is never resurrected.
func (s Store) MarkRunning(ctx context.Context, id string, total int) (bool, error) {
	tag, err := s.DB.Exec(ctx, `UPDATE go_rank_check_runs SET status = 'running', keywords_total = $2
		WHERE id = $1 AND status IN ('pending', 'running')`, id, total)
	if err != nil {
		return false, fmt.Errorf("mark rank check run running: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// SetRunErrorIfEmpty records why a keyword failed. The first reason wins, so
// the run shows the provider's own message.
func (s Store) SetRunErrorIfEmpty(ctx context.Context, id, message string) error {
	if _, err := s.DB.Exec(ctx, `UPDATE go_rank_check_runs SET error_message = $2 WHERE id = $1 AND error_message IS NULL`, id, message); err != nil {
		return fmt.Errorf("record rank check error: %w", err)
	}
	return nil
}

// InsertSnapshots stores snapshots. A pair already stored for the run is
// skipped, so re-running a job never duplicates rows.
func (s Store) InsertSnapshots(ctx context.Context, snaps []SnapshotInput) error {
	if len(snaps) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, snap := range snaps {
		features := snap.SerpFeatures
		if features == nil {
			features = []string{}
		}
		batch.Queue(`INSERT INTO go_rank_snapshots (run_id, tracking_keyword_id, keyword, device, position, url, serp_features)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (run_id, tracking_keyword_id, device) DO NOTHING`,
			snap.RunID, snap.TrackingKeywordID, snap.Keyword, snap.Device, snap.Position, snap.URL, features)
	}
	results := s.DB.SendBatch(ctx, batch)
	for range snaps {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("insert rank snapshot: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("insert rank snapshots: %w", err)
	}
	return nil
}

// CheckedPairs returns the "keywordID:device" pairs a run already has.
func (s Store) CheckedPairs(ctx context.Context, runID string) (map[string]bool, error) {
	rows, err := s.DB.Query(ctx, `SELECT tracking_keyword_id || ':' || device FROM go_rank_snapshots WHERE run_id = $1`, runID)
	if err != nil {
		return nil, fmt.Errorf("list checked rank pairs: %w", err)
	}
	pairs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("read checked rank pairs: %w", err)
	}
	set := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		set[p] = true
	}
	return set, nil
}

// CountCheckedKeywords counts distinct keywords with a snapshot in the run.
func (s Store) CountCheckedKeywords(ctx context.Context, runID string) (int, error) {
	var n int
	if err := s.DB.QueryRow(ctx, `SELECT count(DISTINCT tracking_keyword_id) FROM go_rank_snapshots WHERE run_id = $1`, runID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count checked rank keywords: %w", err)
	}
	return n, nil
}

// FinishRun ends an active run. On completed it also stamps the config's
// last_checked_at and clears its skip reason; a failed run must not move
// last_checked_at because nothing was checked. It reports false when the run
// had already ended, so a superseded run never overwrites the newer decision.
func (s Store) FinishRun(ctx context.Context, run Run, status string, checked int, message *string) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin finish rank check run: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	tag, err := tx.Exec(ctx, `UPDATE go_rank_check_runs SET status = $2, keywords_checked = $3, completed_at = now(), error_message = $4
		WHERE id = $1 AND status IN ('pending', 'running')`, run.ID, status, checked, message)
	if err != nil {
		return false, fmt.Errorf("finish rank check run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if status == "completed" {
		if _, err := tx.Exec(ctx, `UPDATE go_rank_tracking_configs SET last_checked_at = now(), last_skip_reason = NULL WHERE id = $1`, run.ConfigID); err != nil {
			return false, fmt.Errorf("stamp rank config checked: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit finish rank check run: %w", err)
	}
	return true, nil
}

// FailRunIfActive fails a run that is still pending or running, which frees
// its slot. It is safe to call on runs that already ended.
func (s Store) FailRunIfActive(ctx context.Context, id, reason string) error {
	if _, err := s.DB.Exec(ctx, `UPDATE go_rank_check_runs SET status = 'failed', completed_at = now(), error_message = $2
		WHERE id = $1 AND status IN ('pending', 'running')`, id, reason); err != nil {
		return fmt.Errorf("fail rank check run: %w", err)
	}
	return nil
}

// JobState returns the queue state of a run's job, or "" when it has none.
func (s Store) JobState(ctx context.Context, queue, runID string) (string, error) {
	var state string
	err := s.DB.QueryRow(ctx, `SELECT state FROM go_jobs WHERE queue = $1 AND idempotency_key = $2`, queue, runID).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get rank check job state: %w", err)
	}
	return state, nil
}
