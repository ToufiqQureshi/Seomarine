package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists audits, pages, issues and Lighthouse results in Postgres
// using plain SQL. Every query is scoped by project (tenant isolation).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository returns a repository backed by pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// AuditRecord is one audit row.
type AuditRecord struct {
	ID                  string     `json:"id"`
	ProjectID           string     `json:"-"`
	StartedByUserID     string     `json:"-"`
	StartURL            string     `json:"startUrl"`
	Status              string     `json:"status"`
	WorkflowInstanceID  *string    `json:"-"`
	Config              string     `json:"-"`
	PagesCrawled        int        `json:"pagesCrawled"`
	PagesTotal          int        `json:"pagesTotal"`
	LighthouseTotal     int        `json:"lighthouseTotal"`
	LighthouseCompleted int        `json:"lighthouseCompleted"`
	LighthouseFailed    int        `json:"lighthouseFailed"`
	CurrentPhase        *string    `json:"currentPhase"`
	ErrorCode           *string    `json:"errorCode,omitempty"`
	ErrorDetail         *string    `json:"-"`
	FailedPhase         *string    `json:"-"`
	StartedAt           time.Time  `json:"startedAt"`
	CompletedAt         *time.Time `json:"completedAt"`
}

// PageRecord is one crawled page row, shaped as the UI consumes it.
type PageRecord struct {
	ID                 string         `json:"id"`
	URL                string         `json:"url"`
	StatusCode         *int           `json:"statusCode"`
	FetchClass         PageFetchClass `json:"fetchClass"`
	RedirectURL        *string        `json:"redirectUrl"`
	Title              *string        `json:"title"`
	MetaDescription    *string        `json:"metaDescription"`
	CanonicalURL       *string        `json:"canonicalUrl"`
	RobotsMeta         *string        `json:"robotsMeta"`
	XRobotsTag         *string        `json:"xRobotsTag"`
	HeaderCanonicalURL *string        `json:"headerCanonicalUrl"`
	OgTitle            *string        `json:"ogTitle"`
	OgDescription      *string        `json:"ogDescription"`
	OgImage            *string        `json:"ogImage"`
	H1Count            int            `json:"h1Count"`
	H2Count            int            `json:"h2Count"`
	H3Count            int            `json:"h3Count"`
	H4Count            int            `json:"h4Count"`
	H5Count            int            `json:"h5Count"`
	H6Count            int            `json:"h6Count"`
	HeadingOrder       []int          `json:"headingOrder"`
	WordCount          int            `json:"wordCount"`
	ContentHash        *string        `json:"contentHash"`
	ImagesTotal        int            `json:"imagesTotal"`
	ImagesMissingAlt   int            `json:"imagesMissingAlt"`
	Images             []ImageRef     `json:"images"`
	InternalLinkCount  int            `json:"internalLinkCount"`
	ExternalLinkCount  int            `json:"externalLinkCount"`
	HasStructuredData  bool           `json:"hasStructuredData"`
	HreflangTags       []string       `json:"hreflangTags"`
	IsIndexable        bool           `json:"isIndexable"`
	CrawlDepth         *int           `json:"crawlDepth"`
	InSitemap          bool           `json:"inSitemap"`
	ResponseTimeMs     *int           `json:"responseTimeMs"`
}

// IssueRecord is one issue row.
type IssueRecord struct {
	ID        string         `json:"id"`
	PageID    *string        `json:"pageId"`
	PageURL   string         `json:"pageUrl"`
	IssueType string         `json:"issueType"`
	Severity  IssueSeverity  `json:"severity"`
	Details   map[string]any `json:"details,omitempty"`
}

// LighthouseRecord is one Lighthouse result row.
type LighthouseRecord struct {
	ID                 string   `json:"id"`
	PageID             string   `json:"pageId"`
	URL                string   `json:"url"`
	Strategy           string   `json:"strategy"`
	PerformanceScore   *int     `json:"performanceScore"`
	AccessibilityScore *int     `json:"accessibilityScore"`
	BestPracticesScore *int     `json:"bestPracticesScore"`
	SeoScore           *int     `json:"seoScore"`
	LcpMs              *float64 `json:"lcpMs"`
	CLS                *float64 `json:"cls"`
	InpMs              *float64 `json:"inpMs"`
	TtfbMs             *float64 `json:"ttfbMs"`
	ErrorMessage       *string  `json:"errorMessage,omitempty"`
	PayloadJSON        *string  `json:"-"`
	HasPayload         bool     `json:"hasPayload"`
	PayloadSizeBytes   *int     `json:"payloadSizeBytes,omitempty"`
}

