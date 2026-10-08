// Package jobs provides a PostgreSQL-backed, at-least-once job queue.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrInvalidJob marks invalid queue input or options.
	ErrInvalidJob = errors.New("invalid job")
	ErrClaimLost  = errors.New("job claim is no longer owned by this worker")
)

const (
	defaultAttempts = 5
	defaultTimeout  = 5 * time.Minute
	maxBackoff      = time.Hour
)

// Queue is a PostgreSQL-backed job queue.
type Queue struct{ db *pgxpool.Pool }

// EnqueueInput describes a job to enqueue. Zero MaxAttempts and Timeout select defaults.
type EnqueueInput struct {
	Queue          string
	IdempotencyKey string
	Payload        json.RawMessage
	MaxAttempts    int
	Timeout        time.Duration
}

// Job is a claimed or enqueued job.
type Job struct {
	ID             int64
	Queue          string
	IdempotencyKey string
	Payload        json.RawMessage
	Attempts       int
	MaxAttempts    int
	Timeout        time.Duration
	ClaimVersion   int64
}

// New returns a Queue backed by db.
func New(db *pgxpool.Pool) (*Queue, error) {
	if db == nil {
		return nil, errors.New("jobs: database pool is required")
	}
	return &Queue{db: db}, nil
}

// Enqueue inserts a job or returns the existing job for the same queue/key.
// Idempotency keys are retained after completion to prevent accidental replay.
func (q *Queue) Enqueue(ctx context.Context, in EnqueueInput) (Job, error) {
	if err := validateInput(in); err != nil {
		return Job{}, err
	}
	if in.MaxAttempts == 0 {
		in.MaxAttempts = defaultAttempts
	}
	if in.Timeout == 0 {
		in.Timeout = defaultTimeout
	}
	var job Job
	var jobTimeoutSeconds int
	err := q.db.QueryRow(ctx, `
		INSERT INTO go_jobs (queue, idempotency_key, payload, max_attempts, timeout_seconds)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5)
		ON CONFLICT (queue, idempotency_key) WHERE idempotency_key IS NOT NULL
		DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key
		RETURNING id, queue, COALESCE(idempotency_key, ''), payload, attempts, max_attempts, timeout_seconds, claim_version`,
		in.Queue, in.IdempotencyKey, []byte(in.Payload), in.MaxAttempts, int((in.Timeout+time.Second-1)/time.Second)).Scan(
		&job.ID, &job.Queue, &job.IdempotencyKey, &job.Payload, &job.Attempts, &job.MaxAttempts, &jobTimeoutSeconds, &job.ClaimVersion)
	if err != nil {
		return Job{}, fmt.Errorf("enqueue job: %w", err)
	}
	job.Timeout = time.Duration(jobTimeoutSeconds) * time.Second
	return job, nil
}

// Claim atomically leases up to limit ready jobs. Expired leases are reclaimed;
// SKIP LOCKED lets independent workers claim disjoint jobs without a queue lock.
func (q *Queue) Claim(ctx context.Context, queue, workerID string, lease time.Duration, limit int) ([]Job, error) {
	if strings.TrimSpace(queue) == "" || strings.TrimSpace(workerID) == "" || len(workerID) > 128 || lease < time.Second || lease > 24*time.Hour || limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("%w: invalid claim options", ErrInvalidJob)
	}
	rows, err := q.db.Query(ctx, `
		WITH exhausted AS (
			UPDATE go_jobs SET state = 'failed', lease_until = NULL, worker_id = NULL,
				last_error = 'lease expired after final attempt', finished_at = now()
			WHERE queue = $1 AND state = 'running' AND lease_until <= now() AND attempts >= max_attempts
		), candidates AS (
			SELECT id FROM go_jobs
			WHERE queue = $1 AND available_at <= now() AND attempts < max_attempts
				AND (state = 'queued' OR (state = 'running' AND lease_until <= now()))
			ORDER BY available_at, id
			LIMIT $4 FOR UPDATE SKIP LOCKED
		)
		UPDATE go_jobs AS j SET state = 'running', attempts = j.attempts + 1,
			lease_until = now() + ($3::double precision * interval '1 millisecond'), worker_id = $2,
			claim_version = j.claim_version + 1
		FROM candidates c WHERE j.id = c.id
		RETURNING j.id, j.queue, COALESCE(j.idempotency_key, ''), j.payload,
			j.attempts, j.max_attempts, j.timeout_seconds, j.claim_version`,
		queue, workerID, lease.Milliseconds(), limit)
	if err != nil {
		return nil, fmt.Errorf("claim jobs: %w", err)
	}
	defer rows.Close()
	jobs := make([]Job, 0)
	for rows.Next() {
		var job Job
		var timeoutSeconds int
		if err := rows.Scan(&job.ID, &job.Queue, &job.IdempotencyKey, &job.Payload, &job.Attempts,
			&job.MaxAttempts, &timeoutSeconds, &job.ClaimVersion); err != nil {
			return nil, fmt.Errorf("scan claimed job: %w", err)
		}
		job.Timeout = time.Duration(timeoutSeconds) * time.Second
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read claimed jobs: %w", err)
	}
	return jobs, nil
}

