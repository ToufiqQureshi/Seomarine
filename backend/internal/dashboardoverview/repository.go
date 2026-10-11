package dashboardoverview

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrProjectNotFound = errors.New("dashboard project not found")

type Repository struct{ DB *pgxpool.Pool }

func (r Repository) ProjectDomain(ctx context.Context, projectID, organizationID string) (*string, error) {
	var domain *string
	err := r.DB.QueryRow(ctx, `SELECT domain FROM projects WHERE id=$1 AND organization_id=$2 AND archived_at IS NULL`, projectID, organizationID).Scan(&domain)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load dashboard project: %w", err)
	}
	return domain, nil
}
func (r Repository) LatestAudit(ctx context.Context, projectID string) (*AuditSummary, error) {
	var row AuditSummary
	err := r.DB.QueryRow(ctx, `SELECT status,pages_crawled,started_at FROM audits WHERE project_id=$1 ORDER BY started_at DESC,id DESC LIMIT 1`, projectID).Scan(&row.Status, &row.PagesCrawled, &row.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load dashboard audit: %w", err)
	}
	rows, err := r.DB.Query(ctx, `SELECT issue_type,severity,count(DISTINCT page_url)::int FROM audit_issues WHERE audit_id=(SELECT id FROM audits WHERE project_id=$1 ORDER BY started_at DESC,id DESC LIMIT 1) GROUP BY issue_type,severity`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query dashboard audit issues: %w", err)
	}
	defer rows.Close()
	issues := make([]TopIssue, 0)
	for rows.Next() {
		var item TopIssue
		if err := rows.Scan(&item.IssueType, &item.Severity, &item.Count); err != nil {
			return nil, fmt.Errorf("read dashboard audit issue: %w", err)
		}
		issues = append(issues, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard audit issues: %w", err)
	}
	sortIssues(issues)
	row.TotalIssueTypes = len(issues)
	if len(issues) > 3 {
		issues = issues[:3]
	}
	row.TopIssues = issues
	return &row, nil
}
func (r Repository) LatestBacklinkSnapshot(ctx context.Context, projectID string) (BacklinkSummary, bool, error) {
	var item BacklinkSummary
	err := r.DB.QueryRow(ctx, `SELECT domain,rank,backlinks,referring_domains,new_backlinks,lost_backlinks,new_referring_domains,lost_referring_domains,captured_at FROM backlink_snapshots WHERE project_id=$1 ORDER BY id DESC LIMIT 1`, projectID).Scan(&item.Domain, &item.Rank, &item.Backlinks, &item.ReferringDomains, &item.NewBacklinks, &item.LostBacklinks, &item.NewReferringDomains, &item.LostReferringDomains, &item.CapturedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BacklinkSummary{}, false, nil
	}
	if err != nil {
		return BacklinkSummary{}, false, fmt.Errorf("load dashboard backlink snapshot: %w", err)
	}
	return item, true, nil
}

func (r Repository) InsertBacklinkSnapshot(ctx context.Context, projectID string, item BacklinkSummary) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO backlink_snapshots
		(project_id,domain,rank,backlinks,referring_domains,new_backlinks,lost_backlinks,new_referring_domains,lost_referring_domains,captured_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, projectID, item.Domain, item.Rank, item.Backlinks,
		item.ReferringDomains, item.NewBacklinks, item.LostBacklinks, item.NewReferringDomains,
		item.LostReferringDomains, item.CapturedAt)
	if err != nil {
		return fmt.Errorf("insert dashboard backlink snapshot: %w", err)
	}
	return nil
}
