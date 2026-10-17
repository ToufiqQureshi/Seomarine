package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
)

// CreditsForUSD converts provider cost to credits: the markup, rounded up to
// a whole credit, so a real charge is never zero.
func CreditsForUSD(costUSD float64) int64 {
	if costUSD <= 0 || math.IsNaN(costUSD) || math.IsInf(costUSD, 0) {
		return 0
	}
	return int64(math.Ceil(costUSD * CreditMarkupMultiplier * CreditsPerUSD))
}

// SpendGuard is the one gate every paid provider request goes through. It
// implements spend.Guard for DataForSEO and, through AIMeter, meters AI
// models. It fails closed: when it cannot read the ledger, nothing is sent.
type SpendGuard struct {
	ledger *CreditLedgerService
	db     *pgxpool.Pool
	// dailyLimitUSD stops all paid provider work, for every workspace, once
	// today's (UTC) provider spend would pass it. It is the last line against
	// a bug or an abuser costing Uplumo more than one day's limit.
	dailyLimitUSD float64
	// chargeCredits is false on a server without payments (self-hosted):
	// workspaces are not charged and only the daily limit applies.
	chargeCredits bool
	logger        *slog.Logger
	// WorkspaceDailyLimitUSD is the most one workspace may spend at the
	// providers in a UTC day (0 is no limit), so one heavy workspace uses up
	// only its own day and never pauses the others. Nil means no limit.
	WorkspaceDailyLimitUSD func(ctx context.Context, orgID string) (float64, error)
	// WithinPlanLimits pauses paid work after a downgrade without deleting data.
	WithinPlanLimits func(ctx context.Context, orgID string) (bool, error)
}

// NewSpendGuard returns the guard. dailyLimitUSD must be positive.
func NewSpendGuard(db *pgxpool.Pool, ledger *CreditLedgerService, dailyLimitUSD float64, chargeCredits bool, logger *slog.Logger) (*SpendGuard, error) {
	if db == nil || ledger == nil || logger == nil {
		return nil, errors.New("spend guard needs a database, a ledger and a logger")
	}
	if !(dailyLimitUSD > 0) || math.IsInf(dailyLimitUSD, 0) {
		return nil, fmt.Errorf("daily provider spend limit must be a positive number of dollars, got %v", dailyLimitUSD)
	}
	return &SpendGuard{ledger: ledger, db: db, dailyLimitUSD: dailyLimitUSD, chargeCredits: chargeCredits, logger: logger}, nil
}

