package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
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
	// CancelAtPeriodEnd is set when the customer cancelled and keeps the
	// plan until CurrentPeriodEnd.
	CancelAtPeriodEnd bool
	// EventAt is the Razorpay time of this state.
	EventAt time.Time
}

// subscription returns orgID's subscription, and false when it has none.
func (r repository) subscription(ctx context.Context, orgID string) (subscription, bool, error) {
	var s subscription
	err := r.db.QueryRow(ctx, `
		SELECT razorpay_subscription_id, plan, status, current_period_end, cancel_at_period_end, event_at
		FROM go_billing_subscriptions WHERE organization_id = $1`,
		orgID,
	).Scan(&s.ID, &s.Plan, &s.Status, &s.CurrentPeriodEnd, &s.CancelAtPeriodEnd, &s.EventAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return subscription{}, false, nil
	}
	if err != nil {
		return subscription{}, false, fmt.Errorf("load subscription: %w", err)
	}
	return s, true, nil
}

// ownedByAny reports whether an owner of orgID has one of emails as their
// verified address. emails must be lower case.
func (r repository) ownedByAny(ctx context.Context, orgID string, emails []string) (bool, error) {
	var owned bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM member m JOIN "user" u ON u.id = m.user_id
			WHERE m.organization_id = $1 AND m.role = 'owner'
				AND u.email_verified AND lower(u.email) = ANY($2))`,
		orgID, emails,
	).Scan(&owned)
	if err != nil {
		return false, fmt.Errorf("look up the owners of organization %s: %w", orgID, err)
	}
	return owned, nil
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
			cancel_at_period_end = false,
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

// markCancelled records that the customer cancelled subscription subID of
// orgID. status is the state Razorpay answered with: unchanged when the plan
// runs to the end of the paid cycle (atPeriodEnd), "cancelled" when it ended
// now. It reports false when orgID's subscription is no longer subID.
//
// event_at is left alone: the webhook that follows carries the full state
// and must still be applied.
func (r repository) markCancelled(ctx context.Context, orgID, subID, status string, atPeriodEnd bool) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE go_billing_subscriptions
		SET status = $3, cancel_at_period_end = $4, updated_at = now()
		WHERE organization_id = $1 AND razorpay_subscription_id = $2`,
		orgID, subID, status, atPeriodEnd,
	)
	if err != nil {
		return false, fmt.Errorf("mark subscription %s cancelled: %w", subID, err)
	}
	return tag.RowsAffected() == 1, nil
}

// applyEvent records eventID and stores s as its organization's
// subscription state, in one transaction. It returns false, changing
// nothing, when eventID was recorded before.
//
// The state is stored when it is at least as new as the stored state, and
// is either the same subscription or replaces one that is not in
// liveStatuses. An organization that no longer exists gets no row.
func (r repository) applyEvent(ctx context.Context, eventID string, s subscription, liveStatuses []string, grantCredits int64) (bool, error) {
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
		if grantCredits > 0 {
			// Same transaction as the event record, so a failed grant is
			// retried by Razorpay's redelivery instead of being lost.
			// Unused credits roll over, up to two months of the plan.
			if err := grantUpToTx(ctx, tx, s.OrganizationID, grantCredits, 2*grantCredits, "monthly plan credits"); err != nil {
				return err
			}
		}
		firstDelivery = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return firstDelivery, nil
}
