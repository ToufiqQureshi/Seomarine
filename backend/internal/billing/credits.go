package billing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
)

var (
	// ErrInsufficientBalance means the org does not have enough credits.
	ErrInsufficientBalance = errors.New("insufficient credit balance")
	// ErrReservationNotFound means the reservation ID does not exist.
	ErrReservationNotFound = errors.New("credit reservation not found")
	// ErrReservationAlreadyClosed means the reservation is already settled or refunded.
	ErrReservationAlreadyClosed = errors.New("credit reservation already settled or refunded")
	ErrIdempotentReplay         = errors.New("request with this idempotency key was already reserved")
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
	Reused         bool      `json:"-"`
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
	if err := s.expireCreditsForOrganization(ctx, orgID); err != nil {
		return 0, err
	}
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
		INSERT INTO go_billing_credit_ledger (id, organization_id, amount_credits, type, reason, feature, status, created_at)
		VALUES ($1, $2, $3, 'grant', $4, 'billing', 'granted', now())
	`, generateID("led"), orgID, amount, reason)
	if err != nil {
		return fmt.Errorf("insert ledger grant: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_grants (id, organization_id, granted_credits, remaining_credits, expires_at)
		VALUES ($1, $2, $3, $3, now() + interval '2 months')
	`, generateID("grt"), orgID, amount)
	if err != nil {
		return fmt.Errorf("insert expiring credit grant: %w", err)
	}
	return nil
}

