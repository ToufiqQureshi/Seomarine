package audit

import (
	"context"
	"math"
	"strconv"
	"strings"
)

const (
	// CloudflareRenderTimeoutMs is how long one browser attempt may run.
	// Cloudflare bills browser time per attempt at its full timeout.
	CloudflareRenderTimeoutMs   = 20_000
	cloudflareUSDPerBrowserHour = 0.09
	cloudflareUSDPerAttempt     = (CloudflareRenderTimeoutMs / 3_600_000.0) * cloudflareUSDPerBrowserHour
	// contextUSDPerCredit is Context.dev's Developer-plan list price; one
	// scrape is one credit.
	contextUSDPerCredit = 0.0025
	// creditsPerUSD is the shared usage-credit exchange rate.
	creditsPerUSD = 1000
	// seoDataCostMarkup is the platform markup applied to raw provider USD.
	seoDataCostMarkup = 1.28
)

// IsAuditRenderingAllowed reports whether this deployment can render
// JavaScript: a Context.dev key, or a bound browser renderer flag. Docker and
// legacy installs have no browser, so they render only through a Context key.
func IsAuditRenderingAllowed(env func(string) string) bool {
	if env == nil {
		return false
	}
	if strings.TrimSpace(env("CONTEXT_API_KEY")) != "" {
		return true
	}
	return env("AUDIT_BROWSER_RENDERING") == "true"
}

// roundUsdForBilling rounds a USD value to five decimal places.
func roundUsdForBilling(value float64) float64 {
	return math.Round(value*100000) / 100000
}

// applyBillingMarkupUsd applies the platform markup to a raw provider cost.
func applyBillingMarkupUsd(rawUSD float64) float64 {
	return roundUsdForBilling(rawUSD * seoDataCostMarkup)
}

// creditsForProviderUsd converts a raw provider USD cost into usage credits:
// marked up, rounded, then ceiled. The single charging formula keeps a pre-call
// estimate and the post-call deduction from drifting apart.
func creditsForProviderUsd(rawUSD float64) int {
	return int(math.Ceil(applyBillingMarkupUsd(rawUSD) * creditsPerUSD))
}

// autumnSeoDataCreditsToUsd converts usage credits back into dollars.
func autumnSeoDataCreditsToUsd(credits float64) float64 {
	return credits / creditsPerUSD
}

// RenderUsage counts the provider units one audit's rendering used.
type RenderUsage struct {
	CloudflareAttempts int `json:"cloudflareAttempts"`
	ContextCredits     int `json:"contextCredits"`
}

// RenderUsageCredits converts a render usage into usage credits.
func RenderUsageCredits(usage RenderUsage) int {
	return creditsForProviderUsd(
		float64(usage.CloudflareAttempts)*cloudflareUSDPerAttempt +
			float64(usage.ContextCredits)*contextUSDPerCredit,
	)
}

// RenderingCreditEstimate is the low/high credit range for rendering maxPages.
type RenderingCreditEstimate struct {
	Low  int
	High int
}

// EstimateRenderingCredits estimates the credits for rendering up to maxPages.
// The low end renders every page on Cloudflare; the high end is every page
// falling back to Context, which is also what a hosted audit reserves.
func EstimateRenderingCredits(maxPages int) RenderingCreditEstimate {
	return RenderingCreditEstimate{
		Low:  RenderUsageCredits(RenderUsage{CloudflareAttempts: maxPages}),
		High: RenderUsageCredits(RenderUsage{CloudflareAttempts: maxPages, ContextCredits: maxPages}),
	}
}

// formatRenderingUSD renders credits the way the pricing copy does: rounded up
// to the cent so an account holding the shown amount always covers the hold.
func formatRenderingUSD(credits int) string {
	cents := int(math.Ceil(autumnSeoDataCreditsToUsd(float64(credits) * 100)))
	return "$" + strconv.FormatFloat(float64(cents)/100, 'f', 2, 64)
}

// formatUSNumber renders an integer with en-US thousands separators.
func formatUSNumber(value int) string {
	negative := value < 0
	digits := strconv.Itoa(value)
	if negative {
		digits = digits[1:]
	}
	var builder strings.Builder
	if negative {
		builder.WriteByte('-')
	}
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			builder.WriteByte(',')
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// RenderingCreditsNeededText explains why a hosted account cannot start a
// rendered audit of maxPages pages.
func RenderingCreditsNeededText(maxPages int) string {
	estimate := EstimateRenderingCredits(maxPages)
	return "Rendering up to " + formatUSNumber(maxPages) + " pages needs " + formatRenderingUSD(estimate.High) +
		" of credits available. Lower the page limit or add credits."
}

// RenderingEstimateText is the user-facing estimate shown before a rendered
// audit starts.
func RenderingEstimateText(maxPages int) string {
	estimate := EstimateRenderingCredits(maxPages)
	return "Estimated " + formatRenderingUSD(estimate.Low) + " to " + formatRenderingUSD(estimate.High) +
		" for up to " + formatUSNumber(maxPages) + " pages. Sites with aggressive bot detection need our more capable renderer and cost more."
}

// RenderingLock is one held credit reservation.
type RenderingLock struct {
	LockID           string `json:"lockId"`
	FeatureID        string `json:"featureId"`
	EstimatedCredits int    `json:"estimatedCredits"`
}

// RenderingMeter holds and settles rendering credits around a hosted audit.
// Self-hosted deployments pass nil, where rendering is unmetered.
type RenderingMeter interface {
	// Lock reserves the worst-case rendering cost. It returns ErrPaymentRequired
	// (wrapped) when the balances cannot cover it.
	Lock(ctx context.Context, organizationID, auditID string, maxPages int) ([]RenderingLock, error)
	// Release returns every held credit for an audit that never started.
	Release(ctx context.Context, locks []RenderingLock) error
	// Settle charges the credits used and releases the rest. It never fails a
	// caller: an unsettled hold expires on its own.
	Settle(ctx context.Context, organizationID, auditID string, locks []RenderingLock, usage RenderUsage) error
}
