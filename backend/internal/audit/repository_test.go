package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
)

// auditTestPool opens the test database, applies migrations and ensures a
// minimal projects table exists for the organization join. It skips when
// TEST_DATABASE_URL is unset outside CI.
func auditTestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	pool, err := pgdb.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS projects (
		id text PRIMARY KEY,
		organization_id text NOT NULL,
		name text,
		archived_at timestamptz)`); err != nil {
		t.Fatalf("ensure projects table: %v", err)
	}
	return pool
}

func suffix(t *testing.T) string {
	t.Helper()
	var value [6]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(value[:])
}

// seedProject inserts a project and returns its id.
func seedProject(ctx context.Context, t *testing.T, pool *pgxpool.Pool, organizationID string) string {
	t.Helper()
	projectID := "proj-" + suffix(t)
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, organization_id, name) VALUES ($1, $2, $3)`, projectID, organizationID, "test"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.WithoutCancel(ctx), `DELETE FROM projects WHERE id = $1`, projectID); err != nil {
			t.Errorf("clean project: %v", err)
		}
	})
	return projectID
}

func seedRepositoryAudit(ctx context.Context, t *testing.T, repo *Repository, auditID, projectID string) {
	t.Helper()
	config, _ := MarshalAuditConfig(Config{MaxPages: 50, LighthouseStrategy: LighthouseAuto})
	if err := repo.CreateAudit(ctx, CreateAuditInput{
		ID: auditID, ProjectID: projectID, StartedByUserID: "user-1", StartURL: "https://example.com/",
		WorkflowInstanceID: auditID, Config: config, PagesTotal: 50, LighthouseTotal: 20,
	}); err != nil {
		t.Fatalf("create audit: %v", err)
	}
	t.Cleanup(func() {
		if _, err := repo.pool.Exec(context.WithoutCancel(ctx), `DELETE FROM go_audits WHERE id = $1`, auditID); err != nil {
			t.Errorf("clean audit: %v", err)
		}
	})
}

func samplePage(id, rawURL string) CrawledPageResult {
	depth := 1
	return CrawledPageResult{
		ID: id, URL: rawURL, StatusCode: 200, FetchClass: FetchOK, Title: "A reasonable page title",
		MetaDescription: "A meta description long enough to pass the minimum length check the audit applies.",
		WordCount:       200, ContentHash: "hash-" + id, IsIndexable: true, IsHTML: true, ResponseTimeMs: 120,
		CrawlDepth: &depth, HeadingOrder: []int{1, 2}, H1Count: 1,
		Images: []ImageRef{{Src: "/a.png", Alt: "a"}}, HreflangTags: []string{"en"},
		Links: []PageLink{{TargetURL: "https://example.com/other", IsInternal: true}},
	}
}

func TestRepositoryAuditLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := auditTestPool(ctx, t)
	repo := NewRepository(pool)
	projectID := seedProject(ctx, t, pool, "org-1")
	otherProject := seedProject(ctx, t, pool, "org-2")
	auditID := "audit-" + suffix(t)
	seedRepositoryAudit(ctx, t, repo, auditID, projectID)

	if _, err := repo.GetAuditForProject(ctx, auditID, projectID); err != nil {
		t.Fatalf("load audit: %v", err)
	}
	// Tenant isolation: another project can never read it.
	if _, err := repo.GetAuditForProject(ctx, auditID, otherProject); !errors.Is(err, ErrAuditNotFound) {
		t.Fatalf("cross-project read error = %v, want ErrAuditNotFound", err)
	}
	// And can never change it.
	if err := repo.CompleteAudit(ctx, auditID, auditID, 3, 50); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := repo.GetAuditForProject(ctx, auditID, otherProject); !errors.Is(err, ErrAuditNotFound) {
		t.Fatalf("cross-project read after update error = %v", err)
	}
	audit, err := repo.GetAuditForProject(ctx, auditID, projectID)
	if err != nil || audit.Status != "completed" {
		t.Fatalf("audit = %+v err = %v", audit, err)
	}
	// A completed audit cannot be failed afterwards.
	if err := repo.FailAudit(ctx, auditID, auditID, ErrorInfo{ErrorCode: ErrorUnknown}, ""); !errors.Is(err, ErrAuditNotFound) {
		t.Fatalf("failing a completed audit error = %v, want ErrAuditNotFound", err)
	}

	history, err := repo.ListAuditsByProject(ctx, projectID)
	if err != nil || len(history) != 1 {
		t.Fatalf("history = %d err = %v", len(history), err)
	}

	if err := repo.DeleteAuditForProject(ctx, auditID, otherProject); !errors.Is(err, ErrAuditNotFound) {
		t.Fatalf("cross-project delete error = %v, want ErrAuditNotFound", err)
	}
	if err := repo.DeleteAuditForProject(ctx, auditID, projectID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestRepositoryInsertCrawledBatchIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := auditTestPool(ctx, t)
	repo := NewRepository(pool)
	projectID := seedProject(ctx, t, pool, "org-1")
	auditID := "audit-" + suffix(t)
	seedRepositoryAudit(ctx, t, repo, auditID, projectID)

	page := samplePage("page-1", "https://example.com/")
	issue := DetectedIssue{IssueType: IssueMissingTitle, PageURL: page.URL, PageID: &page.ID}
	if err := repo.InsertCrawledBatch(ctx, auditID, []CrawledPageResult{page}, []DetectedIssue{issue}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// A retried step re-inserts the same batch; ids make it a no-op.
	if err := repo.InsertCrawledBatch(ctx, auditID, []CrawledPageResult{page}, []DetectedIssue{issue}); err != nil {
		t.Fatalf("retry insert: %v", err)
	}

	results, err := repo.GetAuditResultsForProject(ctx, auditID, projectID)
	if err != nil {
		t.Fatalf("load results: %v", err)
	}
	if len(results.Pages) != 1 {
		t.Fatalf("pages = %d, want 1 (idempotent upsert)", len(results.Pages))
	}
	if len(results.Issues) != 1 {
		t.Fatalf("issues = %d, want 1 (insert-or-ignore)", len(results.Issues))
	}
	if results.Pages[0].HeadingOrder == nil || len(results.Pages[0].HeadingOrder) != 2 {
		t.Fatalf("heading order round-trip failed: %v", results.Pages[0].HeadingOrder)
	}
	if results.Pages[0].FetchClass != FetchOK || results.Pages[0].CrawlDepth == nil {
		t.Fatalf("page round-trip failed: %+v", results.Pages[0])
	}

	count, err := repo.CountPagesByFetchClass(ctx, auditID, FetchOK)
	if err != nil || count != 1 {
		t.Fatalf("count = %d err = %v", count, err)
	}
}

func TestRepositoryMultipageChecks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := auditTestPool(ctx, t)
	repo := NewRepository(pool)
	projectID := seedProject(ctx, t, pool, "org-1")
	auditID := "audit-" + suffix(t)
	seedRepositoryAudit(ctx, t, repo, auditID, projectID)

	first := samplePage("page-1", "https://example.com/a")
	second := samplePage("page-2", "https://example.com/b")
	second.ContentHash = first.ContentHash
	shell := samplePage("page-3", "https://example.com/c")
	shell.JavaScriptShell = true
	issues := []DetectedIssue{
		{IssueType: IssueJavaScriptRenderingSuspected, PageURL: shell.URL, PageID: &shell.ID},
	}
	if err := repo.InsertCrawledBatch(ctx, auditID, []CrawledPageResult{first, second, shell}, issues); err != nil {
		t.Fatalf("insert batch: %v", err)
	}

	shellIDs, err := repo.ShellPageIDs(ctx, auditID)
	if err != nil || len(shellIDs) != 1 {
		t.Fatalf("shell ids = %v err = %v", shellIDs, err)
	}

	found, hasShells, err := repo.RunMultipageChecks(ctx, auditID)
	if err != nil {
		t.Fatalf("multipage checks: %v", err)
	}
	if !hasShells {
		t.Error("expected unrendered shells to be reported")
	}
	duplicates := 0
	for _, issue := range found {
		if issue.IssueType == IssueDuplicateContent || issue.IssueType == IssueDuplicateTitle || issue.IssueType == IssueDuplicateMetaDescription {
			duplicates++
		}
	}
	if duplicates == 0 {
		t.Fatalf("expected duplicate issues, got %v", found)
	}
}

func TestRepositoryLighthouseIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := auditTestPool(ctx, t)
	repo := NewRepository(pool)
	projectID := seedProject(ctx, t, pool, "org-1")
	auditID := "audit-" + suffix(t)
	seedRepositoryAudit(ctx, t, repo, auditID, projectID)
	page := samplePage("page-1", "https://example.com/")
	if err := repo.InsertCrawledBatch(ctx, auditID, []CrawledPageResult{page}, nil); err != nil {
		t.Fatalf("insert page: %v", err)
	}

	score := 90
	payload := `{"version":2}`
	results := []LighthouseRecord{{PageID: page.ID, URL: page.URL, Strategy: "mobile", PerformanceScore: &score, PayloadJSON: &payload}}
	if err := repo.InsertLighthouseResults(ctx, auditID, results); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := repo.InsertLighthouseResults(ctx, auditID, results); err != nil {
		t.Fatalf("retry insert: %v", err)
	}
	loaded, err := repo.GetAuditResultsForProject(ctx, auditID, projectID)
	if err != nil {
		t.Fatalf("load results: %v", err)
	}
	if len(loaded.Lighthouse) != 1 {
		t.Fatalf("lighthouse = %d, want 1", len(loaded.Lighthouse))
	}
	if loaded.Lighthouse[0].PayloadSizeBytes == nil || *loaded.Lighthouse[0].PayloadSizeBytes != len(payload) {
		t.Fatalf("payload size = %v", loaded.Lighthouse[0].PayloadSizeBytes)
	}
}

