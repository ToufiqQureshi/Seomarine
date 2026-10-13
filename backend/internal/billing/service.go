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
	"github.com/toufiqqureshi/uplumo/backend/internal/plans"
	"github.com/toufiqqureshi/uplumo/backend/internal/razorpay"
)

var (
	// ErrAlreadySubscribed means the organization's subscription is live, so
	// a second checkout would bill it twice.
	ErrAlreadySubscribed = errors.New("organization already has a subscription")
	// ErrNoSubscription means the organization has no subscription that
	// could still charge it, so there is nothing to cancel.
	ErrNoSubscription = errors.New("organization has no live subscription")
	// ErrPaymentProvider means Razorpay failed or refused the request.
	ErrPaymentProvider = errors.New("payment provider failed")
	// ErrInvalidSignature means the webhook is not signed with the webhook
	// secret.
	ErrInvalidSignature = errors.New("invalid webhook signature")
	// ErrInvalidEvent means a correctly signed webhook could not be read.
	ErrInvalidEvent = errors.New("invalid webhook event")
)

// ErrPaymentUnavailable means checkout, cancellation and webhooks are disabled.
var ErrPaymentUnavailable = errors.New("payment provider is not configured")

// ErrPlanUnavailable means the selected plan has no configured Razorpay plan ID.
var ErrPlanUnavailable = errors.New("selected plan is not configured for checkout")

// Plans.
const (
	PlanFree   = plans.Free
	PlanBasic  = plans.Basic
	PlanPro    = plans.Pro
	PlanAgency = plans.Agency
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
	// CancelAtPeriodEnd is true when the customer cancelled: the plan runs
	// until CurrentPeriodEnd and is not charged again.
	CancelAtPeriodEnd bool `json:"cancelAtPeriodEnd"`
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
	repo            repository
	razorpay        *razorpay.Client
	planID          string
	planIDs         map[string]string
	legacyPlanIDPro string
	webhookSecret   string
	// MonthlyGrantCredits is added to the organization's credit balance on
	// every successful subscription charge. Zero grants nothing.
	MonthlyGrantCredits       int64
	MonthlyGrantCreditsByPlan map[string]int64
}

// NewService returns a Service that sells the Razorpay plan planID through
// rzp and verifies webhooks with webhookSecret.
func NewService(db *pgxpool.Pool, rzp *razorpay.Client, planID, webhookSecret string) *Service {
	return &Service{repo: repository{db: db}, razorpay: rzp, planID: planID, planIDs: map[string]string{PlanPro: planID}, webhookSecret: webhookSecret}
}

// NewPlanService returns a service that can sell the configured plan IDs.
func NewPlanService(db *pgxpool.Pool, rzp *razorpay.Client, planIDs map[string]string, legacyPlanIDPro, webhookSecret string) *Service {
	copyIDs := make(map[string]string, len(planIDs))
	for plan, id := range planIDs {
		if id != "" {
			copyIDs[plan] = id
		}
	}
	return &Service{repo: repository{db: db}, razorpay: rzp, planIDs: copyIDs, legacyPlanIDPro: legacyPlanIDPro, webhookSecret: webhookSecret}
}

// NewReadOnlyService exposes existing subscription state without enabling payments.
func NewReadOnlyService(db *pgxpool.Pool) *Service {
	return &Service{repo: repository{db: db}}
}

// PaymentsConfigured reports whether this service can accept payment actions.
func (s *Service) PaymentsConfigured() bool {
	return s.providerConfigured()
}

func (s *Service) providerConfigured() bool {
	return s != nil && s.razorpay != nil && s.webhookSecret != ""
}

// AvailablePlans reports plans configured for checkout.
func (s *Service) AvailablePlans() []string {
	if s == nil {
		return nil
	}
	available := make([]string, 0, len(s.planIDs))
	for _, plan := range []string{PlanBasic, PlanPro, PlanAgency} {
		if s.planIDs[plan] != "" {
			available = append(available, plan)
		}
	}
	return available
}

func (s *Service) MonthlyGrantFor(plan string) int64 {
	if amount, ok := s.MonthlyGrantCreditsByPlan[plan]; ok {
		return amount
	}
	if plan == PlanPro {
		return s.MonthlyGrantCredits
	}
	return 0
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
	periodEnd := sub.CurrentPeriodEnd
	if periodEnd != nil {
		utc := periodEnd.UTC()
		periodEnd = &utc
	}
	return Status{
		Plan: plan, Status: sub.Status, CurrentPeriodEnd: periodEnd,
		CancelAtPeriodEnd: sub.CancelAtPeriodEnd && slices.Contains(liveStatuses, sub.Status),
	}, nil
}

// HasPaidPlan reports whether orgID is on a paid plan.
func (s *Service) HasPaidPlan(ctx context.Context, orgID string) (bool, error) {
	status, err := s.Status(ctx, orgID)
	if err != nil {
		return false, err
	}
	return status.Plan == PlanPro || status.Plan == PlanAgency, nil
}

// SiteLimit returns the active-site limit for the organization's current plan.
func (s *Service) SiteLimit(ctx context.Context, orgID string) (int, error) {
	status, err := s.Status(ctx, orgID)
	if err != nil {
		return 0, err
	}
	definition, ok := plans.Get(status.Plan)
	if !ok {
		return 1, nil
	}
	return definition.Sites, nil
}

func (s *Service) HasAgencyPlan(ctx context.Context, orgID string) (bool, error) {
	status, err := s.Status(ctx, orgID)
	if err != nil {
		return false, err
	}
	return status.Plan == PlanAgency, nil
}

