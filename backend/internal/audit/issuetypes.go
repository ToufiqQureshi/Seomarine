package audit

// IssueSeverity orders and groups issues in the report.
type IssueSeverity string

// The three severities, mirroring IssueSeverity.
const (
	SeverityCritical IssueSeverity = "critical"
	SeverityWarning  IssueSeverity = "warning"
	SeverityInfo     IssueSeverity = "info"
)

// IssueType identifies one issue kind.
type IssueType string

// The closed registry of issue types, mirroring AUDIT_ISSUE_TYPES.
const (
	IssueBlockedPage                  IssueType = "blocked-page"
	IssueJavaScriptRenderingSuspected IssueType = "javascript-rendering-suspected"
	IssueRateLimitedPage              IssueType = "rate-limited-page"
	IssueCrawlRateLimited             IssueType = "crawl-rate-limited"
	IssueServerError                  IssueType = "server-error"
	IssueBrokenInternalLink           IssueType = "broken-internal-link"
	IssueMissingTitle                 IssueType = "missing-title"
	IssueBrokenPage                   IssueType = "broken-page"
	IssueDuplicateTitle               IssueType = "duplicate-title"
	IssueDuplicateMetaDescription     IssueType = "duplicate-meta-description"
	IssueDuplicateContent             IssueType = "duplicate-content"
	IssueMissingMetaDescription       IssueType = "missing-meta-description"
	IssueMissingH1                    IssueType = "missing-h1"
	IssueMultipleH1                   IssueType = "multiple-h1"
	IssueRedirectChain                IssueType = "redirect-chain"
	IssueRedirectLoop                 IssueType = "redirect-loop"
	IssueCanonicalConflict            IssueType = "canonical-conflict"
	IssueThinContent                  IssueType = "thin-content"
	IssueImagesMissingAlt             IssueType = "images-missing-alt"
	IssueOrphanPage                   IssueType = "orphan-page"
	IssueNoOutgoingLinks              IssueType = "no-outgoing-links"
	IssueTitleTooLong                 IssueType = "title-too-long"
	IssueTitleTooShort                IssueType = "title-too-short"
	IssueMetaDescriptionTooLong       IssueType = "meta-description-too-long"
	IssueMetaDescriptionTooShort      IssueType = "meta-description-too-short"
	IssueHeadingOrderSkip             IssueType = "heading-order-skip"
	IssueSlowResponse                 IssueType = "slow-response"
	IssueNoindexPage                  IssueType = "noindex-page"
	IssueCanonicalizedPage            IssueType = "canonicalized-page"
	IssueDeepPage                     IssueType = "deep-page"
)

// issueDescriptor is the server-side part of an issue descriptor: the severity
// (persisted with every row) and the human title. The full user-facing copy
// stays in the shared TypeScript registry, which the React pages already import.
type issueDescriptor struct {
	Severity IssueSeverity
	Title    string
}

// issueRegistry carries the severity and title for every known issue type.
var issueRegistry = map[IssueType]issueDescriptor{
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
func getIssueDescriptor(issueType IssueType) (issueDescriptor, bool) {
	descriptor, ok := issueRegistry[issueType]
	return descriptor, ok
}

// severityOf returns the severity for an issue type, defaulting to warning so a
// future type cannot make a write fail.
func severityOf(issueType IssueType) IssueSeverity {
	if descriptor, ok := issueRegistry[issueType]; ok {
		return descriptor.Severity
	}
	return SeverityWarning
}
