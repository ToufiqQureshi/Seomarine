package dashboardoverview

import "time"

// Overview contains the audit and backlink summaries shown on a project dashboard.
type Overview struct {
	Audit     *AuditSummary    `json:"audit"`
	Backlinks *BacklinkSummary `json:"backlinks"`
}

// AuditSummary contains the latest audit status and leading issues.
type AuditSummary struct {
	Status          string     `json:"status"`
	PagesCrawled    int        `json:"pagesCrawled"`
	StartedAt       string     `json:"startedAt"`
	TopIssues       []TopIssue `json:"topIssues"`
	TotalIssueTypes int        `json:"totalIssueTypes"`
}

// TopIssue summarizes one audit issue type and severity.
type TopIssue struct {
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Count     int    `json:"count"`
}

// BacklinkSummary contains a project's latest backlink metrics.
type BacklinkSummary struct {
	Domain               string `json:"domain"`
	Rank                 *int64 `json:"rank"`
	Backlinks            *int64 `json:"backlinks"`
	ReferringDomains     *int64 `json:"referringDomains"`
	NewBacklinks         *int64 `json:"newBacklinks"`
	LostBacklinks        *int64 `json:"lostBacklinks"`
	NewReferringDomains  *int64 `json:"newReferringDomains"`
	LostReferringDomains *int64 `json:"lostReferringDomains"`
	CapturedAt           string `json:"capturedAt"`
	Stale                bool   `json:"stale"`
}

const snapshotMaxAge = 24 * time.Hour
