package ranktracking

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

// Polling and fan-out limits for queued tasks.
const (
	taskGetConcurrency = 25  // concurrent task_get calls
	taskGetsPerRound   = 500 // task_get calls per round, to bound one round's fan-out
)

// DefaultPollWaits is the wait before each collect round. Standard-priority
// tasks take about five minutes on average, so the first look waits four;
// the total of 15 minutes is when a straggler falls back to the live endpoint.
var DefaultPollWaits = []time.Duration{4 * time.Minute, 2 * time.Minute, 2 * time.Minute, 2 * time.Minute, 2 * time.Minute, 3 * time.Minute}

// TaskStore is the storage of provider task ids; Store implements it.
type TaskStore interface {
	RecordTasks(ctx context.Context, runID string, tasks []PostedTask) error
	Tasks(ctx context.Context, runID string) ([]TaskRow, error)
	SetTaskState(ctx context.Context, runID, keywordID, device, state string) error
}

// executeQueued checks keywords through the provider's task queue: post every
// pair, poll until the tasks finish, and look up anything that was rejected,
// failed or timed out on the live endpoint, so a run never hangs on a stuck
// queue. Posted task ids are stored at once, so a retry collects them instead
// of posting, and paying, again.
func (c *Checks) executeQueued(ctx context.Context, e *execution) error {
	if c.Queued == nil || c.Tasks == nil {
		return c.fail(ctx, e.run, "Scheduled rank checks are not available")
	}
	done, err := c.Runs.CheckedPairs(ctx, e.run.ID)
	if err != nil {
		return err
	}
	stored, err := c.Tasks.Tasks(ctx, e.run.ID)
	if err != nil {
		return err
	}
	known := map[string]TaskRow{}
	for _, t := range stored {
		known[t.TrackingKeywordID+":"+t.Device] = t
	}
	byKey := map[string]pair{}
	var toPost []TaskInput
	var fallback []pair
	for _, p := range e.pairs() {
		key := p.keyword.ID + ":" + p.device
		byKey[key] = p
		if done[key] {
			continue
		}
		switch t, has := known[key]; {
		case !has:
			toPost = append(toPost, TaskInput{KeywordID: p.keyword.ID, Keyword: p.keyword.Keyword, Device: p.device})
		case t.State == TaskFailed:
			fallback = append(fallback, p)
		}
	}

	pending := map[string]string{} // pair key -> task id, tasks still to collect
	for _, t := range stored {
		if key := t.TrackingKeywordID + ":" + t.Device; t.State == TaskPosted && !done[key] {
			pending[key] = t.TaskID
		}
	}
	for chunk := range slices.Chunk(toPost, MaxTasksPerPost) {
		posted, err := c.Queued.PostTasks(ctx, e.org, PostRequest{
			Tasks: chunk, LocationCode: e.cfg.LocationCode, LanguageCode: e.cfg.LanguageCode,
			LocationName: e.cfg.LocationName, Depth: e.cfg.SerpDepth, TargetDomain: e.cfg.Domain,
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Nothing was posted, and a live lookup would be refused the same
			// way, so stop instead of paying for each pair.
			if errors.Is(err, dataforseo.ErrBillingIssue) || isInvalidField(err) {
				if err := c.Runs.SetRunErrorIfEmpty(ctx, e.run.ID, err.Error()); err != nil {
					return err
				}
				e.firstError = err.Error()
				return c.finalize(ctx, e.run, e.firstError)
			}
			// Earlier chunks are already charged, so their results still have to
			// be collected; this chunk goes to the live fallback.
			c.Logger.WarnContext(ctx, "rank check task_post failed", "run", e.run.ID, "err", err)
			for _, t := range chunk {
				fallback = append(fallback, byKey[t.KeywordID+":"+t.Device])
			}
			continue
		}
		if err := c.Tasks.RecordTasks(ctx, e.run.ID, posted); err != nil {
			return err
		}
		accepted := map[string]bool{}
		for _, t := range posted {
			key := t.KeywordID + ":" + t.Device
			accepted[key] = true
			pending[key] = t.TaskID
		}
		for _, t := range chunk {
			if key := t.KeywordID + ":" + t.Device; !accepted[key] {
				fallback = append(fallback, byKey[key])
			}
		}
	}

	for _, wait := range c.pollWaits() {
		if len(pending) == 0 {
			break
		}
		if err := c.sleep(ctx, wait); err != nil {
			return err
		}
		failed, err := c.collectRound(ctx, e, byKey, pending)
		if err != nil {
			return err
		}
		fallback = append(fallback, failed...)
	}

	// Whatever is still pending after the window goes live as well.
	for key := range pending {
		if err := c.Tasks.SetTaskState(ctx, e.run.ID, byKey[key].keyword.ID, byKey[key].device, TaskFailed); err != nil {
			return err
		}
		fallback = append(fallback, byKey[key])
	}
	if len(fallback) > 0 {
		var batches [][]pair
		for batch := range slices.Chunk(fallback, keywordsPerBatch) {
			batches = append(batches, batch)
		}
		c.Logger.InfoContext(ctx, "rank check live fallback", "run", e.run.ID, "pairs", len(fallback))
		if _, err := c.liveCheck(ctx, e, batches); err != nil {
			return err
		}
	}
	return c.finalize(ctx, e.run, e.firstError)
}

// collectRound reads up to taskGetsPerRound pending tasks. Completed ones are
// stored as snapshots and removed from pending; failed ones are returned for
// the live fallback; a transient read error leaves the task for the next round.
func (c *Checks) collectRound(ctx context.Context, e *execution, byKey map[string]pair, pending map[string]string) ([]pair, error) {
	keys := make([]string, 0, len(pending))
	for key := range pending {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	keys = keys[:min(len(keys), taskGetsPerRound)]

	type outcome struct {
		key string
		out TaskOutcome
		err error
	}
	outcomes := make([]outcome, len(keys))
	sem := make(chan struct{}, taskGetConcurrency)
	var wg sync.WaitGroup
	for i, key := range keys {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			outcomes[i].key = key
			outcomes[i].out, outcomes[i].err = c.Queued.CollectTask(ctx, pending[key], e.cfg.Domain)
		})
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var snaps []SnapshotInput
	var failed []pair
	var collected []pair
	for _, o := range outcomes {
		p := byKey[o.key]
		switch {
		case o.err != nil:
			c.Logger.WarnContext(ctx, "rank check task_get failed", "run", e.run.ID, "keyword", p.keyword.Keyword, "err", o.err)
		case o.out.Pending:
		case o.out.Failed != "":
			c.Logger.WarnContext(ctx, "rank check task failed", "run", e.run.ID, "keyword", p.keyword.Keyword, "err", o.out.Failed)
			if e.firstError == "" {
				e.firstError = o.out.Failed
				if err := c.Runs.SetRunErrorIfEmpty(ctx, e.run.ID, e.firstError); err != nil {
					return nil, err
				}
			}
			if err := c.Tasks.SetTaskState(ctx, e.run.ID, p.keyword.ID, p.device, TaskFailed); err != nil {
				return nil, err
			}
			delete(pending, o.key)
			failed = append(failed, p)
		default:
			snaps = append(snaps, snapshotOf(e.run.ID, pairResult{pair: p, result: o.out.Result}))
			collected = append(collected, p)
			delete(pending, o.key)
		}
	}
	if err := c.Runs.InsertSnapshots(ctx, snaps); err != nil {
		return nil, err
	}
	for _, p := range collected {
		if err := c.Tasks.SetTaskState(ctx, e.run.ID, p.keyword.ID, p.device, TaskCollected); err != nil {
			return nil, err
		}
	}
	return failed, nil
}

func (c *Checks) pollWaits() []time.Duration {
	if c.PollWaits != nil {
		return c.PollWaits
	}
	return DefaultPollWaits
}

func (c *Checks) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for queued rank checks: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
