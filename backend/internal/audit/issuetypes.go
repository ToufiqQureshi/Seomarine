package audit

// IssueSeverity orders and groups issues in the report.
type IssueSeverity string

// The three severities, mirroring IssueSeverity.
const (
	SeverityCritical IssueSeverity = "critical"
	SeverityWarning  IssueSeverity = "warning"
	SeverityInfo     IssueSeverity = "info"
)

// AuditIssueType identifies one issue kind.
type AuditIssueType string

// The closed registry of issue types, mirroring AUDIT_ISSUE_TYPES.
const (
	IssueBlockedPage                  AuditIssueType = "blocked-page"
	IssueJavaScriptRenderingSuspected AuditIssueType = "javascript-rendering-suspected"
	IssueRateLimitedPage              AuditIssueType = "rate-limited-page"
	IssueCrawlRateLimited             AuditIssueType = "crawl-rate-limited"
	IssueServerError                  AuditIssueType = "server-error"
	IssueBrokenInternalLink           AuditIssueType = "broken-internal-link"
	IssueMissingTitle                 AuditIssueType = "missing-title"
	IssueBrokenPage                   AuditIssueType = "broken-page"
	IssueDuplicateTitle               AuditIssueType = "duplicate-title"
	IssueDuplicateMetaDescription     AuditIssueType = "duplicate-meta-description"
	IssueDuplicateContent             AuditIssueType = "duplicate-content"
	IssueMissingMetaDescription       AuditIssueType = "missing-meta-description"
	IssueMissingH1                    AuditIssueType = "missing-h1"
	IssueMultipleH1                   AuditIssueType = "multiple-h1"
	IssueRedirectChain                AuditIssueType = "redirect-chain"
	IssueRedirectLoop                 AuditIssueType = "redirect-loop"
	IssueCanonicalConflict            AuditIssueType = "canonical-conflict"
	IssueThinContent                  AuditIssueType = "thin-content"
	IssueImagesMissingAlt             AuditIssueType = "images-missing-alt"
	IssueOrphanPage                   AuditIssueType = "orphan-page"
	IssueNoOutgoingLinks              AuditIssueType = "no-outgoing-links"
	IssueTitleTooLong                 AuditIssueType = "title-too-long"
	IssueTitleTooShort                AuditIssueType = "title-too-short"
	IssueMetaDescriptionTooLong       AuditIssueType = "meta-description-too-long"
	IssueMetaDescriptionTooShort      AuditIssueType = "meta-description-too-short"
	IssueHeadingOrderSkip             AuditIssueType = "heading-order-skip"
	IssueSlowResponse                 AuditIssueType = "slow-response"
	IssueNoindexPage                  AuditIssueType = "noindex-page"
	IssueCanonicalizedPage            AuditIssueType = "canonicalized-page"
	IssueDeepPage                     AuditIssueType = "deep-page"
)

// issueDescriptor is the server-side part of an issue descriptor: the severity
// (persisted with every row) and the human title. The full user-facing copy
// stays in the shared TypeScript registry, which the React pages already import.
type issueDescriptor struct {
	Severity IssueSeverity
	Title    string
}

// issueRegistry carries the severity and title for every known issue type.
var issueRegistry = map[AuditIssueType]issueDescriptor{
	IssueBlockedPage:                  {SeverityCritical, "Crawler was blocked"},
	IssueJavaScriptRenderingSuspected: {SeverityWarning, "Content may require JavaScript"},
	IssueRateLimitedPage:              {SeverityWarning, "Rate limited (429)"},
	IssueCrawlRateLimited:             {SeverityWarning, "Crawl stopped early: rate limit"},
	IssueServerError:                  {SeverityCritical, "Server error (5xx)"},
	IssueBrokenInternalLink:           {SeverityCritical, "Broken internal link"},
	IssueMissingTitle:                 {SeverityCritical, "Missing title tag"},
	IssueBrokenPage:                   {SeverityWarning, "Page returns an error (4xx)"},
	IssueDuplicateTitle:               {SeverityWarning, "Duplicate title"},
	IssueDuplicateMetaDescription:     {SeverityWarning, "Duplicate meta description"},
	IssueDuplicateContent:             {SeverityWarning, "Duplicate page content"},
	IssueMissingMetaDescription:       {SeverityWarning, "Missing meta description"},
	IssueMissingH1:                    {SeverityWarning, "Missing H1 heading"},
	IssueMultipleH1:                   {SeverityWarning, "Multiple H1 headings"},
	IssueRedirectChain:                {SeverityWarning, "Redirect chain"},
	IssueRedirectLoop:                 {SeverityWarning, "Redirect loop"},
	IssueCanonicalConflict:            {SeverityWarning, "Conflicting canonical signals"},
	IssueThinContent:                  {SeverityWarning, "Thin content"},
	IssueImagesMissingAlt:             {SeverityWarning, "Images missing alt text"},
	IssueOrphanPage:                   {SeverityWarning, "Orphan page"},
	IssueNoOutgoingLinks:              {SeverityWarning, "Page has no outgoing links"},
	IssueTitleTooLong:                 {SeverityInfo, "Title too long"},
	IssueTitleTooShort:                {SeverityInfo, "Title too short"},
	IssueMetaDescriptionTooLong:       {SeverityInfo, "Meta description too long"},
	IssueMetaDescriptionTooShort:      {SeverityInfo, "Meta description too short"},
	IssueHeadingOrderSkip:             {SeverityInfo, "Heading levels skip"},
	IssueSlowResponse:                 {SeverityInfo, "Slow server response"},
	IssueNoindexPage:                  {SeverityInfo, "Page is noindex"},
	IssueCanonicalizedPage:            {SeverityInfo, "Canonicalized to another URL"},
	IssueDeepPage:                     {SeverityInfo, "Page is deep in the site structure"},
}

// getIssueDescriptor returns the descriptor for an issue type, or ok=false when
// the type is unknown.
func getIssueDescriptor(issueType AuditIssueType) (issueDescriptor, bool) {
	descriptor, ok := issueRegistry[issueType]
	return descriptor, ok
}

// severityOf returns the severity for an issue type, defaulting to warning so a
// future type cannot make a write fail.
func severityOf(issueType AuditIssueType) IssueSeverity {
	if descriptor, ok := issueRegistry[issueType]; ok {
		return descriptor.Severity
	}
	return SeverityWarning
}
