// Package spend is the contract between work that costs Uplumo money at a
// provider and the credit ledger that pays for it. A provider transport asks a
// Guard for a Hold before it sends a billed request, and settles the hold with
// the real cost afterwards. Nothing billed may be sent without a hold.
package spend

import (
	"context"
	"errors"
	"net/http"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/httpx"
)

var (
	// ErrNoCredits means the workspace has not got the credits the work needs.
	ErrNoCredits = errors.New("not enough credits for this request")
	// ErrPaused means Uplumo stopped all paid provider work for the day: the
	// day's total spend reached the safety limit.
	ErrPaused = errors.New("paid data requests are paused for today")
)

// Feature names what the money is spent on. It labels the spend in the
// ledger, so the Billing screen can show where credits went.
type Feature string

// Features that spend credits.
const (
	RankTracking    Feature = "rank_tracking"
	KeywordResearch Feature = "keyword_research"
	LocalSEO        Feature = "local_seo"
	DomainResearch  Feature = "domain_research"
	Backlinks       Feature = "backlinks"
	AIVisibility    Feature = "ai_visibility"
	SiteAudit       Feature = "site_audit"
	AIAssistant     Feature = "ai_assistant"
	AIWriting       Feature = "ai_writing"
	// Reference is free lookup data such as location lists. It is held and
	// recorded like any other request, but never needs credits.
	Reference Feature = "reference"
)

// Features lists every feature that can spend credits, in the order the
// Billing screen shows them.
func Features() []Feature {
	return []Feature{RankTracking, KeywordResearch, LocalSEO, DomainResearch, Backlinks, AIVisibility, SiteAudit, AIAssistant, AIWriting}
}

// Request is one billed piece of provider work about to be sent.
type Request struct {
	OrganizationID string
	Feature        Feature
	// EstimateUSD is the most the request can cost at the provider. It is
	// held before the request is sent, so it must not be an average.
	EstimateUSD float64
}

// Hold is credit set aside for one request.
type Hold interface {
	// Settle charges the real provider cost and releases the rest. Call it
	// exactly once, also when the request failed (with the cost the provider
	// reported, often zero).
	Settle(ctx context.Context, costUSD float64) error
}

// Guard decides whether billed work may start.
type Guard interface {
	// Hold sets credit aside for req, or refuses with ErrNoCredits or
	// ErrPaused. Any other error must also stop the request: a guard that
	// cannot decide fails closed.
	Hold(ctx context.Context, req Request) (Hold, error)
}

// Refused reports whether err is the guard refusing the work: no credits,
// or paid work paused for the day.
func Refused(err error) bool {
	return errors.Is(err, ErrNoCredits) || errors.Is(err, ErrPaused)
}

const (
	noCreditsMessage = "You have used this month's credits. They refill on your next billing date, or upgrade your plan for more."
	pausedMessage    = "Live data is paused for a short while. Please try again in a few hours."
)

// Message is what to tell the customer about a refusal, or "" when err is
// not one.
func Message(err error) string {
	switch {
	case errors.Is(err, ErrNoCredits):
		return noCreditsMessage
	case errors.Is(err, ErrPaused):
		return pausedMessage
	}
	return ""
}

// WriteHTTPError answers a refused request and reports whether err was one:
// 402 credits_exhausted, or 503 usage_paused.
func WriteHTTPError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, ErrNoCredits):
		httpx.WriteError(w, http.StatusPaymentRequired, "credits_exhausted", noCreditsMessage)
	case errors.Is(err, ErrPaused):
		httpx.WriteError(w, http.StatusServiceUnavailable, "usage_paused", pausedMessage)
	default:
		return false
	}
	return true
}
