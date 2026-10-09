package ranktracking

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Skip reasons written to a config's last_skip_reason.
const (
	SkipPlanRequired = "plan_required"
	SkipNoKeywords   = "no_keywords"
)

// Tick limits.
const (
	// DefaultUnitBudget caps the work one tick starts, in tasks (keywords times
	// devices). The first start of a tick is always admitted, so a config bigger
	// than the budget (at most 2,000 tasks) can never starve. It is sized against
	// the provider's request cap, where polling is the binding term.
	DefaultUnitBudget = 1000
	// DefaultTickDeadline stops the loop early; unprocessed configs stay due and
	// the next tick resumes oldest-first.
	DefaultTickDeadline = 3 * time.Minute
	dueConfigsPerTick   = 500
)

// DueStore is the storage the scheduler needs; Store implements it.
type DueStore interface {
	DueConfigs(ctx context.Context, now time.Time, limit int) ([]DueConfig, error)
	KeywordCounts(ctx context.Context, configIDs []string) (map[string]int, error)
	ClaimDueConfig(ctx context.Context, in ClaimInput) (bool, error)
}

// ScheduledStarter queues a scheduled run; *Checks implements it.
type ScheduledStarter interface {
	StartScheduled(ctx context.Context, organizationID string, cfg Config, keywords int) (string, error)
}

// Ticker starts a check for every config that is due. Run Tick on a timer, for
// example every five minutes; it is safe to run on several instances at once
// because each config is claimed with a compare-and-set on its anchor.
type Ticker struct {
	Store      DueStore
	Starter    ScheduledStarter
	Plans      PaidPlans // nil treats every config as paid, like a self-hosted install
	Schedule   Scheduler
	Logger     *slog.Logger
	UnitBudget int           // 0 means DefaultUnitBudget
	Deadline   time.Duration // 0 means DefaultTickDeadline
}

// TickSummary counts what one tick did.
type TickSummary struct {
	Candidates        int
	Started           int
	UnitsStarted      int
	StoppedByBudget   bool
	StoppedByDeadline bool
	SkippedFree       int
	SkippedNoKeywords int
	ConcurrentSkips   int
	AlreadyRunning    int
	PlanCheckErrors   int
	StartErrors       int
	ConfigErrors      int
}

// Tick starts the due checks. The anchor is advanced before the run starts, so
// a slow or failing start cannot make the config due again on the next tick
// and bill twice (a retry storm). It is the anchor, not the run, that decides
// when a check is owed.
func (t *Ticker) Tick(ctx context.Context) (TickSummary, error) {
	var sum TickSummary
	now := t.Schedule.Now()
	due, err := t.Store.DueConfigs(ctx, now, dueConfigsPerTick)
	if err != nil {
		return sum, err
	}
	sum.Candidates = len(due)
	ids := make([]string, len(due))
	for i, d := range due {
		ids[i] = d.ID
	}
	counts, err := t.Store.KeywordCounts(ctx, ids)
	if err != nil {
		return sum, err
	}
	budget, deadline := t.UnitBudget, t.Deadline
	if budget == 0 {
		budget = DefaultUnitBudget
	}
	if deadline == 0 {
		deadline = DefaultTickDeadline
	}
	stopAt := now.Add(deadline)
	type planResult struct {
		paid bool
		err  error
	}
	plans := map[string]planResult{}
	checkPlan := func(org string) (bool, error) {
		if t.Plans == nil {
			return true, nil
		}
		// One lookup per organization per tick. A failure stays cached within
		// the tick: that organization's configs simply stay due.
		result, ok := plans[org]
		if !ok {
			result.paid, result.err = t.Plans.HasPaidPlan(ctx, org)
			plans[org] = result
		}
		return result.paid, result.err
	}

	for _, d := range due {
		if err := ctx.Err(); err != nil {
			return sum, err
		}
		if !t.Schedule.Now().Before(stopAt) {
			sum.StoppedByDeadline = true
			break
		}
		// One bad row or a transient error must not starve the rest of the tick.
		if stop := t.process(ctx, d, counts[d.ID], budget, checkPlan, &sum); stop {
			break
		}
	}
	level := slog.LevelInfo
	if sum.PlanCheckErrors+sum.StartErrors+sum.ConfigErrors > 0 {
		level = slog.LevelError
	}
	t.Logger.Log(ctx, level, "rank_tracking_scheduler_summary", "summary", fmt.Sprintf("%+v", sum))
	return sum, nil
}

