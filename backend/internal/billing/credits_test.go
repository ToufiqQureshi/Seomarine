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