// Hold implements spend.Guard: it checks the daily limits and reserves the
// request's ceiling from the workspace's credits.
//
// The workspace's daily check and its reservation happen in one transaction
// under a per-workspace advisory lock, and both daily limits count holds still
// in flight, so parallel requests cannot together start more than a
// workspace's limit allows. The lock is per workspace: one workspace's
// requests never wait on another's.
func (g *SpendGuard) Hold(ctx context.Context, req spend.Request) (spend.Hold, error) {
	if req.OrganizationID == "" || req.Feature == "" {
		return nil, errors.New("a spend hold needs an organization and a feature")
	}
	if req.EstimateUSD < 0 || math.IsNaN(req.EstimateUSD) || math.IsInf(req.EstimateUSD, 0) {
		return nil, fmt.Errorf("invalid spend estimate %v", req.EstimateUSD)
	}
	if len(req.IdempotencyKey) > 128 || strings.TrimSpace(req.IdempotencyKey) != req.IdempotencyKey {
		return nil, errors.New("invalid spend idempotency key")
	}
	h := &creditHold{guard: g, org: req.OrganizationID, feature: req.Feature, endpoint: req.Endpoint}
	if g.chargeCredits && g.WithinPlanLimits != nil {
		within, err := g.WithinPlanLimits(ctx, req.OrganizationID)
		if err != nil {
			return nil, fmt.Errorf("check workspace plan limits: %w", err)
		}
		if !within {
			return nil, spend.ErrPlanLimits
		}
	}
	credits := CreditsForUSD(req.EstimateUSD)
	if !g.chargeCredits || credits == 0 {
		// Nothing is reserved, so there is nothing to make atomic.
		if err := g.checkDaily(ctx, g.db, req.EstimateUSD); err != nil {
			return nil, err
		}
		if err := g.checkWorkspaceDaily(ctx, g.db, req.OrganizationID, req.EstimateUSD); err != nil {
			return nil, err
		}
		return h, nil
	}
	if err := g.ledger.topUpStaff(ctx, req.OrganizationID, credits); err != nil {
		return nil, err
	}
	if err := g.ledger.expireCreditsForOrganization(ctx, req.OrganizationID); err != nil {
		return nil, fmt.Errorf("expire workspace credits: %w", err)
	}
	tx, err := g.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin spend hold: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('spend:' || $1, 0))`, req.OrganizationID); err != nil {
		return nil, fmt.Errorf("lock workspace spend: %w", err)
	}
	if err := g.checkDaily(ctx, tx, req.EstimateUSD); err != nil {
		return nil, err
	}
	if err := g.checkWorkspaceDaily(ctx, tx, req.OrganizationID, req.EstimateUSD); err != nil {
		return nil, err
	}
	reservation, err := reserveTx(ctx, tx, req.OrganizationID, credits, req.IdempotencyKey, spend.UserIDFromContext(ctx), string(req.Feature), req.Endpoint)
	if errors.Is(err, ErrInsufficientBalance) {
		return nil, spend.ErrNoCredits
	}
	if err != nil {
		return nil, fmt.Errorf("hold %d credits for %s: %w", credits, req.Feature, err)
	}
	if reservation.Reused {
		return nil, spend.ErrIdempotentReplay
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit spend hold: %w", err)
	}
	h.reservationID, h.reserved = reservation.ID, reservation.AmountCredits
	return h, nil
}

// checkDaily refuses with spend.ErrPaused when today's spend plus estimate
// would pass the global emergency limit. It is not locked across workspaces
// (that would make every workspace wait on every other), so concurrent holds
// in different workspaces can pass it by at most their own estimates.
func (g *SpendGuard) checkDaily(ctx context.Context, q querier, estimateUSD float64) error {
	spent, err := spendTodayUSD(ctx, q, "")
	if err != nil {
		return err
	}
	if spent+estimateUSD > g.dailyLimitUSD {
		g.logger.ErrorContext(ctx, "daily provider spend limit reached; paid provider work is paused",
			"spent_usd", spent, "limit_usd", g.dailyLimitUSD)
		return spend.ErrPaused
	}
	return nil
}

// checkWorkspaceDaily refuses with spend.ErrWorkspaceDailyLimit when orgID's
// spend today plus estimate would pass its own daily limit.
func (g *SpendGuard) checkWorkspaceDaily(ctx context.Context, q querier, orgID string, estimateUSD float64) error {
	if !g.chargeCredits || g.WorkspaceDailyLimitUSD == nil {
		return nil
	}
	limit, err := g.WorkspaceDailyLimitUSD(ctx, orgID)
	if err != nil {
		return fmt.Errorf("read workspace daily limit: %w", err)
	}
	if limit <= 0 {
		return nil
	}
	spent, err := spendTodayUSD(ctx, q, orgID)
	if err != nil {
		return err
	}
	if spent+estimateUSD > limit {
		return spend.ErrWorkspaceDailyLimit
	}
	return nil
}

// spendTodayUSD is the provider spend settled since midnight UTC plus the
// holds still in flight from today, for one workspace or ("" ) all of them.
// A hold is counted at its ceiling until it settles; a hold left open by a
// crash counts until midnight, which errs on the side of spending less.
func spendTodayUSD(ctx context.Context, q querier, orgID string) (float64, error) {
	var spent float64
	err := q.QueryRow(ctx, `
		SELECT
			(SELECT COALESCE(sum(cost_usd), 0)::float8 FROM go_provider_spend
			 WHERE created_at >= date_trunc('day', now(), 'UTC') AND ($1 = '' OR organization_id = $1))
			+ (SELECT COALESCE(sum(amount_credits), 0)::float8 FROM go_billing_credit_reservations
			 WHERE status = 'reserved' AND created_at >= date_trunc('day', now(), 'UTC') AND ($1 = '' OR organization_id = $1))
			  / $2
	`, orgID, CreditMarkupMultiplier*CreditsPerUSD).Scan(&spent)
	if err != nil {
		return 0, fmt.Errorf("read today's provider spend: %w", err)
	}
	return spent, nil
}

type creditHold struct {
	guard         *SpendGuard
	org           string
	feature       spend.Feature
	endpoint      string
	reservationID string
	reserved      int64

	mu      sync.Mutex
	settled bool
}

// Settle charges the real cost, releases the rest of the hold and logs the
// spend for the daily limit. It may be called once.
func (h *creditHold) Settle(ctx context.Context, costUSD float64) error {
	return h.SettleProviderCost(ctx, costUSD, costUSD, "")
}

func (h *creditHold) SettleProviderCost(ctx context.Context, providerCostUSD, customerCostUSD float64, endpoint string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.settled {
		return errors.New("spend hold already settled")
	}
	h.settled = true
	if providerCostUSD < 0 || math.IsNaN(providerCostUSD) || math.IsInf(providerCostUSD, 0) || customerCostUSD < 0 || math.IsNaN(customerCostUSD) || math.IsInf(customerCostUSD, 0) {
		// The cost is unknown, so keep the whole hold rather than give it back.
		if h.reservationID != "" {
			if _, err := h.guard.ledger.SettleUpTo(ctx, h.reservationID, h.reserved); err != nil {
				return fmt.Errorf("settle hold at its ceiling: %w", err)
			}
		}
		return fmt.Errorf("invalid provider/customer cost %v/%v for %s", providerCostUSD, customerCostUSD, h.feature)
	}
	if endpoint == "" {
		endpoint = h.endpoint
	}
	return h.guard.settle(ctx, h.org, h.feature, endpoint, h.reservationID, providerCostUSD, customerCostUSD)
}

// settle charges costUSD against reservationID, or against the balance when
// there is no reservation, and logs it.
func (g *SpendGuard) settle(ctx context.Context, org string, feature spend.Feature, endpoint, reservationID string, providerCostUSD, customerCostUSD float64) error {
	if providerCostUSD > 0 {
		if _, err := g.db.Exec(ctx, `INSERT INTO go_provider_spend (organization_id, feature, cost_usd) VALUES ($1, $2, $3)`,
			org, string(feature), providerCostUSD); err != nil {
			return fmt.Errorf("log provider spend: %w", err)
		}
	}
	credits := CreditsForUSD(customerCostUSD)
	if reservationID != "" {
		charged, err := g.ledger.SettleProviderUpTo(ctx, reservationID, credits, providerCostUSD, endpoint)
		if err != nil {
			return fmt.Errorf("settle %s hold: %w", feature, err)
		}
		g.warnShort(ctx, org, feature, credits, charged)
		return nil
	}
	if !g.chargeCredits || credits == 0 {
		return nil
	}
	return g.chargeDown(ctx, org, feature, endpoint, credits, providerCostUSD)
}

// chargeDown charges credits for work already paid for, down to a zero
// balance.
func (g *SpendGuard) chargeDown(ctx context.Context, org string, feature spend.Feature, endpoint string, credits int64, costUSD float64) error {
	reservation, err := g.ledger.ReserveFor(ctx, org, credits, "", string(feature))
	if errors.Is(err, ErrInsufficientBalance) {
		balance, balanceErr := g.ledger.GetBalance(ctx, org)
		if balanceErr != nil {
			return fmt.Errorf("read balance after shortfall: %w", balanceErr)
		}
		g.warnShort(ctx, org, feature, credits, balance)
		if balance <= 0 {
			return nil
		}
		reservation, err = g.ledger.ReserveFor(ctx, org, balance, "", string(feature))
	}
	if err != nil {
		return fmt.Errorf("charge %d credits for %s: %w", credits, feature, err)
	}
	if _, err := g.ledger.SettleProviderUpTo(ctx, reservation.ID, credits, costUSD, endpoint); err != nil {
		return fmt.Errorf("settle %s charge: %w", feature, err)
	}
	return nil
}

// warnShort logs provider work the workspace could not fully pay for. The
// hold should make this rare; frequent warnings mean an estimate is too low.
func (g *SpendGuard) warnShort(ctx context.Context, org string, feature spend.Feature, wanted, charged int64) {
	if charged < wanted {
		g.logger.WarnContext(ctx, "provider work cost more credits than the workspace had",
			"organization", org, "feature", feature, "credits_wanted", wanted, "credits_charged", charged)
	}
}

// AIMeter meters one AI feature: it says whether a workspace may start AI
// work and charges the model's cost afterwards.
type AIMeter struct {
	guard    *SpendGuard
	feature  spend.Feature
	endpoint string
}

// AIMeter returns the meter for feature.
func (g *SpendGuard) AIMeter(feature spend.Feature, endpoint ...string) AIMeter {
	var provider string
	if len(endpoint) > 0 {
		provider = endpoint[0]
	}
	return AIMeter{guard: g, feature: feature, endpoint: provider}
}

// Hold reserves the most one AI model call can cost before it is sent; the
// caller settles it with the real cost afterwards. A model's cost is only
// known once it answers, so holding the ceiling is what keeps parallel AI
// work from spending past the workspace's credits.
func (m AIMeter) Hold(ctx context.Context, org string, ceilingUSD float64) (spend.Hold, error) {
	return m.guard.Hold(ctx, spend.Request{OrganizationID: org, Feature: m.feature, Endpoint: m.endpoint, IdempotencyKey: spend.IdempotencyKeyFromContext(ctx), EstimateUSD: ceilingUSD})
}

func (m AIMeter) HoldWithIdempotencyKey(ctx context.Context, org string, ceilingUSD float64, key string) (spend.Hold, error) {
	return m.Hold(spend.WithIdempotencyKey(ctx, key), org, ceilingUSD)
}
