package dataforseo

import (
	"context"
	"crypto/rand"
	"os"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/database"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
)

func TestUsageRecorderStoresCostPerOrganization(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	ctx := context.Background()
	pool, err := pgdb.Open(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	schema, err := os.ReadFile("../../database/testdata/legacy_schema.sql")
	if err != nil {
		t.Fatalf("read legacy schema: %v", err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	org, other := "org-usage-"+rand.Text(), "org-usage-"+rand.Text()
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'A', $1, now()), ($2, 'B', $2, now())`, org, other); err != nil {
		t.Fatalf("insert organizations: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.WithoutCancel(ctx), `DELETE FROM organization WHERE id IN ($1, $2)`, org, other); err != nil {
			t.Errorf("clean organizations: %v", err)
		}
	})

	recorder := NewUsageRecorder(pool)
	for _, cost := range []Cost{{Path: []string{"v3", "ai_optimization", "llm_mentions", "search", "live"}, USD: 0.0125}, {Path: []string{"v3", "x"}, USD: 0}} {
		if err := recorder.RecordDataForSEO(ctx, org, cost); err != nil {
			t.Fatalf("RecordDataForSEO(%+v) error = %v", cost, err)
		}
	}

	var count int
	var total float64
	var path string
	err = pool.QueryRow(ctx, `SELECT count(*), sum(cost_usd)::float8, min(path) FILTER (WHERE cost_usd > 0) FROM go_dataforseo_usage WHERE organization_id = $1`, org).Scan(&count, &total, &path)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || total != 0.0125 || path != "/v3/ai_optimization/llm_mentions/search/live" {
		t.Errorf("stored = %d rows, %v USD, path %q; want 2 rows, 0.0125 USD, the full path", count, total, path)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM go_dataforseo_usage WHERE organization_id = $1`, other).Scan(&count); err != nil || count != 0 {
		t.Errorf("another organization has %d rows (err %v), want 0", count, err)
	}

	// A cost for an organization that does not exist must fail loudly: the
	// caller reports it for reconciliation instead of dropping the charge.
	if err := recorder.RecordDataForSEO(ctx, "no-such-org-"+rand.Text(), Cost{Path: []string{"v3"}, USD: 1}); err == nil {
		t.Error("RecordDataForSEO() for an unknown organization succeeded, want an error")
	}
}
