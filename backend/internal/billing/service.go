// Package billing runs the paid plan on Razorpay subscriptions: checkout,
// the webhook that keeps an organization's subscription state current, and
// the plan the organization is on.
package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/razorpay"
)

var (
	// ErrAlreadySubscribed means the organization's subscription is live, so
	// a second checkout would bill it twice.
	ErrAlreadySubscribed = errors.New("organization already has a subscription")
	// ErrPaymentProvider means Razorpay failed or refused the request.
	ErrPaymentProvider = errors.New("payment provider failed")
	// ErrInvalidSignature means the webhook is not signed with the webhook
	// secret.
	ErrInvalidSignature = errors.New("invalid webhook signature")
	// ErrInvalidEvent means a correctly signed webhook could not be read.
	ErrInvalidEvent = errors.New("invalid webhook event")
)

// Plans.
const (
	PlanFree = "free"
	PlanPro  = "pro"
)

// totalCount is how many billing cycles a subscription runs before Razorpay
// completes it: ten years of a monthly plan. Razorpay requires a finite count.
const totalCount = 120

// orgNote is the subscription note that ties a Razorpay subscription to its
// organization.
const orgNote = "organization_id"

// liveStatuses are the states of a subscription that may still charge the
// customer; a new checkout must not replace one.
var liveStatuses = []string{"authenticated", "active", "pending", "halted", "paused"}

// proStatuses are the states that grant the paid plan. "pending" is
// Razorpay retrying a failed charge, a grace period; "halted" is retries
// exhausted.
var proStatuses = []string{"authenticated", "active", "pending"}

// knownStatuses are every Razorpay subscription state.
var knownStatuses = append([]string{"created", "cancelled", "completed", "expired"}, liveStatuses...)

// handledEvents change the stored subscription state; other events are
// acknowledged and ignored.
var handledEvents = []string{
	"subscription.activated",
	"subscription.charged",
	"subscription.cancelled",
	"subscription.halted",
	"subscription.completed",
}

// Status is an organization's plan.
type Status struct {
	Plan string `json:"plan"`
	// Status is the Razorpay subscription state, or "none".
	Status           string     `json:"status"`
	CurrentPeriodEnd *time.Time `json:"currentPeriodEnd"`
}

// Checkout is what Razorpay Checkout in the browser needs to start paying
// for a subscription.
type Checkout struct {
	SubscriptionID string `json:"subscriptionId"`
	KeyID          string `json:"keyId"`
}

// WebhookResult says what a webhook delivery did.
type WebhookResult string

// Webhook results.
const (
	WebhookProcessed WebhookResult = "processed"
	WebhookDuplicate WebhookResult = "duplicate"
	WebhookIgnored   WebhookResult = "ignored"
)

// Service is the billing use cases.
type Service struct {
	repo          repository
	razorpay      *razorpay.Client
	planID        string
	webhookSecret string
}

// NewService returns a Service that sells the Razorpay plan planID through
// rzp and verifies webhooks with webhookSecret.
func NewService(db *pgxpool.Pool, rzp *razorpay.Client, planID, webhookSecret string) *Service {
	return &Service{repo: repository{db: db}, razorpay: rzp, planID: planID, webhookSecret: webhookSecret}
}

// Status returns orgID's plan. Without a subscription it is the free plan.
func (s *Service) Status(ctx context.Context, orgID string) (Status, error) {
	sub, found, err := s.repo.subscription(ctx, orgID)
	if err != nil {
		return Status{}, fmt.Errorf("billing status of organization %s: %w", orgID, err)
	}
	if !found {
		return Status{Plan: PlanFree, Status: "none"}, nil
	}
	plan := PlanFree
	if slices.Contains(proStatuses, sub.Status) {
		plan = sub.Plan
	}
	return Status{Plan: plan, Status: sub.Status, CurrentPeriodEnd: sub.CurrentPeriodEnd}, nil
}

// Checkout creates a Razorpay subscription to the pro plan for orgID. The
// caller must have checked that the user may manage the organization's
// billing. It returns ErrAlreadySubscribed when orgID's subscription is live.
//
// A subscription that was created but never paid is not reused: Razorpay
// expires those, so every checkout starts a fresh one.
func (s *Service) Checkout(ctx context.Context, orgID string) (Checkout, error) {
	current, found, err := s.repo.subscription(ctx, orgID)
	if err != nil {
		return Checkout{}, fmt.Errorf("checkout for organization %s: %w", orgID, err)
	}
	if found && slices.Contains(liveStatuses, current.Status) {
		return Checkout{}, ErrAlreadySubscribed
	}

	sub, err := s.razorpay.CreateSubscription(ctx, s.planID, totalCount, map[string]string{orgNote: orgID})
	if err != nil {
		return Checkout{}, fmt.Errorf("checkout for organization %s: %w: %w", orgID, ErrPaymentProvider, err)
	}
	// A webhook may have made another subscription live since the check
	// above; the new one then stays unpaid and expires.
	replaced, err := s.repo.startSubscription(ctx, orgID, PlanPro, sub.ID, sub.Status, time.Unix(sub.CreatedAt, 0), liveStatuses)
	if err != nil {
		return Checkout{}, fmt.Errorf("checkout for organization %s: %w", orgID, err)
	}
	if !replaced {
		return Checkout{}, ErrAlreadySubscribed
	}
	return Checkout{SubscriptionID: sub.ID, KeyID: s.razorpay.KeyID()}, nil
}

// HandleWebhook applies a Razorpay webhook delivery. body is the raw request
// body, signature the X-Razorpay-Signature header and eventID the
// X-Razorpay-Event-Id header, which is the same on every redelivery.
//
// Each subscription event carries the subscription's full state, which
// replaces the stored state unless that is newer, so deliveries may arrive
// in any order. Events for organizations that no longer exist, or without
// the organization note, are ignored.
func (s *Service) HandleWebhook(ctx context.Context, body []byte, signature, eventID string) (WebhookResult, error) {
	if !razorpay.ValidSignature(body, signature, s.webhookSecret) {
		return "", ErrInvalidSignature
	}
	var event razorpay.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidEvent, err)
	}
	if !slices.Contains(handledEvents, event.Event) {
		return WebhookIgnored, nil
	}
	if eventID == "" {
		return "", fmt.Errorf("%w: no event id", ErrInvalidEvent)
	}
	if event.Payload.Subscription == nil {
		return "", fmt.Errorf("%w: %s has no subscription", ErrInvalidEvent, event.Event)
	}
	sub := event.Payload.Subscription.Entity
	if sub.ID == "" || !slices.Contains(knownStatuses, sub.Status) {
		return "", fmt.Errorf("%w: subscription %q has status %q", ErrInvalidEvent, sub.ID, sub.Status)
	}
	orgID := sub.Note(orgNote)
	if orgID == "" {
		return WebhookIgnored, nil
	}

	var periodEnd *time.Time
	if sub.CurrentEnd != nil {
		end := time.Unix(*sub.CurrentEnd, 0)
		periodEnd = &end
	}
	firstDelivery, err := s.repo.applyEvent(ctx, eventID, subscription{
		OrganizationID:   orgID,
		ID:               sub.ID,
		Plan:             PlanPro,
		Status:           sub.Status,
		CurrentPeriodEnd: periodEnd,
		EventAt:          time.Unix(event.CreatedAt, 0),
	}, liveStatuses)
	if err != nil {
		return "", fmt.Errorf("apply %s event %s: %w", event.Event, eventID, err)
	}
	if !firstDelivery {
		return WebhookDuplicate, nil
	}
	return WebhookProcessed, nil
}
