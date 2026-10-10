package dashboardoverview

import (
	"context"
	"fmt"
	"slices"
	"time"
)

type Store interface {
	ProjectDomain(context.Context, string, string) (*string, error)
	LatestAudit(context.Context, string) (*AuditSummary, error)
	LatestBacklinkSnapshot(context.Context, string) (BacklinkSummary, bool, error)
}
type Service struct {
	Store Store
	Now   func() time.Time
}

func (s *Service) Get(ctx context.Context, organizationID, projectID string) (Overview, error) {
	if s == nil || s.Store == nil {
		return Overview{}, fmt.Errorf("dashboard overview store unavailable")
	}
	domain, err := s.Store.ProjectDomain(ctx, projectID, organizationID)
	if err != nil {
		return Overview{}, err
	}
	audit, err := s.Store.LatestAudit(ctx, projectID)
	if err != nil {
		return Overview{}, err
	}
	snapshot, found, err := s.Store.LatestBacklinkSnapshot(ctx, projectID)
	if err != nil {
		return Overview{}, err
	}
	var backlinks *BacklinkSummary
	if found && domain != nil && snapshot.Domain == *domain {
		snapshot.Stale = !fresh(snapshot.CapturedAt, s.now())
		backlinks = &snapshot
	}
	return Overview{Audit: audit, Backlinks: backlinks}, nil
}
func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func fresh(stamp string, now time.Time) bool {
	captured, err := time.Parse(time.RFC3339Nano, stamp)
	return err == nil && now.Sub(captured) < snapshotMaxAge
}
func sortIssues(rows []TopIssue) {
	rank := func(s string) int {
		switch s {
		case "critical":
			return 0
		case "warning":
			return 1
		default:
			return 2
		}
	}
	slices.SortFunc(rows, func(a, b TopIssue) int {
		if d := rank(a.Severity) - rank(b.Severity); d != 0 {
			return d
		}
		if d := b.Count - a.Count; d != 0 {
			return d
		}
		if a.IssueType < b.IssueType {
			return -1
		}
		if a.IssueType > b.IssueType {
			return 1
		}
		return 0
	})
}
