package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const defaultLease = 30 * time.Second

type Handler func(context.Context, Job) error

type Worker struct {
	Queue        *Queue
	QueueName    string
	WorkerID     string
	Lease        time.Duration
	PollInterval time.Duration
	Handle       Handler
}

// Run polls and processes one job at a time. Multiple Worker instances can
// safely share a queue; each claim is fenced and its lease is renewed while
// the handler runs. Handler failures are retried with bounded exponential delay.
func (w Worker) Run(ctx context.Context) error {
	if w.Queue == nil || w.QueueName == "" || w.Handle == nil {
		return fmt.Errorf("%w: worker queue, name, and handler are required", ErrInvalidJob)
	}
	if w.Lease == 0 {
		w.Lease = defaultLease
	}
	if w.PollInterval == 0 {
		w.PollInterval = time.Second
	}
	if w.Lease < time.Second || w.PollInterval < 0 {
		return fmt.Errorf("%w: lease must be at least 1 second and poll interval non-negative", ErrInvalidJob)
	}
	if w.WorkerID == "" {
		id, err := workerID()
		if err != nil {
			return fmt.Errorf("create worker id: %w", err)
		}
		w.WorkerID = id
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		jobs, err := w.Queue.Claim(ctx, w.QueueName, w.WorkerID, w.Lease, 1)
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			timer := time.NewTimer(w.PollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
				continue
			}
		}
		if err := w.process(ctx, jobs[0]); err != nil && ctx.Err() == nil {
			return err
		}
	}
}

func (w Worker) process(ctx context.Context, job Job) error {
	handlerCtx, cancel := context.WithTimeout(ctx, job.Timeout)
	done := make(chan struct{})
	heartbeatErr := make(chan error, 1)
	go w.heartbeat(handlerCtx, job, done, cancel, heartbeatErr)
	err := w.Handle(handlerCtx, job)
	cancel()
	close(done)
	if heartbeat := <-heartbeatErr; heartbeat != nil {
		return heartbeat
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		return w.Queue.Complete(ctx, job)
	}
	retryCtx, retryCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer retryCancel()
	if retryErr := w.Queue.Retry(retryCtx, job, err); retryErr != nil {
		return errors.Join(fmt.Errorf("job handler: %w", err), retryErr)
	}
	return nil
}

func (w Worker) heartbeat(ctx context.Context, job Job, done <-chan struct{}, cancel context.CancelFunc, result chan<- error) {
	ticker := time.NewTicker(w.Lease / 3)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			result <- nil
			return
		case <-ctx.Done():
			result <- nil
			return
		case <-ticker.C:
			heartbeatCtx, heartbeatCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			err := w.Queue.Extend(heartbeatCtx, job, w.WorkerID, w.Lease)
			heartbeatCancel()
			if err != nil {
				cancel()
				result <- err
				return
			}
		}
	}
}

func workerID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(random[:]), nil
}
