package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
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
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credits (organization_id, balance_credits, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (organization_id)
		DO UPDATE SET balance_credits = go_billing_credits.balance_credits + EXCLUDED.balance_credits, updated_at = now()
	`, orgID, amount)
	if err != nil {
		return fmt.Errorf("upsert credits: %w", err)
	}

	ledgerID, err := ids.New()
	if err != nil {
		return fmt.Errorf("generate credit ledger id: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_ledger (id, organization_id, amount_credits, type, reason, created_at)
		VALUES ($1, $2, $3, 'grant', $4, now())
	`, ledgerID, orgID, amount, reason)
	if err != nil {
		return fmt.Errorf("insert ledger grant: %w", err)
	}

	return tx.Commit(ctx)
}

// Reserve atomically reserves credits for an operation using an organization-scoped idempotency key.
func (s *CreditLedgerService) Reserve(ctx context.Context, orgID string, amount int64, idempotencyKey string) (CreditReservation, error) {
	if amount <= 0 {
		return CreditReservation{}, errors.New("amount must be positive")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CreditReservation{}, fmt.Errorf("begin reservation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	loadExisting := func() (CreditReservation, error) {
		var res CreditReservation
		var key *string
		err := tx.QueryRow(ctx, `
			SELECT id, organization_id, amount_credits, status, idempotency_key, created_at
			FROM go_billing_credit_reservations
			WHERE organization_id = $1 AND idempotency_key = $2
		`, orgID, idempotencyKey).Scan(&res.ID, &res.OrganizationID, &res.AmountCredits, &res.Status, &key, &res.CreatedAt)
		if err == nil && key != nil {
			res.IdempotencyKey = *key
		}
		return res, err
	}

	if idempotencyKey != "" {
		res, err := loadExisting()
		if err == nil {
			return res, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return CreditReservation{}, fmt.Errorf("find existing reservation: %w", err)
		}
	}

	var balance int64
	err = tx.QueryRow(ctx, `
		SELECT balance_credits FROM go_billing_credits WHERE organization_id = $1 FOR UPDATE
	`, orgID).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return CreditReservation{}, ErrInsufficientBalance
	}
	if err != nil {
		return CreditReservation{}, fmt.Errorf("lock credit balance: %w", err)
	}

	// Recheck after the organization balance lock. Concurrent reservations for
	// the same organization serialize here and return the same reservation.
	if idempotencyKey != "" {
		res, err := loadExisting()
		if err == nil {
			return res, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return CreditReservation{}, fmt.Errorf("recheck existing reservation: %w", err)
		}
	}
	if balance < amount {
		return CreditReservation{}, ErrInsufficientBalance
	}

	resID, err := ids.New()
	if err != nil {
		return CreditReservation{}, fmt.Errorf("generate reservation id: %w", err)
	}
	var key any
	if idempotencyKey != "" {
		key = idempotencyKey
	}
	now := time.Now().UTC()
	var insertedID string
	err = tx.QueryRow(ctx, `
		INSERT INTO go_billing_credit_reservations
			(id, organization_id, amount_credits, status, idempotency_key, created_at, updated_at)
		VALUES ($1, $2, $3, 'reserved', $4, $5, $5)
		ON CONFLICT (organization_id, idempotency_key) DO NOTHING
		RETURNING id
	`, resID, orgID, amount, key, now).Scan(&insertedID)
	if errors.Is(err, pgx.ErrNoRows) {
		res, selectErr := loadExisting()
		if selectErr != nil {
			return CreditReservation{}, fmt.Errorf("load concurrent reservation: %w", selectErr)
		}
		if err := tx.Commit(ctx); err != nil {
			return CreditReservation{}, fmt.Errorf("commit idempotent reservation: %w", err)
		}
		return res, nil
	}
	if err != nil {
		return CreditReservation{}, fmt.Errorf("insert reservation: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE go_billing_credits
		SET balance_credits = balance_credits - $1, updated_at = now()
		WHERE organization_id = $2
	`, amount, orgID)
	if err != nil {
		return CreditReservation{}, fmt.Errorf("deduct reserved credits: %w", err)
	}

	ledgerID, err := ids.New()
	if err != nil {
		return CreditReservation{}, fmt.Errorf("generate reservation ledger id: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_ledger (id, organization_id, reservation_id, amount_credits, type, reason, created_at)
		VALUES ($1, $2, $3, $4, 'reserve', 'credit reservation', $5)
	`, ledgerID, orgID, resID, -amount, now)
	if err != nil {
		return CreditReservation{}, fmt.Errorf("insert reserve ledger entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CreditReservation{}, fmt.Errorf("commit reservation: %w", err)
	}
	return CreditReservation{
		ID:             resID,
		OrganizationID: orgID,
		AmountCredits:  amount,
		Status:         "reserved",
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
	}, nil
}

// Settle settles the reservation with actual usage. The ledger stores only the
// balance adjustment; any uncollectable extra usage is recorded as debt.
func (s *CreditLedgerService) Settle(ctx context.Context, reservationID string, actualAmount int64) error {
	if actualAmount < 0 {
		return errors.New("actual amount cannot be negative")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin settlement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

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
		return fmt.Errorf("load reservation for settlement: %w", err)
	}
	if status != "reserved" {
		return ErrReservationAlreadyClosed
	}

	adjustment := reservedAmount - actualAmount
	debt := int64(0)
	if adjustment > 0 {
		_, err = tx.Exec(ctx, `
			UPDATE go_billing_credits
			SET balance_credits = balance_credits + $1, updated_at = now()
			WHERE organization_id = $2
		`, adjustment, orgID)
		if err != nil {
			return fmt.Errorf("refund unused credits: %w", err)
		}
	} else if adjustment < 0 {
		extra := -adjustment
		var balance int64
		err = tx.QueryRow(ctx, `
			SELECT balance_credits FROM go_billing_credits WHERE organization_id = $1 FOR UPDATE
		`, orgID).Scan(&balance)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("load settlement balance: %w", err)
		}
		if err != nil {
			return fmt.Errorf("lock settlement balance: %w", err)
		}
		deduct := balance
		if deduct > extra {
			deduct = extra
		}
		debt = extra - deduct
		if deduct > 0 {
			_, err = tx.Exec(ctx, `
				UPDATE go_billing_credits
				SET balance_credits = balance_credits - $1, updated_at = now()
				WHERE organization_id = $2
			`, deduct, orgID)
			if err != nil {
				return fmt.Errorf("deduct extra credits: %w", err)
			}
		}
		adjustment = -deduct
	}

	_, err = tx.Exec(ctx, `
		UPDATE go_billing_credit_reservations SET status = 'settled', updated_at = now() WHERE id = $1
	`, reservationID)
	if err != nil {
		return fmt.Errorf("update reservation status: %w", err)
	}
	ledgerID, err := ids.New()
	if err != nil {
		return fmt.Errorf("generate settlement ledger id: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_ledger (id, organization_id, reservation_id, amount_credits, type, reason, created_at)
		VALUES ($1, $2, $3, $4, 'settle', 'settlement balance adjustment', now())
	`, ledgerID, orgID, reservationID, adjustment)
	if err != nil {
		return fmt.Errorf("insert settlement ledger entry: %w", err)
	}
	if debt > 0 {
		debtID, err := ids.New()
		if err != nil {
			return fmt.Errorf("generate debt ledger id: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO go_billing_credit_ledger (id, organization_id, reservation_id, amount_credits, type, reason, created_at)
			VALUES ($1, $2, $3, $4, 'debt', 'uncollected settlement shortfall', now())
		`, debtID, orgID, reservationID, -debt)
		if err != nil {
			return fmt.Errorf("record settlement debt: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit settlement: %w", err)
	}
	return nil
}

// Refund refunds a reservation in full.
func (s *CreditLedgerService) Refund(ctx context.Context, reservationID string, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

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

	ledgerID, err := ids.New()
	if err != nil {
		return fmt.Errorf("generate refund ledger id: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_ledger (id, organization_id, reservation_id, amount_credits, type, reason, created_at)
		VALUES ($1, $2, $3, $4, 'refund', $5, now())
	`, ledgerID, orgID, reservationID, reservedAmount, reason)
	if err != nil {
		return fmt.Errorf("insert ledger refund: %w", err)
	}

	return tx.Commit(ctx)
}
