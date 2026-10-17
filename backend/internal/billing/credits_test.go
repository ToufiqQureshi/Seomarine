package billing

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/database"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	pool, err := pgdb.Open(context.Background(), url)
	if err != nil {
		t.Fatalf("open PostgreSQL integration database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(context.Background(), pool); err != nil {
		pool.Close()
		t.Fatalf("migrate PostgreSQL integration database: %v", err)
	}
	return pool
}

func TestCreditLedgerService_Unit(t *testing.T) {
	t.Run("constants check", func(t *testing.T) {
		if CreditMarkupMultiplier != 1.28 {
			t.Errorf("expected 1.28 markup, got %f", CreditMarkupMultiplier)
		}
		if CreditsPerUSD != 1000 {
			t.Errorf("expected 1000 credits per USD, got %d", CreditsPerUSD)
		}
	})
}

func TestWorkspaceIdempotencyKeyIsStableAndScoped(t *testing.T) {
	first := workspaceIdempotencyKey("org-one", "same-request")
	if first == "" || first != workspaceIdempotencyKey("org-one", "same-request") {
		t.Fatalf("workspace idempotency key is not stable: %q", first)
	}
	if first == workspaceIdempotencyKey("org-two", "same-request") {
		t.Fatal("the same request in a different workspace reused its idempotency key")
	}
	if workspaceIdempotencyKey("org-one", "") != "" {
		t.Fatal("empty idempotency key should remain empty")
	}
}

func TestReserveForWorkspaceWithoutBalanceReturnsInsufficientCredits(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	orgID := "org_empty_" + rand.Text()
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Empty Org', $1, now())`, orgID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.WithoutCancel(ctx), `DELETE FROM organization WHERE id = $1`, orgID); err != nil {
			t.Errorf("delete integration test organization: %v", err)
		}
	})
	_, err := NewCreditLedgerService(pool).Reserve(ctx, orgID, 1, "")
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("Reserve() error = %v, want ErrInsufficientBalance", err)
	}
}

func TestRefundLegacyReservationWithoutGrantAllocations(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	svc := NewCreditLedgerService(pool)
	orgID := "org_legacy_refund_" + rand.Text()
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Legacy Refund Org', $1, now())`, orgID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organization WHERE id = $1`, orgID) })
	if err := svc.AddCredits(ctx, orgID, 100, "test grant"); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Reserve(ctx, orgID, 40, "legacy-reservation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM go_billing_credit_reservation_grants WHERE reservation_id = $1`, res.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Refund(ctx, res.ID, "legacy reservation refund"); err != nil {
		t.Fatalf("Refund() error = %v, want success", err)
	}
	if balance, err := svc.GetBalance(ctx, orgID); err != nil || balance != 100 {
		t.Fatalf("balance = %d, error = %v; want all 100 credits restored", balance, err)
	}
}

func TestRankQueuedRetryCanReserveAfterRefund(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	svc := NewCreditLedgerService(pool)
	orgID := "org_rank_retry_" + rand.Text()
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Rank Retry Org', $1, now())`, orgID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organization WHERE id = $1`, orgID) })
	if err := svc.AddCredits(ctx, orgID, 100, "test grant"); err != nil {
		t.Fatal(err)
	}
	const key = "rank-run-queued-task-post"
	first, err := svc.ReserveFor(ctx, orgID, 40, key, "rank_tracking")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Refund(ctx, first.ID, "provider rejected task post"); err != nil {
		t.Fatal(err)
	}
	retry, err := svc.ReserveFor(ctx, orgID, 40, key, "rank_tracking")
	if err != nil {
		t.Fatalf("retry reservation: %v", err)
	}
	if retry.ID == first.ID || retry.Reused || retry.Status != "reserved" {
		t.Fatalf("retry reservation = %+v; want a new reserved hold after refund", retry)
	}
	if balance, err := svc.GetBalance(ctx, orgID); err != nil || balance != 60 {
		t.Fatalf("balance = %d, error = %v; want one 40-credit hold", balance, err)
	}
}