// CreateAuditInput describes a new audit row.
type CreateAuditInput struct {
	ID                 string
	ProjectID          string
	StartedByUserID    string
	StartURL           string
	WorkflowInstanceID string
	Config             string
	PagesTotal         int
	LighthouseTotal    int
}

// CreateAudit inserts a running audit row.
func (r *Repository) CreateAudit(ctx context.Context, input CreateAuditInput) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO go_audits (id, project_id, started_by_user_id, start_url, workflow_instance_id, config,
			status, pages_total, lighthouse_total, current_phase)
		VALUES ($1, $2, $3, $4, $5, $6, 'running', $7, $8, 'discovery')`,
		input.ID, input.ProjectID, input.StartedByUserID, input.StartURL, input.WorkflowInstanceID,
		input.Config, input.PagesTotal, input.LighthouseTotal)
	if err != nil {
		return fmt.Errorf("create audit: %w", err)
	}
	return nil
}

// ProgressUpdate carries the mutable progress fields of an audit.
type ProgressUpdate struct {
	PagesCrawled        *int
	PagesTotal          *int
	LighthouseTotal     *int
	LighthouseCompleted *int
	LighthouseFailed    *int
	CurrentPhase        *string
}

// UpdateAuditProgress updates an audit's progress, fenced by its workflow
// instance id so a superseded run cannot overwrite a live one.
func (r *Repository) UpdateAuditProgress(ctx context.Context, auditID, workflowInstanceID string, update ProgressUpdate) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE go_audits SET
			pages_crawled = COALESCE($3, pages_crawled),
			pages_total = COALESCE($4, pages_total),
			lighthouse_total = COALESCE($5, lighthouse_total),
			lighthouse_completed = COALESCE($6, lighthouse_completed),
			lighthouse_failed = COALESCE($7, lighthouse_failed),
			current_phase = COALESCE($8, current_phase)
		WHERE id = $1 AND workflow_instance_id = $2`,
		auditID, workflowInstanceID, update.PagesCrawled, update.PagesTotal, update.LighthouseTotal,
		update.LighthouseCompleted, update.LighthouseFailed, update.CurrentPhase)
	if err != nil {
		return fmt.Errorf("update audit progress: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrAuditNotFound, auditID)
	}
	return nil
}

// CompleteAudit marks an audit completed with its final counts.
func (r *Repository) CompleteAudit(ctx context.Context, auditID, workflowInstanceID string, pagesCrawled, pagesTotal int) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE go_audits SET status = 'completed', completed_at = now(), current_phase = 'completed',
			pages_crawled = $3, pages_total = $4
		WHERE id = $1 AND workflow_instance_id = $2`,
		auditID, workflowInstanceID, pagesCrawled, pagesTotal)
	if err != nil {
		return fmt.Errorf("complete audit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrAuditNotFound, auditID)
	}
	return nil
}

// FailAudit marks a running audit failed with its error classification. Only a
// running audit can transition to failed, so the reconciler cannot flip a
// just-completed audit.
func (r *Repository) FailAudit(ctx context.Context, auditID, workflowInstanceID string, info AuditErrorInfo, failedPhase string) error {
	var phase *string
	if failedPhase != "" {
		phase = &failedPhase
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE go_audits SET status = 'failed', completed_at = now(), current_phase = 'failed',
			error_code = $3, error_detail = $4, failed_phase = $5
		WHERE id = $1 AND workflow_instance_id = $2 AND status = 'running'`,
		auditID, workflowInstanceID, string(info.ErrorCode), info.ErrorDetail, phase)
	if err != nil {
		return fmt.Errorf("fail audit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrAuditNotFound, auditID)
	}
	return nil
}

const auditColumns = `id, project_id, started_by_user_id, start_url, status, workflow_instance_id, config,
	pages_crawled, pages_total, lighthouse_total, lighthouse_completed, lighthouse_failed, current_phase,
	error_code, error_detail, failed_phase, started_at, completed_at`

