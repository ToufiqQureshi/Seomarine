package ranktracking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/jobs"
)

// QueueName is the jobs queue that carries rank check runs.
const QueueName = "rank_check"

const (
	keywordsPerBatch = 10 // keywords checked together, as in the legacy workflow
	startupGrace     = time.Minute
	checkJobTimeout  = 30 * time.Minute
)

// Errors a caller can react to.
var (
	ErrChecksUnavailable = errors.New("rank checks are not available")
	ErrPaymentRequired   = errors.New("a paid plan is required to run rank checks")
)

// ActiveRunError says which run blocks a new one.
type ActiveRunError struct{ RunID string }

func (e *ActiveRunError) Error() string { return ErrRunActive.Error() }
func (e *ActiveRunError) Unwrap() error { return ErrRunActive }

// RunJob is the payload of a rank check job.
type RunJob struct {
	RunID          string   `json:"runId"`
	OrganizationID string   `json:"organizationId"`
	KeywordIDs     []string `json:"keywordIds,omitempty"`
	// Trigger is TriggerScheduled for cron runs, which use the cheaper queued
	// SERP path; empty or TriggerManual runs use the live endpoint.
	Trigger string `json:"trigger,omitempty"`
}

// Triggers of a run.
const (
	TriggerManual    = "manual"
	TriggerScheduled = "scheduled"
)

// RunStore is the write side of runs and snapshots; Store implements it.
type RunStore interface {
	InsertRun(ctx context.Context, r Run) error
	GetRun(ctx context.Context, id string) (*Run, error)
	MarkRunning(ctx context.Context, id string, total int) (bool, error)
	SetRunErrorIfEmpty(ctx context.Context, id, message string) error
	InsertSnapshots(ctx context.Context, snaps []SnapshotInput) error
	CheckedPairs(ctx context.Context, runID string) (map[string]bool, error)
	CountCheckedKeywords(ctx context.Context, runID string) (int, error)
	FinishRun(ctx context.Context, run Run, status string, checked int, message *string) (bool, error)
	FailRunIfActive(ctx context.Context, id, reason string) error
	JobState(ctx context.Context, queue, runID string) (string, error)
}

// RunScheduler hands a run to a worker.
type RunScheduler interface {
	EnqueueRun(ctx context.Context, job RunJob) error
}

// PaidPlans says whether an organization may spend provider credits.
type PaidPlans interface {
	HasPaidPlan(ctx context.Context, organizationID string) (bool, error)
}

// Checks starts rank checks and runs them.
type Checks struct {
	Repo      Repository
	Results   ResultsRepository
	Runs      RunStore
	Serp      SerpProvider
	Queued    QueuedSerp // nil refuses scheduled runs
	Tasks     TaskStore
	Scheduler RunScheduler
	Plans     PaidPlans // nil runs ungated, like a self-hosted deployment
	Logger    *slog.Logger
	Now       func() time.Time
	// PollWaits and Sleep drive the queued path's polling; tests shorten them.
	PollWaits []time.Duration
	Sleep     func(ctx context.Context, d time.Duration) error
}

// StartInput asks for a manual check. KeywordIDs limits it to some keywords.
type StartInput struct {
	OrganizationID string
	ProjectID      string
	ConfigID       string
	KeywordIDs     []string
	MaxCostCredits *int
}

// Start creates a run for the config and queues it. A second start while one
// is active returns *ActiveRunError, unless the active run is stale.
func (c *Checks) Start(ctx context.Context, in StartInput) (string, error) {
	if c.Serp == nil || c.Scheduler == nil {
		return "", ErrChecksUnavailable
	}
	cfg, err := c.Repo.GetConfig(ctx, in.ProjectID, in.ConfigID)
	if err != nil {
		return "", err
	}
	if c.Plans != nil {
		paid, err := c.Plans.HasPaidPlan(ctx, in.OrganizationID)
		if err != nil {
			return "", fmt.Errorf("check plan: %w", err)
		}
		if !paid {
			return "", ErrPaymentRequired
		}
	}
	keywords, err := c.Repo.Keywords(ctx, cfg.ID)
	if err != nil {
		return "", err
	}
	selected := selectKeywords(keywords, in.KeywordIDs)
	if len(selected) == 0 {
		return "", ValidationError("No keywords to track. Add keywords to this domain first.")
	}
	if in.MaxCostCredits != nil {
		texts := make([]string, len(selected))
		for i, k := range selected {
			texts[i] = k.Keyword
		}
		if est := EstimateCheck(texts, cfg.Devices, cfg.SerpDepth, Live); est.CostCredits > *in.MaxCostCredits {
			return "", ValidationError(fmt.Sprintf("The current rank check costs %d credits, above the approved maximum of %d. Estimate the cost again and ask the user to approve the updated amount.", est.CostCredits, *in.MaxCostCredits))
		}
	}

	return c.queueRun(ctx, queueSpec{
		OrganizationID: in.OrganizationID, ProjectID: in.ProjectID, ConfigID: cfg.ID,
		KeywordsTotal: len(selected), KeywordIDs: in.KeywordIDs, Trigger: TriggerManual,
	})
}

