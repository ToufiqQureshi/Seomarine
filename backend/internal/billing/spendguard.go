package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
)

// minAICredits is the least a workspace needs to start AI work: one cent of
// provider cost. AI cost is only known after the model answers, so a turn is
// let in while there is something left and charged down to zero.
var minAICredits = CreditsForUSD(0.01)

// aiStepEstimateUSD is what one AI model call is assumed to cost when
// checking the daily limit before it starts.
const aiStepEstimateUSD = 0.05

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

// Hold implements spend.Guard: it checks the daily limit and reserves the
// request's ceiling from the workspace's credits.
func (g *SpendGuard) Hold(ctx context.Context, req spend.Request) (spend.Hold, error) {
	if req.OrganizationID == "" || req.Feature == "" {
		return nil, errors.New("a spend hold needs an organization and a feature")
	}
	if req.EstimateUSD < 0 || math.IsNaN(req.EstimateUSD) || math.IsInf(req.EstimateUSD, 0) {
		return nil, fmt.Errorf("invalid spend estimate %v", req.EstimateUSD)
	}
	if err := g.checkDaily(ctx, req.EstimateUSD); err != nil {
		return nil, err
	}
	if err := g.checkWorkspaceDaily(ctx, req.OrganizationID, req.EstimateUSD); err != nil {
		return nil, err
	}
	h := &creditHold{guard: g, org: req.OrganizationID, feature: req.Feature}
	credits := CreditsForUSD(req.EstimateUSD)
	if !g.chargeCredits || credits == 0 {
		return h, nil
	}
	reservation, err := g.ledger.ReserveFor(ctx, req.OrganizationID, credits, "", string(req.Feature))
	if errors.Is(err, ErrInsufficientBalance) {
		return nil, spend.ErrNoCredits
	}
	if err != nil {
		return nil, fmt.Errorf("hold %d credits for %s: %w", credits, req.Feature, err)
	}
	h.reservationID, h.reserved = reservation.ID, reservation.AmountCredits
	return h, nil
}