func TestCreditLedgerService_Integration(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}

	ctx := context.Background()
	svc := NewCreditLedgerService(pool)
	orgID := "org_test_" + rand.Text()

	// Create dummy organization
	_, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Test Org', $1, now()) ON CONFLICT DO NOTHING`, orgID)
	if err != nil {
		t.Fatalf("failed to insert org: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM organization WHERE id = $1`, orgID); err != nil {
			t.Errorf("delete integration test organization: %v", err)
		}
	})

	t.Run("balance and grant", func(t *testing.T) {
		bal, err := svc.GetBalance(ctx, orgID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bal != 0 {
			t.Errorf("expected balance 0, got %d", bal)
		}

		if err := svc.AddCredits(ctx, orgID, 5000, "test grant"); err != nil {
			t.Fatalf("grant failed: %v", err)
		}

		bal, err = svc.GetBalance(ctx, orgID)
		if err != nil || bal != 5000 {
			t.Errorf("expected balance 5000, got %d (err: %v)", bal, err)
		}
	})

	t.Run("insufficient balance", func(t *testing.T) {
		_, err := svc.Reserve(ctx, orgID, 10000, orgID+"_ik_over")
		if !errors.Is(err, ErrInsufficientBalance) {
			t.Errorf("expected ErrInsufficientBalance, got %v", err)
		}
	})

	t.Run("reserve, settle, refund", func(t *testing.T) {
		res, err := svc.Reserve(ctx, orgID, 2000, orgID+"_ik_1")
		if err != nil {
			t.Fatalf("reserve failed: %v", err)
		}
		if res.AmountCredits != 2000 {
			t.Errorf("expected reserved 2000, got %d", res.AmountCredits)
		}

		// Idempotent reserve check
		res2, err := svc.Reserve(ctx, orgID, 2000, orgID+"_ik_1")
		if err != nil || res2.ID != res.ID {
			t.Errorf("expected idempotent reservation, got id %s, err %v", res2.ID, err)
		}

		// Balance should be 3000 now
		bal, _ := svc.GetBalance(ctx, orgID)
		if bal != 3000 {
			t.Errorf("expected balance 3000, got %d", bal)
		}

		// Settle 1500 (refunds 500 unused)
		if err := svc.Settle(ctx, res.ID, 1500); err != nil {
			t.Fatalf("settle failed: %v", err)
		}

		// Double settle should fail
		if err := svc.Settle(ctx, res.ID, 1500); !errors.Is(err, ErrReservationAlreadyClosed) {
			t.Errorf("expected ErrReservationAlreadyClosed on double settle, got %v", err)
		}

		bal, _ = svc.GetBalance(ctx, orgID)
		if bal != 3500 {
			t.Errorf("expected balance 3500 after partial settle, got %d", bal)
		}

		// Reserve and refund
		resRefund, err := svc.Reserve(ctx, orgID, 1000, orgID+"_ik_ref")
		if err != nil {
			t.Fatalf("reserve refund failed: %v", err)
		}

		if err := svc.Refund(ctx, resRefund.ID, "provider error"); err != nil {
			t.Fatalf("refund failed: %v", err)
		}

		bal, _ = svc.GetBalance(ctx, orgID)
		if bal != 3500 {
			t.Errorf("expected balance restored to 3500, got %d", bal)
		}
	})
}

func TestCreditGrantsAreConsumedFIFOAndExpireAfterTwoMonths(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	svc := NewCreditLedgerService(pool)
	org := "org_credit_expiry_" + rand.Text()
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Test Org', $1, now())`, org); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organization WHERE id = $1`, org) })
	if err := svc.AddCredits(ctx, org, 100, "older grant"); err != nil {
		t.Fatal(err)
	}
	var older string
	if err := pool.QueryRow(ctx, `SELECT id FROM go_billing_credit_grants WHERE organization_id=$1`, org).Scan(&older); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE go_billing_credit_grants SET expires_at=now()+interval '1 day' WHERE id=$1`, older); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddCredits(ctx, org, 100, "newer grant"); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Reserve(ctx, org, 150, org+"-fifo")
	if err != nil {
		t.Fatal(err)
	}
	var remaining int64
	if err := pool.QueryRow(ctx, `SELECT remaining_credits FROM go_billing_credit_grants WHERE id=$1`, older).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("oldest grant remaining=%d err=%v", remaining, err)
	}
	if err := svc.Refund(ctx, res.ID, "test refund"); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT remaining_credits FROM go_billing_credit_grants WHERE id=$1`, older).Scan(&remaining); err != nil || remaining != 100 {
		t.Fatalf("refunded oldest grant remaining=%d err=%v", remaining, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE go_billing_credit_grants SET expires_at=now()-interval '1 second' WHERE organization_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	expired, err := svc.ExpireCredits(ctx)
	if err != nil || expired != 200 {
		t.Fatalf("ExpireCredits()=%d err=%v, want 200", expired, err)
	}
	if balance, err := svc.GetBalance(ctx, org); err != nil || balance != 0 {
		t.Fatalf("balance=%d err=%v, want zero", balance, err)
	}
}
