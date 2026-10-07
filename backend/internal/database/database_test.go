package database

import (
	"context"
	"os"
	"testing"
	"time"
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

func TestOpenConnectsToPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := Open(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer pool.Close()

	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("query: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 = %d", one)
	}
}

func TestOpenFailsFastOnUnreachableDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Port 1 on localhost refuses connections, so the startup ping must fail.
	pool, err := Open(ctx, "postgres://seomarine@127.0.0.1:1/seomarine?connect_timeout=2")
	if err == nil {
		pool.Close()
		t.Fatal("Open() succeeded against an unreachable database")
	}
}

func TestOpenRejectsMalformedURL(t *testing.T) {
	pool, err := Open(context.Background(), "postgres://%zz")
	if err == nil {
		pool.Close()
		t.Fatal("Open() accepted a malformed URL")
	}
}
