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

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
)

var (
	// ErrInsufficientBalance means the org does not have enough credits.
	ErrInsufficientBalance = errors.New("insufficient credit balance")
	// ErrReservationNotFound means the reservation ID does not exist.
	ErrReservationNotFound = errors.New("credit reservation not found")
	// ErrReservationAlreadyClosed means the reservation is already settled or refunded.
	ErrReservationAlreadyClosed = errors.New("credit reservation already settled or refunded")
)

// staffTopUpCredits is the least a staff workspace is granted when it runs
// low: ten dollars of provider cost.
const staffTopUpCredits = 10 * CreditsPerUSD

const (
	// CreditMarkupMultiplier is legacy 1.28 markup on provider cost.
	CreditMarkupMultiplier = 1.28
	// CreditsPerUSD is legacy rate: 1000 credits = $1.00 USD.
	CreditsPerUSD = 1000
)

// CreditReservation is a pending reservation of organization AI credits.
type CreditReservation struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	AmountCredits  int64     `json:"amountCredits"`
	Status         string    `json:"status"`
	IdempotencyKey string    `json:"idempotencyKey,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

// CreditLedgerService reserves and settles organization AI credits.
type CreditLedgerService struct {
	db *pgxpool.Pool
	// Unlimited reports whether an organization is a staff workspace, which
	// is topped up instead of refused. Nil means no workspace is.
	Unlimited func(ctx context.Context, orgID string) (bool, error)
}

// NewCreditLedgerService creates a credit ledger service for db.
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
	return pgdb.InTx(ctx, s.db, func(tx pgx.Tx) error {
		return grantCreditsTx(ctx, tx, orgID, amount, reason)
	})
}

// grantUpToTx grants amount, but never past a balance of ceiling, inside the
// caller's transaction. It grants nothing when the balance is already there.
func grantUpToTx(ctx context.Context, tx pgx.Tx, orgID string, amount, ceiling int64, reason string) error {
	var balance int64
	err := tx.QueryRow(ctx, `SELECT balance_credits FROM go_billing_credits WHERE organization_id = $1 FOR UPDATE`, orgID).Scan(&balance)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lock balance for grant: %w", err)
	}
	if grant := min(amount, ceiling-balance); grant > 0 {
		return grantCreditsTx(ctx, tx, orgID, grant, reason)
	}
	return nil
}

// grantCreditsTx adds amount to the balance and writes the ledger row inside
// the caller's transaction.
func grantCreditsTx(ctx context.Context, tx pgx.Tx, orgID string, amount int64, reason string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO go_billing_credits (organization_id, balance_credits, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (organization_id)
		DO UPDATE SET balance_credits = go_billing_credits.balance_credits + EXCLUDED.balance_credits, updated_at = now()
	`, orgID, amount)
	if err != nil {
		return fmt.Errorf("upsert credits: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_ledger (id, organization_id, amount_credits, type, reason, created_at)
		VALUES ($1, $2, $3, 'grant', $4, now())
	`, generateID("led"), orgID, amount, reason)
	if err != nil {
		return fmt.Errorf("insert ledger grant: %w", err)
	}
	return nil
}

// UsageEvent is one settled spend, shaped for the billing usage chart.
type UsageEvent struct {
	Timestamp  float64        `json:"timestamp"`
	Value      float64        `json:"value"`
	Properties map[string]any `json:"properties"`
}

// UsageEvents lists the organization's settled spends between two Unix
// millisecond timestamps, oldest first.
func (s *CreditLedgerService) UsageEvents(ctx context.Context, orgID string, startMs, endMs float64) ([]UsageEvent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT (extract(epoch FROM created_at) * 1000)::float8, (-amount_credits)::float8
		FROM go_billing_credit_ledger
		WHERE organization_id = $1 AND type = 'settle' AND amount_credits < 0
			AND created_at >= to_timestamp($2 / 1000.0) AND created_at <= to_timestamp($3 / 1000.0)
		ORDER BY created_at ASC, id ASC
		LIMIT 5000
	`, orgID, startMs, endMs)
	if err != nil {
		return nil, fmt.Errorf("list usage events: %w", err)
	}
	defer rows.Close()
	events := make([]UsageEvent, 0)
	for rows.Next() {
		e := UsageEvent{Properties: map[string]any{}}
		if err := rows.Scan(&e.Timestamp, &e.Value); err != nil {
			return nil, fmt.Errorf("scan usage event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage events: %w", err)
	}
	return events, nil
}

// topUpStaff keeps a staff workspace from running out: when its balance
// cannot cover amount, it is granted enough. The grant and the spend are
// both in the ledger, so what staff use is still counted.
func (s *CreditLedgerService) topUpStaff(ctx context.Context, orgID string, amount int64) error {
	if s.Unlimited == nil {
		return nil
	}
	unlimited, err := s.Unlimited(ctx, orgID)
	if err != nil {
		return fmt.Errorf("check staff workspace: %w", err)
	}
	if !unlimited {
		return nil
	}
	balance, err := s.GetBalance(ctx, orgID)
	if err != nil {
		return err
	}
	if balance >= amount {
		return nil
	}
	return s.AddCredits(ctx, orgID, max(amount-balance, staffTopUpCredits), "staff workspace")
}

// Reserve atomically reserves credits for an operation using idempotency key.
func (s *CreditLedgerService) Reserve(ctx context.Context, orgID string, amount int64, idempotencyKey string) (CreditReservation, error) {
	return s.ReserveFor(ctx, orgID, amount, idempotencyKey, "")
}

// ReserveFor is Reserve for a named feature; the feature labels the settled
// spend in the ledger.
func (s *CreditLedgerService) ReserveFor(ctx context.Context, orgID string, amount int64, idempotencyKey, feature string) (CreditReservation, error) {
	if amount <= 0 {
		return CreditReservation{}, errors.New("amount must be positive")
	}
	if err := s.topUpStaff(ctx, orgID, amount); err != nil {
		return CreditReservation{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CreditReservation{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	reservation, err := reserveTx(ctx, tx, orgID, amount, idempotencyKey, feature)
	if err != nil {
		return CreditReservation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CreditReservation{}, fmt.Errorf("commit tx: %w", err)
	}
	return reservation, nil
}

// reserveTx deducts amount from the balance inside tx and records the
// reservation. The balance row is locked, so concurrent reservations cannot
// both take the last credits. A known idempotency key returns its reservation.
func reserveTx(ctx context.Context, tx pgx.Tx, orgID string, amount int64, idempotencyKey, feature string) (CreditReservation, error) {
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
	err := tx.QueryRow(ctx, `
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
		INSERT INTO go_billing_credit_reservations (id, organization_id, amount_credits, status, idempotency_key, feature, created_at, updated_at)
		VALUES ($1, $2, $3, 'reserved', $4, $6, $5, $5)
	`, resID, orgID, amount, ik, nowTime, feature)
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

	return CreditReservation{
		ID:             resID,
		OrganizationID: orgID,
		AmountCredits:  amount,
		Status:         "reserved",
		IdempotencyKey: idempotencyKey,
		CreatedAt:      nowTime,
	}, nil
}

// Settle settles a reservation with actual used credits, returning any unused
// credits to balance. When actual exceeds the reservation and the balance
// cannot cover the difference, it fails with ErrInsufficientBalance.
func (s *CreditLedgerService) Settle(ctx context.Context, reservationID string, actualAmount int64) error {
	_, err := s.settle(ctx, reservationID, actualAmount, false)
	return err
}

// SettleUpTo settles like Settle for work that has already been paid for at
// the provider: when the balance cannot cover what actual exceeds the
// reservation by, it charges what is left down to zero instead of failing.
// It returns the credits charged.
func (s *CreditLedgerService) SettleUpTo(ctx context.Context, reservationID string, actualAmount int64) (int64, error) {
	return s.settle(ctx, reservationID, actualAmount, true)
}

func (s *CreditLedgerService) settle(ctx context.Context, reservationID string, actualAmount int64, capped bool) (int64, error) {
	if actualAmount < 0 {
		return 0, errors.New("actual amount cannot be negative")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var orgID, status, feature string
	var reservedAmount int64
	err = tx.QueryRow(ctx, `
		SELECT organization_id, amount_credits, status, feature
		FROM go_billing_credit_reservations
		WHERE id = $1 FOR UPDATE
	`, reservationID).Scan(&orgID, &reservedAmount, &status, &feature)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrReservationNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("query reservation: %w", err)
	}

	if status != "reserved" {
		return 0, ErrReservationAlreadyClosed
	}

	refundAmount := reservedAmount - actualAmount
	if refundAmount < 0 {
		extraNeeded := -refundAmount
		var currentBal int64
		err = tx.QueryRow(ctx, `SELECT balance_credits FROM go_billing_credits WHERE organization_id = $1 FOR UPDATE`, orgID).Scan(&currentBal)
		if errors.Is(err, pgx.ErrNoRows) {
			currentBal = 0
		} else if err != nil {
			return 0, fmt.Errorf("lock balance: %w", err)
		}
		if currentBal < extraNeeded {
			if !capped {
				return 0, ErrInsufficientBalance
			}
			extraNeeded = currentBal
			actualAmount = reservedAmount + currentBal
		}
		if extraNeeded > 0 {
			_, err = tx.Exec(ctx, `UPDATE go_billing_credits SET balance_credits = balance_credits - $1, updated_at = now() WHERE organization_id = $2`, extraNeeded, orgID)
			if err != nil {
				return 0, fmt.Errorf("deduct extra balance: %w", err)
			}
		}
	} else if refundAmount > 0 {
		_, err = tx.Exec(ctx, `UPDATE go_billing_credits SET balance_credits = balance_credits + $1, updated_at = now() WHERE organization_id = $2`, refundAmount, orgID)
		if err != nil {
			return 0, fmt.Errorf("refund unused balance: %w", err)
		}
	}

	_, err = tx.Exec(ctx, `UPDATE go_billing_credit_reservations SET status = 'settled', updated_at = now() WHERE id = $1`, reservationID)
	if err != nil {
		return 0, fmt.Errorf("update reservation status: %w", err)
	}

	reason := "settled reservation"
	if feature != "" {
		reason = feature
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_ledger (id, organization_id, reservation_id, amount_credits, type, reason, created_at)
		VALUES ($1, $2, $3, $4, 'settle', $5, now())
	`, generateID("led"), orgID, reservationID, -actualAmount, reason)
	if err != nil {
		return 0, fmt.Errorf("insert ledger settle: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit settle: %w", err)
	}
	return actualAmount, nil
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

// FeatureSpend is the credits a workspace spent on one feature.
type FeatureSpend struct {
	Feature string `json:"feature"`
	Credits int64  `json:"credits"`
}

// SpendByFeature sums the credits orgID was charged since since, per
// feature, largest first. Charges made before features were labelled are
// grouped as "other".
func (s *CreditLedgerService) SpendByFeature(ctx context.Context, orgID string, since time.Time) ([]FeatureSpend, error) {
	rows, err := s.db.Query(ctx, `
		SELECT CASE WHEN reason = ANY($3) THEN reason ELSE 'other' END AS feature, sum(-amount_credits)::bigint
		FROM go_billing_credit_ledger
		WHERE organization_id = $1 AND type = 'settle' AND created_at >= $2
		GROUP BY 1
		HAVING sum(-amount_credits) > 0
		ORDER BY 2 DESC, 1
	`, orgID, since, featureNames())
	if err != nil {
		return nil, fmt.Errorf("sum spend by feature: %w", err)
	}
	defer rows.Close()
	out := make([]FeatureSpend, 0)
	for rows.Next() {
		var f FeatureSpend
		if err := rows.Scan(&f.Feature, &f.Credits); err != nil {
			return nil, fmt.Errorf("scan feature spend: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feature spend: %w", err)
	}
	return out, nil
}
