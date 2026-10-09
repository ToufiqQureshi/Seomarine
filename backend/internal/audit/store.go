package audit

import "context"

// Store is the persistence surface the Service and Runner need. *Repository
// implements it against Postgres; tests use an in-memory fake.
type Store interface {
	CreateAudit(ctx context.Context, input CreateAuditInput) error
	UpdateAuditProgress(ctx context.Context, auditID, workflowInstanceID string, update ProgressUpdate) error
	CompleteAudit(ctx context.Context, auditID, workflowInstanceID string, pagesCrawled, pagesTotal int) error
	FailAudit(ctx context.Context, auditID, workflowInstanceID string, info AuditErrorInfo, failedPhase string) error
	GetAuditForProject(ctx context.Context, auditID, projectID string) (AuditRecord, error)
	GetAuditResultsForProject(ctx context.Context, auditID, projectID string) (Results, error)
	ListAuditsByProject(ctx context.Context, projectID string) ([]AuditRecord, error)
	AuditUsageForOrganization(ctx context.Context, organizationID string) (AuditUsage, error)
	DeleteAuditForProject(ctx context.Context, auditID, projectID string) error
	InsertCrawledBatch(ctx context.Context, auditID string, pages []CrawledPageResult, issues []DetectedIssue) error
	InsertIssues(ctx context.Context, auditID string, issues []DetectedIssue) error
	InsertLighthouseResults(ctx context.Context, auditID string, results []LighthouseRecord) error
	GetLighthouseForProject(ctx context.Context, resultID, projectID string) (LighthouseDetail, error)
	ListSlimPages(ctx context.Context, auditID string) ([]SlimPage, error)
	ShellPageIDs(ctx context.Context, auditID string) (map[string]struct{}, error)
	RunMultipageChecks(ctx context.Context, auditID string) ([]DetectedIssue, bool, error)
	CountPagesByFetchClass(ctx context.Context, auditID string, class PageFetchClass) (int, error)
}

// compile-time assertion that *Repository satisfies Store.
var _ Store = (*Repository)(nil)
