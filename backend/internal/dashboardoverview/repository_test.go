package dashboardoverview

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

func dashboardDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("dashboard_overview_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	for _, sql := range []string{`CREATE TABLE projects(id text,organization_id text,domain text,archived_at text)`, `CREATE TABLE audits(id text,project_id text,status text,pages_crawled int,started_at text)`, `CREATE TABLE audit_issues(id text,audit_id text,page_url text,issue_type text,severity text)`, `CREATE TABLE backlink_snapshots(id serial,project_id text,domain text,rank int,backlinks bigint,referring_domains bigint,new_backlinks bigint,lost_backlinks bigint,new_referring_domains bigint,lost_referring_domains bigint,captured_at text)`, `INSERT INTO projects VALUES('p','o','example.com',NULL)`, `INSERT INTO audits VALUES('a','p','completed',12,'2026-10-10T10:00:00.000Z')`, `INSERT INTO audit_issues VALUES('1','a','https://example.com/a','title','critical'),('2','a','https://example.com/a','title','critical'),('3','a','https://example.com/b','links','warning'),('4','a','https://example.com/c','meta','info')`, `INSERT INTO backlink_snapshots(project_id,domain,rank,backlinks,referring_domains,captured_at) VALUES('p','example.com',12,500,40,'2026-10-10T11:00:00.000Z'),('p','old.example',9,20,3,'2026-10-10T11:30:00.000Z')`} {
		if _, err = db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	return db
}
func TestRepositoryReadsProjectScopedDashboardCards(t *testing.T) {
	db := dashboardDB(t)
	repo := Repository{DB: db}
	domain, err := repo.ProjectDomain(context.Background(), "p", "o")
	if err != nil || domain == nil || *domain != "example.com" {
		t.Fatalf("project domain=%v err=%v", domain, err)
	}
	audit, err := repo.LatestAudit(context.Background(), "p")
	if err != nil || audit == nil || audit.PagesCrawled != 12 || audit.TotalIssueTypes != 3 || len(audit.TopIssues) != 3 || audit.TopIssues[0].Count != 1 {
		t.Fatalf("audit=%+v err=%v", audit, err)
	}
	snapshot, found, err := repo.LatestBacklinkSnapshot(context.Background(), "p")
	if err != nil || !found || snapshot.Domain != "old.example" {
		t.Fatalf("snapshot=%+v found=%v err=%v", snapshot, found, err)
	}
	if _, err = repo.ProjectDomain(context.Background(), "p", "other-org"); err != ErrProjectNotFound {
		t.Fatalf("unauthorized organization err=%v", err)
	}
}