// scanAudit reads one audit row.
func scanAudit(row pgx.Row) (AuditRecord, error) {
	var record AuditRecord
	err := row.Scan(&record.ID, &record.ProjectID, &record.StartedByUserID, &record.StartURL, &record.Status,
		&record.WorkflowInstanceID, &record.Config, &record.PagesCrawled, &record.PagesTotal, &record.LighthouseTotal,
		&record.LighthouseCompleted, &record.LighthouseFailed, &record.CurrentPhase, &record.ErrorCode,
		&record.ErrorDetail, &record.FailedPhase, &record.StartedAt, &record.CompletedAt)
	if err != nil {
		return AuditRecord{}, err
	}
	return record, nil
}

// GetAuditForProject returns an audit scoped to its project.
func (r *Repository) GetAuditForProject(ctx context.Context, auditID, projectID string) (AuditRecord, error) {
	record, err := scanAudit(r.pool.QueryRow(ctx, `SELECT `+auditColumns+` FROM go_audits WHERE id = $1 AND project_id = $2`, auditID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return AuditRecord{}, fmt.Errorf("%w: %s", ErrAuditNotFound, auditID)
	}
	if err != nil {
		return AuditRecord{}, fmt.Errorf("load audit: %w", err)
	}
	return record, nil
}

// LatestAuditForProject returns the most recently started audit of a project.
func (r *Repository) LatestAuditForProject(ctx context.Context, projectID string) (AuditRecord, bool, error) {
	record, err := scanAudit(r.pool.QueryRow(ctx, `SELECT `+auditColumns+` FROM go_audits WHERE project_id = $1 ORDER BY started_at DESC LIMIT 1`, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return AuditRecord{}, false, nil
	}
	if err != nil {
		return AuditRecord{}, false, fmt.Errorf("load latest audit: %w", err)
	}
	return record, true, nil
}

// ListAuditsByProject returns a project's audits, newest first.
func (r *Repository) ListAuditsByProject(ctx context.Context, projectID string) ([]AuditRecord, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+auditColumns+` FROM go_audits WHERE project_id = $1 ORDER BY started_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list audits: %w", err)
	}
	defer rows.Close()
	records := []AuditRecord{}
	for rows.Next() {
		record, err := scanAudit(rows)
		if err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read audits: %w", err)
	}
	return records, nil
}

// AuditUsage is an organization's aggregate audit usage.
type AuditUsage struct {
	CapacityUnits int
	RunningCount  int
}

// AuditUsageForOrganization aggregates usage across every project of an
// organization. The free-plan ceiling belongs to the organization, so counting
// per starting user would multiply it by the member count.
func (r *Repository) AuditUsageForOrganization(ctx context.Context, organizationID string) (AuditUsage, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(a.pages_total + a.lighthouse_total), 0)::bigint,
			COALESCE(COUNT(*) FILTER (WHERE a.status = 'running'), 0)::bigint
		FROM go_audits a
		JOIN projects p ON p.id = a.project_id
		WHERE p.organization_id = $1`, organizationID)
	var usage AuditUsage
	if err := row.Scan(&usage.CapacityUnits, &usage.RunningCount); err != nil {
		return AuditUsage{}, fmt.Errorf("audit usage: %w", err)
	}
	return usage, nil
}

// DeleteAuditForProject deletes an audit scoped to its project. Cascades remove
// its pages, issues and Lighthouse results.
func (r *Repository) DeleteAuditForProject(ctx context.Context, auditID, projectID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM go_audits WHERE id = $1 AND project_id = $2`, auditID, projectID)
	if err != nil {
		return fmt.Errorf("delete audit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrAuditNotFound, auditID)
	}
	return nil
}

// CountPagesByFetchClass counts a page's fetch class within an audit.
func (r *Repository) CountPagesByFetchClass(ctx context.Context, auditID string, class PageFetchClass) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM go_audit_pages WHERE audit_id = $1 AND fetch_class = $2`, auditID, string(class)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pages: %w", err)
	}
	return count, nil
}

// InsertCrawledBatch persists one crawled sub-batch (pages + issues) in a
// single transaction, so a retry either completes the batch or repeats it
// idempotently. Pages upsert (last attempt wins); issues insert-or-ignore.
func (r *Repository) InsertCrawledBatch(ctx context.Context, auditID string, pages []CrawledPageResult, issues []DetectedIssue) error {
	if len(pages) == 0 && len(issues) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin crawl batch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if len(pages) > 0 {
		batch := &pgx.Batch{}
		for _, page := range pages {
			batch.Queue(upsertPageSQL, pageArgs(auditID, page)...)
		}
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return fmt.Errorf("insert crawled pages: %w", err)
		}
	}
	if len(issues) > 0 {
		if err := insertIssuesTx(ctx, tx, auditID, issues); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit crawl batch: %w", err)
	}
	return nil
}

