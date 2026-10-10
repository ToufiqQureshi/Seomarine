package billing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrInsufficientBalance means the org does not have enough credits.
	ErrInsufficientBalance = errors.New("insufficient credit balance")
	// ErrReservationNotFound means the reservation ID does not exist.
	ErrReservationNotFound = errors.New("credit reservation not found")
	// ErrReservationAlreadyClosed means the reservation is already settled or refunded.
	ErrReservationAlreadyClosed = errors.New("credit reservation already settled or refunded")
)

const (
	// CreditMarkupMultiplier is legacy 1.28 markup on provider cost.
	CreditMarkupMultiplier = 1.28
	// CreditsPerUSD is legacy rate: 1000 credits = $1.00 USD.
	CreditsPerUSD = 1000
)

type CreditReservation struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	AmountCredits  int64     `json:"amountCredits"`
	Status         string    `json:"status"`
	IdempotencyKey string    `json:"idempotencyKey,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

type CreditLedgerService struct {
	db *pgxpool.Pool
}

func NewCreditLedgerService(db *pgxpool.Pool) *CreditLedgerService {
	return &CreditLedgerService{db: db}
}

func generateID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

// GetBalance returns the organization's credit balance.
func (s *CreditLedgerService) GetBalance(ctx context.Context, orgID string) (int64, error) {
	var balance int64
	err := s.db.QueryRow(ctx, `
		SELECT balance_credits FROM go_billing_credits WHERE organization_id = $1
	`, orgID).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get credit balance: %w", err)
	}
	return balance, nil
}

// AddCredits grants credits to an organization balance.
func (s *CreditLedgerService) AddCredits(ctx context.Context, orgID string, amount int64, reason string) error {
	if amount <= 0 {
		return errors.New("amount must be positive")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credits (organization_id, balance_credits, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (organization_id)
		DO UPDATE SET balance_credits = go_billing_credits.balance_credits + EXCLUDED.balance_credits, updated_at = now()
	`, orgID, amount)
	if err != nil {
		return fmt.Errorf("upsert credits: %w", err)
	}

	ledgerID := generateID("led")
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_ledger (id, organization_id, amount_credits, type, reason, created_at)
		VALUES ($1, $2, $3, 'grant', $4, now())
	`, ledgerID, orgID, amount, reason)
	if err != nil {
		return fmt.Errorf("insert ledger grant: %w", err)
	}

	return tx.Commit(ctx)
}

// Reserve atomically reserves credits for an operation using idempotency key.
func (s *CreditLedgerService) Reserve(ctx context.Context, orgID string, amount int64, idempotencyKey string) (CreditReservation, error) {
	if amount <= 0 {
		return CreditReservation{}, errors.New("amount must be positive")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CreditReservation{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if idempotencyKey != "" {
		var res CreditReservation
		var ik *string
		err := tx.QueryRow(ctx, `
			SELECT id, organization_id, amount_credits, status, idempotency_key, created_at
			FROM go_billing_credit_reservations
			WHERE idempotency_key = $1
		`, idempotencyKey).Scan(&res.ID, &res.OrganizationID, &res.AmountCredits, &res.Status, &ik, &res.CreatedAt)
		if err == nil {
			if ik != nil {
				res.IdempotencyKey = *ik
			}
			return res, nil
		}
	}

	var balance int64
	err = tx.QueryRow(ctx, `
		SELECT balance_credits FROM go_billing_credits WHERE organization_id = $1 FOR UPDATE
	`, orgID).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) || balance < amount {
		return CreditReservation{}, ErrInsufficientBalance
	}
	if err != nil {
		return CreditReservation{}, fmt.Errorf("lock balance: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE go_billing_credits SET balance_credits = balance_credits - $1, updated_at = now()
		WHERE organization_id = $2
	`, amount, orgID)
	if err != nil {
		return CreditReservation{}, fmt.Errorf("deduct balance: %w", err)
	}

	resID := generateID("res")
	var ik *string
	if idempotencyKey != "" {
		ik = &idempotencyKey
	}

	var nowTime = time.Now().UTC()
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_reservations (id, organization_id, amount_credits, status, idempotency_key, created_at, updated_at)
		VALUES ($1, $2, $3, 'reserved', $4, $5, $5)
	`, resID, orgID, amount, ik, nowTime)
	if err != nil {
		return CreditReservation{}, fmt.Errorf("insert reservation: %w", err)
	}

	ledgerID := generateID("led")
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_ledger (id, organization_id, reservation_id, amount_credits, type, reason, created_at)
		VALUES ($1, $2, $3, $4, 'reserve', 'credit reservation', $5)
	`, ledgerID, orgID, resID, -amount, nowTime)
	if err != nil {
		return CreditReservation{}, fmt.Errorf("insert ledger reserve: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CreditReservation{}, fmt.Errorf("commit tx: %w", err)
	}

	return CreditReservation{
		ID:             resID,
		OrganizationID: orgID,
		AmountCredits:  amount,
		Status:         "reserved",
		IdempotencyKey: idempotencyKey,
		CreatedAt:      nowTime,
	}, nil
}

