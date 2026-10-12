// Package dashboard serves the project dashboard: the onboarding checklist,
// the latest-audit card, the backlinks card and the visit-triggered backlink
// snapshot refresh. It reads the legacy TypeScript app's tables over plain
// SQL; the schema is unchanged from that app.
package dashboard

// GA4Card is the dashboard's Google Analytics connection card.
type GA4Card struct {
	Connected           bool    `json:"connected"`
	PropertyDisplayName *string `json:"propertyDisplayName"`
	CardDismissedAt     *string `json:"cardDismissedAt"`
}

// GSCCard is the dashboard's Google Search Console connection card.
type GSCCard struct {
	Connected bool    `json:"connected"`
	SiteURL   *string `json:"siteUrl"`
}

// MCPCard reports the organization's external MCP client milestones.
type MCPCard struct {
	AuthorizedAt    *string `json:"authorizedAt"`
	FirstToolCallAt *string `json:"firstToolCallAt"`
	CardDismissedAt *string `json:"cardDismissedAt"`
}

// Activation is the dashboard's onboarding checklist state. Steps that
// complete from product state are derived here; the competitor and keyword
// steps record a click-through, and the dismissals are per user.
type Activation struct {
	Domain              *string  `json:"domain"`
	HasMultipleProjects bool     `json:"hasMultipleProjects"`
	HasTeammate         bool     `json:"hasTeammate"`
	DismissedSteps      []string `json:"dismissedSteps"`
	GA4                 GA4Card  `json:"ga4"`
	GSC                 GSCCard  `json:"gsc"`
	MCP                 MCPCard  `json:"mcp"`
	CompetitorClickedAt *string  `json:"competitorClickedAt"`
	KeywordsClickedAt   *string  `json:"keywordsClickedAt"`
	HasAudit            bool     `json:"hasAudit"`
}

// IssueSummary counts the distinct pages one issue type affected.
type IssueSummary struct {
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Count     int64  `json:"count"`
}

// AuditSummary is the dashboard's one-paragraph audit card. It reads the
// legacy audits table: the dashboard has always shown whatever the last audit
// run recorded there, and the port does not change which table it reads.
type AuditSummary struct {
	Status          string         `json:"status"`
	PagesCrawled    int            `json:"pagesCrawled"`
	StartedAt       string         `json:"startedAt"`
	TopIssues       []IssueSummary `json:"topIssues"`
	TotalIssueTypes int            `json:"totalIssueTypes"`
}

// BacklinkSummary is the stored point-in-time backlinks snapshot for the
// project's domain. Stale reports a snapshot older than a day; the caller
// refreshes it by re-reading the provider.
type BacklinkSummary struct {
	Domain               string `json:"domain"`
	Rank                 *int32 `json:"rank"`
	Backlinks            *int64 `json:"backlinks"`
	ReferringDomains     *int64 `json:"referringDomains"`
	NewBacklinks         *int64 `json:"newBacklinks"`
	LostBacklinks        *int64 `json:"lostBacklinks"`
	NewReferringDomains  *int64 `json:"newReferringDomains"`
	LostReferringDomains *int64 `json:"lostReferringDomains"`
	CapturedAt           string `json:"capturedAt"`
	Stale                bool   `json:"stale"`
}

// Overview is the dashboard's two cards.
type Overview struct {
	Audit     *AuditSummary    `json:"audit"`
	Backlinks *BacklinkSummary `json:"backlinks"`
}