const upsertPageSQL = `
	INSERT INTO go_audit_pages (
		id, audit_id, url, status_code, redirect_url, title, meta_description, canonical_url, robots_meta,
		x_robots_tag, header_canonical_url, og_title, og_description, og_image, h1_count, h2_count, h3_count,
		h4_count, h5_count, h6_count, heading_order_json, word_count, content_hash, images_total,
		images_missing_alt, images_json, internal_link_count, external_link_count, has_structured_data,
		hreflang_tags_json, is_indexable, fetch_class, crawl_depth, in_sitemap, response_time_ms
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35)
	ON CONFLICT (id) DO UPDATE SET
		url = EXCLUDED.url, status_code = EXCLUDED.status_code, redirect_url = EXCLUDED.redirect_url,
		title = EXCLUDED.title, meta_description = EXCLUDED.meta_description, canonical_url = EXCLUDED.canonical_url,
		robots_meta = EXCLUDED.robots_meta, x_robots_tag = EXCLUDED.x_robots_tag,
		header_canonical_url = EXCLUDED.header_canonical_url, og_title = EXCLUDED.og_title,
		og_description = EXCLUDED.og_description, og_image = EXCLUDED.og_image, h1_count = EXCLUDED.h1_count,
		h2_count = EXCLUDED.h2_count, h3_count = EXCLUDED.h3_count, h4_count = EXCLUDED.h4_count,
		h5_count = EXCLUDED.h5_count, h6_count = EXCLUDED.h6_count, heading_order_json = EXCLUDED.heading_order_json,
		word_count = EXCLUDED.word_count, content_hash = EXCLUDED.content_hash, images_total = EXCLUDED.images_total,
		images_missing_alt = EXCLUDED.images_missing_alt, images_json = EXCLUDED.images_json,
		internal_link_count = EXCLUDED.internal_link_count, external_link_count = EXCLUDED.external_link_count,
		has_structured_data = EXCLUDED.has_structured_data, hreflang_tags_json = EXCLUDED.hreflang_tags_json,
		is_indexable = EXCLUDED.is_indexable, fetch_class = EXCLUDED.fetch_class, crawl_depth = EXCLUDED.crawl_depth,
		in_sitemap = EXCLUDED.in_sitemap, response_time_ms = EXCLUDED.response_time_ms`

// pageArgs flattens a crawled page into the upsert column order.
func pageArgs(auditID string, page CrawledPageResult) []any {
	internal, external := 0, 0
	for _, link := range page.Links {
		if link.IsInternal {
			internal++
		} else {
			external++
		}
	}
	headingOrder, _ := json.Marshal(defaultIntSlice(page.HeadingOrder))
	images, _ := json.Marshal(defaultImageSlice(page.Images))
	hreflang, _ := json.Marshal(defaultStringSlice(page.HreflangTags))
	var responseTime *int
	if page.ResponseTimeMs > 0 {
		value := int(page.ResponseTimeMs)
		responseTime = &value
	}
	return []any{
		page.ID, auditID, page.URL, nullableInt(page.StatusCode), nullableString(page.RedirectURL),
		nullableString(page.Title), nullableString(page.MetaDescription), nullableString(page.CanonicalURL),
		nullableString(page.RobotsMeta), nullableString(page.XRobotsTag), nullableString(page.HeaderCanonicalURL),
		nullableString(page.OgTitle), nullableString(page.OgDescription), nullableString(page.OgImage),
		page.H1Count, page.H2Count, page.H3Count, page.H4Count, page.H5Count, page.H6Count, string(headingOrder),
		page.WordCount, nullableString(page.ContentHash), page.ImagesTotal, page.ImagesMissingAlt, string(images),
		internal, external, page.HasStructuredData, string(hreflang), page.IsIndexable, string(page.FetchClass),
		page.CrawlDepth, page.InSitemap, responseTime,
	}
}

func defaultIntSlice(value []int) []int {
	if value == nil {
		return []int{}
	}
	return value
}

func defaultImageSlice(value []ImageRef) []ImageRef {
	if value == nil {
		return []ImageRef{}
	}
	return value
}

