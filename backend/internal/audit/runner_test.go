package audit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/jobs"
)

// auditPageHTML is a healthy page so the crawl produces no issues.
const auditPageHTML = `<!doctype html><html><head>
<title>A reasonably descriptive audit test title</title>
<meta name="description" content="A meta description that is comfortably longer than the minimum length the audit checks for.">
</head><body><h1>Hello audit</h1><a href="/a">Go to A</a>
<p>` + auditFiller + `</p></body></html>`

const auditFiller = "lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua ut enim ad minim veniam quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur excepteur sint occaecat cupidatat non proident sunt in culpa qui officia deserunt mollit anim id est laborum"

// auditTestServer serves a tiny crawlable site: robots.txt, a sitemap and two
// HTML pages.
func auditTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	page := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(auditPageHTML))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("User-agent: *\nSitemap: http://audit.test/sitemap.xml\n"))
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset><url><loc>http://audit.test/a</loc></url></urlset>`))
	})
	mux.HandleFunc("/a", page)
	mux.HandleFunc("/", page)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// newTestRunner builds a Runner whose guard dials server for every host.
func newTestRunner(store Store, server *httptest.Server, lighthouse LighthouseProvider, rendering RenderingMeter) *Runner {
	guard := &Guard{
		Resolver: fakeResolver{addrs: map[string][]string{"audit.test": {"93.184.216.34"}}},
		Client:   dialAllTo(server),
	}
	return NewRunner(RunnerConfig{
		Repository: store, Guard: guard, Lighthouse: lighthouse, Rendering: rendering,
		Logger: discardLogger(), Now: time.Now,
	})
}

func seedAudit(t *testing.T, store *memoryStore, config Config) {
	t.Helper()
	encoded, err := MarshalAuditConfig(config)
	if err != nil {
		t.Fatalf("encode config: %v", err)
	}
	if err := store.CreateAudit(context.Background(), CreateAuditInput{
		ID: "audit-1", ProjectID: "proj-1", StartedByUserID: "user-1", StartURL: "http://audit.test/",
		WorkflowInstanceID: "audit-1", Config: encoded, PagesTotal: config.MaxPages, LighthouseTotal: 0,
	}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}
}

func TestRunnerCrawlsAndCompletes(t *testing.T) {
	server := auditTestServer(t)
	store := newMemoryStore()
	seedAudit(t, store, Config{MaxPages: 10, LighthouseStrategy: LighthouseNone})
	runner := newTestRunner(store, server, nil, nil)

	job := JobPayload{AuditID: "audit-1", ProjectID: "proj-1", StartURL: "http://audit.test/", OrganizationID: "org-1", Config: Config{MaxPages: 10, LighthouseStrategy: LighthouseNone}}
	if err := runner.Run(context.Background(), job); err != nil {
		t.Fatalf("run: %v", err)
	}
	audit, err := store.GetAuditForProject(context.Background(), "audit-1", "proj-1")
	if err != nil {
		t.Fatalf("load audit: %v", err)
	}
	if audit.Status != "completed" {
		t.Fatalf("status = %q, want completed", audit.Status)
	}
	if len(store.pages["audit-1"]) < 2 {
		t.Fatalf("expected the homepage and /a to be crawled, got %d", len(store.pages["audit-1"]))
	}
	for _, page := range store.pages["audit-1"] {
		if page.FetchClass != FetchOK {
			t.Errorf("page %s fetchClass = %s", page.URL, page.FetchClass)
		}
		if page.IsHTML && page.WordCount == 0 {
			t.Errorf("page %s has no word count", page.URL)
		}
	}
}

func TestRunnerRunsLighthouse(t *testing.T) {
	server := auditTestServer(t)
	store := newMemoryStore()
	seedAudit(t, store, Config{MaxPages: 10, LighthouseStrategy: LighthouseAuto})
	provider := &fakeLighthouseProvider{}
	runner := newTestRunner(store, server, provider, nil)

	job := JobPayload{AuditID: "audit-1", ProjectID: "proj-1", StartURL: "http://audit.test/", OrganizationID: "org-1", Config: Config{MaxPages: 10, LighthouseStrategy: LighthouseAuto}}
	if err := runner.Run(context.Background(), job); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(store.lighthouse["audit-1"]) == 0 {
		t.Fatal("expected lighthouse results to be stored")
	}
	audit, _ := store.GetAuditForProject(context.Background(), "audit-1", "proj-1")
	if audit.LighthouseCompleted == 0 {
		t.Fatalf("lighthouseCompleted = %d", audit.LighthouseCompleted)
	}
}

