package ranktracking

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

type fakeQueued struct {
	mu        sync.Mutex
	posts     [][]TaskInput
	collects  int
	next      int
	postFn    func(call int, req PostRequest) ([]PostedTask, error)
	collectFn func(taskID string, calls int) (TaskOutcome, error)
	perTask   map[string]int
}

func (f *fakeQueued) PostTasks(_ context.Context, _ string, req PostRequest) ([]PostedTask, error) {
	f.mu.Lock()
	f.posts = append(f.posts, req.Tasks)
	call := len(f.posts)
	f.mu.Unlock()
	if f.postFn != nil {
		return f.postFn(call, req)
	}
	return f.accept(req.Tasks), nil
}

func (f *fakeQueued) accept(tasks []TaskInput) []PostedTask {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]PostedTask, len(tasks))
	for i, t := range tasks {
		f.next++
		out[i] = PostedTask{TaskInput: t, TaskID: fmt.Sprintf("task-%d", f.next)}
	}
	return out
}

func (f *fakeQueued) CollectTask(_ context.Context, taskID, _ string) (TaskOutcome, error) {
	f.mu.Lock()
	f.collects++
	if f.perTask == nil {
		f.perTask = map[string]int{}
	}
	f.perTask[taskID]++
	calls := f.perTask[taskID]
	f.mu.Unlock()
	if f.collectFn != nil {
		return f.collectFn(taskID, calls)
	}
	four := 4
	return TaskOutcome{Result: CheckResult{Position: &four, SerpFeatures: []string{"organic"}}}, nil
}

func (f *fakeQueued) postCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.posts)
}

type queuedFixture struct {
	checkFixture
	queued *fakeQueued
	sleeps *atomic.Int32
}

func newQueuedFixture(t *testing.T, keywords ...string) queuedFixture {
	t.Helper()
	f := newCheckFixture(t, keywords...)
	q, sleeps := &fakeQueued{}, &atomic.Int32{}
	f.checks.Queued, f.checks.Tasks = q, f.s.Repo.(Store)
	f.checks.PollWaits = []time.Duration{time.Minute, time.Minute, time.Minute}
	f.checks.Sleep = func(context.Context, time.Duration) error { sleeps.Add(1); return nil }
	return queuedFixture{checkFixture: f, queued: q, sleeps: sleeps}
}

func (f queuedFixture) startScheduled(t *testing.T) (string, RunJob) {
	t.Helper()
	cfg := f.config(t)
	kws, _ := f.s.ListKeywords(context.Background(), f.project, f.cfg.ID)
	id, err := f.checks.StartScheduled(context.Background(), "org-test", cfg, len(kws))
	if err != nil {
		t.Fatal(err)
	}
	return id, f.sched.jobs[len(f.sched.jobs)-1]
}