// Settle settles a reservation with actual used credits, returning any unused credits to balance.
func (s *CreditLedgerService) Settle(ctx context.Context, reservationID string, actualAmount int64) error {
	if actualAmount < 0 {
		return errors.New("actual amount cannot be negative")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var orgID, status string
	var reservedAmount int64
	err = tx.QueryRow(ctx, `
		SELECT organization_id, amount_credits, status
		FROM go_billing_credit_reservations
		WHERE id = $1 FOR UPDATE
	`, reservationID).Scan(&orgID, &reservedAmount, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrReservationNotFound
	}
	if err != nil {
		return fmt.Errorf("query reservation: %w", err)
	}

	if status != "reserved" {
		return ErrReservationAlreadyClosed
	}

	refundAmount := reservedAmount - actualAmount
	if refundAmount < 0 {
		// If actual exceeded reserved, try to deduct extra
		extraNeeded := -refundAmount
		var currentBal int64
		err = tx.QueryRow(ctx, `SELECT balance_credits FROM go_billing_credits WHERE organization_id = $1 FOR UPDATE`, orgID).Scan(&currentBal)
		if err != nil || currentBal < extraNeeded {
			return ErrInsufficientBalance
		}
		_, err = tx.Exec(ctx, `UPDATE go_billing_credits SET balance_credits = balance_credits - $1, updated_at = now() WHERE organization_id = $2`, extraNeeded, orgID)
		if err != nil {
			return fmt.Errorf("deduct extra balance: %w", err)
		}
	} else if refundAmount > 0 {
		_, err = tx.Exec(ctx, `UPDATE go_billing_credits SET balance_credits = balance_credits + $1, updated_at = now() WHERE organization_id = $2`, refundAmount, orgID)
		if err != nil {
			return fmt.Errorf("refund unused balance: %w", err)
		}
	}

	_, err = tx.Exec(ctx, `UPDATE go_billing_credit_reservations SET status = 'settled', updated_at = now() WHERE id = $1`, reservationID)
	if err != nil {
		return fmt.Errorf("update reservation status: %w", err)
	}

	ledgerID := generateID("led")
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_ledger (id, organization_id, reservation_id, amount_credits, type, reason, created_at)
		VALUES ($1, $2, $3, $4, 'settle', 'settled reservation', now())
	`, ledgerID, orgID, reservationID, -actualAmount, err)
	if err != nil {
		return fmt.Errorf("insert ledger settle: %w", err)
	}

	return tx.Commit(ctx)
}

// Refund refunds a reservation in full.
func (s *CreditLedgerService) Refund(ctx context.Context, reservationID string, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var orgID, status string
	var reservedAmount int64
	err = tx.QueryRow(ctx, `
		SELECT organization_id, amount_credits, status
		FROM go_billing_credit_reservations
		WHERE id = $1 FOR UPDATE
	`, reservationID).Scan(&orgID, &reservedAmount, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrReservationNotFound
	}
	if err != nil {
		return fmt.Errorf("query reservation: %w", err)
	}

	if status != "reserved" {
		return ErrReservationAlreadyClosed
	}

	_, err = tx.Exec(ctx, `UPDATE go_billing_credits SET balance_credits = balance_credits + $1, updated_at = now() WHERE organization_id = $2`, reservedAmount, orgID)
	if err != nil {
		return fmt.Errorf("refund balance: %w", err)
	}

	_, err = tx.Exec(ctx, `UPDATE go_billing_credit_reservations SET status = 'refunded', updated_at = now() WHERE id = $1`, reservationID)
	if err != nil {
		return fmt.Errorf("update reservation status: %w", err)
	}

	ledgerID := generateID("led")
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_ledger (id, organization_id, reservation_id, amount_credits, type, reason, created_at)
		VALUES ($1, $2, $3, $4, 'refund', $5, now())
	`, ledgerID, orgID, reservationID, reservedAmount, reason)
	if err != nil {
		return fmt.Errorf("insert ledger refund: %w", err)
	}

	return tx.Commit(ctx)
}