// queueSpec describes a run to create and queue.
type queueSpec struct {
	OrganizationID string
	ProjectID      string
	ConfigID       string
	KeywordsTotal  int
	KeywordIDs     []string
	Trigger        string
}

// StartScheduled queues a scheduled run for a due config. The scheduler has
// already checked the plan and counted the keywords.
func (c *Checks) StartScheduled(ctx context.Context, organizationID string, cfg Config, keywords int) (string, error) {
	if c.Serp == nil || c.Queued == nil || c.Tasks == nil || c.Scheduler == nil {
		return "", ErrChecksUnavailable
	}
	return c.queueRun(ctx, queueSpec{
		OrganizationID: organizationID, ProjectID: cfg.ProjectID, ConfigID: cfg.ID,
		KeywordsTotal: keywords, Trigger: TriggerScheduled,
	})
}

// queueRun creates a pending run and queues its job. A second run for the same
// config returns *ActiveRunError, unless the active one is stale.
func (c *Checks) queueRun(ctx context.Context, spec queueSpec) (string, error) {
	// Two attempts: once normally, once after clearing a stale blocker.
	for attempt := range 2 {
		id, err := ids.New()
		if err != nil {
			return "", err
		}
		run := Run{ID: id, ConfigID: spec.ConfigID, ProjectID: spec.ProjectID, KeywordsTotal: spec.KeywordsTotal, IsSubsetRun: len(spec.KeywordIDs) > 0}
		err = c.Runs.InsertRun(ctx, run)
		if err == nil {
			job := RunJob{RunID: id, OrganizationID: spec.OrganizationID, KeywordIDs: spec.KeywordIDs, Trigger: spec.Trigger}
			if err := c.Scheduler.EnqueueRun(ctx, job); err != nil {
				// Free the slot, or the config would stay blocked until stale cleanup.
				if failErr := c.Runs.FailRunIfActive(context.WithoutCancel(ctx), id, "Failed to start rank check"); failErr != nil {
					c.Logger.ErrorContext(ctx, "release rank check slot", "err", failErr)
				}
				return "", err
			}
			return id, nil
		}
		if !errors.Is(err, ErrRunActive) {
			return "", err
		}
		blocker, err := c.Results.ActiveRun(ctx, spec.ConfigID)
		if err != nil {
			return "", err
		}
		if blocker == nil {
			continue // its status flipped between insert and select
		}
		if attempt == 0 {
			if reason, err := c.staleReason(ctx, *blocker); err != nil {
				return "", err
			} else if reason != "" {
				if err := c.Runs.FailRunIfActive(ctx, blocker.ID, reason); err != nil {
					return "", err
				}
				continue
			}
		}
		return "", &ActiveRunError{RunID: blocker.ID}
	}
	blocker, err := c.Results.ActiveRun(ctx, spec.ConfigID)
	if err != nil {
		return "", err
	}
	if blocker == nil {
		return "", ErrRunActive
	}
	return "", &ActiveRunError{RunID: blocker.ID}
}

// staleReason says why an active run is dead, or "" while it may still be
// live. A run is dead when its job ended without finishing it, or when it has
// no job a minute after it started.
func (c *Checks) staleReason(ctx context.Context, run Run) (string, error) {
	state, err := c.Runs.JobState(ctx, QueueName, run.ID)
	if err != nil {
		return "", err
	}
	switch state {
	case "queued", "running":
		return "", nil
	case "":
		if c.Now().Sub(run.StartedAt) < startupGrace {
			return "", nil
		}
		return "The job for this run was not found", nil
	default:
		return "The job ended without finishing the run (" + state + ")", nil
	}
}

func selectKeywords(all []Keyword, ids []string) []Keyword {
	if len(ids) == 0 {
		return all
	}
	var out []Keyword
	for _, k := range all {
		if slices.Contains(ids, k.ID) {
			out = append(out, k)
		}
	}
	return out
}

