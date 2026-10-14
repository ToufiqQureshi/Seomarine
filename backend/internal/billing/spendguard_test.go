package billing

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
)

func spendOrg(ctx context.Context, t *testing.T, pool *pgxpool.Pool, credits int64) string {
	t.Helper()
	org := "org_spend_" + rand.Text()
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Spend Org', $1, now())`, org); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.WithoutCancel(ctx), `DELETE FROM organization WHERE id = $1`, org); err != nil {
			t.Errorf("delete org: %v", err)
		}
	})
	if credits > 0 {
		if err := NewCreditLedgerService(pool).AddCredits(ctx, org, credits, "test grant"); err != nil {
			t.Fatal(err)
		}
	}
	return org
}

func testGuard(t *testing.T, pool *pgxpool.Pool, ledger *CreditLedgerService, limit float64, charge bool) *SpendGuard {
	t.Helper()
	guard, err := NewSpendGuard(pool, ledger, limit, charge, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func balanceOf(ctx context.Context, t *testing.T, ledger *CreditLedgerService, org string) int64 {
	t.Helper()
	balance, err := ledger.GetBalance(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	return balance
}

func providerSpend(t *testing.T, pool *pgxpool.Pool, org string) (float64, string) {
	t.Helper()
	var total float64
	var features string
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(sum(cost_usd), 0)::float8, COALESCE(string_agg(DISTINCT feature, ','), '') FROM go_provider_spend WHERE organization_id = $1`, org).
		Scan(&total, &features); err != nil {
		t.Fatal(err)
	}
	return total, features
}

const noDailyLimit = 1e9

func TestNewSpendGuardRejectsANonPositiveDailyLimit(t *testing.T) {
	pool := setupTestDB(t)
	for _, limit := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := NewSpendGuard(pool, NewCreditLedgerService(pool), limit, true, slog.New(slog.DiscardHandler)); err == nil {
			t.Errorf("NewSpendGuard(limit %v) succeeded", limit)
		}
	}
}

func TestHoldRefusesAWorkspaceWithoutEnoughCredits(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	org := spendOrg(ctx, t, pool, 100)
	guard := testGuard(t, pool, ledger, noDailyLimit, true)

	// $0.10 is 128 credits; the workspace has 100.
	_, err := guard.Hold(context.Background(), spend.Request{OrganizationID: org, Feature: spend.Backlinks, EstimateUSD: 0.1})
	if !errors.Is(err, spend.ErrNoCredits) {
		t.Fatalf("Hold() error = %v, want ErrNoCredits", err)
	}
	if got := balanceOf(ctx, t, ledger, org); got != 100 {
		t.Fatalf("balance = %d after a refusal, want 100 untouched", got)
	}
}

func TestHoldReservesTheCeilingAndSettleChargesTheRealCost(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	org := spendOrg(ctx, t, pool, 1000)
	guard := testGuard(t, pool, ledger, noDailyLimit, true)

	hold, err := guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: spend.Backlinks, EstimateUSD: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	if got := balanceOf(ctx, t, ledger, org); got != 1000-128 {
		t.Fatalf("balance while held = %d, want %d", got, 1000-128)
	}
	if err := hold.Settle(ctx, 0.02); err != nil {
		t.Fatal(err)
	}
	if got := balanceOf(ctx, t, ledger, org); got != 1000-26 {
		t.Fatalf("balance after settle = %d, want %d (0.02 USD is 26 credits)", got, 1000-26)
	}
	var reason string
	if err := pool.QueryRow(ctx, `SELECT reason FROM go_billing_credit_ledger WHERE organization_id = $1 AND type = 'settle'`, org).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != string(spend.Backlinks) {
		t.Fatalf("settle reason = %q, want the feature", reason)
	}
	if total, features := providerSpend(t, pool, org); total != 0.02 || features != string(spend.Backlinks) {
		t.Fatalf("provider spend = %v %q", total, features)
	}
	if err := hold.Settle(ctx, 0.02); err == nil {
		t.Fatal("a second Settle succeeded; it would charge twice")
	}
	if got := balanceOf(ctx, t, ledger, org); got != 1000-26 {
		t.Fatalf("balance after a second settle = %d", got)
	}
}

func TestSettleChargesDownToZeroWhenTheProviderCostMoreThanHeld(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	org := spendOrg(ctx, t, pool, 200)
	guard := testGuard(t, pool, ledger, noDailyLimit, true)

	hold, err := guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: spend.RankTracking, EstimateUSD: 0.01})
	if err != nil {
		t.Fatal(err)
	}
	// $1 is 1280 credits, far more than the 200 the workspace had.
	if err := hold.Settle(ctx, 1); err != nil {
		t.Fatalf("Settle() error = %v; the work was already paid for and must not fail", err)
	}
	if got := balanceOf(ctx, t, ledger, org); got != 0 {
		t.Fatalf("balance = %d, want 0", got)
	}
	if total, _ := providerSpend(t, pool, org); total != 1 {
		t.Fatalf("provider spend = %v, want the full dollar logged for the daily limit", total)
	}
}

func TestSettleWithAnInvalidCostKeepsTheWholeHold(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	org := spendOrg(ctx, t, pool, 1000)
	guard := testGuard(t, pool, ledger, noDailyLimit, true)

	for _, cost := range []float64{-1, math.NaN(), math.Inf(1)} {
		hold, err := guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: spend.DomainResearch, EstimateUSD: 0.01})
		if err != nil {
			t.Fatal(err)
		}
		before := balanceOf(ctx, t, ledger, org)
		if err := hold.Settle(ctx, cost); err == nil {
			t.Fatalf("Settle(%v) succeeded", cost)
		}
		if got := balanceOf(ctx, t, ledger, org); got != before {
			t.Fatalf("Settle(%v) gave credits back: balance %d, want %d", cost, got, before)
		}
	}
}

func TestHoldIsRefusedOnceTodaysSpendReachesTheDailyLimit(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	org := spendOrg(ctx, t, pool, 100_000)
	spent, err := spendTodayUSD(ctx, pool, "")
	if err != nil {
		t.Fatal(err)
	}
	// Yesterday's spend does not count towards today.
	if _, err := pool.Exec(ctx, `INSERT INTO go_provider_spend (organization_id, feature, cost_usd, created_at) VALUES ($1, 'backlinks', 500, now() - interval '1 day 1 hour')`, org); err != nil {
		t.Fatal(err)
	}
	guard := testGuard(t, pool, ledger, spent+0.5, true)

	hold, err := guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: spend.Backlinks, EstimateUSD: 0.05})
	if err != nil {
		t.Fatalf("Hold() under the limit error = %v", err)
	}
	if err := hold.Settle(ctx, 0.48); err != nil {
		t.Fatal(err)
	}
	_, err = guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: spend.Backlinks, EstimateUSD: 0.05})
	if !errors.Is(err, spend.ErrPaused) {
		t.Fatalf("Hold() over the limit error = %v, want ErrPaused", err)
	}
	if _, err := guard.AIMeter(spend.AIAssistant).Hold(ctx, org, 0.05); !errors.Is(err, spend.ErrPaused) {
		t.Fatalf("AI Hold while paused error = %v, want ErrPaused", err)
	}
}

func TestHoldWithoutPaymentsChargesNothingButStillLogs(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	org := spendOrg(ctx, t, pool, 0)
	guard := testGuard(t, pool, ledger, noDailyLimit, false)

	hold, err := guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: spend.KeywordResearch, EstimateUSD: 0.1})
	if err != nil {
		t.Fatalf("Hold() on a server without payments error = %v", err)
	}
	if err := hold.Settle(ctx, 0.03); err != nil {
		t.Fatal(err)
	}
	if total, _ := providerSpend(t, pool, org); total != 0.03 {
		t.Fatalf("provider spend = %v, want 0.03 logged for the daily limit", total)
	}
	if got := balanceOf(ctx, t, ledger, org); got != 0 {
		t.Fatalf("balance = %d", got)
	}
}

func TestAFreeRequestIsNotHeldButAnUnexpectedCostIsCharged(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	org := spendOrg(ctx, t, pool, 0)
	guard := testGuard(t, pool, ledger, noDailyLimit, true)

	hold, err := guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: spend.Reference, EstimateUSD: 0})
	if err != nil {
		t.Fatalf("a free lookup was refused to a workspace without credits: %v", err)
	}
	if err := hold.Settle(ctx, 0); err != nil {
		t.Fatal(err)
	}

	org2 := spendOrg(ctx, t, pool, 50)
	hold, err = guard.Hold(ctx, spend.Request{OrganizationID: org2, Feature: spend.Reference, EstimateUSD: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := hold.Settle(ctx, 0.01); err != nil {
		t.Fatal(err)
	}
	if got := balanceOf(ctx, t, ledger, org2); got != 50-13 {
		t.Fatalf("balance = %d, want %d: a lookup that turned out to cost money is still charged", got, 50-13)
	}
}

func TestConcurrentHoldsNeverSpendMoreThanTheBalance(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	// Room for exactly three holds of 128 credits.
	org := spendOrg(ctx, t, pool, 3*128+10)
	guard := testGuard(t, pool, ledger, noDailyLimit, true)

	var allowed atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			if _, err := guard.Hold(context.Background(), spend.Request{OrganizationID: org, Feature: spend.Backlinks, EstimateUSD: 0.1}); err == nil {
				allowed.Add(1)
			} else if !errors.Is(err, spend.ErrNoCredits) {
				t.Errorf("Hold() error = %v", err)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 3 {
		t.Fatalf("%d concurrent holds were allowed, want exactly 3", allowed.Load())
	}
	if got := balanceOf(ctx, t, ledger, org); got != 10 {
		t.Fatalf("balance = %d, want 10", got)
	}
}

func TestStaffWorkspacesAreToppedUpInsteadOfRefused(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	org := spendOrg(ctx, t, pool, 0)
	ledger.Unlimited = func(_ context.Context, id string) (bool, error) { return id == org, nil }
	guard := testGuard(t, pool, ledger, noDailyLimit, true)

	hold, err := guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: spend.AIVisibility, EstimateUSD: 0.5})
	if err != nil {
		t.Fatalf("staff Hold() error = %v", err)
	}
	if err := hold.Settle(ctx, 0.1); err != nil {
		t.Fatal(err)
	}
	aiHold, err := guard.AIMeter(spend.AIAssistant).Hold(ctx, org, 0.5)
	if err != nil {
		t.Fatalf("staff AI Hold error = %v", err)
	}
	if err := aiHold.Settle(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if total, _ := providerSpend(t, pool, org); total != 0.1 {
		t.Fatalf("staff spend = %v, want it still logged", total)
	}
}

func TestAIMeter(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	guard := testGuard(t, pool, ledger, noDailyLimit, true)
	meter := guard.AIMeter(spend.AIWriting)

	t.Run("needs the whole ceiling to start", func(t *testing.T) {
		org := spendOrg(ctx, t, pool, CreditsForUSD(0.05)-1)
		if _, err := meter.Hold(ctx, org, 0.05); !errors.Is(err, spend.ErrNoCredits) {
			t.Fatalf("Hold with one credit short error = %v, want ErrNoCredits", err)
		}
	})

	t.Run("charges the real cost, returns the rest, labels the feature", func(t *testing.T) {
		org := spendOrg(ctx, t, pool, 1000)
		hold, err := meter.Hold(ctx, org, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		if got := balanceOf(ctx, t, ledger, org); got != 1000-640 {
			t.Fatalf("balance while held = %d, want %d", got, 1000-640)
		}
		if err := hold.Settle(ctx, 0.05); err != nil {
			t.Fatal(err)
		}
		if got := balanceOf(ctx, t, ledger, org); got != 1000-64 {
			t.Fatalf("balance after settle = %d, want %d", got, 1000-64)
		}
		if err := hold.Settle(ctx, 0.05); err == nil {
			t.Fatal("a hold settled twice must fail")
		}
		var reason string
		if err := pool.QueryRow(ctx, `SELECT reason FROM go_billing_credit_ledger WHERE organization_id = $1 AND type = 'settle'`, org).Scan(&reason); err != nil {
			t.Fatal(err)
		}
		if reason != string(spend.AIWriting) {
			t.Fatalf("reason = %q", reason)
		}
	})

	t.Run("a model that cost more than its ceiling is charged down to zero", func(t *testing.T) {
		org := spendOrg(ctx, t, pool, 100)
		hold, err := meter.Hold(ctx, org, 0.05)
		if err != nil {
			t.Fatal(err)
		}
		if err := hold.Settle(ctx, 1); err != nil {
			t.Fatal(err)
		}
		if got := balanceOf(ctx, t, ledger, org); got != 0 {
			t.Fatalf("balance = %d, want 0", got)
		}
	})

	t.Run("an invalid cost keeps the whole hold", func(t *testing.T) {
		org := spendOrg(ctx, t, pool, 1000)
		hold, err := meter.Hold(ctx, org, 0.05)
		if err != nil {
			t.Fatal(err)
		}
		if err := hold.Settle(ctx, math.NaN()); err == nil {
			t.Fatal("Settle(NaN) succeeded")
		}
		if got := balanceOf(ctx, t, ledger, org); got != 1000-64 {
			t.Fatalf("balance = %d, want the ceiling kept", got)
		}
	})
}

func TestCreditsForUSD(t *testing.T) {
	for _, tt := range []struct {
		usd  float64
		want int64
	}{
		{0, 0}, {-1, 0}, {math.NaN(), 0}, {math.Inf(1), 0},
		{0.0001, 1}, // rounds up so a real charge is never free
		{0.01, 13}, {1, 1280},
	} {
		if got := CreditsForUSD(tt.usd); got != tt.want {
			t.Errorf("CreditsForUSD(%v) = %d, want %d", tt.usd, got, tt.want)
		}
	}
}

func TestAHoldWithoutAReservationCannotBeSettledTwice(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	org := spendOrg(ctx, t, pool, 100)
	guard := testGuard(t, pool, ledger, noDailyLimit, true)

	hold, err := guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: spend.Reference, EstimateUSD: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := hold.Settle(ctx, 0.01); err != nil {
		t.Fatal(err)
	}
	if err := hold.Settle(ctx, 0.01); err == nil {
		t.Fatal("a second Settle succeeded")
	}
	if got := balanceOf(ctx, t, ledger, org); got != 100-13 {
		t.Fatalf("balance = %d, want %d: charged twice for one request", got, 100-13)
	}
}

// One heavy workspace must use up only its own day: the others keep working.
func TestOneWorkspacesDailyLimitNeverStopsTheOthers(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	heavy := spendOrg(ctx, t, pool, 100_000)
	quiet := spendOrg(ctx, t, pool, 100_000)
	guard := testGuard(t, pool, ledger, noDailyLimit, true)
	guard.WorkspaceDailyLimitUSD = func(context.Context, string) (float64, error) { return 0.1, nil }

	// Yesterday's spend does not count towards today.
	if _, err := pool.Exec(ctx, `INSERT INTO go_provider_spend (organization_id, feature, cost_usd, created_at) VALUES ($1, 'backlinks', 50, now() - interval '1 day 1 hour')`, heavy); err != nil {
		t.Fatal(err)
	}
	hold, err := guard.Hold(ctx, spend.Request{OrganizationID: heavy, Feature: spend.Backlinks, EstimateUSD: 0.05})
	if err != nil {
		t.Fatalf("first hold of the day error = %v", err)
	}
	if err := hold.Settle(ctx, 0.09); err != nil {
		t.Fatal(err)
	}
	if _, err := guard.Hold(ctx, spend.Request{OrganizationID: heavy, Feature: spend.Backlinks, EstimateUSD: 0.05}); !errors.Is(err, spend.ErrWorkspaceDailyLimit) {
		t.Fatalf("heavy workspace over its day error = %v, want ErrWorkspaceDailyLimit", err)
	}
	if _, err := guard.AIMeter(spend.AIAssistant).Hold(ctx, heavy, 0.05); !errors.Is(err, spend.ErrWorkspaceDailyLimit) {
		t.Fatalf("heavy workspace AI error = %v; want stopped for the day", err)
	}
	if _, err := guard.Hold(ctx, spend.Request{OrganizationID: quiet, Feature: spend.Backlinks, EstimateUSD: 0.05}); err != nil {
		t.Fatalf("quiet workspace was stopped by another's spend: %v", err)
	}
	if _, err := guard.AIMeter(spend.AIAssistant).Hold(ctx, quiet, 0.01); err != nil {
		t.Fatalf("quiet workspace AI error = %v; want allowed", err)
	}

	guard.WorkspaceDailyLimitUSD = func(context.Context, string) (float64, error) { return 0, nil }
	if _, err := guard.Hold(ctx, spend.Request{OrganizationID: heavy, Feature: spend.Backlinks, EstimateUSD: 0.05}); err != nil {
		t.Fatalf("a zero limit (staff) must not stop the workspace: %v", err)
	}
	failure := errors.New("billing down")
	guard.WorkspaceDailyLimitUSD = func(context.Context, string) (float64, error) { return 0, failure }
	if _, err := guard.Hold(ctx, spend.Request{OrganizationID: quiet, Feature: spend.Backlinks, EstimateUSD: 0.05}); !errors.Is(err, failure) {
		t.Fatalf("hold when the limit cannot be read error = %v, want it to fail closed", err)
	}
}

// Reproduction: parallel AI work each passes HasCredits against the same
// balance, then charges after the fact, so the workspace runs past its credits
// and Uplumo pays the rest.
func TestRegressionParallelAIWorkCannotSpendPastTheBalance(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	org := spendOrg(ctx, t, pool, 100) // 100 credits is about $0.078 of provider cost
	meter := testGuard(t, pool, ledger, noDailyLimit, true).AIMeter(spend.AIAssistant)

	const workers, stepUSD = 5, 0.05 // each step really costs 64 credits
	var wg sync.WaitGroup
	var started atomic.Int32
	for range workers {
		wg.Go(func() {
			hold, err := meter.Hold(ctx, org, stepUSD)
			if err != nil {
				return
			}
			started.Add(1)
			if err := hold.Settle(ctx, stepUSD); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	spent, _ := providerSpend(t, pool, org)
	if paidFor := float64(100) / CreditMarkupMultiplier / CreditsPerUSD; spent > paidFor+1e-9 {
		t.Fatalf("%d of %d AI steps ran and cost $%.4f; the workspace's 100 credits cover $%.4f", started.Load(), workers, spent, paidFor)
	}
}

// Reproduction: the daily limits count only settled spend, so holds still in
// flight let a workspace start more work than its daily limit allows.
func TestRegressionInFlightHoldsCountTowardTheDailyLimits(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	org := spendOrg(ctx, t, pool, 100_000)
	guard := testGuard(t, pool, ledger, noDailyLimit, true)
	guard.WorkspaceDailyLimitUSD = func(context.Context, string) (float64, error) { return 1, nil }

	first, err := guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: spend.Backlinks, EstimateUSD: 0.8})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Settle(context.WithoutCancel(ctx), 0) }()
	if second, err := guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: spend.Backlinks, EstimateUSD: 0.8}); !errors.Is(err, spend.ErrWorkspaceDailyLimit) {
		if second != nil {
			_ = second.Settle(context.WithoutCancel(ctx), 0)
		}
		t.Fatalf("second $0.80 hold with $0.80 in flight against a $1 limit: err = %v, want ErrWorkspaceDailyLimit", err)
	}
}

func TestConcurrentHoldsCannotTogetherPassTheWorkspaceDailyLimit(t *testing.T) {
	pool := setupTestDB(t)
	ctx := t.Context()
	ledger := NewCreditLedgerService(pool)
	org := spendOrg(ctx, t, pool, 100_000)
	guard := testGuard(t, pool, ledger, noDailyLimit, true)
	guard.WorkspaceDailyLimitUSD = func(context.Context, string) (float64, error) { return 1, nil }

	var wg sync.WaitGroup
	var granted atomic.Int32
	holds := make(chan spend.Hold, 10)
	for range 10 {
		wg.Go(func() {
			hold, err := guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: spend.Backlinks, EstimateUSD: 0.3})
			if err == nil {
				granted.Add(1)
				holds <- hold
			} else if !errors.Is(err, spend.ErrWorkspaceDailyLimit) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	close(holds)
	for hold := range holds {
		_ = hold.Settle(context.WithoutCancel(ctx), 0)
	}
	if got := granted.Load(); got != 3 {
		t.Fatalf("%d holds of $0.30 granted against a $1 limit, want 3", got)
	}
}