func TestRepositoryAuditUsagePerOrganization(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := auditTestPool(ctx, t)
	repo := NewRepository(pool)
	organizationID := "org-" + suffix(t)
	firstProject := seedProject(ctx, t, pool, organizationID)
	secondProject := seedProject(ctx, t, pool, organizationID)
	auditOne := "audit-" + suffix(t)
	auditTwo := "audit-" + suffix(t)
	seedRepositoryAudit(ctx, t, repo, auditOne, firstProject)
	seedRepositoryAudit(ctx, t, repo, auditTwo, secondProject)

	usage, err := repo.AuditUsageForOrganization(ctx, organizationID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if usage.RunningCount != 2 || usage.CapacityUnits != 140 {
		t.Fatalf("usage = %+v, want 2 running and 140 units", usage)
	}
}

func TestRepositoryUpdateProgressFencesWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := auditTestPool(ctx, t)
	repo := NewRepository(pool)
	projectID := seedProject(ctx, t, pool, "org-1")
	auditID := "audit-" + suffix(t)
	seedRepositoryAudit(ctx, t, repo, auditID, projectID)

	phase := "crawling"
	pages := 5
	if err := repo.UpdateAuditProgress(ctx, auditID, auditID, ProgressUpdate{PagesCrawled: &pages, CurrentPhase: &phase}); err != nil {
		t.Fatalf("update: %v", err)
	}
	// A superseded workflow instance cannot write.
	if err := repo.UpdateAuditProgress(ctx, auditID, "other-instance", ProgressUpdate{PagesCrawled: &pages}); !errors.Is(err, ErrAuditNotFound) {
		t.Fatalf("stale update error = %v, want ErrAuditNotFound", err)
	}
	audit, err := repo.GetAuditForProject(ctx, auditID, projectID)
	if err != nil || audit.PagesCrawled != 5 || audit.CurrentPhase == nil || *audit.CurrentPhase != "crawling" {
		t.Fatalf("audit = %+v err = %v", audit, err)
	}
}

func TestRepositoryGetLighthouseForProject(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := auditTestPool(ctx, t)
	repo := NewRepository(pool)
	projectID := seedProject(ctx, t, pool, "org-1")
	otherProject := seedProject(ctx, t, pool, "org-2")
	auditID := "audit-" + suffix(t)
	seedRepositoryAudit(ctx, t, repo, auditID, projectID)
	page := samplePage("page-1", "https://example.com/")
	if err := repo.InsertCrawledBatch(ctx, auditID, []CrawledPageResult{page}, nil); err != nil {
		t.Fatalf("insert page: %v", err)
	}
	payload := `{"version":2}`
	if err := repo.InsertLighthouseResults(ctx, auditID, []LighthouseRecord{
		{PageID: page.ID, URL: page.URL, Strategy: "mobile", PayloadJSON: &payload},
		{PageID: page.ID, URL: page.URL, Strategy: "desktop", ErrorMessage: ptr("failed")},
	}); err != nil {
		t.Fatalf("insert lighthouse: %v", err)
	}
	results, err := repo.GetAuditResultsForProject(ctx, auditID, projectID)
	if err != nil || len(results.Lighthouse) != 2 {
		t.Fatalf("load results: %v %+v", err, results.Lighthouse)
	}
	withPayload, withoutPayload := "", ""
	for _, record := range results.Lighthouse {
		if record.HasPayload {
			withPayload = record.ID
		} else {
			withoutPayload = record.ID
		}
	}
	if withPayload == "" || withoutPayload == "" {
		t.Fatalf("hasPayload flags wrong: %+v", results.Lighthouse)
	}

	detail, err := repo.GetLighthouseForProject(ctx, withPayload, projectID)
	if err != nil || detail.PayloadJSON != payload || detail.PageURL != page.URL || detail.Strategy != "mobile" || detail.StartedAt.IsZero() {
		t.Fatalf("detail = %+v err = %v", detail, err)
	}
	for name, lookup := range map[string]struct{ id, project string }{
		"another project":     {withPayload, otherProject},
		"result without data": {withoutPayload, projectID},
		"unknown result":      {"missing", projectID},
	} {
		if _, err := repo.GetLighthouseForProject(ctx, lookup.id, lookup.project); !errors.Is(err, ErrLighthouseNotFound) {
			t.Fatalf("%s: err = %v, want ErrLighthouseNotFound", name, err)
		}
	}
}