// Checkout creates a Razorpay subscription to the pro plan for orgID. The
// caller must have checked that the user may manage the organization's
// billing. It returns ErrAlreadySubscribed when orgID's subscription is live.
//
// A subscription that was created but never paid is not reused: Razorpay
// expires those, so every checkout starts a fresh one.
func (s *Service) Checkout(ctx context.Context, orgID string) (Checkout, error) {
	return s.CheckoutPlan(ctx, orgID, PlanPro)
}

// CheckoutPlan starts a subscription for a selected configured plan.
func (s *Service) CheckoutPlan(ctx context.Context, orgID, plan string) (Checkout, error) {
	if !s.providerConfigured() {
		return Checkout{}, ErrPaymentUnavailable
	}
	planID := s.planIDs[plan]
	if planID == "" || (plan != PlanBasic && plan != PlanPro && plan != PlanAgency) {
		return Checkout{}, ErrPlanUnavailable
	}
	current, found, err := s.repo.subscription(ctx, orgID)
	if err != nil {
		return Checkout{}, fmt.Errorf("checkout for organization %s: %w", orgID, err)
	}
	if found && slices.Contains(liveStatuses, current.Status) {
		return Checkout{}, ErrAlreadySubscribed
	}

	sub, err := s.razorpay.CreateSubscription(ctx, planID, totalCount, map[string]string{orgNote: orgID})
	if err != nil {
		return Checkout{}, fmt.Errorf("checkout for organization %s: %w: %w", orgID, ErrPaymentProvider, err)
	}
	// A webhook may have made another subscription live since the check
	// above; the new one then stays unpaid and expires.
	replaced, err := s.repo.startSubscription(ctx, orgID, plan, sub.ID, sub.Status, time.Unix(sub.CreatedAt, 0), liveStatuses)
	if err != nil {
		return Checkout{}, fmt.Errorf("checkout for organization %s: %w", orgID, err)
	}
	if !replaced {
		return Checkout{}, ErrAlreadySubscribed
	}
	return Checkout{SubscriptionID: sub.ID, KeyID: s.razorpay.KeyID()}, nil
}

// Cancel stops orgID's subscription from charging again and returns the
// plan that results. The caller must have checked that the user may manage
// the organization's billing. It returns ErrNoSubscription when there is
// nothing live to cancel.
//
// A subscription with a paid cycle running is cancelled at the end of that
// cycle, so the customer keeps what they paid for. Any other live state
// (the first charge has not happened, or a charge is failing) has no paid
// time left and ends now. Cancelling twice is not an error.
func (s *Service) Cancel(ctx context.Context, orgID string) (Status, error) {
	if !s.providerConfigured() {
		return Status{}, ErrPaymentUnavailable
	}
	current, found, err := s.repo.subscription(ctx, orgID)
	if err != nil {
		return Status{}, fmt.Errorf("cancel for organization %s: %w", orgID, err)
	}
	if !found || !slices.Contains(liveStatuses, current.Status) {
		return Status{}, ErrNoSubscription
	}
	if current.CancelAtPeriodEnd {
		return s.Status(ctx, orgID)
	}

	atPeriodEnd := current.Status == "active"
	sub, err := s.razorpay.CancelSubscription(ctx, current.ID, atPeriodEnd)
	if err != nil {
		return Status{}, fmt.Errorf("cancel for organization %s: %w: %w", orgID, ErrPaymentProvider, err)
	}
	if !slices.Contains(knownStatuses, sub.Status) {
		return Status{}, fmt.Errorf("cancel for organization %s: %w: subscription has status %q", orgID, ErrPaymentProvider, sub.Status)
	}
	// Razorpay has cancelled it either way. If the row moved on to another
	// subscription meanwhile, that one is not the one cancelled.
	if _, err := s.repo.markCancelled(ctx, orgID, current.ID, sub.Status, atPeriodEnd && slices.Contains(liveStatuses, sub.Status)); err != nil {
		return Status{}, fmt.Errorf("cancel for organization %s: %w", orgID, err)
	}
	return s.Status(ctx, orgID)
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
	if !s.providerConfigured() {
		return "", ErrPaymentUnavailable
	}
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
		end := time.Unix(*sub.CurrentEnd, 0).UTC()
		periodEnd = &end
	}
	plan := s.planForRazorpayID(sub.PlanID)
	if plan == "" {
		// Old single-plan Razorpay events may omit plan_id in fixtures and older payloads.
		if sub.PlanID == "" && s.planIDs[PlanPro] != "" {
			plan = PlanPro
		} else {
			return "", fmt.Errorf("%w: unknown Razorpay plan %q", ErrInvalidEvent, sub.PlanID)
		}
	}
	firstDelivery, err := s.repo.applyEvent(ctx, eventID, subscription{
		OrganizationID:   orgID,
		ID:               sub.ID,
		Plan:             plan,
		Status:           sub.Status,
		CurrentPeriodEnd: periodEnd,
		EventAt:          time.Unix(event.CreatedAt, 0).UTC(),
	}, liveStatuses, s.grantFor(event.Event, sub.Status, plan))
	if err != nil {
		return "", fmt.Errorf("apply %s event %s: %w", event.Event, eventID, err)
	}
	if !firstDelivery {
		return WebhookDuplicate, nil
	}
	return WebhookProcessed, nil
}

// grantFor is the credits a webhook delivery earns: one monthly grant per
// charge of a live subscription.
func (s *Service) grantFor(event, status, plan string) int64 {
	if event != "subscription.charged" || !slices.Contains(liveStatuses, status) {
		return 0
	}
	return s.MonthlyGrantFor(plan)
}

func (s *Service) planForRazorpayID(id string) string {
	if id != "" && id == s.legacyPlanIDPro {
		return PlanPro
	}
	for plan, configuredID := range s.planIDs {
		if id != "" && id == configuredID {
			return plan
		}
	}
	return ""
}
