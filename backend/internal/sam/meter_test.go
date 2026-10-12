package sam

import (
	"context"
	"errors"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/billing"
)

type fakeLedger struct {
	balance      int64
	reserved     []int64
	settled      []int64
	reserveErr   error
	existing     *billing.CreditReservation
	balanceCalls int
}

func (l *fakeLedger) GetBalance(context.Context, string) (int64, error) {
	l.balanceCalls++
	return l.balance, nil
}

func (l *fakeLedger) Reserve(_ context.Context, _ string, amount int64, key string) (billing.CreditReservation, error) {
	if l.existing != nil {
		return *l.existing, nil
	}
	if l.reserveErr != nil && amount > l.balance {
		return billing.CreditReservation{}, l.reserveErr
	}
	l.reserved = append(l.reserved, amount)
	l.balance -= amount
	return billing.CreditReservation{ID: "res-" + key, AmountCredits: amount, Status: "reserved"}, nil
}

func (l *fakeLedger) Settle(_ context.Context, _ string, actual int64) error {
	l.settled = append(l.settled, actual)
	return nil
}

func TestCreditsForUSD(t *testing.T) {
	tests := []struct {
		usd  float64
		want int64
	}{
		{0, 0},
		{-1, 0},
		{0.0000001, 1}, // a real charge is never free
		{0.01, 13},     // 0.01 * 1.28 * 1000 = 12.8
		{0.05, 64},     // exactly 64
		{1, 1280},
	}
	for _, tt := range tests {
		if got := CreditsForUSD(tt.usd); got != tt.want {
			t.Errorf("CreditsForUSD(%v) = %d, want %d", tt.usd, got, tt.want)
		}
	}
}

func TestLedgerMeterHasCredits(t *testing.T) {
	m := LedgerMeter{Ledger: &fakeLedger{balance: 0}}
	if ok, _ := m.HasCredits(context.Background(), "o"); ok {
		t.Fatal("a zero balance has no credits")
	}
	m = LedgerMeter{Ledger: &fakeLedger{balance: 1}}
	if ok, _ := m.HasCredits(context.Background(), "o"); !ok {
		t.Fatal("a positive balance has credits")
	}
}

func TestLedgerMeterChargesAndSettles(t *testing.T) {
	ledger := &fakeLedger{balance: 1000}
	if err := (LedgerMeter{Ledger: ledger}).Charge(context.Background(), "o", 0.05, "k1"); err != nil {
		t.Fatal(err)
	}
	if len(ledger.reserved) != 1 || ledger.reserved[0] != 64 || len(ledger.settled) != 1 || ledger.settled[0] != 64 {
		t.Fatalf("reserved = %v settled = %v", ledger.reserved, ledger.settled)
	}
	if err := (LedgerMeter{Ledger: ledger}).Charge(context.Background(), "o", 0, "k2"); err != nil || len(ledger.reserved) != 1 {
		t.Fatal("a zero-cost charge must not touch the ledger")
	}
}

func TestLedgerMeterChargesDownToTheBalanceOnShortfall(t *testing.T) {
	ledger := &fakeLedger{balance: 10, reserveErr: billing.ErrInsufficientBalance}
	if err := (LedgerMeter{Ledger: ledger}).Charge(context.Background(), "o", 1, "k"); err != nil {
		t.Fatal(err)
	}
	if len(ledger.reserved) != 1 || ledger.reserved[0] != 10 {
		t.Fatalf("the spend already happened; charge what remains: %v", ledger.reserved)
	}

	empty := &fakeLedger{balance: 0, reserveErr: billing.ErrInsufficientBalance}
	if err := (LedgerMeter{Ledger: empty}).Charge(context.Background(), "o", 1, "k"); err != nil || len(empty.reserved) != 0 {
		t.Fatalf("an empty balance cannot go negative: err = %v reserved = %v", err, empty.reserved)
	}
}

func TestLedgerMeterReplayDoesNotDoubleCharge(t *testing.T) {
	ledger := &fakeLedger{existing: &billing.CreditReservation{ID: "r", AmountCredits: 5, Status: "settled"}}
	if err := (LedgerMeter{Ledger: ledger}).Charge(context.Background(), "o", 0.01, "k"); err != nil {
		t.Fatal(err)
	}
	if len(ledger.settled) != 0 {
		t.Fatalf("an already settled reservation must not settle again: %v", ledger.settled)
	}
}

func TestLedgerMeterSurfacesUnexpectedErrors(t *testing.T) {
	ledger := &fakeLedger{balance: 100, reserveErr: errors.New("db down")}
	// Make the reserve fail with a non-balance error regardless of amount.
	failing := failingLedger{fakeLedger: ledger}
	if err := (LedgerMeter{Ledger: failing}).Charge(context.Background(), "o", 0.01, "k"); err == nil {
		t.Fatal("a ledger failure must be reported so it is logged")
	}
}

type failingLedger struct{ *fakeLedger }

func (failingLedger) Reserve(context.Context, string, int64, string) (billing.CreditReservation, error) {
	return billing.CreditReservation{}, errors.New("db down")
}
