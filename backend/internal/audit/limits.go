package audit

// Per-audit page bounds, shared with the launch form and the tier gate so the
// UI and the server cannot drift apart. Mirrors src/shared/audit-limits.ts.
const (
	// MinAuditPages is the smallest crawl a user can request.
	MinAuditPages = 10
	// DefaultAuditPages is used when the request omits maxPages.
	DefaultAuditPages = 50
	// FreeMaxAuditPages caps a free-plan crawl.
	FreeMaxAuditPages = 50
	// PaidMaxAuditPages caps a paid crawl.
	PaidMaxAuditPages = 10_000
	// RenderedMaxAuditPages caps a crawl that renders JavaScript (page [dot])
	// rendering holds credits for at most 24 hours.
	RenderedMaxAuditPages = 1_000
)

// PageFetchClass is how a page fetch resolved. "blocked" means a WAF or bot
// challenge stood in the way; "rate_limited" means a 429 prevented the crawler
// from reading the page. Mirrors src/shared/audit-fetch-class.ts.
type PageFetchClass string

// The four fetch outcomes, persisted verbatim in the audit_pages column.
const (
	FetchOK          PageFetchClass = "ok"
	FetchBlocked     PageFetchClass = "blocked"
	FetchRateLimited PageFetchClass = "rate_limited"
	FetchError       PageFetchClass = "error"
)

// valid reports whether c is one of the four known classes.
func (c PageFetchClass) valid() bool {
	switch c {
	case FetchOK, FetchBlocked, FetchRateLimited, FetchError:
		return true
	default:
		return false
	}
}

// LighthouseStrategy selects how many pages get a paid Lighthouse check.
type LighthouseStrategy string

// The two surviving strategies. "auto" samples the homepage plus one page per
// URL template (capped at 10); "none" skips Lighthouse entirely.
const (
	LighthouseAuto LighthouseStrategy = "auto"
	LighthouseNone LighthouseStrategy = "none"
)

// clampMaxPages applies the launch form's bounds to a requested page count,
// mirroring clampAuditMaxPages.
func clampMaxPages(maxPages int) int {
	if maxPages <= 0 {
		maxPages = DefaultAuditPages
	}
	if maxPages < MinAuditPages {
		return MinAuditPages
	}
	if maxPages > PaidMaxAuditPages {
		return PaidMaxAuditPages
	}
	return maxPages
}

// AuditLimitTier names the plan ceiling applied to a new audit.
type AuditLimitTier string

// The three tiers. Self-hosted is not gated.
const (
	TierFree       AuditLimitTier = "free"
	TierPaid       AuditLimitTier = "paid"
	TierSelfHosted AuditLimitTier = "self_hosted"
)

// tierLimits is the abuse bound per tier (mirrors AUDIT_LIMITS).
type tierLimits struct {
	MaxPagesPerAudit int
	// MaxCapacityUnits is the cumulative pages + lighthouse checks allowed
	// across the organization. maxInt means unlimited.
	MaxCapacityUnits int
	// MaxRunningAudits bounds concurrent runs. maxInt means unlimited.
	MaxRunningAudits int
}

const maxInt = int(^uint(0) >> 1)

// auditLimits mirrors AUDIT_LIMITS in audit-capacity.ts.
var auditLimits = map[AuditLimitTier]tierLimits{
	TierFree:       {MaxPagesPerAudit: FreeMaxAuditPages, MaxCapacityUnits: 2_000, MaxRunningAudits: 5},
	TierPaid:       {MaxPagesPerAudit: PaidMaxAuditPages, MaxCapacityUnits: 100_000, MaxRunningAudits: maxInt},
	TierSelfHosted: {MaxPagesPerAudit: PaidMaxAuditPages, MaxCapacityUnits: maxInt, MaxRunningAudits: maxInt},
}

// estimatedCapacity mirrors getEstimatedAuditCapacity.
type estimatedCapacity struct {
	PagesTotal      int
	LighthouseTotal int
	Total           int
}

// getEstimatedCapacity reserves pages plus the worst-case Lighthouse checks.
func getEstimatedCapacity(maxPages int, strategy LighthouseStrategy) estimatedCapacity {
	pagesTotal := clampMaxPages(maxPages)
	if strategy == "" {
		strategy = LighthouseAuto
	}
	// "auto" samples up to 10 pages, checked on mobile + desktop.
	lighthouseChecks := 0
	if strategy == LighthouseAuto {
		lighthouseChecks = 20
	}
	return estimatedCapacity{PagesTotal: pagesTotal, LighthouseTotal: lighthouseChecks, Total: pagesTotal + lighthouseChecks}
}