// checkDaily refuses with spend.ErrPaused when today's spend plus estimate
// would pass the daily limit.
func (g *SpendGuard) checkDaily(ctx context.Context, estimateUSD float64) error {
	spent, err := g.SpentTodayUSD(ctx)
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
func (g *SpendGuard) checkWorkspaceDaily(ctx context.Context, orgID string, estimateUSD float64) error {
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
	var spent float64
	if err := g.db.QueryRow(ctx, `
		SELECT COALESCE(sum(cost_usd), 0)::float8 FROM go_provider_spend
		WHERE organization_id = $1 AND created_at >= date_trunc('day', now(), 'UTC')
	`, orgID).Scan(&spent); err != nil {
		return fmt.Errorf("read workspace spend today: %w", err)
	}
	if spent+estimateUSD > limit {
		return spend.ErrWorkspaceDailyLimit
	}
	return nil
}

// SpentTodayUSD is the provider spend recorded since midnight UTC.
func (g *SpendGuard) SpentTodayUSD(ctx context.Context) (float64, error) {
	var spent float64
	err := g.db.QueryRow(ctx, `
		SELECT COALESCE(sum(cost_usd), 0)::float8 FROM go_provider_spend
		WHERE created_at >= date_trunc('day', now(), 'UTC')
	`).Scan(&spent)
	if err != nil {
		return 0, fmt.Errorf("read today's provider spend: %w", err)
	}
	return spent, nil
}

type creditHold struct {
	guard         *SpendGuard
	org           string
	feature       spend.Feature
	reservationID string
	reserved      int64

	mu      sync.Mutex
	settled bool
}

// Settle charges the real cost, releases the rest of the hold and logs the
// spend for the daily limit. It may be called once.
func (h *creditHold) Settle(ctx context.Context, costUSD float64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.settled {
		return errors.New("spend hold already settled")
	}
	h.settled = true
	if costUSD < 0 || math.IsNaN(costUSD) || math.IsInf(costUSD, 0) {
		// The cost is unknown, so keep the whole hold rather than give it back.
		if h.reservationID != "" {
			if _, err := h.guard.ledger.SettleUpTo(ctx, h.reservationID, h.reserved); err != nil {
				return fmt.Errorf("settle hold at its ceiling: %w", err)
			}
		}
		return fmt.Errorf("invalid provider cost %v for %s", costUSD, h.feature)
	}
	return h.guard.settle(ctx, h.org, h.feature, h.reservationID, costUSD, "")
}

// settle charges costUSD against reservationID, or against the balance when
// there is no reservation, and logs it. key makes a charge without a
// reservation idempotent.
func (g *SpendGuard) settle(ctx context.Context, org string, feature spend.Feature, reservationID string, costUSD float64, key string) error {
	if costUSD > 0 {
		if _, err := g.db.Exec(ctx, `INSERT INTO go_provider_spend (organization_id, feature, cost_usd) VALUES ($1, $2, $3)`,
			org, string(feature), costUSD); err != nil {
			return fmt.Errorf("log provider spend: %w", err)
		}
	}
	credits := CreditsForUSD(costUSD)
	if reservationID != "" {
		charged, err := g.ledger.SettleUpTo(ctx, reservationID, credits)
		if err != nil {
			return fmt.Errorf("settle %s hold: %w", feature, err)
		}
		g.warnShort(ctx, org, feature, credits, charged)
		return nil
	}
	if !g.chargeCredits || credits == 0 {
		return nil
	}
	return g.chargeDown(ctx, org, feature, credits, key)
}

// chargeDown charges credits for work already paid for, down to a zero
// balance. A replayed key is not charged twice.
func (g *SpendGuard) chargeDown(ctx context.Context, org string, feature spend.Feature, credits int64, key string) error {
	reservation, err := g.ledger.ReserveFor(ctx, org, credits, key, string(feature))
	if errors.Is(err, ErrInsufficientBalance) {
		balance, balanceErr := g.ledger.GetBalance(ctx, org)
		if balanceErr != nil {
			return fmt.Errorf("read balance after shortfall: %w", balanceErr)
		}
		g.warnShort(ctx, org, feature, credits, balance)
		if balance <= 0 {
			return nil
		}
		reservation, err = g.ledger.ReserveFor(ctx, org, balance, key, string(feature))
	}
	if err != nil {
		return fmt.Errorf("charge %d credits for %s: %w", credits, feature, err)
	}
	// A replayed key returns its earlier reservation, already settled.
	if _, err := g.ledger.SettleUpTo(ctx, reservation.ID, reservation.AmountCredits); err != nil && !errors.Is(err, ErrReservationAlreadyClosed) {
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
	guard   *SpendGuard
	feature spend.Feature
}

// AIMeter returns the meter for feature.
func (g *SpendGuard) AIMeter(feature spend.Feature) AIMeter {
	return AIMeter{guard: g, feature: feature}
}

// HasCredits reports whether the workspace may start AI work: paid work is
// not paused and, where workspaces are charged, it has at least a cent of
// credit left (staff workspaces are topped up). It fails closed.
func (m AIMeter) HasCredits(ctx context.Context, org string) (bool, error) {
	if err := m.guard.checkDaily(ctx, aiStepEstimateUSD); errors.Is(err, spend.ErrPaused) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if !m.guard.chargeCredits {
		return true, nil
	}
	if err := m.guard.checkWorkspaceDaily(ctx, org, aiStepEstimateUSD); errors.Is(err, spend.ErrWorkspaceDailyLimit) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if m.guard.ledger.Unlimited != nil {
		unlimited, err := m.guard.ledger.Unlimited(ctx, org)
		if err != nil {
			return false, fmt.Errorf("check staff workspace: %w", err)
		}
		if unlimited {
			return true, nil
		}
	}
	balance, err := m.guard.ledger.GetBalance(ctx, org)
	if err != nil {
		return false, err
	}
	return balance >= minAICredits, nil
}

// Charge charges an AI model's cost after the fact, down to a zero balance.
// key makes a retried charge idempotent.
func (m AIMeter) Charge(ctx context.Context, org string, costUSD float64, key string) error {
	if costUSD < 0 || math.IsNaN(costUSD) || math.IsInf(costUSD, 0) {
		return fmt.Errorf("invalid AI cost %v", costUSD)
	}
	if costUSD == 0 {
		return nil
	}
	return m.guard.settle(ctx, org, m.feature, "", costUSD, key)
}