// process handles one due config and reports whether the tick should stop.
func (t *Ticker) process(ctx context.Context, d DueConfig, keywords, budget int, checkPlan func(string) (bool, error), sum *TickSummary) (stop bool) {
	if d.NextCheckAt == nil || d.ScheduleInterval == Manual {
		return false // the query excludes both; narrow rather than assert
	}
	units := keywords * DevicesCount(d.Devices)
	// Admit only what fits the budget; the first start is exempt and zero-unit
	// rows (no keywords) always advance.
	if sum.Started > 0 && sum.UnitsStarted+units > budget {
		sum.StoppedByBudget = true
		return true
	}
	observed := *d.NextCheckAt
	next := t.Schedule.Next(d.ScheduleInterval, &observed, nil)
	claim := ClaimInput{ConfigID: d.ID, ProjectID: d.ProjectID, Observed: observed, Next: next}
	fail := func(err error, counter *int, msg string) {
		*counter++
		t.Logger.ErrorContext(ctx, msg, "config", d.ID, "domain", d.Domain, "err", err)
	}
	skip := func(reason *string, counter *int) {
		claim.SetSkipReason, claim.SkipReason = true, reason
		claimed, err := t.Store.ClaimDueConfig(ctx, claim)
		switch {
		case err != nil:
			fail(err, &sum.ConfigErrors, "claim skipped rank check")
		case claimed:
			*counter++
		default:
			sum.ConcurrentSkips++
		}
	}

	if keywords == 0 {
		skip(strPtr(SkipNoKeywords), &sum.SkippedNoKeywords)
		return false
	}
	hasPlan, err := checkPlan(d.OrganizationID)
	if err != nil {
		// Never move the anchor on an error: it is the schedule, and an error
		// write would shift this config's slot for good. Leaving it due is the retry.
		fail(err, &sum.PlanCheckErrors, "check plan for scheduled rank check")
		return false
	}
	if !hasPlan {
		skip(strPtr(SkipPlanRequired), &sum.SkippedFree)
		return false
	}

	// Claim the slot before starting. Clearing the skip reason here lets an
	// upgraded organization drop its "plan_required" badge.
	claim.SetSkipReason, claim.SkipReason = true, nil
	claimed, err := t.Store.ClaimDueConfig(ctx, claim)
	if err != nil {
		fail(err, &sum.ConfigErrors, "claim scheduled rank check")
		return false
	}
	if !claimed {
		sum.ConcurrentSkips++
		return false
	}
	_, err = t.Starter.StartScheduled(ctx, d.OrganizationID, d.Config, keywords)
	switch {
	case err == nil:
		sum.UnitsStarted += units
		sum.Started++
	case errors.Is(err, ErrRunActive):
		sum.AlreadyRunning++
		// Nothing started, so hand the slot back and retry once the blocking run
		// clears. A manual edit landing in between wins the compare-and-set.
		restore := ClaimInput{ConfigID: d.ID, ProjectID: d.ProjectID, Observed: next, Next: observed}
		if ok, err := t.Store.ClaimDueConfig(ctx, restore); err != nil || !ok {
			t.Logger.InfoContext(ctx, "could not restore rank check schedule", "config", d.ID, "err", err)
		}
	default:
		// Leave the anchor advanced: a systemic outage must not make hundreds of
		// configs due again on the next tick.
		fail(err, &sum.StartErrors, "start scheduled rank check")
	}
	return false
}

func strPtr(s string) *string { return &s }
