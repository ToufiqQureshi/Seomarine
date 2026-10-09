package audit

import "errors"

var (
	// errInvalidURL marks a URL the crawler cannot use.
	errInvalidURL = errors.New("invalid url")
	// ErrStartURLInvalid means the audit's start URL failed validation.
	ErrStartURLInvalid = errors.New("invalid start url")
	// ErrCrawlTargetBlocked means the start URL points at a blocked or
	// private address (SSRF guard).
	ErrCrawlTargetBlocked = errors.New("crawl target blocked")
	// ErrAuditNotFound means the audit does not exist in the caller's project.
	ErrAuditNotFound = errors.New("audit not found")
	// ErrLighthouseNotFound means the Lighthouse result, or its stored payload,
	// does not exist in the caller's project.
	ErrLighthouseNotFound = errors.New("lighthouse result not found")
	// ErrInvalidConfig means a stored audit config could not be parsed.
	ErrInvalidConfig = errors.New("invalid audit configuration")
	// ErrAuditRunning means the audit exists and is still running.
	ErrAuditRunning = errors.New("audit is still running")
	// ErrAuditPageLimitExceeded means the requested page count exceeds the
	// caller's plan limit.
	ErrAuditPageLimitExceeded = errors.New("audit page limit exceeded")
	// ErrAuditCapacityReached means the organization has no capacity left.
	ErrAuditCapacityReached = errors.New("audit capacity reached")
	// ErrAuditAlreadyRunning means the organization has too many audits running.
	ErrAuditAlreadyRunning = errors.New("audit already running")
	// ErrRenderingUnavailable means JavaScript rendering is not configured.
	ErrRenderingUnavailable = errors.New("javascript rendering is unavailable")
	// ErrPaymentRequired means the organization has no managed access.
	ErrPaymentRequired = errors.New("payment required")
)
