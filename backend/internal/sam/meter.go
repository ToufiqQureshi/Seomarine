package sam

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/toufiqqureshi/uplumo/backend/internal/billing"
)

// CreditLedger is the part of the billing ledger the meter uses.
type CreditLedger interface {
	GetBalance(ctx context.Context, organizationID string) (int64, error)
	Reserve(ctx context.Context, organizationID string, amount int64, idempotencyKey string) (billing.CreditReservation, error)
	Settle(ctx context.Context, reservationID string, actualAmount int64) error
}

// LedgerMeter bills SAM's provider spend against the organization's credits.
type LedgerMeter struct{ Ledger CreditLedger }

// HasCredits reports whether the organization has any credits left.
func (m LedgerMeter) HasCredits(ctx context.Context, organizationID string) (bool, error) {
	balance, err := m.Ledger.GetBalance(ctx, organizationID)
	if err != nil {
		return false, err
	}
	return balance > 0, nil
}

// CreditsForUSD converts provider cost to credits: the usual markup, rounded
// up to a whole credit, so a real charge is never zero.
func CreditsForUSD(costUSD float64) int64 {
	if costUSD <= 0 {
		return 0
	}
	return int64(math.Ceil(costUSD * billing.CreditMarkupMultiplier * billing.CreditsPerUSD))
}

// Charge deducts the cost. The spend already happened, so a balance that has
// run short is charged down to zero rather than refused.
func (m LedgerMeter) Charge(ctx context.Context, organizationID string, costUSD float64, key string) error {
	credits := CreditsForUSD(costUSD)
	if credits == 0 {
		return nil
	}
	reservation, err := m.Ledger.Reserve(ctx, organizationID, credits, key)
	if errors.Is(err, billing.ErrInsufficientBalance) {
		balance, balanceErr := m.Ledger.GetBalance(ctx, organizationID)
		if balanceErr != nil {
			return fmt.Errorf("read balance after shortfall: %w", balanceErr)
		}
		if balance <= 0 {
			return nil
		}
		reservation, err = m.Ledger.Reserve(ctx, organizationID, min(credits, balance), key)
		credits = min(credits, balance)
	}
	if err != nil {
		return fmt.Errorf("reserve %d credits: %w", credits, err)
	}
	// An idempotent replay returns the already-settled reservation.
	if reservation.Status == "settled" {
		return nil
	}
	if err := m.Ledger.Settle(ctx, reservation.ID, reservation.AmountCredits); err != nil && !errors.Is(err, billing.ErrReservationAlreadyClosed) {
		return fmt.Errorf("settle reservation: %w", err)
	}
	return nil
}