func defaultStringSlice(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

// InsertIssues persists issues insert-or-ignore, so a retried batch does not
// duplicate rows.
func (r *Repository) InsertIssues(ctx context.Context, auditID string, issues []DetectedIssue) error {
	if len(issues) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin issue insert: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := insertIssuesTx(ctx, tx, auditID, issues); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit issue insert: %w", err)
	}
	return nil
}

// insertIssuesTx writes issues inside an existing transaction.
func insertIssuesTx(ctx context.Context, tx pgx.Tx, auditID string, issues []DetectedIssue) error {
	batch := &pgx.Batch{}
	for _, issue := range issues {
		id := DeterministicAuditRowID(auditID, issue.PageURL, string(issue.IssueType), issue.DedupeKey)
		var detailsJSON *string
		if len(issue.Details) > 0 {
			encoded, err := json.Marshal(issue.Details)
			if err != nil {
				return fmt.Errorf("encode issue details: %w", err)
			}
			value := string(encoded)
			detailsJSON = &value
		}
		batch.Queue(`
			INSERT INTO go_audit_issues (id, audit_id, page_id, page_url, issue_type, severity, details_json)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (id) DO NOTHING`,
			id, auditID, issue.PageID, issue.PageURL, string(issue.IssueType), string(severityOf(issue.IssueType)), detailsJSON)
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("insert issues: %w", err)
	}
	return nil
}

// InsertLighthouseResults upserts Lighthouse results idempotently.
func (r *Repository) InsertLighthouseResults(ctx context.Context, auditID string, results []LighthouseRecord) error {
	if len(results) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, result := range results {
		id := DeterministicAuditRowID(auditID, result.PageID, result.Strategy)
		var payloadSize *int
		if result.PayloadJSON != nil {
			size := len(*result.PayloadJSON)
			payloadSize = &size
		}
		batch.Queue(`
			INSERT INTO go_audit_lighthouse_results (
				id, audit_id, page_id, url, strategy, performance_score, accessibility_score, best_practices_score,
				seo_score, lcp_ms, cls, inp_ms, ttfb_ms, error_message, payload_json, payload_size_bytes
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
			ON CONFLICT (id) DO UPDATE SET
				url = EXCLUDED.url, performance_score = EXCLUDED.performance_score,
				accessibility_score = EXCLUDED.accessibility_score, best_practices_score = EXCLUDED.best_practices_score,
				seo_score = EXCLUDED.seo_score, lcp_ms = EXCLUDED.lcp_ms, cls = EXCLUDED.cls, inp_ms = EXCLUDED.inp_ms,
				ttfb_ms = EXCLUDED.ttfb_ms, error_message = EXCLUDED.error_message, payload_json = EXCLUDED.payload_json,
				payload_size_bytes = EXCLUDED.payload_size_bytes`,
			id, auditID, result.PageID, result.URL, result.Strategy, result.PerformanceScore, result.AccessibilityScore,
			result.BestPracticesScore, result.SeoScore, result.LcpMs, result.CLS, result.InpMs, result.TtfbMs,
			result.ErrorMessage, result.PayloadJSON, payloadSize)
	}
	if err := r.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("insert lighthouse results: %w", err)
	}
	return nil
}

// Results bundles one audit's pages, issues and Lighthouse results.
type Results struct {
	Audit      AuditRecord
	Pages      []PageRecord
	Issues     []IssueRecord
	Lighthouse []LighthouseRecord
}

// GetAuditResultsForProject loads an audit and its children, scoped to the
// project.
func (r *Repository) GetAuditResultsForProject(ctx context.Context, auditID, projectID string) (Results, error) {
	audit, err := r.GetAuditForProject(ctx, auditID, projectID)
	if err != nil {
		return Results{}, err
	}
	results := Results{Audit: audit, Pages: []PageRecord{}, Issues: []IssueRecord{}, Lighthouse: []LighthouseRecord{}}
	if results.Pages, err = r.listPages(ctx, auditID); err != nil {
		return Results{}, err
	}
	if results.Issues, err = r.listIssues(ctx, auditID); err != nil {
		return Results{}, err
	}
	if results.Lighthouse, err = r.listLighthouse(ctx, auditID); err != nil {
		return Results{}, err
	}
	return results, nil
}

