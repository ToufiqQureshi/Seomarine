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

type IssueDescriptor struct {
	Severity IssueSeverity
	Title string
	HowToFix string
}

var issueRemediation = map[IssueType]string{
	IssueType("blocked-page"): "If you own this site, allowlist the \"Seomarine-Audit\" user agent in your WAF/bot-protection settings (on Cloudflare: a WAF custom rule that skips bot protection when the user agent contains \"Seomarine-Audit\"; on some free tiers you may need to relax bot protection). Then re-run the audit. On Shopify, use Crawler access instead (Online Store → Preferences → Crawler access) and paste the signature into Seomarine under Settings → Crawler access.",
	IssueType("javascript-rendering-suspected"): "If this audit did not render JavaScript, start a new audit with \"Render JavaScript\" enabled to check the loaded content. For reliable crawling, serve important content and navigation in the initial HTML using server-side rendering or prerendering. This warning does not prove that search engines cannot index the page.",
	IssueType("rate-limited-page"): "Raise the rate limit for crawlers, or allowlist the \"Seomarine-Audit\" user agent in your rate-limiting rules (on Cloudflare: a rate-limiting rule exception matching that user agent). Then re-run the audit. Re-running with fewer pages also helps if the limit is strict. On Shopify, use Crawler access instead (Online Store → Preferences → Crawler access) and paste the signature into Seomarine under Settings → Crawler access.",
	IssueType("crawl-rate-limited"): "Re-run the audit after the site's rate limit resets, or ask the site owner to allow the Seomarine-Audit crawler. On Shopify, use Crawler access instead (Online Store → Preferences → Crawler access) and paste the signature into Seomarine under Settings → Crawler access.",
	IssueType("server-error"): "Check the server logs for this URL and fix the underlying error. If the page is gone, return a 404/410 or redirect it to a relevant page instead of erroring.",
	IssueType("broken-internal-link"): "Update the link to point at the correct live URL, or remove it. If the target was moved, prefer linking directly to the new URL rather than relying on a redirect.",
	IssueType("missing-title"): "Add a unique, descriptive <title> of roughly 50–60 characters that includes the page's primary topic.",
	IssueType("broken-page"): "If the page should exist, restore it. If it is intentionally gone, remove it from the sitemap and internal links, and consider a 301 redirect to the closest live page.",
	IssueType("duplicate-title"): "Write a unique title for each page describing its specific content. For templated pages, include the distinguishing attribute (name, category, location) in the template.",
	IssueType("duplicate-meta-description"): "Write a unique meta description per page, or remove the duplicated one entirely — search engines will generate a snippet from page content, which beats a wrong duplicate.",
	IssueType("duplicate-content"): "Consolidate duplicates: pick the canonical URL, add rel=canonical from the others, and 301-redirect duplicate URLs where possible (common causes: trailing-slash variants, URL parameters, http/https or www variants).",
	IssueType("missing-meta-description"): "Add a meta description of roughly 70–160 characters that summarizes the page and gives a reason to click.",
	IssueType("missing-h1"): "Add a single H1 that states the page's main topic, consistent with the title tag.",
	IssueType("multiple-h1"): "Keep one H1 for the page's main heading and demote the others to H2/H3 (or unstyled elements for non-headings like logos).",
	IssueType("redirect-chain"): "Point the first URL (and any internal links) directly at the final destination so there is at most one redirect.",
	IssueType("redirect-loop"): "Trace the redirect rules for this URL and break the cycle so the chain terminates at a real 200 page.",
	IssueType("canonical-conflict"): "Pick one canonical URL and declare it in exactly one place (HTML head is the most common); remove or align the other declaration.",
	IssueType("thin-content"): "Either expand the page with genuinely useful content, noindex it, or consolidate it into a stronger page. If the content exists but is rendered by JavaScript, ensure it is server-rendered or pre-rendered.",
	IssueType("images-missing-alt"): "Add descriptive alt text to meaningful images; use an empty alt (alt=\"\") only for purely decorative ones.",
	IssueType("orphan-page"): "Link to this page from relevant pages (navigation, related content, hub pages), or remove it from the sitemap if it shouldn't be indexed.",
	IssueType("no-outgoing-links"): "Add links to related pages, the parent category, or the homepage. If the page's navigation is rendered by JavaScript, make sure it also exists in the server-rendered HTML.",
	IssueType("title-too-long"): "Shorten the title to roughly 50–60 characters, front-loading the most important words.",
	IssueType("title-too-short"): "Expand the title into a descriptive phrase (roughly 30–60 characters) that states what the page offers.",
	IssueType("meta-description-too-long"): "Trim the description to roughly 70–160 characters while keeping the core message and call to action.",
	IssueType("meta-description-too-short"): "Expand the description to roughly 70–160 characters that summarize the page and give a reason to click.",
	IssueType("heading-order-skip"): "Adjust heading levels so they descend one step at a time (H1 → H2 → H3) without skipping.",
	IssueType("slow-response"): "Investigate server/database time and caching for this route; serving cached or statically generated HTML usually fixes it.",
	IssueType("noindex-page"): "If this page should rank, remove the noindex directive. If it's intentional (admin, thank-you, filter pages), no action is needed.",
	IssueType("canonicalized-page"): "If this page should rank on its own, set its canonical to itself. Otherwise no action is needed.",
	IssueType("deep-page"): "Add links from higher-level pages (hubs, category pages, navigation) to flatten the path to this page.",
}

func DescribeIssueType(issueType string) (IssueDescriptor, bool) {
	key := IssueType(issueType)
	descriptor, ok := issueRegistry[key]
	if !ok { return IssueDescriptor{}, false }
	return IssueDescriptor{Severity: descriptor.Severity, Title: descriptor.Title, HowToFix: issueRemediation[key]}, true
}
