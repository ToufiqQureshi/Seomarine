package dashboardoverview

import "time"

type Overview struct {
	Audit     *AuditSummary    `json:"audit"`
	Backlinks *BacklinkSummary `json:"backlinks"`
}
type AuditSummary struct {
	Status          string     `json:"status"`
	PagesCrawled    int        `json:"pagesCrawled"`
	StartedAt       string     `json:"startedAt"`
	TopIssues       []TopIssue `json:"topIssues"`
	TotalIssueTypes int        `json:"totalIssueTypes"`
}
type TopIssue struct {
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Count     int    `json:"count"`
}
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