func TestRunnerMarksAuditFailedOnStoreError(t *testing.T) {
	server := auditTestServer(t)
	store := newMemoryStore()
	seedAudit(t, store, Config{MaxPages: 10, LighthouseStrategy: LighthouseNone})
	store.failInsert = true
	runner := newTestRunner(store, server, nil, nil)

	job := JobPayload{AuditID: "audit-1", ProjectID: "proj-1", StartURL: "http://audit.test/", OrganizationID: "org-1", Config: Config{MaxPages: 10, LighthouseStrategy: LighthouseNone}}
	if err := runner.Run(context.Background(), job); err == nil {
		t.Fatal("expected the run to fail")
	}
	audit, _ := store.GetAuditForProject(context.Background(), "audit-1", "proj-1")
	if audit.Status != "failed" {
		t.Fatalf("status = %q, want failed", audit.Status)
	}
}

func TestRunnerReleasesRenderingLocksOnFailure(t *testing.T) {
	server := auditTestServer(t)
	store := newMemoryStore()
	seedAudit(t, store, Config{MaxPages: 10, LighthouseStrategy: LighthouseNone})
	store.failInsert = true
	meter := &fakeRenderingMeter{}
	runner := newTestRunner(store, server, nil, meter)

	job := JobPayload{
		AuditID: "audit-1", ProjectID: "proj-1", StartURL: "http://audit.test/", OrganizationID: "org-1",
		Config:         Config{MaxPages: 10, LighthouseStrategy: LighthouseNone},
		RenderingLocks: []RenderingLock{{LockID: "lock-1", EstimatedCredits: 5}},
	}
	if err := runner.Run(context.Background(), job); err == nil {
		t.Fatal("expected the run to fail")
	}
	if meter.released != 1 {
		t.Fatalf("expected the rendering hold to be released, meter = %+v", meter)
	}
}

func TestRunnerSettlesRenderingLocksOnSuccess(t *testing.T) {
	server := auditTestServer(t)
	store := newMemoryStore()
	seedAudit(t, store, Config{MaxPages: 10, LighthouseStrategy: LighthouseNone})
	meter := &fakeRenderingMeter{}
	runner := newTestRunner(store, server, nil, meter)

	job := JobPayload{
		AuditID: "audit-1", ProjectID: "proj-1", StartURL: "http://audit.test/", OrganizationID: "org-1",
		Config:         Config{MaxPages: 10, LighthouseStrategy: LighthouseNone},
		RenderingLocks: []RenderingLock{{LockID: "lock-1", EstimatedCredits: 5}},
	}
	if err := runner.Run(context.Background(), job); err != nil {
		t.Fatalf("run: %v", err)
	}
	if meter.settled != 1 {
		t.Fatalf("expected the hold to be settled, meter = %+v", meter)
	}
}

func TestRunnerRejectsBlockedOrigin(t *testing.T) {
	store := newMemoryStore()
	runner := NewRunner(RunnerConfig{Repository: store, Logger: discardLogger()})
	err := runner.Run(context.Background(), JobPayload{AuditID: "audit-1", StartURL: "http://127.0.0.1/"})
	if err == nil {
		t.Fatal("expected a blocked origin to fail")
	}
}

