package database

import (
	"context"
	"crypto/rand"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
)

// testDatabaseURL returns the Postgres URL for integration tests. CI always
// sets it, so a missing value there fails instead of silently skipping.
func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url != "" {
		return url
	}
	if os.Getenv("CI") != "" {
		t.Fatal("TEST_DATABASE_URL must be set in CI")
	}
	t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	return ""
}

// Every instance runs Migrate at startup, and Railway can start several at
// once: concurrent runs must all succeed and leave the schema migrated once.
func TestMigrateIsIdempotentAndConcurrencySafe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := openFresh(ctx, t)

	errs := make(chan error, 3)
	for range cap(errs) {
		go func() { errs <- Migrate(ctx, pool) }()
	}
	for range cap(errs) {
		if err := <-errs; err != nil {
			t.Fatalf("Migrate() error = %v", err)
		}
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	var applied int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM go_schema_migrations WHERE version_id = 1").Scan(&applied); err != nil {
		t.Fatalf("read migration history: %v", err)
	}
	if applied != 1 {
		t.Fatalf("migration 1 recorded %d times, want 1", applied)
	}

	// The table exists and its foreign key ties it to a legacy project.
	_, err := pool.Exec(ctx, "INSERT INTO go_analytics_sites (project_id, site_key) VALUES ('no-such-project', 'k')")
	if err == nil {
		t.Fatal("go_analytics_sites accepted a site for a project that does not exist")
	}
}

func TestMigrateFailsOnCanceledContext(t *testing.T) {
	pool := openFresh(context.Background(), t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Migrate(ctx, pool); err == nil {
		t.Fatal("Migrate() succeeded with a canceled context")
	}
}

// openFresh returns a pool on an empty private database: the baseline
// migration must build the whole legacy schema itself, as on a new production
// database. The baseline names the public schema, so a private schema is not
// enough.
func openFresh(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	admin, err := pgdb.Open(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	name := strings.ToLower("mig_" + rand.Text())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.WithoutCancel(ctx), "DROP DATABASE "+name+" WITH (FORCE)")
		admin.Close()
	})
	u, err := url.Parse(testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	pool, err := pgdb.Open(ctx, u.String())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
