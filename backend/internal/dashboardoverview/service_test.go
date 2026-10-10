package dashboardoverview

import (
	"context"
	"testing"
	"time"
)

type storeStub struct {
	domain                *string
	audit                 *AuditSummary
	snapshot              BacklinkSummary
	found                 bool
	project, organization string
}

func (s *storeStub) ProjectDomain(_ context.Context, p, o string) (*string, error) {
	s.project, s.organization = p, o
	return s.domain, nil
}
func (s *storeStub) LatestAudit(context.Context, string) (*AuditSummary, error) { return s.audit, nil }
func (s *storeStub) LatestBacklinkSnapshot(context.Context, string) (BacklinkSummary, bool, error) {
	return s.snapshot, s.found, nil
}
func TestServiceScopesProjectAndMarksSnapshotStaleness(t *testing.T) {
	domain := "example.com"
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	store := &storeStub{domain: &domain, audit: &AuditSummary{Status: "completed", TopIssues: []TopIssue{}}, snapshot: BacklinkSummary{Domain: domain, CapturedAt: now.Add(-25 * time.Hour).Format(time.RFC3339Nano)}, found: true}
	svc := &Service{Store: store, Now: func() time.Time { return now }}
	got, err := svc.Get(context.Background(), "org-1", "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if store.organization != "org-1" || store.project != "project-1" || !got.Backlinks.Stale || got.Audit == nil {
		t.Fatalf("overview=%+v scope=%s/%s", got, store.organization, store.project)
	}
	store.snapshot.Domain = "another.example"
	got, err = svc.Get(context.Background(), "org-1", "project-1")
	if err != nil || got.Backlinks != nil {
		t.Fatalf("foreign-domain snapshot=%+v err=%v", got.Backlinks, err)
	}
}
func TestSortIssuesMatchesSeverityThenPageCount(t *testing.T) {
	rows := []TopIssue{{IssueType: "few-critical", Severity: "critical", Count: 1}, {IssueType: "warning", Severity: "warning", Count: 40}, {IssueType: "many-critical", Severity: "critical", Count: 8}, {IssueType: "info", Severity: "info", Count: 100}}
	sortIssues(rows)
	if rows[0].IssueType != "many-critical" || rows[1].IssueType != "few-critical" || rows[2].IssueType != "warning" {
		t.Fatalf("sorted issues=%+v", rows)
	}
}