func TestRunnerHandlerDecodesJobs(t *testing.T) {
	server := auditTestServer(t)
	store := newMemoryStore()
	seedAudit(t, store, Config{MaxPages: 10, LighthouseStrategy: LighthouseNone})
	runner := newTestRunner(store, server, nil, nil)

	payload, err := json.Marshal(JobPayload{AuditID: "audit-1", ProjectID: "proj-1", StartURL: "http://audit.test/", Config: Config{MaxPages: 10, LighthouseStrategy: LighthouseNone}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	handler := RunnerHandler(runner)
	if err := handler(context.Background(), jobs.Job{ID: 1, Payload: payload}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if _, err := store.GetAuditForProject(context.Background(), "audit-1", "proj-1"); err != nil {
		t.Fatalf("audit missing: %v", err)
	}
	if err := handler(context.Background(), jobs.Job{ID: 2, Payload: []byte(`not json`)}); err == nil {
		t.Fatal("expected a decode error")
	}
	if err := handler(context.Background(), jobs.Job{ID: 3, Payload: []byte(`{}`)}); err == nil {
		t.Fatal("expected a missing audit id error")
	}
}

func TestQueueSchedulerRequiresQueue(t *testing.T) {
	scheduler := &QueueScheduler{}
	if err := scheduler.EnqueueAudit(context.Background(), "audit-1", JobPayload{}); err == nil {
		t.Fatal("expected an error without a queue")
	}
	if err := scheduler.TerminateAudit(context.Background(), "audit-1"); err != nil {
		t.Fatalf("terminate should be a no-op: %v", err)
	}
}

func TestRunnerContextCanceled(t *testing.T) {
	server := auditTestServer(t)
	store := newMemoryStore()
	seedAudit(t, store, Config{MaxPages: 10, LighthouseStrategy: LighthouseNone})
	runner := newTestRunner(store, server, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runner.Run(ctx, JobPayload{AuditID: "audit-1", StartURL: "http://audit.test/", Config: Config{MaxPages: 10}})
	if err == nil {
		t.Fatal("expected a canceled context to fail the run")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

// ctxStore fails writes once their context is done, as Postgres does.
type ctxStore struct{ *memoryStore }

func (s ctxStore) InsertLighthouseResults(ctx context.Context, auditID string, results []LighthouseRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.memoryStore.InsertLighthouseResults(ctx, auditID, results)
}

func (s ctxStore) UpdateAuditProgress(ctx context.Context, auditID, workflowInstanceID string, update ProgressUpdate) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.memoryStore.UpdateAuditProgress(ctx, auditID, workflowInstanceID, update)
}

// slowAfterFirstProvider answers the first check, then hangs until the caller
// gives up, like a provider that stopped responding mid-audit.
type slowAfterFirstProvider struct{ calls int }

func (p *slowAfterFirstProvider) FetchLighthouse(ctx context.Context, _ string, input LighthouseInput) (StoredLighthousePayload, error) {
	p.calls++
	if p.calls == 1 {
		return ParsePageSpeedPayload([]byte(pageSpeedPayload), input, time.Now())
	}
	<-ctx.Done()
	return StoredLighthousePayload{}, ctx.Err()
}

func TestRegressionLighthouseDeadlineKeepsCompletedChecks(t *testing.T) {
	memory := newMemoryStore()
	seedAudit(t, memory, Config{MaxPages: 10, LighthouseStrategy: LighthouseAuto})
	if err := memory.InsertCrawledBatch(context.Background(), "audit-1", []CrawledPageResult{
		{ID: "p1", URL: "http://audit.test/", StatusCode: 200, FetchClass: FetchOK},
		{ID: "p2", URL: "http://audit.test/blog/post-one", StatusCode: 200, FetchClass: FetchOK},
	}, nil); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(RunnerConfig{Repository: ctxStore{memory}, Lighthouse: &slowAfterFirstProvider{}, Logger: discardLogger(), Now: time.Now})
	runner.minCheckTime = 50 * time.Millisecond
	job := JobPayload{AuditID: "audit-1", ProjectID: "proj-1", StartURL: "http://audit.test/", OrganizationID: "org-1", Config: Config{MaxPages: 10, LighthouseStrategy: LighthouseAuto}}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := runner.lighthousePhase(ctx, job); err != nil {
		t.Fatalf("lighthouse phase failed the audit: %v; a slow provider must not", err)
	}
	completed := 0
	for _, record := range memory.lighthouse["audit-1"] {
		if record.ErrorMessage == nil {
			completed++
		}
	}
	if completed != 1 || len(memory.lighthouse["audit-1"]) != 4 {
		t.Fatalf("stored %d results, %d completed; want all 4 recorded and the finished check kept", len(memory.lighthouse["audit-1"]), completed)
	}
	last := memory.lighthouse["audit-1"][3]
	if last.ErrorMessage == nil || !strings.Contains(*last.ErrorMessage, "ran out of time") {
		t.Fatalf("last check = %+v, want it skipped once the budget is spent instead of started", last)
	}
}