func (f queuedFixture) taskStates(t *testing.T, runID string) map[string]string {
	t.Helper()
	rows, err := f.checks.Tasks.Tasks(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, r := range rows {
		out[r.TrackingKeywordID+":"+r.Device] = r.State
	}
	return out
}

func TestQueuedRunCollectsPostedTasks(t *testing.T) {
	f := newQueuedFixture(t, "one", "two")
	f.queued.collectFn = func(_ string, calls int) (TaskOutcome, error) {
		if calls == 1 {
			return TaskOutcome{Pending: true}, nil
		}
		four := 4
		return TaskOutcome{Result: CheckResult{Position: &four}}, nil
	}
	id, job := f.startScheduled(t)
	if job.Trigger != TriggerScheduled {
		t.Fatalf("job trigger = %q", job.Trigger)
	}
	if err := execute(t, f.checkFixture, job); err != nil {
		t.Fatal(err)
	}
	run := f.run(t, id)
	if run.Status != "completed" || run.KeywordsChecked != 2 || run.ErrorMessage != nil {
		t.Errorf("run = %+v, want completed 2/2", run)
	}
	if f.queued.postCalls() != 1 || len(f.queued.posts[0]) != 4 {
		t.Errorf("posts = %v, want one post of 4 tasks", f.queued.posts)
	}
	if f.serp.count() != 0 {
		t.Errorf("live calls = %d, want none when the queue delivers", f.serp.count())
	}
	if f.sleeps.Load() != 2 {
		t.Errorf("polling rounds = %d, want 2 (pending, then done)", f.sleeps.Load())
	}
	for key, state := range f.taskStates(t, id) {
		if state != TaskCollected {
			t.Errorf("task %s = %s, want collected", key, state)
		}
	}
	if f.config(t).LastCheckedAt == nil {
		t.Error("last_checked_at not stamped")
	}
}

func TestQueuedRunResumesWithoutPostingAgain(t *testing.T) {
	f := newQueuedFixture(t, "one", "two")
	id, job := f.startScheduled(t)
	ctx := context.Background()
	// A previous attempt posted (and paid for) all four tasks, then crashed.
	var posted []PostedTask
	for _, k := range []string{"one", "two"} {
		for _, d := range []string{"desktop", "mobile"} {
			posted = append(posted, PostedTask{TaskInput: TaskInput{KeywordID: f.kw[k], Keyword: k, Device: d}, TaskID: "paid-" + k + d})
		}
	}
	if err := f.checks.Tasks.RecordTasks(ctx, id, posted); err != nil {
		t.Fatal(err)
	}
	if err := execute(t, f.checkFixture, job); err != nil {
		t.Fatal(err)
	}
	if f.queued.postCalls() != 0 {
		t.Errorf("posted %d times after a crash, want 0: the paid tasks are collected instead", f.queued.postCalls())
	}
	if run := f.run(t, id); run.Status != "completed" || run.KeywordsChecked != 2 {
		t.Errorf("run = %+v", run)
	}
}

func TestQueuedRunFallsBackToLive(t *testing.T) {
	t.Run("rejected at post time", func(t *testing.T) {
		f := newQueuedFixture(t, "one", "two")
		f.queued.postFn = func(_ int, req PostRequest) ([]PostedTask, error) { return f.queued.accept(req.Tasks[:2]), nil }
		id, job := f.startScheduled(t)
		if err := execute(t, f.checkFixture, job); err != nil {
			t.Fatal(err)
		}
		if f.serp.count() != 2 {
			t.Errorf("live calls = %d, want the 2 rejected pairs", f.serp.count())
		}
		if run := f.run(t, id); run.Status != "completed" || run.KeywordsChecked != 2 {
			t.Errorf("run = %+v, want completed 2/2", run)
		}
	})
	t.Run("failed task", func(t *testing.T) {
		f := newQueuedFixture(t, "one")
		f.queued.collectFn = func(id string, _ int) (TaskOutcome, error) {
			if id == "task-1" {
				return TaskOutcome{Failed: "task failed upstream"}, nil
			}
			return TaskOutcome{Result: CheckResult{}}, nil
		}
		id, job := f.startScheduled(t)
		if err := execute(t, f.checkFixture, job); err != nil {
			t.Fatal(err)
		}
		if f.serp.count() != 1 {
			t.Errorf("live calls = %d, want the one failed pair", f.serp.count())
		}
		states := f.taskStates(t, id)
		if states[f.kw["one"]+":desktop"] != TaskFailed || states[f.kw["one"]+":mobile"] != TaskCollected {
			t.Errorf("task states = %v", states)
		}
		if run := f.run(t, id); run.Status != "completed" || run.KeywordsChecked != 1 {
			t.Errorf("run = %+v", run)
		}
	})
	t.Run("never finishes", func(t *testing.T) {
		f := newQueuedFixture(t, "one", "two")
		f.queued.collectFn = func(string, int) (TaskOutcome, error) { return TaskOutcome{Pending: true}, nil }
		id, job := f.startScheduled(t)
		if err := execute(t, f.checkFixture, job); err != nil {
			t.Fatal(err)
		}
		if f.sleeps.Load() != 3 {
			t.Errorf("polling rounds = %d, want the whole window (3)", f.sleeps.Load())
		}
		if f.serp.count() != 4 {
			t.Errorf("live calls = %d, want every straggler", f.serp.count())
		}
		if run := f.run(t, id); run.Status != "completed" || run.KeywordsChecked != 2 {
			t.Errorf("run = %+v", run)
		}
	})
	t.Run("a transient collect error waits for the next round", func(t *testing.T) {
		f := newQueuedFixture(t, "one")
		f.queued.collectFn = func(_ string, calls int) (TaskOutcome, error) {
			if calls == 1 {
				return TaskOutcome{}, errors.New("network blip")
			}
			return TaskOutcome{Result: CheckResult{}}, nil
		}
		_, job := f.startScheduled(t)
		if err := execute(t, f.checkFixture, job); err != nil {
			t.Fatal(err)
		}
		if f.serp.count() != 0 || f.sleeps.Load() != 2 {
			t.Errorf("live calls %d, rounds %d; want 0 live and 2 rounds", f.serp.count(), f.sleeps.Load())
		}
	})
}

func TestQueuedRunPostFailures(t *testing.T) {
	t.Run("a failed post chunk goes live", func(t *testing.T) {
		f := newQueuedFixture(t, "one")
		f.queued.postFn = func(int, PostRequest) ([]PostedTask, error) { return nil, errors.New("post down") }
		id, job := f.startScheduled(t)
		if err := execute(t, f.checkFixture, job); err != nil {
			t.Fatal(err)
		}
		if f.serp.count() != 2 {
			t.Errorf("live calls = %d, want both pairs", f.serp.count())
		}
		if run := f.run(t, id); run.Status != "completed" {
			t.Errorf("run = %+v", run)
		}
	})
	t.Run("a billing error stops without paying twice", func(t *testing.T) {
		f := newQueuedFixture(t, "one")
		f.queued.postFn = func(int, PostRequest) ([]PostedTask, error) {
			return nil, fmt.Errorf("DataForSEO: %w", dataforseo.ErrBillingIssue)
		}
		id, job := f.startScheduled(t)
		if err := execute(t, f.checkFixture, job); err != nil {
			t.Fatal(err)
		}
		if f.serp.count() != 0 {
			t.Errorf("live calls = %d, want none after a billing error", f.serp.count())
		}
		if run := f.run(t, id); run.Status != "failed" || run.ErrorMessage == nil {
			t.Errorf("run = %+v, want failed with the reason", run)
		}
	})
	t.Run("posts are split at 100 tasks", func(t *testing.T) {
		keywords := make([]string, 60)
		for i := range keywords {
			keywords[i] = fmt.Sprintf("keyword %02d", i)
		}
		f := newQueuedFixture(t, keywords...)
		_, job := f.startScheduled(t)
		if err := execute(t, f.checkFixture, job); err != nil {
			t.Fatal(err)
		}
		if f.queued.postCalls() != 2 || len(f.queued.posts[0]) != MaxTasksPerPost || len(f.queued.posts[1]) != 20 {
			sizes := []int{}
			for _, p := range f.queued.posts {
				sizes = append(sizes, len(p))
			}
			t.Errorf("post sizes = %v, want [100 20]", sizes)
		}
	})
	t.Run("without queued support the run fails before any spend", func(t *testing.T) {
		f := newQueuedFixture(t, "one")
		_, job := f.startScheduled(t)
		f.checks.Queued = nil
		if err := execute(t, f.checkFixture, job); err != nil {
			t.Fatal(err)
		}
		if run := f.run(t, job.RunID); run.Status != "failed" || f.serp.count() != 0 {
			t.Errorf("run = %+v, live calls %d", run, f.serp.count())
		}
	})
}

func TestQueuedRunResumesAfterCancelDuringWait(t *testing.T) {
	f := newQueuedFixture(t, "one")
	ctx, cancel := context.WithCancel(context.Background())
	f.checks.Sleep = func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() }
	id, job := f.startScheduled(t)
	if err := f.checks.Execute(ctx, job); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled so the queue retries", err)
	}
	if f.queued.postCalls() != 1 {
		t.Fatalf("posts = %d, want 1", f.queued.postCalls())
	}
	f.checks.Sleep = func(context.Context, time.Duration) error { return nil }
	if err := execute(t, f.checkFixture, job); err != nil {
		t.Fatal(err)
	}
	if f.queued.postCalls() != 1 {
		t.Errorf("posts after resume = %d, want still 1: the paid tasks are collected", f.queued.postCalls())
	}
	if run := f.run(t, id); run.Status != "completed" || run.KeywordsChecked != 1 {
		t.Errorf("run = %+v", run)
	}
}

