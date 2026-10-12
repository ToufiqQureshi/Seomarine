package projects

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/database"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
)

func TestRepositoryConcurrentArchivePreservesOneActiveProject(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := projectTestPool(ctx, t)
	orgID, userID := seedProjectOwner(ctx, t, pool)
	firstID, secondID := "first-"+orgID, "second-"+orgID
	for _, id := range []string{firstID, secondID} {
		if _, err := pool.Exec(ctx, `INSERT INTO projects (id, organization_id, name) VALUES ($1, $2, $3)`, id, orgID, id); err != nil {
			t.Fatalf("insert project: %v", err)
		}
	}

	svc := Service{Store: Repository{DB: pool}}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var workers sync.WaitGroup
	for _, id := range []string{firstID, secondID} {
		workers.Add(1)
		go func(projectID string) {
			defer workers.Done()
			<-start
			errs <- svc.Archive(ctx, orgID, userID, projectID)
		}(id)
	}
	close(start)
	workers.Wait()
	close(errs)

	succeeded, conflicted := 0, 0
	for err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		var typed *Error
		if !errors.As(err, &typed) || typed.Code != "CONFLICT" {
			t.Fatalf("Archive() error = %v, want only a last-project conflict", err)
		}
		conflicted++
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("archive results: succeeded=%d conflicted=%d, want 1 each", succeeded, conflicted)
	}

	var active, archived int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE archived_at IS NULL), count(*) FILTER (WHERE archived_at IS NOT NULL) FROM projects WHERE organization_id=$1`, orgID).Scan(&active, &archived); err != nil {
		t.Fatalf("count projects after concurrent archive: %v", err)
	}
	if active != 1 || archived != 1 {
		t.Fatalf("active=%d archived=%d after concurrent archive, want 1 each", active, archived)
	}
}

func TestRepositoryRestoreDefaultConflictKeepsProjectArchived(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := projectTestPool(ctx, t)
	orgID, _ := seedProjectOwner(ctx, t, pool)
	activeID, archivedID := "active-default-"+orgID, "archived-default-"+orgID
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, organization_id, name, domain, archived_at) VALUES ($1, $2, 'Default', NULL, NULL), ($3, $2, 'Default', NULL, now())`, activeID, orgID, archivedID); err != nil {
		t.Fatalf("insert default projects: %v", err)
	}

	err := (Repository{DB: pool}).Restore(ctx, orgID, archivedID)
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != "CONFLICT" {
		t.Fatalf("Restore() error = %v, want CONFLICT", err)
	}
	var archivedAt *string
	if err := pool.QueryRow(ctx, `SELECT archived_at FROM projects WHERE id=$1`, archivedID).Scan(&archivedAt); err != nil {
		t.Fatalf("read restored project: %v", err)
	}
	if archivedAt == nil {
		t.Fatal("project became active despite the conflicting active Default project")
	}
}

func projectTestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
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
	return pool
}

func seedProjectOwner(ctx context.Context, t *testing.T, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	suffix := rand.Text()
	orgID, userID := "projects-test-org-"+suffix, "projects-test-user-"+suffix
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Project test', $2, now())`, orgID, orgID); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO "user" (id, name, email) VALUES ($1, 'Project test', $2)`, userID, userID+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO member (id, organization_id, user_id, role, created_at) VALUES ($1, $2, $3, 'owner', now())`, "projects-test-member-"+suffix, orgID, userID); err != nil {
		t.Fatalf("insert owner membership: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM organization WHERE id=$1`, orgID); err != nil {
			t.Errorf("delete test organization: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM "user" WHERE id=$1`, userID); err != nil {
			t.Errorf("delete test user: %v", err)
		}
	})
	return orgID, userID
}