type pair struct {
	keyword Keyword
	device  string
}

type pairResult struct {
	pair
	result CheckResult
	err    error
}

// execution is what a worker needs to run one run.
type execution struct {
	run        Run
	cfg        Config
	keywords   []Keyword
	org        string
	devices    []string
	firstError string
}

func (e *execution) pairs() []pair {
	var out []pair
	for _, k := range e.keywords {
		for _, d := range e.devices {
			out = append(out, pair{k, d})
		}
	}
	return out
}

// Execute runs one queued check. It is safe to run twice for the same run: a
// keyword and device that already has a snapshot is not asked again, so a
// retry after a crash does not bill the same lookup twice.
func (c *Checks) Execute(ctx context.Context, job RunJob) error {
	e, err := c.begin(ctx, job)
	if err != nil || e == nil {
		return err
	}
	if job.Trigger == TriggerScheduled {
		return c.executeQueued(ctx, e)
	}
	return c.executeLive(ctx, e)
}

// begin loads the run and marks it running. It returns nil when there is
// nothing to do: the run is gone, finished, or was failed by a guard.
func (c *Checks) begin(ctx context.Context, job RunJob) (*execution, error) {
	run, err := c.Runs.GetRun(ctx, job.RunID)
	if err != nil {
		return nil, err
	}
	if run == nil || (run.Status != "pending" && run.Status != "running") {
		return nil, nil // superseded or already finished
	}
	cfg, err := c.Repo.GetConfig(ctx, run.ProjectID, run.ConfigID)
	if errors.Is(err, ErrNotFound) {
		return nil, c.fail(ctx, *run, "The rank tracking config no longer exists")
	}
	if err != nil {
		return nil, err
	}
	all, err := c.Repo.Keywords(ctx, cfg.ID)
	if err != nil {
		return nil, err
	}
	keywords := selectKeywords(all, job.KeywordIDs)
	if !cfg.IsActive {
		return nil, c.fail(ctx, *run, "The rank tracking config is archived")
	}
	if len(keywords) == 0 {
		return nil, c.fail(ctx, *run, "No keywords to track")
	}
	if running, err := c.Runs.MarkRunning(ctx, run.ID, len(keywords)); err != nil || !running {
		return nil, err
	}
	run.KeywordsTotal = len(keywords)
	devices := []string{"desktop", "mobile"}
	if cfg.Devices != "both" {
		devices = []string{cfg.Devices}
	}
	return &execution{run: *run, cfg: cfg, keywords: keywords, org: job.OrganizationID, devices: devices}, nil
}

// executeLive checks keywords batch by batch on the live endpoint.
func (c *Checks) executeLive(ctx context.Context, e *execution) error {
	done, err := c.Runs.CheckedPairs(ctx, e.run.ID)
	if err != nil {
		return err
	}
	var batches [][]pair
	for batch := range slices.Chunk(e.keywords, keywordsPerBatch) {
		var pairs []pair
		for _, k := range batch {
			for _, d := range e.devices {
				if !done[k.ID+":"+d] {
					pairs = append(pairs, pair{k, d})
				}
			}
		}
		batches = append(batches, pairs)
	}
	if _, err := c.liveCheck(ctx, e, batches); err != nil {
		return err
	}
	return c.finalize(ctx, e.run, e.firstError)
}

// liveCheck runs each batch's lookups concurrently and stores the snapshots.
// It reports stop when an error that repeats for every keyword (billing,
// invalid field) ended the run early, because a failed task is still billed.
func (c *Checks) liveCheck(ctx context.Context, e *execution, batches [][]pair) (stop bool, err error) {
	for _, batch := range batches {
		results := c.lookupAll(ctx, e, batch)
		if err := ctx.Err(); err != nil {
			return false, err // the queue retries the job; finished pairs are skipped
		}
		var snaps []SnapshotInput
		for _, r := range results {
			if r.err != nil {
				if err := c.recordPairError(ctx, e, r); err != nil {
					return false, err
				}
				if errors.Is(r.err, dataforseo.ErrBillingIssue) || isInvalidField(r.err) {
					stop = true
				}
				continue
			}
			snaps = append(snaps, snapshotOf(e.run.ID, r))
		}
		if err := c.Runs.InsertSnapshots(ctx, snaps); err != nil {
			return false, err
		}
		if stop {
			return true, nil
		}
	}
	return false, nil
}