func TestStartScheduledNeedsQueuedSupport(t *testing.T) {
	f := newCheckFixture(t, "one")
	cfg, _ := f.s.Repo.GetConfig(context.Background(), f.project, f.cfg.ID)
	if _, err := f.checks.StartScheduled(context.Background(), "org", cfg, 1); !errors.Is(err, ErrChecksUnavailable) {
		t.Fatalf("error = %v, want ErrChecksUnavailable", err)
	}
}

type countingRecorder struct {
	mu    sync.Mutex
	costs []dataforseo.Cost
}

func (r *countingRecorder) RecordDataForSEO(_ context.Context, _ string, c dataforseo.Cost) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.costs = append(r.costs, c)
	return nil
}

func TestDataForSEOQueuedTasks(t *testing.T) {
	var postBody []map[string]any
	var getPath string
	var taskGet atomic.Value
	taskGet.Store(`{"status_code":20000,"tasks":[{"status_code":20000,"status_message":"Ok.","cost":0.0004,"path":["v3"],"result":[{"items":[{"type":"organic","rank_group":3,"domain":"example.com","url":"https://example.com/"}]}]}]}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&postBody)
			entries := []map[string]any{}
			for i, task := range postBody {
				status := 20100
				if i == 1 {
					status = 40501 // one rejected entry
				}
				entries = append(entries, map[string]any{"id": fmt.Sprintf("id-%d", i), "status_code": status, "cost": 0.0006,
					"path": []string{"v3", "serp", "google", "organic", "task_post"}, "data": map[string]any{"tag": task["tag"]}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "tasks": entries})
			return
		}
		getPath = r.URL.Path
		_, _ = fmt.Fprint(w, taskGet.Load().(string))
	}))
	t.Cleanup(server.Close)
	recorder := &countingRecorder{}
	client, err := dataforseo.NewClient(dataforseo.Options{BaseURL: server.URL, APIKey: base64.StdEncoding.EncodeToString([]byte("fixture-user:fixture-password")), Recorder: recorder})
	if err != nil {
		t.Fatal(err)
	}
	p := DataForSEOSerp{Client: client}
	ctx := context.Background()
	city := "Pune"

	posted, err := p.PostTasks(ctx, "org", PostRequest{
		Tasks:        []TaskInput{{KeywordID: "k1", Keyword: "a", Device: "desktop"}, {KeywordID: "k2", Keyword: "b", Device: "mobile"}, {KeywordID: "k3", Keyword: "c", Device: "mobile"}},
		LocationCode: 2356, LanguageCode: "en", LocationName: &city, Depth: 30, TargetDomain: "example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 || posted[0].KeywordID != "k1" || posted[0].TaskID != "id-0" || posted[1].KeywordID != "k3" {
		t.Errorf("posted = %+v, want k1 and k3 (the rejected k2 is missing)", posted)
	}
	first := postBody[0]
	if first["tag"] != "k1:desktop" || first["location_name"] != "Pune" || first["location_code"] != nil || first["os"] != "windows" ||
		first["depth"] != float64(30) || first["stop_crawl_on_match"] == nil || postBody[1]["os"] != "android" {
		t.Errorf("request = %v", first)
	}
	if len(recorder.costs) != 3 {
		t.Errorf("recorded %d costs at post time, want one per task entry, accepted or not", len(recorder.costs))
	}

	got, err := p.CollectTask(ctx, "id/0", "example.com")
	if err != nil || pos(got.Result.Position) != 3 || got.Pending || got.Failed != "" {
		t.Fatalf("CollectTask() = %+v, %v", got, err)
	}
	if getPath != "/v3/serp/google/organic/task_get/advanced/id%2F0" && getPath != "/v3/serp/google/organic/task_get/advanced/id/0" {
		t.Errorf("task_get path = %q", getPath)
	}
	if len(recorder.costs) != 3 {
		t.Errorf("collecting recorded a cost (%d total); the task was charged when posted", len(recorder.costs))
	}

	for status, want := range map[int]string{20100: "pending", 40601: "pending", 40602: "pending"} {
		taskGet.Store(fmt.Sprintf(`{"status_code":20000,"tasks":[{"status_code":%d,"status_message":"queued"}]}`, status))
		if out, err := p.CollectTask(ctx, "x", "example.com"); err != nil || !out.Pending {
			t.Errorf("status %d = %+v, %v; want %s", status, out, err, want)
		}
	}
	taskGet.Store(`{"status_code":20000,"tasks":[{"status_code":40501,"status_message":"No Search Results."}]}`)
	if out, err := p.CollectTask(ctx, "x", "example.com"); err != nil || out.Failed != "" || out.Result.Position != nil || out.Result.SerpFeatures == nil {
		t.Errorf("no results = %+v, %v; want an empty result, not a failure", out, err)
	}
	taskGet.Store(`{"status_code":20000,"tasks":[{"status_code":50000,"status_message":"Internal Error"}]}`)
	if out, err := p.CollectTask(ctx, "x", "example.com"); err != nil || out.Failed != "Internal Error" {
		t.Errorf("failed task = %+v, %v", out, err)
	}
	taskGet.Store(`{"status_code":40000,"status_message":"bad"}`)
	if _, err := p.CollectTask(ctx, "x", "example.com"); err == nil {
		t.Error("an error envelope was accepted")
	}

	tooMany := make([]TaskInput, MaxTasksPerPost+1)
	for i := range tooMany {
		tooMany[i] = TaskInput{KeywordID: fmt.Sprint(i), Keyword: "k", Device: "desktop"}
	}
	before := len(recorder.costs)
	for name, req := range map[string]PostRequest{
		"too many tasks": {Tasks: tooMany, Depth: 10},
		"no tasks":       {Depth: 10},
		"bad depth":      {Tasks: tooMany[:1], Depth: 15},
	} {
		if _, err := p.PostTasks(ctx, "org", req); err == nil || !strings.Contains(err.Error(), "") {
			t.Errorf("%s was sent to the provider", name)
		}
	}
	if len(recorder.costs) != before {
		t.Error("an invalid post reached the provider, where it would be billed")
	}
}

func TestQueuedRunResumeSendsStoredFailedTasksLive(t *testing.T) {
	f := newQueuedFixture(t, "one")
	id, job := f.startScheduled(t)
	ctx := context.Background()
	posted := []PostedTask{
		{TaskInput: TaskInput{KeywordID: f.kw["one"], Keyword: "one", Device: "desktop"}, TaskID: "failed-before"},
		{TaskInput: TaskInput{KeywordID: f.kw["one"], Keyword: "one", Device: "mobile"}, TaskID: "still-posted"},
	}
	if err := f.checks.Tasks.RecordTasks(ctx, id, posted); err != nil {
		t.Fatal(err)
	}
	if err := f.checks.Tasks.SetTaskState(ctx, id, f.kw["one"], "desktop", TaskFailed); err != nil {
		t.Fatal(err)
	}
	if err := execute(t, f.checkFixture, job); err != nil {
		t.Fatal(err)
	}
	if f.queued.perTask["failed-before"] != 0 || f.queued.perTask["still-posted"] != 1 {
		t.Errorf("collects = %v, want the failed task skipped and the posted one collected", f.queued.perTask)
	}
	if f.serp.count() != 1 || f.serp.calls[0].Device != "desktop" {
		t.Errorf("live calls = %+v, want only the failed desktop pair", f.serp.calls)
	}
}

func TestQueuedRunCapsTaskGetsPerRound(t *testing.T) {
	keywords := make([]string, 260)
	for i := range keywords {
		keywords[i] = fmt.Sprintf("keyword %03d", i)
	}
	f := newQueuedFixture(t, keywords...)
	f.checks.PollWaits = []time.Duration{time.Minute, time.Minute, time.Minute}
	_, job := f.startScheduled(t)
	if err := execute(t, f.checkFixture, job); err != nil {
		t.Fatal(err)
	}
	if f.sleeps.Load() != 2 {
		t.Errorf("polling rounds = %d, want 2: 520 tasks need a round of %d and one more", f.sleeps.Load(), taskGetsPerRound)
	}
	if f.serp.count() != 0 {
		t.Errorf("live calls = %d, want none", f.serp.count())
	}
}
