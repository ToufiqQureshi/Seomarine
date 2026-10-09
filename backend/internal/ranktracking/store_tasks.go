package ranktracking

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// TaskRow is a queued provider task stored for a run.
type TaskRow struct {
	TrackingKeywordID string
	Device            string
	TaskID            string
	State             string // posted, collected or failed
}

// Task states.
const (
	TaskPosted    = "posted"
	TaskCollected = "collected"
	TaskFailed    = "failed"
)

// RecordTasks stores provider task ids. Call it right after a post: the ids
// are what lets a crashed worker collect paid tasks instead of paying again.
func (s Store) RecordTasks(ctx context.Context, runID string, tasks []PostedTask) error {
	if len(tasks) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, t := range tasks {
		batch.Queue(`INSERT INTO go_rank_check_tasks (run_id, tracking_keyword_id, device, task_id)
			VALUES ($1, $2, $3, $4) ON CONFLICT (run_id, tracking_keyword_id, device) DO NOTHING`,
			runID, t.KeywordID, t.Device, t.TaskID)
	}
	results := s.DB.SendBatch(ctx, batch)
	for range tasks {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("record rank check task: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("record rank check tasks: %w", err)
	}
	return nil
}

// Tasks returns every task stored for a run.
func (s Store) Tasks(ctx context.Context, runID string) ([]TaskRow, error) {
	rows, err := s.DB.Query(ctx, `SELECT tracking_keyword_id, device, task_id, state FROM go_rank_check_tasks
		WHERE run_id = $1 ORDER BY created_at, tracking_keyword_id, device`, runID)
	if err != nil {
		return nil, fmt.Errorf("list rank check tasks: %w", err)
	}
	tasks, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (TaskRow, error) {
		var t TaskRow
		err := r.Scan(&t.TrackingKeywordID, &t.Device, &t.TaskID, &t.State)
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("read rank check tasks: %w", err)
	}
	return tasks, nil
}

// SetTaskState records what became of a task.
func (s Store) SetTaskState(ctx context.Context, runID, keywordID, device, state string) error {
	if _, err := s.DB.Exec(ctx, `UPDATE go_rank_check_tasks SET state = $4
		WHERE run_id = $1 AND tracking_keyword_id = $2 AND device = $3`, runID, keywordID, device, state); err != nil {
		return fmt.Errorf("set rank check task state: %w", err)
	}
	return nil
}
