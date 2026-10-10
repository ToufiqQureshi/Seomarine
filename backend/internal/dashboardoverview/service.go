package dashboardoverview

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/backlinks"
)

type Store interface {
	ProjectDomain(context.Context, string, string) (*string, error)
	LatestAudit(context.Context, string) (*AuditSummary, error)
	LatestBacklinkSnapshot(context.Context, string) (BacklinkSummary, bool, error)
}
type backlinkSnapshotWriter interface {
	InsertBacklinkSnapshot(context.Context, string, BacklinkSummary) error
}
type Service struct {
	Store     Store
	Backlinks interface {
		DashboardSummary(context.Context, string, string) (backlinks.Summary, error)
	}
	Now func() time.Time
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

// RefreshBacklinkSnapshot captures one provider summary per project and domain
// per day. If a refresh fails, an existing stale snapshot remains usable.
func (s *Service) RefreshBacklinkSnapshot(ctx context.Context, organizationID, projectID string) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("dashboard overview store unavailable")
	}
	domain, err := s.Store.ProjectDomain(ctx, projectID, organizationID)
	if err != nil || domain == nil {
		return err
	}
	latest, found, err := s.Store.LatestBacklinkSnapshot(ctx, projectID)
	if err != nil {
		return err
	}
	matches := found && latest.Domain == *domain
	if matches && fresh(latest.CapturedAt, s.now()) {
		return nil
	}
	writer, ok := s.Store.(backlinkSnapshotWriter)
	if !ok {
		return fmt.Errorf("dashboard backlink snapshot writer unavailable")
	}
	if s.Backlinks == nil {
		if matches {
			return nil
		}
		return fmt.Errorf("dashboard backlink provider unavailable")
	}
	summary, err := s.Backlinks.DashboardSummary(ctx, organizationID, *domain)
	if err != nil {
		if matches {
			return nil
		}
		return err
	}
	snapshot := BacklinkSummary{
		Domain: *domain, Rank: toInt64(summary.Rank), Backlinks: toInt64(summary.Backlinks),
		ReferringDomains: toInt64(summary.ReferringDomains), NewBacklinks: toInt64(summary.NewBacklinks),
		LostBacklinks: toInt64(summary.LostBacklinks), NewReferringDomains: toInt64(summary.NewReferringDomains),
		LostReferringDomains: toInt64(summary.LostReferringDomains),
		CapturedAt:           s.now().UTC().Format(time.RFC3339Nano),
	}
	return writer.InsertBacklinkSnapshot(ctx, projectID, snapshot)
}
func toInt64(value *float64) *int64 {
	if value == nil {
		return nil
	}
	converted := int64(*value)
	return &converted
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