// ExpireCredits removes expired unreserved grant balances. It is safe to run
// from both the hourly worker and request paths.
func (s *CreditLedgerService) ExpireCredits(ctx context.Context) (int64, error) {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT organization_id FROM go_billing_credit_grants WHERE expires_at <= now() AND remaining_credits > 0 ORDER BY organization_id`)
	if err != nil {
		return 0, fmt.Errorf("list expired credit workspaces: %w", err)
	}
	var organizations []string
	for rows.Next() {
		var org string
		if err := rows.Scan(&org); err != nil {
			rows.Close()
			return 0, fmt.Errorf("read expired credit workspace: %w", err)
		}
		organizations = append(organizations, org)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	var expired int64
	for _, org := range organizations {
		count, err := s.expireOrganizationCredits(ctx, org)
		if err != nil {
			return expired, err
		}
		expired += count
	}
	return expired, nil
}

func (s *CreditLedgerService) expireOrganizationCredits(ctx context.Context, org string) (int64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin credit expiry: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var balance int64
	err = tx.QueryRow(ctx, `SELECT balance_credits FROM go_billing_credits WHERE organization_id = $1 FOR UPDATE`, org).Scan(&balance)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("lock balance for credit expiry: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT id, remaining_credits FROM go_billing_credit_grants WHERE organization_id = $1 AND expires_at <= now() AND remaining_credits > 0 ORDER BY expires_at, created_at FOR UPDATE`, org)
	if err != nil {
		return 0, fmt.Errorf("lock expired credit grants: %w", err)
	}
	type expiredGrant struct {
		id      string
		credits int64
	}
	var grants []expiredGrant
	var total int64
	for rows.Next() {
		var grant expiredGrant
		if err := rows.Scan(&grant.id, &grant.credits); err != nil {
			rows.Close()
			return 0, err
		}
		grants = append(grants, grant)
		total += grant.credits
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if total > balance {
		return 0, fmt.Errorf("expired credit grants (%d) exceed workspace balance (%d)", total, balance)
	}
	for _, grant := range grants {
		if _, err := tx.Exec(ctx, `UPDATE go_billing_credit_grants SET remaining_credits = 0 WHERE id = $1`, grant.id); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO go_billing_credit_ledger (id, organization_id, amount_credits, type, reason, feature, status) VALUES ($1, $2, $3, 'expire', 'credits expired after two months', 'billing', 'expired')`, generateID("led"), org, -grant.credits); err != nil {
			return 0, err
		}
	}
	if total > 0 {
		if _, err := tx.Exec(ctx, `UPDATE go_billing_credits SET balance_credits = balance_credits - $1, updated_at = now() WHERE organization_id = $2`, total, org); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit credit expiry: %w", err)
	}
	return total, nil
}

func (s *CreditLedgerService) expireCreditsForOrganization(ctx context.Context, org string) error {
	_, err := s.expireOrganizationCredits(ctx, org)
	return err
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
	if err := s.expireCreditsForOrganization(ctx, orgID); err != nil {
		return CreditReservation{}, err
	}
	if err := s.topUpStaff(ctx, orgID, amount); err != nil {
		return CreditReservation{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CreditReservation{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	reservation, err := reserveTx(ctx, tx, orgID, amount, idempotencyKey, spend.UserIDFromContext(ctx), feature, "")
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
func reserveTx(ctx context.Context, tx pgx.Tx, orgID string, amount int64, idempotencyKey, userID, feature, endpoint string) (CreditReservation, error) {
	if idempotencyKey != "" {
		// Identical search requests in separate workspaces must have separate reservations.
		idempotencyKey = workspaceIdempotencyKey(orgID, idempotencyKey)
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('credit-request:' || $1, 0))`, idempotencyKey); err != nil {
			return CreditReservation{}, fmt.Errorf("lock idempotent credit request: %w", err)
		}
		var res CreditReservation
		var ik *string
		err := tx.QueryRow(ctx, `
			SELECT id, organization_id, amount_credits, status, idempotency_key, created_at
			FROM go_billing_credit_reservations
			WHERE idempotency_key = $1
		`, idempotencyKey).Scan(&res.ID, &res.OrganizationID, &res.AmountCredits, &res.Status, &ik, &res.CreatedAt)
		if err == nil {
			if res.OrganizationID != orgID {
				return CreditReservation{}, errors.New("idempotency key belongs to a different workspace")
			}
			if ik != nil {
				res.IdempotencyKey = *ik
			}
			res.Reused = true
			return res, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return CreditReservation{}, fmt.Errorf("read idempotent reservation: %w", err)
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

	resID := generateID("res")
	var ik *string
	if idempotencyKey != "" {
		ik = &idempotencyKey
	}
	nowTime := time.Now().UTC()
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_reservations (id, organization_id, amount_credits, status, idempotency_key, feature, endpoint, user_id, created_at, updated_at)
		VALUES ($1, $2, $3, 'reserved', $4, $6, $7, NULLIF($8, ''), $5, $5)
	`, resID, orgID, amount, ik, nowTime, feature, endpoint, userID)
	if err != nil {
		return CreditReservation{}, fmt.Errorf("insert reservation: %w", err)
	}

	if err := consumeCreditBucketsTx(ctx, tx, orgID, amount, resID); err != nil {
		return CreditReservation{}, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE go_billing_credits SET balance_credits = balance_credits - $1, updated_at = now()
		WHERE organization_id = $2
	`, amount, orgID)
	if err != nil {
		return CreditReservation{}, fmt.Errorf("deduct balance: %w", err)
	}
	ledgerID := generateID("led")
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_ledger (id, organization_id, reservation_id, user_id, amount_credits, type, reason, feature, endpoint, status, created_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, 'reserve', 'credit reservation', $7, $8, 'reserved', $6)
	`, ledgerID, orgID, resID, userID, -amount, nowTime, feature, endpoint)
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

func workspaceIdempotencyKey(orgID, key string) string {
	if key == "" {
		return ""
	}
	keyHash := sha256.Sum256([]byte(orgID + "\x00" + key))
	return hex.EncodeToString(keyHash[:])
}

func consumeCreditBucketsTx(ctx context.Context, tx pgx.Tx, orgID string, amount int64, reservationID string) error {
	rows, err := tx.Query(ctx, `SELECT id, remaining_credits FROM go_billing_credit_grants WHERE organization_id = $1 AND remaining_credits > 0 AND expires_at > now() ORDER BY expires_at, created_at, id FOR UPDATE`, orgID)
	if err != nil {
		return fmt.Errorf("lock credit grants: %w", err)
	}
	type bucket struct {
		id      string
		credits int64
	}
	var buckets []bucket
	for rows.Next() {
		var b bucket
		if err := rows.Scan(&b.id, &b.credits); err != nil {
			rows.Close()
			return err
		}
		buckets = append(buckets, b)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	remaining := amount
	for _, b := range buckets {
		used := min(remaining, b.credits)
		if used <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE go_billing_credit_grants SET remaining_credits = remaining_credits - $1 WHERE id = $2`, used, b.id); err != nil {
			return fmt.Errorf("consume credit grant: %w", err)
		}
		if reservationID != "" {
			if _, err := tx.Exec(ctx, `INSERT INTO go_billing_credit_reservation_grants (reservation_id, grant_id, amount_credits) VALUES ($1, $2, $3)`, reservationID, b.id, used); err != nil {
				return fmt.Errorf("allocate reserved credits: %w", err)
			}
		}
		remaining -= used
		if remaining == 0 {
			return nil
		}
	}
	return ErrInsufficientBalance
}

