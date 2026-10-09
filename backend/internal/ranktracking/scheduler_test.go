package ranktracking

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type tickFixture struct {
	checkFixture
	queued *fakeQueued
	org    string
	ticker *Ticker
}

// newTickFixture makes a weekly config with two keywords that fell due an hour ago.
func newTickFixture(t *testing.T) tickFixture {
	t.Helper()
	f := newCheckFixture(t, "one", "two")
	f.checks.Queued, f.checks.Tasks = &fakeQueued{}, f.s.Repo.(Store)
	ctx := context.Background()
	pool := f.s.Repo.(Store).DB
	org := "org-tick-" + f.project
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Tick', $1, now())`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, organization_id, name, location_code, language_code) VALUES ($1, $2, 'P', 2840, 'en')`, f.project, org); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c := context.WithoutCancel(ctx)
		_, _ = pool.Exec(c, `DELETE FROM go_rank_tracking_configs WHERE project_id = $1`, f.project)
		_, _ = pool.Exec(c, `DELETE FROM projects WHERE id = $1`, f.project)
		_, _ = pool.Exec(c, `DELETE FROM organization WHERE id = $1`, org)
	})
	weekly := Weekly
	if _, err := f.s.UpdateConfig(ctx, f.project, f.cfg.ID, UpdateInput{ScheduleInterval: &weekly}); err != nil {
		t.Fatal(err)
	}
	f.setDue(t, testNow.Add(-time.Hour))
	store := f.s.Repo.(Store)
	ticker := &Ticker{
		Store: store, Starter: f.checks, Schedule: Scheduler{Now: func() time.Time { return testNow }, Rand: func(int) int { return 0 }},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return tickFixture{checkFixture: f, queued: f.checks.Queued.(*fakeQueued), org: org, ticker: ticker}
}

func (f checkFixture) setDue(t *testing.T, at time.Time) {
	t.Helper()
	if _, err := f.s.Repo.(Store).DB.Exec(context.Background(), `UPDATE go_rank_tracking_configs SET next_check_at = $2 WHERE id = $1`, f.cfg.ID, at); err != nil {
		t.Fatal(err)
	}
}

func (f tickFixture) tick(t *testing.T) TickSummary {
	t.Helper()
	sum, err := f.ticker.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return sum
}

func TestTickStartsDueConfigAndAdvancesTheAnchor(t *testing.T) {
	f := newTickFixture(t)
	reason := SkipPlanRequired
	if err := f.s.Repo.UpdateConfig(context.Background(), f.project, f.cfg.ID, ConfigUpdate{SetSkipReason: true, LastSkipReason: &reason}); err != nil {
		t.Fatal(err)
	}
	due := *f.config(t).NextCheckAt

	sum := f.tick(t)
	if sum.Candidates != 1 || sum.Started != 1 || sum.UnitsStarted != 4 {
		t.Fatalf("summary = %+v, want 1 candidate started with 4 task units", sum)
	}
	if len(f.sched.jobs) != 1 || f.sched.jobs[0].Trigger != TriggerScheduled || f.sched.jobs[0].OrganizationID != f.org {
		t.Fatalf("jobs = %+v, want one scheduled job for the project's organization", f.sched.jobs)
	}
	run, _ := f.checks.Results.LatestRun(context.Background(), f.cfg.ID)
	if run == nil || run.Status != "pending" || run.KeywordsTotal != 2 {
		t.Errorf("run = %+v", run)
	}
	cfg := f.config(t)
	// The anchor moves by whole intervals from the one that was due, so a late
	// tick does not shift the schedule.
	if want := due.Add(7 * 24 * time.Hour); !cfg.NextCheckAt.Equal(want) {
		t.Errorf("next check = %v, want %v", cfg.NextCheckAt, want)
	}
	if cfg.LastSkipReason != nil {
		t.Errorf("skip reason = %q, want it cleared once the check starts", *cfg.LastSkipReason)
	}
	if again := f.tick(t); again.Candidates != 0 {
		t.Errorf("second tick = %+v, want nothing due", again)
	}
}

func TestTickIgnoresConfigsThatAreNotDue(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(tickFixture){
		"in the future": func(f tickFixture) { f.setDue(t, testNow.Add(time.Hour)) },
		"manual": func(f tickFixture) {
			_, _ = f.s.Repo.(Store).DB.Exec(ctx, `UPDATE go_rank_tracking_configs SET schedule_interval = 'manual', next_check_at = NULL WHERE id = $1`, f.cfg.ID)
		},
		"archived config": func(f tickFixture) {
			_, _ = f.s.Repo.(Store).DB.Exec(ctx, `UPDATE go_rank_tracking_configs SET is_active = false WHERE id = $1`, f.cfg.ID)
		},
		"archived project": func(f tickFixture) {
			_, _ = f.s.Repo.(Store).DB.Exec(ctx, `UPDATE projects SET archived_at = now() WHERE id = $1`, f.project)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newTickFixture(t)
			mutate(f)
			if sum := f.tick(t); sum.Candidates != 0 || len(f.sched.jobs) != 0 {
				t.Errorf("summary = %+v, jobs %d; want nothing started", sum, len(f.sched.jobs))
			}
		})
	}
}