func snapshotOf(runID string, r pairResult) SnapshotInput {
	return SnapshotInput{
		RunID: runID, TrackingKeywordID: r.keyword.ID, Keyword: r.keyword.Keyword, Device: r.device,
		Position: r.result.Position, URL: r.result.URL, SerpFeatures: r.result.SerpFeatures,
	}
}

// recordPairError keeps the first reason on the run, so it shows the provider's own message.
func (c *Checks) recordPairError(ctx context.Context, e *execution, r pairResult) error {
	if e.firstError == "" {
		e.firstError = r.err.Error()
		if err := c.Runs.SetRunErrorIfEmpty(ctx, e.run.ID, e.firstError); err != nil {
			return err
		}
	}
	level := slog.LevelError
	if errors.Is(r.err, dataforseo.ErrUpstreamUnavailable) {
		level = slog.LevelWarn // a provider flake: the keyword just misses this run
	}
	c.Logger.Log(ctx, level, "rank check lookup failed", "run", e.run.ID, "keyword", r.keyword.Keyword, "device", r.device, "err", r.err)
	return nil
}

func (c *Checks) lookupAll(ctx context.Context, e *execution, pairs []pair) []pairResult {
	results := make([]pairResult, len(pairs))
	var wg sync.WaitGroup
	for i, p := range pairs {
		results[i].pair = p
		wg.Go(func() {
			results[i].result, results[i].err = c.Serp.CheckLive(ctx, e.org, CheckRequest{
				Keyword: p.keyword.Keyword, Device: p.device, LocationCode: e.cfg.LocationCode,
				LanguageCode: e.cfg.LanguageCode, LocationName: e.cfg.LocationName, TargetDomain: e.cfg.Domain, Depth: e.cfg.SerpDepth,
			})
		})
	}
	wg.Wait()
	return results
}

func isInvalidField(err error) bool {
	taskErr, ok := errors.AsType[*dataforseo.TaskError](err)
	return ok && taskErr.InvalidField
}

func (c *Checks) fail(ctx context.Context, run Run, reason string) error {
	return c.Runs.FailRunIfActive(ctx, run.ID, reason)
}

// finalize decides the run's outcome from the snapshots actually stored.
func (c *Checks) finalize(ctx context.Context, run Run, batchError string) error {
	checked, err := c.Runs.CountCheckedKeywords(ctx, run.ID)
	if err != nil {
		return err
	}
	current, err := c.Runs.GetRun(ctx, run.ID)
	if err != nil {
		return err
	}
	if current == nil || (current.Status != "pending" && current.Status != "running") {
		return nil // stale cleanup got there first; do not overwrite its decision
	}
	reason := batchError
	if current.ErrorMessage != nil {
		reason = *current.ErrorMessage
	}
	status, message := "completed", (*string)(nil)
	switch {
	case checked == 0 && run.KeywordsTotal > 0:
		status = "failed"
		text := reason
		if text == "" {
			text = "No keywords could be checked."
		}
		message = &text
	case checked < run.KeywordsTotal:
		text := fmt.Sprintf("Checked %d of %d keyword(s)", checked, run.KeywordsTotal)
		if reason != "" {
			text += ": " + reason
		}
		message = &text
	}
	_, err = c.Runs.FinishRun(ctx, run, status, checked, message)
	return err
}

// QueueScheduler enqueues runs on the platform job queue.
type QueueScheduler struct{ Queue *jobs.Queue }

// EnqueueRun inserts the run's job. The run id is the idempotency key, so a
// retried start does not schedule the run twice.
func (s QueueScheduler) EnqueueRun(ctx context.Context, job RunJob) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode rank check job: %w", err)
	}
	if _, err := s.Queue.Enqueue(ctx, jobs.EnqueueInput{
		Queue: QueueName, IdempotencyKey: job.RunID, Payload: payload, MaxAttempts: 3, Timeout: checkJobTimeout,
	}); err != nil {
		return fmt.Errorf("enqueue rank check job: %w", err)
	}
	return nil
}

// JobHandler adapts Checks to the job worker.
func (c *Checks) JobHandler() jobs.Handler {
	return func(ctx context.Context, job jobs.Job) error {
		var payload RunJob
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("decode rank check job %d: %w", job.ID, err)
		}
		if payload.RunID == "" {
			return fmt.Errorf("rank check job %d has no run id", job.ID)
		}
		return c.Execute(ctx, payload)
	}
}