func restoreReservationCreditsTx(ctx context.Context, tx pgx.Tx, reservationID string, amount int64) (int64, error) {
	rows, err := tx.Query(ctx, `SELECT a.grant_id, a.amount_credits, g.expires_at FROM go_billing_credit_reservation_grants a JOIN go_billing_credit_grants g ON g.id = a.grant_id WHERE a.reservation_id = $1 ORDER BY g.expires_at DESC, g.created_at DESC FOR UPDATE OF g`, reservationID)
	if err != nil {
		return 0, err
	}
	type allocation struct {
		id      string
		credits int64
		expires time.Time
	}
	var allocations []allocation
	for rows.Next() {
		var a allocation
		if err := rows.Scan(&a.id, &a.credits, &a.expires); err != nil {
			rows.Close()
			return 0, err
		}
		allocations = append(allocations, a)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	remaining, restored := amount, int64(0)
	for _, a := range allocations {
		if remaining == 0 {
			break
		}
		give := min(remaining, a.credits)
		if a.expires.After(time.Now().UTC()) {
			if _, err := tx.Exec(ctx, `UPDATE go_billing_credit_grants SET remaining_credits = remaining_credits + $1 WHERE id = $2`, give, a.id); err != nil {
				return 0, err
			}
		} else {
			// Expired credits held by a long-running request stay expired.
		}
		remaining -= give
		if a.expires.After(time.Now().UTC()) {
			restored += give
		}
	}
	if remaining != 0 {
		return 0, errors.New("credit reservation allocation is incomplete")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM go_billing_credit_reservation_grants WHERE reservation_id = $1`, reservationID); err != nil {
		return 0, err
	}
	return restored, nil
}

// Settle settles a reservation with actual used credits, returning any unused
// credits to balance. When actual exceeds the reservation and the balance
// cannot cover the difference, it fails with ErrInsufficientBalance.
func (s *CreditLedgerService) Settle(ctx context.Context, reservationID string, actualAmount int64) error {
	_, err := s.settle(ctx, reservationID, actualAmount, false, 0, "")
	return err
}

// SettleUpTo settles like Settle for work that has already been paid for at
// the provider: when the balance cannot cover what actual exceeds the
// reservation by, it charges what is left down to zero instead of failing.
// It returns the credits charged.
func (s *CreditLedgerService) SettleUpTo(ctx context.Context, reservationID string, actualAmount int64) (int64, error) {
	return s.settle(ctx, reservationID, actualAmount, true, 0, "")
}

func (s *CreditLedgerService) SettleProviderUpTo(ctx context.Context, reservationID string, actualCredits int64, providerCostUSD float64, endpoint string) (int64, error) {
	return s.settle(ctx, reservationID, actualCredits, true, providerCostUSD, endpoint)
}

func (s *CreditLedgerService) settle(ctx context.Context, reservationID string, actualAmount int64, capped bool, providerCostUSD float64, endpoint string) (int64, error) {
	if actualAmount < 0 {
		return 0, errors.New("actual amount cannot be negative")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var orgID, status, feature, reservedEndpoint, userID string
	var reservedAmount int64
	err = tx.QueryRow(ctx, `
		SELECT organization_id, amount_credits, status, feature, endpoint, COALESCE(user_id, '')
		FROM go_billing_credit_reservations
		WHERE id = $1 FOR UPDATE
	`, reservationID).Scan(&orgID, &reservedAmount, &status, &feature, &reservedEndpoint, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrReservationNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("query reservation: %w", err)
	}

	if status != "reserved" {
		return 0, ErrReservationAlreadyClosed
	}
	if endpoint == "" {
		endpoint = reservedEndpoint
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
			if err := consumeCreditBucketsTx(ctx, tx, orgID, extraNeeded, ""); err != nil {
				return 0, fmt.Errorf("consume extra credits: %w", err)
			}
			_, err = tx.Exec(ctx, `UPDATE go_billing_credits SET balance_credits = balance_credits - $1, updated_at = now() WHERE organization_id = $2`, extraNeeded, orgID)
			if err != nil {
				return 0, fmt.Errorf("deduct extra balance: %w", err)
			}
		}
	} else if refundAmount > 0 {
		restored, restoreErr := restoreReservationCreditsTx(ctx, tx, reservationID, refundAmount)
		if restoreErr != nil {
			return 0, fmt.Errorf("restore unused credit grants: %w", restoreErr)
		}
		if restored > 0 {
			_, err = tx.Exec(ctx, `UPDATE go_billing_credits SET balance_credits = balance_credits + $1, updated_at = now() WHERE organization_id = $2`, restored, orgID)
			if err != nil {
				return 0, fmt.Errorf("refund unused balance: %w", err)
			}
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM go_billing_credit_reservation_grants WHERE reservation_id = $1`, reservationID); err != nil {
		return 0, fmt.Errorf("close credit grant allocations: %w", err)
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
		INSERT INTO go_billing_credit_ledger (id, organization_id, reservation_id, user_id, amount_credits, type, reason, feature, endpoint, provider_cost_usd, cached, status, created_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, 'settle', $6, $7, $8, $9, false, 'settled', now())
	`, generateID("led"), orgID, reservationID, userID, -actualAmount, reason, feature, endpoint, providerCostUSD)
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

	var orgID, status, feature, endpoint, userID string
	var reservedAmount int64
	err = tx.QueryRow(ctx, `
		SELECT organization_id, amount_credits, status, feature, endpoint, COALESCE(user_id, '')
		FROM go_billing_credit_reservations
		WHERE id = $1 FOR UPDATE
	`, reservationID).Scan(&orgID, &reservedAmount, &status, &feature, &endpoint, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrReservationNotFound
	}
	if err != nil {
		return fmt.Errorf("query reservation: %w", err)
	}

	if status != "reserved" {
		return ErrReservationAlreadyClosed
	}

	restored, err := restoreReservationCreditsTx(ctx, tx, reservationID, reservedAmount)
	if err != nil {
		return fmt.Errorf("restore refunded credit grants: %w", err)
	}
	if restored > 0 {
		_, err = tx.Exec(ctx, `UPDATE go_billing_credits SET balance_credits = balance_credits + $1, updated_at = now() WHERE organization_id = $2`, restored, orgID)
		if err != nil {
			return fmt.Errorf("refund balance: %w", err)
		}
	}

	_, err = tx.Exec(ctx, `UPDATE go_billing_credit_reservations SET status = 'refunded', updated_at = now() WHERE id = $1`, reservationID)
	if err != nil {
		return fmt.Errorf("update reservation status: %w", err)
	}

	ledgerID := generateID("led")
	_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_credit_ledger (id, organization_id, reservation_id, user_id, amount_credits, type, reason, feature, endpoint, status, created_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, 'refund', $6, $7, $8, 'refunded', now())
	`, ledgerID, orgID, reservationID, userID, restored, reason, feature, endpoint)
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
