// Package billing runs the paid plan on Razorpay subscriptions: checkout,
// the webhook that keeps an organization's subscription state current, and
// the plan the organization is on.
package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
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

// ErrUnknownPlan means Razorpay reported a subscription to a plan id this
// server was not given. It is our configuration that is wrong, not the
// event: the customer has paid and their plan is not switched on.
var ErrUnknownPlan = errors.New("razorpay plan id matches no configured plan")

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
	// Unlimited is true for Uplumo's own workspaces: every feature, no
	// limits, nothing to pay.
	Unlimited bool `json:"unlimited"`
}

// LimitUsage compares retained workspace data with the active plan. Existing
// data is never removed after a downgrade.
type LimitUsage struct {
	Used     int  `json:"used"`
	Limit    int  `json:"limit"`
	Exceeded bool `json:"exceeded"`
}

// PlanUsage is the customer-visible state of every count-based plan limit.
type PlanUsage struct {
	Sites           LimitUsage `json:"sites"`
	Members         LimitUsage `json:"members"`
	TrackedKeywords LimitUsage `json:"trackedKeywords"`
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
	planIDs       map[string]string
	webhookSecret string
	staffEmails   []string
}

// NewService returns a Service that sells only Pro, as the Razorpay plan
// planID, and verifies webhooks with webhookSecret.
func NewService(db *pgxpool.Pool, rzp *razorpay.Client, planID, webhookSecret string) *Service {
	return NewPlanService(db, rzp, map[string]string{PlanPro: planID}, webhookSecret)
}

// NewPlanService returns a Service that sells each plan in planIDs as the
// Razorpay plan it maps to. A plan with no id cannot be bought.
func NewPlanService(db *pgxpool.Pool, rzp *razorpay.Client, planIDs map[string]string, webhookSecret string) *Service {
	configured := make(map[string]string, len(planIDs))
	for plan, id := range planIDs {
		if id != "" {
			configured[plan] = id
		}
	}
	return &Service{repo: repository{db: db}, razorpay: rzp, planIDs: configured, webhookSecret: webhookSecret}
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
	for _, plan := range plans.Paid() {
		if s.planIDs[plan] != "" {
			available = append(available, plan)
		}
	}
	return available
}

// MonthlyGrantFor returns the credits a paid month of plan grants.
func (s *Service) MonthlyGrantFor(plan string) int64 {
	definition, _ := plans.Get(plan)
	return definition.MonthlyCredits
}

// SetStaffEmails names the people whose own workspaces are never limited by
// a plan: a workspace is unlimited when its owner has one of these verified
// addresses. Call it before the service is used.
func (s *Service) SetStaffEmails(emails []string) {
	s.staffEmails = make([]string, 0, len(emails))
	for _, email := range emails {
		if email = strings.ToLower(strings.TrimSpace(email)); email != "" {
			s.staffEmails = append(s.staffEmails, email)
		}
	}
}

// Status returns orgID's plan. Without a subscription it is the free plan.
// A staff workspace reads as Agency and unlimited whatever it subscribed to.
func (s *Service) Status(ctx context.Context, orgID string) (Status, error) {
	status, err := s.subscribedStatus(ctx, orgID)
	if err != nil || len(s.staffEmails) == 0 {
		return status, err
	}
	staff, err := s.repo.ownedByAny(ctx, orgID, s.staffEmails)
	if err != nil {
		return Status{}, fmt.Errorf("billing status of organization %s: %w", orgID, err)
	}
	if staff {
		status.Plan, status.Unlimited = plans.Unlimited.ID, true
	}
	return status, nil
}

