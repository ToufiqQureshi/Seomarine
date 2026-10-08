package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
)

type repository struct {
	db *pgxpool.Pool
}

// subscription is an organization's stored subscription state.
type subscription struct {
	OrganizationID   string
	ID               string
	Plan             string
	Status           string
	CurrentPeriodEnd *time.Time
	// EventAt is the Razorpay time of this state.
	EventAt time.Time
}

// subscription returns orgID's subscription, and false when it has none.
func (r repository) subscription(ctx context.Context, orgID string) (subscription, bool, error) {
	var s subscription
	err := r.db.QueryRow(ctx, `
		SELECT razorpay_subscription_id, plan, status, current_period_end, event_at
		FROM go_billing_subscriptions WHERE organization_id = $1`,
		orgID,
	).Scan(&s.ID, &s.Plan, &s.Status, &s.CurrentPeriodEnd, &s.EventAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return subscription{}, false, nil
	}
	if err != nil {
		return subscription{}, false, fmt.Errorf("load subscription: %w", err)
	}
	return s, true, nil
}

// startSubscription stores a just-created subscription as orgID's, unless
// orgID's current subscription has one of liveStatuses. It reports whether
// the subscription was stored. event_at never moves back, so a late webhook
// about the replaced subscription stays stale.
func (r repository) startSubscription(ctx context.Context, orgID, plan, subID, status string, createdAt time.Time, liveStatuses []string) (bool, error) {
	var stored bool
	err := r.db.QueryRow(ctx, `
		INSERT INTO go_billing_subscriptions
			(organization_id, razorpay_subscription_id, plan, status, current_period_end, event_at)
		VALUES ($1, $2, $3, $4, NULL, $5)
		ON CONFLICT (organization_id) DO UPDATE SET
			razorpay_subscription_id = EXCLUDED.razorpay_subscription_id,
			plan = EXCLUDED.plan,
			status = EXCLUDED.status,
			current_period_end = NULL,
			event_at = greatest(go_billing_subscriptions.event_at, EXCLUDED.event_at),
			updated_at = now()
		WHERE go_billing_subscriptions.status <> ALL ($6)
		RETURNING true`,
		orgID, subID, plan, status, createdAt, liveStatuses,
	).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store subscription %s: %w", subID, err)
	}
	return true, nil
}

// applyEvent records eventID and stores s as its organization's
// subscription state, in one transaction. It returns false, changing
// nothing, when eventID was recorded before.
//
// The state is stored when it is at least as new as the stored state, and
// is either the same subscription or replaces one that is not in
// liveStatuses. An organization that no longer exists gets no row.
func (r repository) applyEvent(ctx context.Context, eventID string, s subscription, liveStatuses []string) (bool, error) {
	var firstDelivery bool
	err := pgdb.InTx(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
		INSERT INTO go_billing_webhook_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING`,
			eventID,
		)
		if err != nil {
			return fmt.Errorf("record event: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}

		_, err = tx.Exec(ctx, `
		INSERT INTO go_billing_subscriptions
			(organization_id, razorpay_subscription_id, plan, status, current_period_end, event_at)
		SELECT $1, $2, $3, $4, $5, $6 FROM organization WHERE id = $1
		ON CONFLICT (organization_id) DO UPDATE SET
			razorpay_subscription_id = EXCLUDED.razorpay_subscription_id,
			plan = EXCLUDED.plan,
			status = EXCLUDED.status,
			current_period_end = EXCLUDED.current_period_end,
			event_at = EXCLUDED.event_at,
			updated_at = now()
		WHERE go_billing_subscriptions.event_at <= EXCLUDED.event_at
			AND (go_billing_subscriptions.razorpay_subscription_id = EXCLUDED.razorpay_subscription_id
				OR go_billing_subscriptions.status <> ALL ($7))`,
			s.OrganizationID, s.ID, s.Plan, s.Status, s.CurrentPeriodEnd, s.EventAt, liveStatuses,
		)
		if err != nil {
			return fmt.Errorf("store subscription %s: %w", s.ID, err)
		}
		firstDelivery = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return firstDelivery, nil
}