func TestTickSkipsWithAReason(t *testing.T) {
	ctx := context.Background()
	t.Run("no keywords", func(t *testing.T) {
		//nolint:contextcheck // A fixture owns its isolated test database context.
		f := newTickFixture(t)
		ids := []string{f.kw["one"], f.kw["two"]}
		if _, err := f.s.RemoveKeywords(ctx, f.project, f.cfg.ID, ids); err != nil {
			t.Fatal(err)
		}
		//nolint:contextcheck // Fixture reads use their own context.
		due := *f.config(t).NextCheckAt
		//nolint:contextcheck // Fixture ticker owns its context.
		sum := f.tick(t)
		//nolint:contextcheck // Fixture reads use their own context.
		cfg := f.config(t)
		if sum.SkippedNoKeywords != 1 || len(f.sched.jobs) != 0 || cfg.LastSkipReason == nil || *cfg.LastSkipReason != SkipNoKeywords || !cfg.NextCheckAt.After(due) {
			t.Errorf("summary %+v, reason %v, next %v; want a no_keywords skip that still advances", sum, cfg.LastSkipReason, cfg.NextCheckAt)
		}
	})
	t.Run("free plan", func(t *testing.T) {
		f := newTickFixture(t)
		f.ticker.Plans = fakePlans{paid: false}
		due := *f.config(t).NextCheckAt
		sum := f.tick(t)
		cfg := f.config(t)
		if sum.SkippedFree != 1 || len(f.sched.jobs) != 0 || cfg.LastSkipReason == nil || *cfg.LastSkipReason != SkipPlanRequired || !cfg.NextCheckAt.After(due) {
			t.Errorf("summary %+v, reason %v; want a plan_required skip that advances", sum, cfg.LastSkipReason)
		}
		// After an upgrade the next due check starts and the badge goes away.
		f.ticker.Plans = fakePlans{paid: true}
		f.setDue(t, testNow.Add(-time.Minute))
		if sum := f.tick(t); sum.Started != 1 || f.config(t).LastSkipReason != nil {
			t.Errorf("after upgrade: summary %+v, reason %v", sum, f.config(t).LastSkipReason)
		}
	})
}

type erroringPlans struct{}

func (erroringPlans) HasPaidPlan(context.Context, string) (bool, error) {
	return false, errors.New("billing is down")
}

func TestTickLeavesTheAnchorAloneWhenThePlanCheckFails(t *testing.T) {
	f := newTickFixture(t)
	f.ticker.Plans = erroringPlans{}
	due := *f.config(t).NextCheckAt
	sum := f.tick(t)
	if sum.PlanCheckErrors != 1 || len(f.sched.jobs) != 0 || !f.config(t).NextCheckAt.Equal(due) {
		t.Errorf("summary %+v, next %v; want the config left due for the next tick", sum, f.config(t).NextCheckAt)
	}
}