// subscribedStatus is the plan orgID's subscription pays for.
func (s *Service) subscribedStatus(ctx context.Context, orgID string) (Status, error) {
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

// IsUnlimited reports whether orgID is a staff workspace.
func (s *Service) IsUnlimited(ctx context.Context, orgID string) (bool, error) {
	status, err := s.Status(ctx, orgID)
	return status.Unlimited, err
}

// entitlement is what orgID may use: its plan's features and limits.
func (s *Service) entitlement(ctx context.Context, orgID string) (plans.Definition, error) {
	status, err := s.Status(ctx, orgID)
	if err != nil {
		return plans.Definition{}, err
	}
	if status.Unlimited {
		return plans.Unlimited, nil
	}
	definition, ok := plans.Get(status.Plan)
	if !ok {
		// A plan that left the catalog must not unlock anything.
		definition, _ = plans.Get(PlanFree)
	}
	return definition, nil
}

// Allows reports whether orgID's plan includes feature.
func (s *Service) Allows(ctx context.Context, orgID string, feature plans.Feature) (bool, error) {
	definition, err := s.entitlement(ctx, orgID)
	if err != nil {
		return false, err
	}
	return definition.Allows(feature), nil
}

// UsageAgainstPlan reports retained data against the current plan limits.
func (s *Service) UsageAgainstPlan(ctx context.Context, orgID string) (PlanUsage, error) {
	definition, err := s.entitlement(ctx, orgID)
	if err != nil {
		return PlanUsage{}, err
	}
	usage, err := s.repo.planUsage(ctx, orgID)
	if err != nil {
		return PlanUsage{}, err
	}
	item := func(used, limit int) LimitUsage {
		return LimitUsage{Used: used, Limit: limit, Exceeded: limit > 0 && used > limit}
	}
	return PlanUsage{
		Sites:           item(usage.Sites, definition.Sites),
		Members:         item(usage.Members, definition.Members),
		TrackedKeywords: item(usage.TrackedKeywords, definition.TrackedKeywords),
	}, nil
}

// WithinPlanLimits is false after a downgrade leaves retained data above any
// current limit. Paid provider work stays paused until the user archives or
// removes enough data, while reads and deletion remain available.
func (s *Service) WithinPlanLimits(ctx context.Context, orgID string) (bool, error) {
	status, err := s.Status(ctx, orgID)
	if err != nil || status.Unlimited {
		return status.Unlimited, err
	}
	usage, err := s.UsageAgainstPlan(ctx, orgID)
	if err != nil {
		return false, err
	}
	return !usage.Sites.Exceeded && !usage.Members.Exceeded && !usage.TrackedKeywords.Exceeded, nil
}

// FeatureGate answers one question for one feature: does this organization's
// plan include it.
type FeatureGate struct {
	service *Service
	feature plans.Feature
}

// Gate returns the check for feature, in the shape the feature packages ask
// for.
func (s *Service) Gate(feature plans.Feature) FeatureGate {
	return FeatureGate{service: s, feature: feature}
}

// HasPaidPlan reports whether orgID's plan includes the gate's feature. The
// name is the one the feature packages declared when there was one paid plan.
func (g FeatureGate) HasPaidPlan(ctx context.Context, orgID string) (bool, error) {
	return g.service.Allows(ctx, orgID, g.feature)
}

// SiteLimit returns how many active sites orgID may have; 0 is no limit.
func (s *Service) SiteLimit(ctx context.Context, orgID string) (int, error) {
	definition, err := s.entitlement(ctx, orgID)
	return definition.Sites, err
}

// KeywordLimit returns how many keywords orgID may track; 0 is no limit.
// A plan without rank tracking is stopped by its feature gate, not here.
func (s *Service) KeywordLimit(ctx context.Context, orgID string) (int, error) {
	definition, err := s.entitlement(ctx, orgID)
	return definition.TrackedKeywords, err
}

// WorkspaceDailyLimitUSD is the most orgID may spend at the providers in one
// day: a third of its plan's monthly credits, so a busy day is fine but one
// workspace cannot burn its month, or anyone else's share, in hours. 0 is no
// limit (staff workspaces).
func (s *Service) WorkspaceDailyLimitUSD(ctx context.Context, orgID string) (float64, error) {
	definition, err := s.entitlement(ctx, orgID)
	if err != nil {
		return 0, err
	}
	return float64(definition.MonthlyCredits) / 3 / (CreditMarkupMultiplier * CreditsPerUSD), nil
}

// SerpDepthLimit returns how deep orgID's rank tracking may look; 0 is no
// limit.
func (s *Service) SerpDepthLimit(ctx context.Context, orgID string) (int, error) {
	definition, err := s.entitlement(ctx, orgID)
	return definition.MaxSerpDepth, err
}

// MemberLimit returns how many people orgID's workspace may hold; 0 is no
// limit.
func (s *Service) MemberLimit(ctx context.Context, orgID string) (int, error) {
	definition, err := s.entitlement(ctx, orgID)
	return definition.Members, err
}

// Checkout creates a Razorpay subscription to the pro plan for orgID. The
// caller must have checked that the user may manage the organization's
// billing. It returns ErrAlreadySubscribed when orgID's subscription is live.
func (s *Service) Checkout(ctx context.Context, orgID string) (Checkout, error) {
	return s.CheckoutPlan(ctx, orgID, PlanPro)
}

// CheckoutPlan starts a subscription for a selected configured plan.
//
// A workspace has at most one payable subscription at a time, so a second
// tab or a double click can never be charged twice. Checkouts of one
// workspace are serialized; an unpaid subscription to the same plan is
// handed out again (Razorpay keeps it payable, since no start_at is set), and
// one to another plan is cancelled before the new one is created. If that
// cancel fails, checkout fails rather than leave two payable subscriptions.
func (s *Service) CheckoutPlan(ctx context.Context, orgID, plan string) (Checkout, error) {
	if !s.providerConfigured() {
		return Checkout{}, ErrPaymentUnavailable
	}
	planID := s.planIDs[plan]
	if planID == "" {
		return Checkout{}, ErrPlanUnavailable
	}
	q, unlock, err := s.repo.lockCheckout(ctx, orgID)
	if err != nil {
		return Checkout{}, fmt.Errorf("checkout for organization %s: %w", orgID, err)
	}
	defer unlock()
	current, found, err := loadSubscription(ctx, q, orgID)
	if err != nil {
		return Checkout{}, fmt.Errorf("checkout for organization %s: %w", orgID, err)
	}
	if found && slices.Contains(liveStatuses, current.Status) {
		return Checkout{}, ErrAlreadySubscribed
	}
	if found && current.Status == "created" {
		if current.Plan == plan {
			return Checkout{SubscriptionID: current.ID, KeyID: s.razorpay.KeyID()}, nil
		}
		if _, err := s.razorpay.CancelSubscription(ctx, current.ID, false); err != nil {
			return Checkout{}, fmt.Errorf("checkout for organization %s: cancel unpaid subscription %s: %w: %w", orgID, current.ID, ErrPaymentProvider, err)
		}
	}

	sub, err := s.razorpay.CreateSubscription(ctx, planID, totalCount, map[string]string{orgNote: orgID})
	if err != nil {
		return Checkout{}, fmt.Errorf("checkout for organization %s: %w: %w", orgID, ErrPaymentProvider, err)
	}
	// A webhook may have made another subscription live since the check
	// above; the new one then stays unpaid and expires.
	replaced, err := startSubscription(ctx, q, orgID, plan, sub.ID, sub.Status, time.Unix(sub.CreatedAt, 0), liveStatuses)
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
	// The plan comes from Razorpay, which knows what was paid for, never
	// from what the browser asked for at checkout.
	plan := s.planForRazorpayID(sub.PlanID)
	if plan == "" {
		return "", fmt.Errorf("%w: subscription %s of organization %s is on plan %q", ErrUnknownPlan, sub.ID, orgID, sub.PlanID)
	}
	outcome, err := s.repo.applyEvent(ctx, eventID, subscription{
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
	switch outcome {
	case eventDuplicateDelivery:
		return WebhookDuplicate, nil
	case eventSecondLiveSubscription:
		return s.stopSecondSubscription(ctx, orgID, sub.ID, event.Event, eventID)
	}
	return WebhookProcessed, nil
}

// stopSecondSubscription handles a workspace that went live on a second
// subscription while another is live: it earns no credits and is cancelled
// now, so it cannot charge again. If it was charged, that payment needs a
// manual refund, so the error log (which reaches Sentry) says so. The event
// is recorded only once the cancel succeeded; until then Razorpay's
// redelivery retries it.
func (s *Service) stopSecondSubscription(ctx context.Context, orgID, subID, eventName, eventID string) (WebhookResult, error) {
	if _, err := s.razorpay.CancelSubscription(ctx, subID, false); err != nil {
		return "", fmt.Errorf("cancel second live subscription %s of organization %s: %w: %w", subID, orgID, ErrPaymentProvider, err)
	}
	slog.ErrorContext(ctx, "cancelled a second live subscription; refund any charge on it by hand",
		"organization", orgID, "subscription", subID, "event", eventName)
	if err := s.repo.recordEvent(ctx, eventID); err != nil {
		return "", err
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
	for plan, configuredID := range s.planIDs {
		if id != "" && id == configuredID {
			return plan
		}
	}
	return ""
}
