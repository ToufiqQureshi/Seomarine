package ranktracking

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/jobs"
)

type fakeSerp struct {
	mu    sync.Mutex
	calls []CheckRequest
	// fn decides each answer; nil returns position 7 for every lookup.
	fn func(n int, req CheckRequest) (CheckResult, error)
}

func (f *fakeSerp) CheckLive(_ context.Context, _ string, req CheckRequest) (CheckResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	n := len(f.calls)
	f.mu.Unlock()
	if f.fn != nil {
		return f.fn(n, req)
	}
	seven := 7
	return CheckResult{Position: &seven, SerpFeatures: []string{"organic"}}, nil
}

func (f *fakeSerp) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type fakeScheduler struct {
	jobs []RunJob
	err  error
}

func (f *fakeScheduler) EnqueueRun(_ context.Context, job RunJob) error {
	if f.err != nil {
		return f.err
	}
	f.jobs = append(f.jobs, job)
	return nil
}

type fakePlans struct{ paid bool }

func (f fakePlans) HasPaidPlan(context.Context, string) (bool, error) { return f.paid, nil }

type checkFixture struct {
	fixture
	checks *Checks
	serp   *fakeSerp
	sched  *fakeScheduler
}

func newCheckFixture(t *testing.T, keywords ...string) checkFixture {
	t.Helper()
	s, project := newTestService(t, nil)
	ctx := context.Background()
	cfg := create(t, s, project, func(in *CreateInput) { in.ScheduleInterval = Manual })
	if _, err := s.AddKeywords(ctx, project, cfg.ID, keywords, false); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListKeywords(ctx, project, cfg.ID)
	kw := map[string]string{}
	for _, k := range list {
		kw[k.Keyword] = k.ID
	}
	store := s.Repo.(Store)
	serp, sched := &fakeSerp{}, &fakeScheduler{}
	checks := &Checks{
		Repo: store, Results: store, Runs: store, Serp: serp, Scheduler: sched,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return testNow },
	}
	return checkFixture{fixture: fixture{s: s, project: project, cfg: cfg, kw: kw}, checks: checks, serp: serp, sched: sched}
}

func (f checkFixture) start(t *testing.T, mutate func(*StartInput)) (string, error) {
	t.Helper()
	in := StartInput{OrganizationID: "org-test", ProjectID: f.project, ConfigID: f.cfg.ID}
	if mutate != nil {
		mutate(&in)
	}
	return f.checks.Start(context.Background(), in)
}

func (f checkFixture) run(t *testing.T, id string) Run {
	t.Helper()
	r, err := f.checks.Runs.GetRun(context.Background(), id)
	if err != nil || r == nil {
		t.Fatalf("GetRun(%s) = %v, %v", id, r, err)
	}
	return *r
}

