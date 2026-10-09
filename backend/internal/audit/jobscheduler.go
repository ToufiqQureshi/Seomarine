package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/jobs"
)

// AuditQueueName is the jobs queue audits run on.
const AuditQueueName = "site_audit"

// auditJobTimeout bounds one audit job, including Lighthouse.
const auditJobTimeout = 45 * time.Minute

// QueueScheduler schedules audits on the platform jobs queue. The queue is
// at-least-once with a fenced claim, so a crashed worker's job is retried and
// the audit's deterministic row ids make the retry safe.
type QueueScheduler struct {
	queue *jobs.Queue
}

// NewQueueScheduler returns a scheduler backed by a jobs queue.
func NewQueueScheduler(queue *jobs.Queue) *QueueScheduler {
	return &QueueScheduler{queue: queue}
}

// EnqueueAudit inserts the audit's job. The audit id is the idempotency key, so
// a retried start does not schedule the audit twice.
func (s *QueueScheduler) EnqueueAudit(ctx context.Context, auditID string, payload AuditJobPayload) error {
	if s == nil || s.queue == nil {
		return fmt.Errorf("audit scheduler is not configured")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode audit job: %w", err)
	}
	if _, err := s.queue.Enqueue(ctx, jobs.EnqueueInput{
		Queue: AuditQueueName, IdempotencyKey: auditID, Payload: encoded,
		MaxAttempts: 3, Timeout: auditJobTimeout,
	}); err != nil {
		return fmt.Errorf("enqueue audit job: %w", err)
	}
	return nil
}

// TerminateAudit is a no-op: the platform queue is at-least-once and has no
// cancel signal. Deleting a running audit removes its row, so the in-flight
// runner's final write fails with ErrAuditNotFound and the crawl stops. A
// cooperative cancel flag is a deliberate gap (see the README's LEFT OUT).
func (s *QueueScheduler) TerminateAudit(context.Context, string) error { return nil }

// RunnerHandler adapts a Runner to a jobs.Handler.
func RunnerHandler(runner *Runner) jobs.Handler {
	return func(ctx context.Context, job jobs.Job) error {
		var payload AuditJobPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("decode audit job %d: %w", job.ID, err)
		}
		if payload.AuditID == "" {
			return fmt.Errorf("audit job %d has no audit id", job.ID)
		}
		return runner.Run(ctx, payload)
	}
}