func TestTickCachesThePlanCheckPerOrganization(t *testing.T) {
	f := newTickFixture(t)
	ctx := context.Background()
	second := create(t, f.s, f.project, func(in *CreateInput) { in.Domain = "second.example.com" })
	if _, err := f.s.AddKeywords(ctx, f.project, second.ID, []string{"x"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Repo.(Store).DB.Exec(ctx, `UPDATE go_rank_tracking_configs SET next_check_at = $2, schedule_interval = 'weekly' WHERE id = $1`, second.ID, testNow.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	counting := &countingPlans{}
	f.ticker.Plans = counting
	if sum := f.tick(t); sum.Started != 2 {
		t.Fatalf("summary = %+v, want both started", sum)
	}
	if counting.calls != 1 {
		t.Errorf("plan lookups = %d, want 1 for one organization", counting.calls)
	}
}

type countingPlans struct{ calls int }

func (c *countingPlans) HasPaidPlan(context.Context, string) (bool, error) {
	c.calls++
	return true, nil
}

func TestTickHandsTheSlotBackWhenARunIsActive(t *testing.T) {
	f := newTickFixture(t)
	if _, err := f.start(t, nil); err != nil { // a manual run is in flight
		t.Fatal(err)
	}
	due := *f.config(t).NextCheckAt
	sum := f.tick(t)
	if sum.AlreadyRunning != 1 || sum.Started != 0 || !f.config(t).NextCheckAt.Equal(due) {
		t.Errorf("summary %+v, next %v (was %v); want the anchor restored so the next tick retries", sum, f.config(t).NextCheckAt, due)
	}
}

type failingStarter struct{}

func (failingStarter) StartScheduled(context.Context, string, Config, int) (string, error) {
	return "", errors.New("queue is down")
}

func TestTickKeepsTheAnchorAdvancedWhenStartingFails(t *testing.T) {
	f := newTickFixture(t)
	f.ticker.Starter = failingStarter{}
	due := *f.config(t).NextCheckAt
	sum := f.tick(t)
	if sum.StartErrors != 1 || !f.config(t).NextCheckAt.After(due) {
		t.Errorf("summary %+v, next %v; want the failure counted and no retry storm", sum, f.config(t).NextCheckAt)
	}
}

func TestTickTwoSchedulersStartOneRun(t *testing.T) {
	f := newTickFixture(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	started := 0
	for range 4 {
		wg.Go(func() {
			sum, err := f.ticker.Tick(context.Background())
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			started += sum.Started
			mu.Unlock()
		})
	}
	wg.Wait()
	if started != 1 || len(f.sched.jobs) != 1 {
		t.Errorf("started %d runs, %d jobs; want exactly one across 4 racing schedulers", started, len(f.sched.jobs))
	}
}

func TestTickBudgetAndDeadline(t *testing.T) {
	ctx := context.Background()
	addConfigs := func(f tickFixture, n int) {
		for i := range n {
			//nolint:contextcheck // The fixture's service uses its own context.
			cfg := create(t, f.s, f.project, func(in *CreateInput) { in.Domain = fmt.Sprintf("extra%d.example.com", i) })
			if _, err := f.s.AddKeywords(ctx, f.project, cfg.ID, []string{"a", "b"}, false); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.Repo.(Store).DB.Exec(ctx, `UPDATE go_rank_tracking_configs SET next_check_at = $2, schedule_interval = 'weekly' WHERE id = $1`, cfg.ID, testNow.Add(-time.Duration(10+i)*time.Minute)); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Run("budget admits what fits and always the first", func(t *testing.T) {
		f := newTickFixture(t)
		addConfigs(f, 2) // three configs of 4 task units each
		f.ticker.UnitBudget = 9
		sum := f.tick(t)
		if sum.Started != 2 || sum.UnitsStarted != 8 || !sum.StoppedByBudget {
			t.Errorf("summary = %+v, want 2 started (8 units) and stopped by the budget", sum)
		}
		if next := f.tick(t); next.Started != 1 {
			t.Errorf("next tick = %+v, want the leftover config to start", next)
		}
	})
	t.Run("a budget that fits exactly admits all", func(t *testing.T) {
		f := newTickFixture(t)
		addConfigs(f, 1)
		f.ticker.UnitBudget = 8
		if sum := f.tick(t); sum.Started != 2 || sum.StoppedByBudget {
			t.Errorf("summary = %+v, want both started inside a budget of exactly 8", sum)
		}
	})
	t.Run("a config over the budget still starts", func(t *testing.T) {
		f := newTickFixture(t)
		f.ticker.UnitBudget = 1
		if sum := f.tick(t); sum.Started != 1 {
			t.Errorf("summary = %+v, want the oversized first config admitted so it cannot starve", sum)
		}
	})
	t.Run("deadline stops the loop", func(t *testing.T) {
		f := newTickFixture(t)
		addConfigs(f, 2)
		clock := testNow
		f.ticker.Schedule.Now = func() time.Time { clock = clock.Add(2 * time.Minute); return clock }
		f.ticker.Deadline = 5 * time.Minute
		sum := f.tick(t)
		if !sum.StoppedByDeadline || sum.Started >= 3 {
			t.Errorf("summary = %+v, want the tick stopped by its deadline", sum)
		}
	})
}

func TestTickRunsOldestFirst(t *testing.T) {
	f := newTickFixture(t)
	ctx := context.Background()
	older := create(t, f.s, f.project, func(in *CreateInput) { in.Domain = "older.example.com" })
	if _, err := f.s.AddKeywords(ctx, f.project, older.ID, []string{"a", "b"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Repo.(Store).DB.Exec(ctx, `UPDATE go_rank_tracking_configs SET next_check_at = $2, schedule_interval = 'weekly' WHERE id = $1`, older.ID, testNow.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	f.ticker.UnitBudget = 4 // room for one
	f.tick(t)
	if run, _ := f.checks.Results.LatestRun(ctx, older.ID); run == nil {
		t.Error("the config due longest ago did not run first")
	}
	if run, _ := f.checks.Results.LatestRun(ctx, f.cfg.ID); run != nil {
		t.Error("the newer config jumped the queue")
	}
}
