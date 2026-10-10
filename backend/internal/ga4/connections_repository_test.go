package ga4

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupRepositoryDB(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("ga4_setup_test_%d", time.Now().UnixNano())
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
	config, err := pgxpool.ParseConfig(url)
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
		`CREATE TABLE organization (id text PRIMARY KEY)`,
		`CREATE TABLE projects (id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organization(id))`,
		`CREATE TABLE member (organization_id text NOT NULL, user_id text NOT NULL, role text NOT NULL)`,
		`CREATE TABLE account (user_id text NOT NULL, provider_id text NOT NULL, account_id text NOT NULL)`,
		`CREATE TABLE ga4_connections (id text PRIMARY KEY, project_id text UNIQUE NOT NULL REFERENCES projects(id), organization_id text NOT NULL, property_id text NOT NULL, property_display_name text NOT NULL, property_time_zone text NOT NULL, property_currency_code text NOT NULL, connected_by_user_id text NOT NULL, ga4_account_id text NOT NULL, connected_account_email text, created_at text NOT NULL DEFAULT to_char(now() AT TIME ZONE 'utc','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'), updated_at text NOT NULL DEFAULT to_char(now() AT TIME ZONE 'utc','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'))`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func TestSetupRepositoryScopesConnectionAndPersistsLegacyShape(t *testing.T) {
	pool := setupRepositoryDB(t)
	ctx := context.Background()
	for _, statement := range []string{
		`INSERT INTO organization VALUES ('org-a'),('org-b')`,
		`INSERT INTO projects VALUES ('project-a','org-a'),('project-b','org-b')`,
		`INSERT INTO member VALUES ('org-a','owner-a','owner'),('org-a','member-a','member')`,
		`INSERT INTO account VALUES ('owner-a','google-analytics','account-a'),('other','google-analytics','account-b')`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	repo := SetupRepository{DB: pool}
	grants, err := repo.ListGrants(ctx, "owner-a")
	if err != nil || len(grants) != 1 || grants[0].AccountID != "account-a" {
		t.Fatalf("grants=%+v err=%v", grants, err)
	}
	allowed, err := repo.CanManage(ctx, "owner-a", "org-a", "project-a")
	if err != nil || !allowed {
		t.Fatalf("owner canManage=%v err=%v", allowed, err)
	}
	allowed, err = repo.CanManage(ctx, "member-a", "org-a", "project-a")
	if err != nil || allowed {
		t.Fatalf("member canManage=%v err=%v", allowed, err)
	}
	_, err = repo.Upsert(ctx, "org-a", "project-a", ProjectConnection{Connection: Connection{PropertyID: "properties/12", PropertyDisplayName: "Example", PropertyTimeZone: "UTC", PropertyCurrencyCode: "USD", ConnectedByUserID: "owner-a", GA4AccountID: "account-a"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByProjectID(ctx, "org-a", "project-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.PropertyID != "properties/12" || got.CreatedAt == "" {
		t.Fatalf("connection=%+v", got)
	}
	if _, err := repo.GetByProjectID(ctx, "org-b", "project-a"); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("cross-org lookup err=%v", err)
	}
	if err := repo.Delete(ctx, "org-a", "project-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByProjectID(ctx, "org-a", "project-a"); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("deleted connection err=%v", err)
	}
}