func (f checkFixture) config(t *testing.T) Config {
	t.Helper()
	cfg, err := f.s.Repo.GetConfig(context.Background(), f.project, f.cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestStartCheckRules(t *testing.T) {
	f := newCheckFixture(t, "one", "two")
	ctx := context.Background()

	empty := newCheckFixtureWithoutKeywords(t)
	if _, err := empty.start(t, nil); !isValidation(err) {
		t.Errorf("start without keywords error = %v, want ValidationError", err)
	}
	if _, err := f.start(t, func(in *StartInput) { in.KeywordIDs = []string{"not-a-keyword"} }); !isValidation(err) {
		t.Errorf("start with unknown keyword ids error = %v, want ValidationError", err)
	}
	over := 1
	if _, err := f.start(t, func(in *StartInput) { in.MaxCostCredits = &over }); !isValidation(err) || !strings.Contains(err.Error(), "above the approved maximum") {
		t.Errorf("start over the cost ceiling error = %v", err)
	}
	f.checks.Plans = fakePlans{paid: false}
	if _, err := f.start(t, nil); !errors.Is(err, ErrPaymentRequired) {
		t.Errorf("free plan error = %v, want ErrPaymentRequired", err)
	}
	f.checks.Plans = fakePlans{paid: true}
	if _, err := f.start(t, func(in *StartInput) { in.ProjectID = "another-project" }); !errors.Is(err, ErrNotFound) {
		t.Errorf("other project error = %v, want ErrNotFound", err)
	}
	if run, _ := f.checks.Results.LatestRun(ctx, f.cfg.ID); run != nil {
		t.Fatalf("a refused start created run %+v", run)
	}
	if len(f.sched.jobs) != 0 || f.serp.count() != 0 {
		t.Fatal("a refused start queued work or called the provider")
	}

	limit := 1000
	id, err := f.start(t, func(in *StartInput) { in.MaxCostCredits = &limit })
	if err != nil {
		t.Fatal(err)
	}
	run := f.run(t, id)
	if run.Status != "pending" || run.KeywordsTotal != 2 || run.IsSubsetRun {
		t.Errorf("run = %+v, want pending, 2 keywords, full", run)
	}
	if len(f.sched.jobs) != 1 || f.sched.jobs[0].RunID != id || f.sched.jobs[0].OrganizationID != "org-test" {
		t.Errorf("jobs = %+v, want one job for the run", f.sched.jobs)
	}
	if f.serp.count() != 0 {
		t.Error("Start called the provider; only the worker may spend")
	}

	_, err = f.start(t, nil)
	active, ok := errors.AsType[*ActiveRunError](err)
	if !ok || active.RunID != id || !errors.Is(err, ErrRunActive) {
		t.Errorf("second start error = %v, want ActiveRunError for %s", err, id)
	}

	unavailable := *f.checks
	unavailable.Serp = nil
	if _, err := unavailable.Start(ctx, StartInput{OrganizationID: "o", ProjectID: f.project, ConfigID: f.cfg.ID}); !errors.Is(err, ErrChecksUnavailable) {
		t.Errorf("no provider error = %v, want ErrChecksUnavailable", err)
	}
}

func newCheckFixtureWithoutKeywords(t *testing.T) checkFixture {
	t.Helper()
	return newCheckFixture(t)
}

func TestStartCheckSubsetAndEnqueueFailure(t *testing.T) {
	f := newCheckFixture(t, "one", "two")
	f.sched.err = errors.New("queue down")
	if _, err := f.start(t, func(in *StartInput) { in.KeywordIDs = []string{f.kw["one"]} }); err == nil {
		t.Fatal("start succeeded with the queue down")
	}
	run, _ := f.checks.Results.LatestRun(context.Background(), f.cfg.ID)
	if run == nil || run.Status != "failed" {
		t.Fatalf("run after a failed enqueue = %+v, want failed so the slot is free", run)
	}
	f.sched.err = nil
	id, err := f.start(t, func(in *StartInput) { in.KeywordIDs = []string{f.kw["one"]} })
	if err != nil {
		t.Fatalf("start after releasing the slot: %v", err)
	}
	if r := f.run(t, id); !r.IsSubsetRun || r.KeywordsTotal != 1 {
		t.Errorf("run = %+v, want a subset run of 1 keyword", r)
	}
}

func TestStartCheckClearsStaleRuns(t *testing.T) {
	f := newCheckFixture(t, "one")
	ctx := context.Background()
	pool := f.s.Repo.(Store).DB
	first, err := f.start(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	setJob := func(state string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `DELETE FROM go_jobs WHERE queue = $1 AND idempotency_key = $2`, QueueName, first); err != nil {
			t.Fatal(err)
		}
		if state == "" {
			return
		}
		_, err := pool.Exec(ctx, `INSERT INTO go_jobs (queue, idempotency_key, payload, state, finished_at, lease_until, worker_id)
			VALUES ($1, $2, '{}', $3, CASE WHEN $3 IN ('succeeded', 'failed') THEN now() END,
				CASE WHEN $3 = 'running' THEN now() + interval '1 hour' END, CASE WHEN $3 = 'running' THEN 'w' END)`, QueueName, first, state)
		if err != nil {
			t.Fatal(err)
		}
	}
	age := func(d time.Duration) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE go_rank_check_runs SET started_at = $2 WHERE id = $1`, first, testNow.Add(-d)); err != nil {
			t.Fatal(err)
		}
	}

	setJob("") // no job yet, but the run is new
	age(10 * time.Second)
	if _, err := f.start(t, nil); !errors.Is(err, ErrRunActive) {
		t.Errorf("a run inside the startup grace was cleared: %v", err)
	}
	setJob("queued")
	age(time.Hour)
	if _, err := f.start(t, nil); !errors.Is(err, ErrRunActive) {
		t.Errorf("a run with a live job was cleared: %v", err)
	}
	setJob("running")
	if _, err := f.start(t, nil); !errors.Is(err, ErrRunActive) {
		t.Errorf("a run with a running job was cleared: %v", err)
	}
	setJob("failed")
	second, err := f.start(t, nil)
	if err != nil {
		t.Fatalf("a run whose job failed should be cleared: %v", err)
	}
	if old := f.run(t, first); old.Status != "failed" || old.ErrorMessage == nil || !strings.Contains(*old.ErrorMessage, "failed") {
		t.Errorf("stale run = %+v, want failed with a reason", old)
	}
	if second == first {
		t.Error("new run reused the stale id")
	}
}

func execute(t *testing.T, f checkFixture, job RunJob) error {
	t.Helper()
	return f.checks.Execute(context.Background(), job)
}

func TestExecuteCompletesRunAndIsIdempotent(t *testing.T) {
	f := newCheckFixture(t, "one", "two")
	id, _ := f.start(t, nil)
	job := f.sched.jobs[0]
	if err := execute(t, f, job); err != nil {
		t.Fatal(err)
	}
	run := f.run(t, id)
	if run.Status != "completed" || run.KeywordsChecked != 2 || run.KeywordsTotal != 2 || run.ErrorMessage != nil || run.CompletedAt == nil {
		t.Errorf("run = %+v, want completed 2/2", run)
	}
	if f.serp.count() != 4 {
		t.Errorf("provider calls = %d, want 4 (2 keywords x 2 devices)", f.serp.count())
	}
	req := f.serp.calls[0]
	if req.TargetDomain != "example.com" || req.Depth != 10 || req.LocationCode != 2840 || req.LanguageCode != "en" {
		t.Errorf("request = %+v", req)
	}
	if cfg := f.config(t); cfg.LastCheckedAt == nil {
		t.Error("a completed run must stamp last_checked_at")
	}
	got, err := f.s.LatestResults(context.Background(), f.project, f.cfg.ID, "7d")
	if err != nil || len(got.Rows) != 2 || pos(got.Rows[0].Desktop.Position) != 7 {
		t.Errorf("results after the run = %+v, %v", got, err)
	}

	if err := execute(t, f, job); err != nil || f.serp.count() != 4 {
		t.Errorf("re-running a finished run: err %v, calls %d; want no new provider calls", err, f.serp.count())
	}
}

func TestExecuteSkipsPairsAlreadyChecked(t *testing.T) {
	f := newCheckFixture(t, "one", "two")
	id, _ := f.start(t, nil)
	ctx := context.Background()
	store := f.checks.Runs
	if err := store.InsertSnapshots(ctx, []SnapshotInput{{RunID: id, TrackingKeywordID: f.kw["one"], Keyword: "one", Device: "desktop"}}); err != nil {
		t.Fatal(err)
	}
	if err := execute(t, f, f.sched.jobs[0]); err != nil {
		t.Fatal(err)
	}
	if f.serp.count() != 3 {
		t.Errorf("provider calls = %d, want 3: a stored pair must not be asked, and billed, again", f.serp.count())
	}
	for _, c := range f.serp.calls {
		if c.Keyword == "one" && c.Device == "desktop" {
			t.Error("asked for the pair that already had a snapshot")
		}
	}
	if run := f.run(t, id); run.Status != "completed" || run.KeywordsChecked != 2 {
		t.Errorf("run = %+v", run)
	}
	var rows int
	_ = f.s.Repo.(Store).DB.QueryRow(ctx, `SELECT count(*) FROM go_rank_snapshots WHERE run_id = $1`, id).Scan(&rows)
	if rows != 4 {
		t.Errorf("snapshots = %d, want 4 without duplicates", rows)
	}
}

func TestExecutePartialAndTotalFailure(t *testing.T) {
	boom := errors.New("provider says no")
	t.Run("partial keeps the first reason and still counts as checked", func(t *testing.T) {
		f := newCheckFixture(t, "one", "two")
		f.serp.fn = func(_ int, req CheckRequest) (CheckResult, error) {
			if req.Keyword == "two" {
				return CheckResult{}, boom
			}
			return CheckResult{}, nil
		}
		id, _ := f.start(t, nil)
		if err := execute(t, f, f.sched.jobs[0]); err != nil {
			t.Fatal(err)
		}
		run := f.run(t, id)
		if run.Status != "completed" || run.KeywordsChecked != 1 || run.ErrorMessage == nil || *run.ErrorMessage != "Checked 1 of 2 keyword(s): provider says no" {
			t.Errorf("run = %+v (%v)", run, run.ErrorMessage)
		}
		if f.config(t).LastCheckedAt == nil {
			t.Error("a partly successful run must stamp last_checked_at")
		}
	})
	t.Run("nothing checked fails the run and leaves last_checked_at alone", func(t *testing.T) {
		f := newCheckFixture(t, "one", "two")
		f.serp.fn = func(int, CheckRequest) (CheckResult, error) { return CheckResult{}, boom }
		id, _ := f.start(t, nil)
		if err := execute(t, f, f.sched.jobs[0]); err != nil {
			t.Fatal(err)
		}
		run := f.run(t, id)
		if run.Status != "failed" || run.KeywordsChecked != 0 || run.ErrorMessage == nil || *run.ErrorMessage != "provider says no" {
			t.Errorf("run = %+v (%v)", run, run.ErrorMessage)
		}
		if f.config(t).LastCheckedAt != nil {
			t.Error("a failed run moved last_checked_at")
		}
		if _, err := f.start(t, nil); err != nil {
			t.Errorf("the slot should be free after a failed run: %v", err)
		}
	})
}

func TestExecuteStopsOnErrorsThatRepeat(t *testing.T) {
	keywords := make([]string, 25)
	for i := range keywords {
		keywords[i] = fmt.Sprintf("keyword %d", i)
	}
	cases := map[string]error{
		"billing issue":     fmt.Errorf("DataForSEO: %w", dataforseo.ErrBillingIssue),
		"invalid field":     &dataforseo.TaskError{StatusCode: 40501 + 1, Message: "Invalid Field: 'x'", InvalidField: true},
		"invalid (wrapped)": fmt.Errorf("lookup: %w", &dataforseo.TaskError{InvalidField: true, Message: "Invalid Field: 'y'"}),
	}
	for name, cause := range cases {
		t.Run(name, func(t *testing.T) {
			f := newCheckFixture(t, keywords...)
			f.serp.fn = func(int, CheckRequest) (CheckResult, error) { return CheckResult{}, cause }
			id, _ := f.start(t, nil)
			if err := execute(t, f, f.sched.jobs[0]); err != nil {
				t.Fatal(err)
			}
			if f.serp.count() != keywordsPerBatch*2 {
				t.Errorf("provider calls = %d, want only the first batch (%d): failed tasks are billed", f.serp.count(), keywordsPerBatch*2)
			}
			if run := f.run(t, id); run.Status != "failed" {
				t.Errorf("run = %+v, want failed", run)
			}
		})
	}
	t.Run("other errors keep going", func(t *testing.T) {
		f := newCheckFixture(t, keywords...)
		f.serp.fn = func(int, CheckRequest) (CheckResult, error) { return CheckResult{}, dataforseo.ErrUpstreamUnavailable }
		if _, err := f.start(t, nil); err != nil {
			t.Fatal(err)
		}
		_ = execute(t, f, f.sched.jobs[0])
		if f.serp.count() != 25*2 {
			t.Errorf("provider calls = %d, want every pair tried for a provider flake", f.serp.count())
		}
	})
}

func TestExecuteResumesAfterCancel(t *testing.T) {
	keywords := make([]string, 12)
	for i := range keywords {
		keywords[i] = fmt.Sprintf("keyword %02d", i)
	}
	f := newCheckFixture(t, keywords...)
	if _, err := f.s.Repo.(Store).DB.Exec(context.Background(), `UPDATE go_rank_tracking_configs SET devices = 'desktop' WHERE id = $1`, f.cfg.ID); err != nil {
		t.Fatal(err)
	}
	id, _ := f.start(t, nil)

	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	f.serp.fn = func(_ int, _ CheckRequest) (CheckResult, error) {
		if calls.Add(1) == 11 { // the first lookup of the second batch
			cancel()
			return CheckResult{}, ctx.Err()
		}
		return CheckResult{}, nil
	}
	if err := f.checks.Execute(ctx, f.sched.jobs[0]); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled so the queue retries", err)
	}
	if run := f.run(t, id); run.Status != "running" {
		t.Fatalf("run = %+v, want still running", run)
	}
	n, _ := f.checks.Runs.CountCheckedKeywords(context.Background(), id)
	if n != 10 {
		t.Fatalf("checked keywords after the cancel = %d, want the first batch's 10", n)
	}

	before := f.serp.count()
	f.serp.fn = nil
	if err := execute(t, f, f.sched.jobs[0]); err != nil {
		t.Fatal(err)
	}
	if again := f.serp.count() - before; again != 2 {
		t.Errorf("provider calls on resume = %d, want only the 2 keywords of the second batch", again)
	}
	if run := f.run(t, id); run.Status != "completed" || run.KeywordsChecked != 12 {
		t.Errorf("run = %+v, want completed 12/12", run)
	}
}

func TestExecuteGuards(t *testing.T) {
	t.Run("a run failed by stale cleanup is not resurrected", func(t *testing.T) {
		f := newCheckFixture(t, "one")
		id, _ := f.start(t, nil)
		_ = f.checks.Runs.FailRunIfActive(context.Background(), id, "superseded")
		if err := execute(t, f, f.sched.jobs[0]); err != nil || f.serp.count() != 0 {
			t.Errorf("err %v, calls %d; want a quiet no-op", err, f.serp.count())
		}
		if run := f.run(t, id); run.Status != "failed" || *run.ErrorMessage != "superseded" {
			t.Errorf("run = %+v, want it left failed", run)
		}
	})
	t.Run("an archived config fails the run before any spend", func(t *testing.T) {
		f := newCheckFixture(t, "one")
		id, _ := f.start(t, nil)
		off := false
		if _, err := f.s.UpdateConfig(context.Background(), f.project, f.cfg.ID, UpdateInput{IsActive: &off}); err != nil {
			t.Fatal(err)
		}
		if err := execute(t, f, f.sched.jobs[0]); err != nil || f.serp.count() != 0 {
			t.Fatalf("err %v, calls %d", err, f.serp.count())
		}
		if run := f.run(t, id); run.Status != "failed" || !strings.Contains(*run.ErrorMessage, "archived") {
			t.Errorf("run = %+v", run)
		}
	})
	t.Run("a deleted keyword set fails the run", func(t *testing.T) {
		f := newCheckFixture(t, "one")
		id, _ := f.start(t, nil)
		if _, err := f.s.RemoveKeywords(context.Background(), f.project, f.cfg.ID, []string{f.kw["one"]}); err != nil {
			t.Fatal(err)
		}
		if err := execute(t, f, f.sched.jobs[0]); err != nil || f.serp.count() != 0 {
			t.Fatalf("err %v, calls %d", err, f.serp.count())
		}
		if run := f.run(t, id); run.Status != "failed" {
			t.Errorf("run = %+v", run)
		}
	})
	t.Run("a subset job checks only its keywords", func(t *testing.T) {
		f := newCheckFixture(t, "one", "two", "three")
		id, _ := f.start(t, func(in *StartInput) { in.KeywordIDs = []string{f.kw["two"]} })
		if err := execute(t, f, f.sched.jobs[0]); err != nil {
			t.Fatal(err)
		}
		for _, c := range f.serp.calls {
			if c.Keyword != "two" {
				t.Errorf("checked %q outside the subset", c.Keyword)
			}
		}
		if run := f.run(t, id); run.Status != "completed" || run.KeywordsChecked != 1 || !run.IsSubsetRun {
			t.Errorf("run = %+v", run)
		}
	})
	t.Run("an unknown run is ignored", func(t *testing.T) {
		f := newCheckFixture(t, "one")
		if err := f.checks.Execute(context.Background(), RunJob{RunID: "no-such-run"}); err != nil {
			t.Errorf("error = %v, want nil", err)
		}
	})
}

func TestJobHandler(t *testing.T) {
	f := newCheckFixture(t, "one")
	h := f.checks.JobHandler()
	if err := h(context.Background(), jobs.Job{ID: 1, Payload: json.RawMessage(`nope`)}); err == nil {
		t.Error("a malformed payload was accepted")
	}
	if err := h(context.Background(), jobs.Job{ID: 2, Payload: json.RawMessage(`{}`)}); err == nil {
		t.Error("a payload without a run id was accepted")
	}
	id, _ := f.start(t, nil)
	payload, _ := json.Marshal(f.sched.jobs[0])
	if err := h(context.Background(), jobs.Job{ID: 3, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if run := f.run(t, id); run.Status != "completed" {
		t.Errorf("run = %+v", run)
	}
}

func TestParseSerp(t *testing.T) {
	raw := func(items string) []json.RawMessage {
		return []json.RawMessage{json.RawMessage(`{"items":` + items + `}`)}
	}
	tests := []struct {
		name    string
		items   string
		wantPos int
		wantURL string
		wantFt  []string
	}{
		{"organic match", `[{"type":"organic","rank_group":3,"domain":"example.com","url":"https://example.com/a"}]`, 3, "https://example.com/a", []string{"organic"}},
		{"subdomain counts", `[{"type":"organic","rank_group":2,"domain":"blog.example.com","url":"https://blog.example.com/"}]`, 2, "https://blog.example.com/", []string{"organic"}},
		{"look-alike domain does not", `[{"type":"organic","rank_group":1,"domain":"notexample.com","url":"u"}]`, -1, "", []string{"organic"}},
		{"case-insensitive", `[{"type":"organic","rank_group":4,"domain":"EXAMPLE.com","url":"u"}]`, 4, "u", []string{"organic"}},
		{"local pack mention is not organic", `[{"type":"local_pack","rank_group":1,"domain":"example.com"},{"type":"organic","rank_group":9,"domain":"example.com","url":"u9"}]`, 9, "u9", []string{"local_pack", "organic"}},
		{"first organic match wins", `[{"type":"organic","rank_group":5,"domain":"example.com","url":"first"},{"type":"organic","rank_group":8,"domain":"example.com","url":"second"}]`, 5, "first", []string{"organic"}},
		{"falls back to rank_absolute", `[{"type":"organic","rank_absolute":6,"domain":"example.com","url":"u"}]`, 6, "u", []string{"organic"}},
		{"null domain is skipped", `[{"type":"organic","rank_group":1,"domain":null},{"type":"people_also_ask"}]`, -1, "", []string{"organic", "people_also_ask"}},
		{"no items", `[]`, -1, "", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSerp(raw(tt.items), "example.com")
			if err != nil {
				t.Fatal(err)
			}
			url := ""
			if got.URL != nil {
				url = *got.URL
			}
			if pos(got.Position) != tt.wantPos || url != tt.wantURL || strings.Join(got.SerpFeatures, ",") != strings.Join(tt.wantFt, ",") {
				t.Errorf("got pos %d url %q features %v; want %d %q %v", pos(got.Position), url, got.SerpFeatures, tt.wantPos, tt.wantURL, tt.wantFt)
			}
		})
	}
	if got, err := parseSerp(nil, "example.com"); err != nil || got.Position != nil || got.SerpFeatures == nil {
		t.Errorf("empty results = %+v, %v", got, err)
	}
	if _, err := parseSerp([]json.RawMessage{json.RawMessage(`{"items":"x"}`)}, "example.com"); err == nil {
		t.Error("a malformed result was accepted")
	}
}

type noopRecorder struct{}

func (noopRecorder) RecordDataForSEO(context.Context, string, dataforseo.Cost) error { return nil }

func TestDataForSEOSerp(t *testing.T) {
	var seen []map[string]any
	var status atomic.Int32
	status.Store(20000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tasks []map[string]any
		_ = json.NewDecoder(r.Body).Decode(&tasks)
		seen = append(seen, tasks[0])
		task := map[string]any{"status_code": int(status.Load()), "status_message": "msg", "cost": 0.002, "path": []string{"v3"},
			"result": []any{map[string]any{"items": []any{map[string]any{"type": "organic", "rank_group": 2, "domain": "www.example.com", "url": "https://www.example.com/"}}}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "tasks": []any{task}})
	}))
	t.Cleanup(server.Close)
	client, err := dataforseo.NewClient(dataforseo.Options{BaseURL: server.URL, APIKey: base64.StdEncoding.EncodeToString([]byte("fixture-user:fixture-password")), Recorder: noopRecorder{}})
	if err != nil {
		t.Fatal(err)
	}
	p := DataForSEOSerp{Client: client}
	ctx := context.Background()
	city := "Mumbai"

	got, err := p.CheckLive(ctx, "org", CheckRequest{Keyword: "seo", Device: "mobile", LocationCode: 2356, LanguageCode: "en", LocationName: &city, TargetDomain: "example.com", Depth: 20})
	if err != nil || pos(got.Position) != 2 {
		t.Fatalf("CheckLive() = %+v, %v", got, err)
	}
	task := seen[0]
	if task["location_name"] != "Mumbai" || task["location_code"] != nil || task["device"] != "mobile" || task["os"] != "android" ||
		task["depth"] != float64(20) || task["find_targets_in"] == nil || task["stop_crawl_on_match"] == nil {
		t.Errorf("request = %v", task)
	}
	if _, err := p.CheckLive(ctx, "org", CheckRequest{Keyword: "seo", Device: "desktop", LocationCode: 2356, LanguageCode: "en", TargetDomain: "example.com", Depth: 10}); err != nil {
		t.Fatal(err)
	}
	if task := seen[1]; task["location_code"] != float64(2356) || task["location_name"] != nil || task["os"] != "windows" {
		t.Errorf("national request = %v", task)
	}

	status.Store(40501)
	if got, err := p.CheckLive(ctx, "org", CheckRequest{Keyword: "obscure", Device: "desktop", LocationCode: 2356, LanguageCode: "en", TargetDomain: "example.com", Depth: 10}); err != nil || got.Position != nil {
		t.Errorf("no-results SERP = %+v, %v; want not ranking, not an error", got, err)
	}
	status.Store(40200)
	if _, err := p.CheckLive(ctx, "org", CheckRequest{Keyword: "x", Device: "desktop", LocationCode: 2356, LanguageCode: "en", TargetDomain: "example.com", Depth: 10}); !errors.Is(err, dataforseo.ErrBillingIssue) {
		t.Errorf("billing status error = %v, want ErrBillingIssue", err)
	}
	callsBefore := len(seen)
	for _, depth := range []int{0, 15, 110} {
		if _, err := p.CheckLive(ctx, "org", CheckRequest{Keyword: "x", Device: "desktop", LocationCode: 2356, LanguageCode: "en", TargetDomain: "example.com", Depth: depth}); err == nil {
			t.Errorf("depth %d was sent to the provider", depth)
		}
	}
	if len(seen) != callsBefore {
		t.Error("an invalid depth reached the provider, where it would be billed")
	}
}

func TestRunStoreGuards(t *testing.T) {
	f := newCheckFixture(t, "one")
	ctx := context.Background()
	store := f.checks.Runs
	id, _ := f.start(t, nil)

	if err := store.SetRunErrorIfEmpty(ctx, id, "first"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRunErrorIfEmpty(ctx, id, "second"); err != nil {
		t.Fatal(err)
	}
	if run := f.run(t, id); run.ErrorMessage == nil || *run.ErrorMessage != "first" {
		t.Errorf("error = %v, want the first reason kept", run.ErrorMessage)
	}

	if err := store.FailRunIfActive(ctx, id, "cleanup"); err != nil {
		t.Fatal(err)
	}
	run := f.run(t, id)
	if ok, err := store.FinishRun(ctx, run, "completed", 1, nil); err != nil || ok {
		t.Errorf("FinishRun() on a failed run = %v, %v; want false so the earlier decision stands", ok, err)
	}
	if ok, err := store.MarkRunning(ctx, id, 1); err != nil || ok {
		t.Errorf("MarkRunning() on a failed run = %v, %v; want false", ok, err)
	}
	if after := f.run(t, id); after.Status != "failed" || f.config(t).LastCheckedAt != nil {
		t.Errorf("run = %+v, last checked %v; want the failed run untouched", after, f.config(t).LastCheckedAt)
	}
}