func (r *Repository) listPages(ctx context.Context, auditID string) ([]PageRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, url, status_code, fetch_class, redirect_url, title, meta_description, canonical_url, robots_meta,
			x_robots_tag, header_canonical_url, og_title, og_description, og_image, h1_count, h2_count, h3_count,
			h4_count, h5_count, h6_count, heading_order_json, word_count, content_hash, images_total,
			images_missing_alt, images_json, internal_link_count, external_link_count, has_structured_data,
			hreflang_tags_json, is_indexable, crawl_depth, in_sitemap, response_time_ms
		FROM go_audit_pages WHERE audit_id = $1 ORDER BY id`, auditID)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	defer rows.Close()
	pages := []PageRecord{}
	for rows.Next() {
		var page PageRecord
		var headingOrderJSON, imagesJSON, hreflangJSON *string
		if err := rows.Scan(&page.ID, &page.URL, &page.StatusCode, &page.FetchClass, &page.RedirectURL, &page.Title,
			&page.MetaDescription, &page.CanonicalURL, &page.RobotsMeta, &page.XRobotsTag, &page.HeaderCanonicalURL,
			&page.OgTitle, &page.OgDescription, &page.OgImage, &page.H1Count, &page.H2Count, &page.H3Count,
			&page.H4Count, &page.H5Count, &page.H6Count, &headingOrderJSON, &page.WordCount, &page.ContentHash,
			&page.ImagesTotal, &page.ImagesMissingAlt, &imagesJSON, &page.InternalLinkCount, &page.ExternalLinkCount,
			&page.HasStructuredData, &hreflangJSON, &page.IsIndexable, &page.CrawlDepth, &page.InSitemap,
			&page.ResponseTimeMs); err != nil {
			return nil, fmt.Errorf("scan page: %w", err)
		}
		page.HeadingOrder = decodeInts(headingOrderJSON)
		page.Images = decodeImages(imagesJSON)
		page.HreflangTags = decodeStrings(hreflangJSON)
		pages = append(pages, page)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read pages: %w", err)
	}
	return pages, nil
}

func (r *Repository) listIssues(ctx context.Context, auditID string) ([]IssueRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, page_id, page_url, issue_type, severity, details_json
		FROM go_audit_issues WHERE audit_id = $1 ORDER BY id`, auditID)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	defer rows.Close()
	issues := []IssueRecord{}
	for rows.Next() {
		var issue IssueRecord
		var detailsJSON *string
		if err := rows.Scan(&issue.ID, &issue.PageID, &issue.PageURL, &issue.IssueType, &issue.Severity, &detailsJSON); err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		issue.Details = decodeDetails(detailsJSON)
		issues = append(issues, issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read issues: %w", err)
	}
	return issues, nil
}

// GetLighthouseForProject returns one stored Lighthouse result scoped to the
// project. It returns ErrLighthouseNotFound when the result is missing, belongs
// to another project, or has no stored payload.
func (r *Repository) GetLighthouseForProject(ctx context.Context, resultID, projectID string) (LighthouseDetail, error) {
	var detail LighthouseDetail
	err := r.pool.QueryRow(ctx, `
		SELECT l.id, l.strategy, COALESCE(p.url, ''), a.started_at, l.payload_json
		FROM go_audit_lighthouse_results l
		JOIN go_audits a ON a.id = l.audit_id
		LEFT JOIN go_audit_pages p ON p.id = l.page_id
		WHERE l.id = $1 AND a.project_id = $2 AND l.payload_json IS NOT NULL`, resultID, projectID).
		Scan(&detail.ID, &detail.Strategy, &detail.PageURL, &detail.StartedAt, &detail.PayloadJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return LighthouseDetail{}, ErrLighthouseNotFound
	}
	if err != nil {
		return LighthouseDetail{}, fmt.Errorf("get lighthouse result: %w", err)
	}
	return detail, nil
}

