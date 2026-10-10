package projectcontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func openContextTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("project_context_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		admin.Close()
	})
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, stmt := range []string{
		`CREATE TABLE projects(id text PRIMARY KEY, archived_at text)`,
		`CREATE TABLE project_context_sections(project_id text NOT NULL,key text NOT NULL,title text,content text NOT NULL,updated_at text NOT NULL,updated_by text NOT NULL,PRIMARY KEY(project_id,key))`,
		`CREATE TABLE project_competitors(id text PRIMARY KEY,project_id text NOT NULL,domain text NOT NULL,name text,notes text,updated_at text NOT NULL,updated_by text NOT NULL,UNIQUE(project_id,domain))`,
		`CREATE TABLE project_key_pages(id text PRIMARY KEY,project_id text NOT NULL,url text NOT NULL,role text NOT NULL,topic text,notes text,updated_at text NOT NULL,updated_by text NOT NULL,UNIQUE(project_id,url))`,
		`CREATE TABLE project_research_log(id text PRIMARY KEY,project_id text NOT NULL,entry_date text NOT NULL,summary text NOT NULL,created_by text NOT NULL,created_at text NOT NULL)`,
		`CREATE TABLE report_templates(id text PRIMARY KEY,project_id text NOT NULL,name text NOT NULL,description text NOT NULL)`,
		`INSERT INTO projects(id) VALUES('project-1')`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("setup test schema: %v", err)
		}
	}
	return pool
}

func TestPostgresContextApplyIsAtomicAndPreservesOmittedFields(t *testing.T) {
	pool := openContextTestDB(t)
	svc := &Service{Repo: Repository{DB: pool}, Now: func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }}
	ctx := context.Background()
	updates := []json.RawMessage{
		json.RawMessage(`{"section":"current_goal","content":" Grow signups "}`),
		json.RawMessage(`{"customSection":"launch-plan","title":"Launch plan","content":" Ship in Q4 "}`),
		json.RawMessage(`{"addCompetitors":[{"domain":"https://www.acme.com","notes":"Strong content"}]}`),
		json.RawMessage(`{"addKeyPages":[{"url":"http://www.acme.com/pricing#faq","role":"money","topic":"Pricing"}]}`),
		json.RawMessage(`{"appendResearchLog":{"summary":"Keyword research."}}`),
	}
	got, err := svc.Apply(ctx, "project-1", updates, "user")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sections) != 1 || got.Sections[0].Content != "Grow signups" || len(got.CustomSections) != 1 || len(got.Competitors) != 1 || len(got.KeyPages) != 1 || len(got.ResearchLog) != 1 {
		t.Fatalf("context=%+v", got)
	}
	if got.KeyPages[0].URL != "https://acme.com/pricing" || got.KeyPages[0].Role != "money" {
		t.Fatalf("key page=%+v", got.KeyPages[0])
	}
	_, err = svc.Apply(ctx, "project-1", []json.RawMessage{json.RawMessage(`{"section":"positioning","content":"Must roll back"}`), json.RawMessage(`{"addCompetitors":[{"domain":"not a domain"}]}`)}, "user")
	if err == nil {
		t.Fatal("invalid later op unexpectedly committed")
	}
	got, err = svc.Get(ctx, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sections) != 1 || got.Sections[0].Key != "current_goal" {
		t.Fatalf("failed batch partially changed sections: %+v", got.Sections)
	}
}

func TestPostgresContextPreservesKeyPageRoleAndTrimsResearchLog(t *testing.T) {
	pool := openContextTestDB(t)
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	svc := &Service{Repo: Repository{DB: pool}, Now: func() time.Time { return now }}
	ctx := context.Background()
	for i := 0; i < 22; i++ {
		entryDate := now.Add(-time.Duration(i) * 24 * time.Hour).Format("2006-01-02")
		createdAt := now.Add(-time.Duration(i) * time.Minute).Format("2006-01-02T15:04:05.000Z")
		if _, err := pool.Exec(ctx, `INSERT INTO project_research_log(id,project_id,entry_date,summary,created_by,created_at) VALUES($1,'project-1',$2,$3,'sam',$4)`, fmt.Sprintf("entry-%d", i), entryDate, fmt.Sprintf("Entry %d", i), createdAt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO project_research_log(id,project_id,entry_date,summary,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6)`, "expired", "project-1", now.AddDate(0, 0, -100).Format("2006-01-02"), "Expired", "sam", now.AddDate(0, 0, -100).Format("2006-01-02T15:04:05.000Z")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Apply(ctx, "project-1", []json.RawMessage{json.RawMessage(`{"appendResearchLog":{"summary":"Latest"}}`)}, "sam"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Apply(ctx, "project-1", []json.RawMessage{json.RawMessage(`{"addKeyPages":[{"url":"example.com/pricing","role":"money"}]}`)}, "user")
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Apply(ctx, "project-1", []json.RawMessage{json.RawMessage(`{"addKeyPages":[{"url":"example.com/pricing","topic":"Pricing"}]}`)}, "mcp")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.KeyPages) != 1 || got.KeyPages[0].Role != "money" || got.KeyPages[0].Topic == nil {
		t.Fatalf("key page upsert lost fields: %+v", got.KeyPages)
	}
	if len(got.ResearchLog) != 20 {
		t.Fatalf("research log returned %d rows", len(got.ResearchLog))
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM project_research_log`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 23 {
		t.Fatalf("research log row count=%d, want 22 retained plus current", count)
	}
	var expired int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM project_research_log WHERE id='expired'`).Scan(&expired); err != nil {
		t.Fatal(err)
	}
	if expired != 0 {
		t.Fatal("expired log row was not pruned")
	}
}

func TestPostgresContextConcurrentWritesRespectCustomSectionCap(t *testing.T) {
	pool := openContextTestDB(t)
	svc := &Service{Repo: Repository{DB: pool}}
	const writers = 24
	var wg sync.WaitGroup
	var succeeded atomic.Int32
	var rejected atomic.Int32
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			slug := fmt.Sprintf("note-%02d", index)
			_, err := svc.Apply(context.Background(), "project-1", []json.RawMessage{json.RawMessage(fmt.Sprintf(`{"customSection":%q,"content":"value"}`, slug))}, "user")
			if err == nil {
				succeeded.Add(1)
				return
			}
			var typed *Error
			if errors.As(err, &typed) && typed.Code == "VALIDATION_ERROR" {
				rejected.Add(1)
				return
			}
			errCh <- err
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	if succeeded.Load() != maxCustomSections || rejected.Load() != writers-maxCustomSections {
		t.Fatalf("successful writes=%d rejected=%d; want %d and %d", succeeded.Load(), rejected.Load(), maxCustomSections, writers-maxCustomSections)
	}
	got, err := svc.Get(context.Background(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.CustomSections) != maxCustomSections {
		t.Fatalf("custom sections=%d; want %d", len(got.CustomSections), maxCustomSections)
	}
}
