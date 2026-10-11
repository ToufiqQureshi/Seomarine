package billing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func newCreditTestOrg(t *testing.T, ctx context.Context) string {
	t.Helper()
	orgID := fmt.Sprintf("org_credit_%d", time.Now().UnixNano())
	_, err := setupTestDB(t).Exec(ctx,
		`INSERT INTO organization (id, name, slug, created_at) VALUES ($1, $1, $1, now())`,
		orgID,
	)
	if err != nil {
		t.Fatalf("create test organization: %v", err)
	}
	return orgID
}

func TestCreditLedgerConcurrentReserveAndOrgScopedKeys(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	ctx := context.Background()
	svc := NewCreditLedgerService(pool)
	orgID := fmt.Sprintf("org_credit_concurrent_%d", time.Now().UnixNano())
	otherOrgID := orgID + "_other"
	for _, id := range []string{orgID, otherOrgID} {
		_, err := pool.Exec(ctx,
			`INSERT INTO organization (id, name, slug, created_at) VALUES ($1, $1, $1, now())`,
			id,
		)
		if err != nil {
			t.Fatalf("create organization %s: %v", id, err)
		}
	}

	if err := svc.AddCredits(ctx, orgID, 300, "test grant"); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddCredits(ctx, otherOrgID, 100, "test grant"); err != nil {
		t.Fatal(err)
	}

	const callers = 12
	results := make(chan CreditReservation, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := svc.Reserve(ctx, orgID, 100, "same-operation")
			if err != nil {
				errs <- err
				return
			}
			results <- res
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	var firstID string
	for res := range results {
		if firstID == "" {
			firstID = res.ID
		}
		if res.ID != firstID {
			t.Fatalf("same idempotency key returned multiple reservations: %q and %q", firstID, res.ID)
		}
	}
	for err := range errs {
		t.Fatalf("idempotent concurrent reserve failed: %v", err)
	}
	balance, err := svc.GetBalance(ctx, orgID)
	if err != nil || balance != 200 {
		t.Fatalf("same-key concurrent balance = %d, err=%v; want 200", balance, err)
	}

	const distinctCallers = 8
	results2 := make(chan error, distinctCallers)
	for i := range distinctCallers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.Reserve(ctx, orgID, 100, fmt.Sprintf("operation-%d", i))
			results2 <- err
		}(i)
	}
	wg.Wait()
	close(results2)
	var succeeded, insufficient int
	for err := range results2 {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrInsufficientBalance):
			insufficient++
		default:
			t.Fatalf("concurrent reservation returned unexpected error: %v", err)
		}
	}
	if succeeded != 2 || insufficient != distinctCallers-2 {
		t.Fatalf("concurrent reserve successes=%d insufficient=%d; want 2 and %d", succeeded, insufficient, distinctCallers-2)
	}
	balance, err = svc.GetBalance(ctx, orgID)
	if err != nil || balance != 0 {
		t.Fatalf("post-concurrency balance = %d, err=%v; want 0", balance, err)
	}

	// The same text key in a different organization is a different operation.
	other, err := svc.Reserve(ctx, otherOrgID, 100, "same-operation")
	if err != nil {
		t.Fatalf("cross-organization idempotency key: %v", err)
	}
	if other.ID == firstID {
		t.Fatal("cross-organization idempotency key reused another organization's reservation")
	}
	balance, err = svc.GetBalance(ctx, orgID)
	if err != nil || balance != 0 {
		t.Fatalf("organization balance became negative or changed: %d, err=%v", balance, err)
	}
}

func TestCreditLedgerSettlementRefundAndDebtAccounting(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	ctx := context.Background()
	svc := NewCreditLedgerService(pool)
	orgID := fmt.Sprintf("org_credit_accounting_%d", time.Now().UnixNano())
	_, err := pool.Exec(ctx,
		`INSERT INTO organization (id, name, slug, created_at) VALUES ($1, $1, $1, now())`,
		orgID,
	)
	if err != nil {
		t.Fatalf("create test organization: %v", err)
	}

	if err := svc.AddCredits(ctx, orgID, 1000, "test grant"); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Reserve(ctx, orgID, 200, "settle-refund")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Settle(ctx, res.ID, 100); err != nil {
		t.Fatal(err)
	}
	if err := svc.Settle(ctx, res.ID, 100); !errors.Is(err, ErrReservationAlreadyClosed) {
		t.Fatalf("second settle error = %v; want ErrReservationAlreadyClosed", err)
	}
	balance, err := svc.GetBalance(ctx, orgID)
	if err != nil || balance != 900 {
		t.Fatalf("balance after partial settlement = %d, err=%v; want 900", balance, err)
	}
	var ledgerTotal int64
	err = pool.QueryRow(ctx, `SELECT COALESCE(sum(amount_credits), 0) FROM go_billing_credit_ledger WHERE organization_id = $1`, orgID).Scan(&ledgerTotal)
	if err != nil || ledgerTotal != balance {
		t.Fatalf("ledger sum after settlement = %d, balance=%d, err=%v", ledgerTotal, balance, err)
	}

	refundRes, err := svc.Reserve(ctx, orgID, 100, "refund")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Refund(ctx, refundRes.ID, "test provider error"); err != nil {
		t.Fatal(err)
	}
	balance, err = svc.GetBalance(ctx, orgID)
	if err != nil || balance != 900 {
		t.Fatalf("balance after refund = %d, err=%v; want 900", balance, err)
	}
	err = pool.QueryRow(ctx, `SELECT COALESCE(sum(amount_credits), 0) FROM go_billing_credit_ledger WHERE organization_id = $1`, orgID).Scan(&ledgerTotal)
	if err != nil || ledgerTotal != balance {
		t.Fatalf("ledger sum after refund = %d, balance=%d, err=%v", ledgerTotal, balance, err)
	}

	debtOrgID := orgID + "_debt"
	_, err = pool.Exec(ctx,
		`INSERT INTO organization (id, name, slug, created_at) VALUES ($1, $1, $1, now())`,
		debtOrgID,
	)
	if err != nil {
		t.Fatalf("create debt organization: %v", err)
	}
	if err := svc.AddCredits(ctx, debtOrgID, 100, "test grant"); err != nil {
		t.Fatal(err)
	}
	debtRes, err := svc.Reserve(ctx, debtOrgID, 100, "debt")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Settle(ctx, debtRes.ID, 175); err != nil {
		t.Fatalf("settle above available balance: %v", err)
	}
	debtBalance, err := svc.GetBalance(ctx, debtOrgID)
	if err != nil || debtBalance != 0 {
		t.Fatalf("debt path balance = %d, err=%v; want clamped zero", debtBalance, err)
	}
	var debtAmount int64
	err = pool.QueryRow(ctx, `SELECT COALESCE(sum(amount_credits), 0) FROM go_billing_credit_ledger WHERE organization_id = $1 AND type = 'debt'`, debtOrgID).Scan(&debtAmount)
	if err != nil || debtAmount != -75 {
		t.Fatalf("debt ledger amount = %d, err=%v; want -75", debtAmount, err)
	}
}