func (r *Repository) listLighthouse(ctx context.Context, auditID string) ([]LighthouseRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, page_id, url, strategy, performance_score, accessibility_score, best_practices_score, seo_score,
			lcp_ms, cls, inp_ms, ttfb_ms, error_message, payload_json IS NOT NULL, payload_size_bytes
		FROM go_audit_lighthouse_results WHERE audit_id = $1 ORDER BY id`, auditID)
	if err != nil {
		return nil, fmt.Errorf("list lighthouse results: %w", err)
	}
	defer rows.Close()
	records := []LighthouseRecord{}
	for rows.Next() {
		var record LighthouseRecord
		if err := rows.Scan(&record.ID, &record.PageID, &record.URL, &record.Strategy, &record.PerformanceScore,
			&record.AccessibilityScore, &record.BestPracticesScore, &record.SeoScore, &record.LcpMs, &record.CLS,
			&record.InpMs, &record.TtfbMs, &record.ErrorMessage, &record.HasPayload, &record.PayloadSizeBytes); err != nil {
			return nil, fmt.Errorf("scan lighthouse result: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read lighthouse results: %w", err)
	}
	return records, nil
}

// ListSlimPages returns the page subset the cross-page checks need.
func (r *Repository) ListSlimPages(ctx context.Context, auditID string) ([]SlimPage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, url, status_code, fetch_class, redirect_url, title, meta_description, content_hash, word_count,
			is_indexable, canonical_url, header_canonical_url
		FROM go_audit_pages WHERE audit_id = $1`, auditID)
	if err != nil {
		return nil, fmt.Errorf("list slim pages: %w", err)
	}
	defer rows.Close()
	pages := []SlimPage{}
	for rows.Next() {
		var page SlimPage
		var title, metaDescription, contentHash, redirectURL, canonicalURL, headerCanonicalURL *string
		if err := rows.Scan(&page.ID, &page.URL, &page.StatusCode, &page.FetchClass, &redirectURL, &title,
			&metaDescription, &contentHash, &page.WordCount, &page.IsIndexable, &canonicalURL, &headerCanonicalURL); err != nil {
			return nil, fmt.Errorf("scan slim page: %w", err)
		}
		page.Title = derefString(title)
		page.MetaDescription = derefString(metaDescription)
		page.ContentHash = derefString(contentHash)
		page.RedirectURL = derefString(redirectURL)
		page.CanonicalURL = derefString(canonicalURL)
		page.HeaderCanonicalURL = derefString(headerCanonicalURL)
		pages = append(pages, page)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read slim pages: %w", err)
	}
	return pages, nil
}

// ShellPageIDs returns the ids of pages already flagged as unrendered app
// shells, whose placeholder titles must not become duplicate findings.
func (r *Repository) ShellPageIDs(ctx context.Context, auditID string) (map[string]struct{}, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT page_id FROM go_audit_issues WHERE audit_id = $1 AND issue_type = $2 AND page_id IS NOT NULL`,
		auditID, string(IssueJavaScriptRenderingSuspected))
	if err != nil {
		return nil, fmt.Errorf("list shell pages: %w", err)
	}
	defer rows.Close()
	ids := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan shell page: %w", err)
		}
		ids[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read shell pages: %w", err)
	}
	return ids, nil
}

// RunMultipageChecks runs the cross-page checks over a persisted audit,
// returning the issues and whether unrendered shells were present.
func (r *Repository) RunMultipageChecks(ctx context.Context, auditID string) ([]DetectedIssue, bool, error) {
	shellIDs, err := r.ShellPageIDs(ctx, auditID)
	if err != nil {
		return nil, false, err
	}
	pages, err := r.ListSlimPages(ctx, auditID)
	if err != nil {
		return nil, false, err
	}
	filtered := make([]SlimPage, 0, len(pages))
	for _, page := range pages {
		if _, isShell := shellIDs[page.ID]; isShell {
			continue
		}
		filtered = append(filtered, page)
	}
	issues := append(FindDuplicates(filtered), FindRedirectChainsAndLoops(filtered)...)
	return issues, len(shellIDs) > 0, nil
}

// nullableInt returns nil for a zero status code so a missing status stays NULL.
func nullableInt(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

// nullableString returns nil for an empty string.
func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func decodeInts(raw *string) []int {
	if raw == nil || *raw == "" {
		return []int{}
	}
	var values []int
	if err := json.Unmarshal([]byte(*raw), &values); err != nil {
		return []int{}
	}
	return values
}

func decodeStrings(raw *string) []string {
	if raw == nil || *raw == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(*raw), &values); err != nil {
		return []string{}
	}
	return values
}

func decodeImages(raw *string) []ImageRef {
	if raw == nil || *raw == "" {
		return []ImageRef{}
	}
	var values []ImageRef
	if err := json.Unmarshal([]byte(*raw), &values); err != nil {
		return []ImageRef{}
	}
	return values
}

func decodeDetails(raw *string) map[string]any {
	if raw == nil || *raw == "" {
		return nil
	}
	var details map[string]any
	if err := json.Unmarshal([]byte(*raw), &details); err != nil {
		return nil
	}
	return details
}