// Complete marks a claimed job succeeded. It returns ErrClaimLost if the lease was lost.
func (q *Queue) Complete(ctx context.Context, job Job) error {
	tag, err := q.db.Exec(ctx, `UPDATE go_jobs SET state = 'succeeded', lease_until = NULL,
		worker_id = NULL, finished_at = now(), last_error = NULL
		WHERE id = $1 AND state = 'running' AND claim_version = $2 AND lease_until > now()`, job.ID, job.ClaimVersion)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return nil
}

// Extend renews the lease of a claimed job. It returns ErrClaimLost if the lease was lost.
func (q *Queue) Extend(ctx context.Context, job Job, workerID string, lease time.Duration) error {
	if workerID == "" || len(workerID) > 128 || lease < time.Second || lease > 24*time.Hour {
		return fmt.Errorf("%w: invalid lease extension", ErrInvalidJob)
	}
	tag, err := q.db.Exec(ctx, `UPDATE go_jobs SET lease_until = now() + ($4::double precision * interval '1 millisecond')
		WHERE id = $1 AND state = 'running' AND claim_version = $2 AND worker_id = $3 AND lease_until > now()`,
		job.ID, job.ClaimVersion, workerID, lease.Milliseconds())
	if err != nil {
		return fmt.Errorf("extend job lease: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return nil
}

// Retry releases a failed job with exponential backoff, or marks it terminal
// when its attempt budget is exhausted. The claim version fences stale workers.
func (q *Queue) Retry(ctx context.Context, job Job, cause error) error {
	if cause == nil {
		return fmt.Errorf("%w: retry cause is required", ErrInvalidJob)
	}
	delay := backoff(job.Attempts)
	tag, err := q.db.Exec(ctx, `UPDATE go_jobs SET
		state = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'queued' END,
		available_at = CASE WHEN attempts >= max_attempts THEN available_at ELSE now() + ($3::double precision * interval '1 millisecond') END,
		lease_until = NULL, worker_id = NULL, last_error = left($4, 4000),
		finished_at = CASE WHEN attempts >= max_attempts THEN now() ELSE NULL END
		WHERE id = $1 AND state = 'running' AND claim_version = $2 AND lease_until > now()`,
		job.ID, job.ClaimVersion, delay.Milliseconds(), cause.Error())
	if err != nil {
		return fmt.Errorf("retry job: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return nil
}

func validateInput(in EnqueueInput) error {
	if len(in.Queue) == 0 || len(in.Queue) > 128 || strings.TrimSpace(in.Queue) != in.Queue {
		return fmt.Errorf("%w: queue must be 1-128 trimmed characters", ErrInvalidJob)
	}
	if len(in.IdempotencyKey) > 256 || strings.TrimSpace(in.IdempotencyKey) != in.IdempotencyKey {
		return fmt.Errorf("%w: idempotency key must be at most 256 trimmed characters", ErrInvalidJob)
	}
	if !json.Valid(in.Payload) {
		return fmt.Errorf("%w: payload must be valid JSON", ErrInvalidJob)
	}
	if in.MaxAttempts < 0 || in.MaxAttempts > 100 {
		return fmt.Errorf("%w: max attempts must be between 1 and 100", ErrInvalidJob)
	}
	if in.Timeout < 0 || in.Timeout > 24*time.Hour || (in.Timeout > 0 && in.Timeout < time.Second) {
		return fmt.Errorf("%w: timeout must be between 1 second and 24 hours", ErrInvalidJob)
	}
	return nil
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := min(attempt-1, 20)
	d := time.Second * time.Duration(math.Pow(2, float64(shift)))
	return min(d, maxBackoff)
}
