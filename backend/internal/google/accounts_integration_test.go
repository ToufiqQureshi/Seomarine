package google

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func accountTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UnixNano()
	schema := fmt.Sprintf("google_account_test_%d", stamp)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
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
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, statement := range []string{
		`CREATE TABLE account (id text PRIMARY KEY, user_id text NOT NULL, provider_id text NOT NULL, account_id text NOT NULL)`,
		`CREATE TABLE gsc_connections (id text PRIMARY KEY, connected_by_user_id text NOT NULL, gsc_account_id text)`,
		`CREATE TABLE ga4_connections (id text PRIMARY KEY, connected_by_user_id text NOT NULL, ga4_account_id text NOT NULL)`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func TestAccountRemovalIsOwnerScopedAndAtomic(t *testing.T) {
	pool := accountTestDB(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	for _, statement := range []string{
		`INSERT INTO account VALUES ('grant-a','user-a','google-search-console','google-a'),('grant-b','user-b','google-search-console','google-a'),('grant-ga4','user-a','google-analytics','google-a'),('grant-other','user-a','google-search-console','google-b')`,
		`INSERT INTO gsc_connections VALUES ('map-a','user-a','google-a'),('legacy-a','user-a',NULL),('map-b','user-b','google-a'),('other-a','user-a','google-b')`,
		`INSERT INTO ga4_connections VALUES ('ga4-map','user-a','google-a')`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	repo := AccountRepository{Pool: pool}
	count, err := repo.RemovalImpact(ctx, "user-a", "gsc", "google-a")
	if err != nil || count != 2 {
		t.Fatalf("impact = %d, %v; want 2", count, err)
	}
	if err := repo.Remove(ctx, "user-a", "gsc", "google-a"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Remove(ctx, "user-a", "gsc", "google-a"); err != nil {
		t.Fatalf("retry removal: %v", err)
	}
	for _, tc := range []struct {
		query string
		want  int
	}{
		{`SELECT count(*) FROM account WHERE id='grant-a'`, 0},
		{`SELECT count(*) FROM account WHERE id IN ('grant-b','grant-ga4','grant-other')`, 3},
		{`SELECT count(*) FROM gsc_connections WHERE id IN ('map-a','legacy-a')`, 0},
		{`SELECT count(*) FROM gsc_connections WHERE id IN ('map-b','other-a')`, 2},
		{`SELECT count(*) FROM ga4_connections`, 1},
	} {
		var got int
		if err := pool.QueryRow(ctx, tc.query).Scan(&got); err != nil || got != tc.want {
			t.Errorf("%s: got %d, %v; want %d", tc.query, got, err, tc.want)
		}
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION fail_grant_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER fail_grant_delete BEFORE DELETE ON account FOR EACH ROW EXECUTE FUNCTION fail_grant_delete()`); err != nil {
		t.Fatal(err)
	}
	if err := repo.Remove(ctx, "user-b", "gsc", "google-a"); err == nil {
		t.Fatal("grant deletion failure must roll back mapping deletion")
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM gsc_connections WHERE id='map-b'`).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("mapping after rollback = %d, %v", remaining, err)
	}
}
