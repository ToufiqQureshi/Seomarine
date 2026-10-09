package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// PaidPlans reports whether an organization has a paid plan.
type PaidPlans interface {
	HasPaidPlan(ctx context.Context, organizationID string) (bool, error)
}

// ManagedAccess reports whether an organization may use the managed service at
// all. It is the floor for running audit work.
type ManagedAccess interface {
	HasManagedAccess(ctx context.Context, organizationID string) (bool, error)
}

// CrawlerAccessResolver resolves a stored crawler-access credential for a host.
type CrawlerAccessResolver interface {
	Resolve(ctx context.Context, organizationID, projectID, host string) (*CrawlerAccess, string, error)
}

// Scheduler enqueues and terminates audit work.
type Scheduler interface {
	// EnqueueAudit schedules an audit's job. It is idempotent on auditID.
	EnqueueAudit(ctx context.Context, auditID string, payload JobPayload) error
	// TerminateAudit stops a running audit, if one exists.
	TerminateAudit(ctx context.Context, auditID string) error
}

// JobPayload is the schedule payload for one audit.
type JobPayload struct {
	AuditID        string          `json:"auditId"`
	ProjectID      string          `json:"projectId"`
	StartURL       string          `json:"startUrl"`
	Config         Config          `json:"config"`
	OrganizationID string          `json:"organizationId"`
	RenderingLocks []RenderingLock `json:"renderingLocks,omitempty"`
}

// ServiceConfig configures the audit Service.
type ServiceConfig struct {
	Repository Store
	Progress   *Progress
	Guard      *Guard
	Crawler    *Crawler
	Lighthouse LighthouseProvider
	Scheduler  Scheduler
	Plans      PaidPlans
	Access     ManagedAccess
	Rendering  RenderingMeter
	Resolver   CrawlerAccessResolver
	// Hosted is true when audits are metered and plan-gated (the hosted
	// product). Self-hosted deployments are not gated.
	Hosted bool
	// Env reads environment variables for rendering availability.
	Env    func(string) string
	Logger *slog.Logger
	Now    func() time.Time
	NewID  func() string
}

// Service holds the audit rules.
type Service struct {
	repo       Store
	progress   *Progress
	guard      *Guard
	crawler    *Crawler
	lighthouse LighthouseProvider
	scheduler  Scheduler
	plans      PaidPlans
	access     ManagedAccess
	rendering  RenderingMeter
	resolver   CrawlerAccessResolver
	hosted     bool
	env        func(string) string
	logger     *slog.Logger
	now        func() time.Time
	newID      func() string
}

// NewService builds a Service. A nil logger uses the default; a nil clock uses
// time.Now; a nil id generator uses random ids.
func NewService(config ServiceConfig) *Service {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	newID := config.NewID
	if newID == nil {
		newID = randomID
	}
	return &Service{
		repo: config.Repository, progress: config.Progress, guard: config.Guard, crawler: config.Crawler,
		lighthouse: config.Lighthouse, scheduler: config.Scheduler, plans: config.Plans, access: config.Access,
		rendering: config.Rendering, resolver: config.Resolver, hosted: config.Hosted, env: config.Env,
		logger: logger, now: now, newID: newID,
	}
}

// randomID returns a random 32-character hex id.
func randomID() string {
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return fmt.Sprintf("audit-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer[:])
}

// ResolveAuditLimitTier resolves the plan ceiling for a new audit.
func (s *Service) ResolveAuditLimitTier(ctx context.Context, organizationID string) (LimitTier, error) {
	if !s.hosted {
		return TierSelfHosted, nil
	}
	if s.access != nil {
		allowed, err := s.access.HasManagedAccess(ctx, organizationID)
		if err != nil {
			return "", fmt.Errorf("check managed access: %w", err)
		}
		if !allowed {
			return "", fmt.Errorf("%w: subscribe to run site audits", ErrPaymentRequired)
		}
	}
	if s.plans != nil {
		paid, err := s.plans.HasPaidPlan(ctx, organizationID)
		if err != nil {
			return "", fmt.Errorf("check paid plan: %w", err)
		}
		if paid {
			return TierPaid, nil
		}
	}
	return TierFree, nil
}

// StartAuditInput is a validated request to start an audit.
type StartAuditInput struct {
	ActorUserID        string
	OrganizationID     string
	ProjectID          string
	StartURL           string
	MaxPages           int
	LighthouseStrategy LighthouseStrategy
	RenderJavaScript   bool
	LimitTier          LimitTier
}

// StartAuditResult carries the new audit's id.
type StartAuditResult struct {
	AuditID string `json:"auditId"`
}

// StartAudit validates the request, gates it by plan, creates the audit row and
// schedules its job. Mirrors AuditService.startAudit.
func (s *Service) StartAudit(ctx context.Context, input StartAuditInput) (StartAuditResult, error) {
	if s.guard == nil {
		return StartAuditResult{}, errors.New("audit service: SSRF guard is required")
	}
	if input.RenderJavaScript && !IsAuditRenderingAllowed(s.env) {
		return StartAuditResult{}, fmt.Errorf("%w: JavaScript rendering is not available on this deployment", ErrRenderingUnavailable)
	}
	limits, ok := auditLimits[input.LimitTier]
	if !ok {
		limits = auditLimits[TierSelfHosted]
	}
	maxPages := clampMaxPages(input.MaxPages)
	if maxPages > limits.MaxPagesPerAudit {
		return StartAuditResult{}, fmt.Errorf("%w: at most %d pages for this plan", ErrAuditPageLimitExceeded, limits.MaxPagesPerAudit)
	}
	if input.RenderJavaScript && maxPages > RenderedMaxAuditPages {
		return StartAuditResult{}, fmt.Errorf("%w: audits that render JavaScript are limited to %d pages", ErrAuditPageLimitExceeded, RenderedMaxAuditPages)
	}

	strategy := input.LighthouseStrategy
	if strategy == "" {
		strategy = LighthouseAuto
	}
	if strategy != LighthouseAuto && strategy != LighthouseNone {
		return StartAuditResult{}, fmt.Errorf("%w: unknown lighthouse strategy", ErrStartURLInvalid)
	}
	reservation := getEstimatedCapacity(maxPages, strategy)

	requestedURL, err := s.guard.NormalizeAndValidateStartURL(ctx, input.StartURL)
	if err != nil {
		return StartAuditResult{}, err
	}
	var credential *CrawlerAccess
	var credentialID string
	if s.resolver != nil {
		host := hostOf(requestedURL)
		credential, credentialID, err = s.resolver.Resolve(ctx, input.OrganizationID, input.ProjectID, host)
		if err != nil {
			return StartAuditResult{}, fmt.Errorf("resolve crawler access: %w", err)
		}
	}
	probe, err := s.guard.ResolveStartURLRedirects(ctx, requestedURL)
	if err != nil {
		return StartAuditResult{}, err
	}
	startURL := probe.URL
	if startHost := hostOf(startURL); s.resolver != nil && startHost != hostOf(requestedURL) {
		credential, credentialID, err = s.resolver.Resolve(ctx, input.OrganizationID, input.ProjectID, startHost)
		if err != nil {
			return StartAuditResult{}, fmt.Errorf("resolve crawler access: %w", err)
		}
	}

	config := Config{
		MaxPages:            maxPages,
		LighthouseStrategy:  strategy,
		RenderJavaScript:    input.RenderJavaScript,
		CrawlerCredentialID: credentialID,
	}
	if strings.Contains(strings.ToLower(probe.PoweredBy), "shopify") {
		config.SitePlatform = "shopify"
	}
	encodedConfig, err := MarshalAuditConfig(config)
	if err != nil {
		return StartAuditResult{}, fmt.Errorf("encode audit config: %w", err)
	}

	auditID := s.newID()
	if err := s.repo.CreateAudit(ctx, CreateAuditInput{
		ID: auditID, ProjectID: input.ProjectID, StartedByUserID: input.ActorUserID, StartURL: startURL,
		WorkflowInstanceID: auditID, Config: encodedConfig, PagesTotal: reservation.PagesTotal,
		LighthouseTotal: reservation.LighthouseTotal,
	}); err != nil {
		return StartAuditResult{}, err
	}

	// Concurrency and capacity are enforced after the insert, not before: a
	// pre-insert read is a check-then-act race, so parallel requests would all
	// pass the free tier's running-audits gate. Post-insert, each request sees
	// at least its own row; the losers roll back via the cleanup below.
	var renderLocks []RenderingLock
	if err := s.postInsertGate(ctx, input.OrganizationID, limits, input.RenderJavaScript, auditID, maxPages, &renderLocks); err != nil {
		s.cleanupFailedStart(ctx, auditID, input.ProjectID, renderLocks)
		return StartAuditResult{}, err
	}

	payload := JobPayload{
		AuditID: auditID, ProjectID: input.ProjectID, StartURL: startURL, Config: config,
		OrganizationID: input.OrganizationID, RenderingLocks: renderLocks,
	}
	if s.scheduler != nil {
		if err := s.scheduler.EnqueueAudit(ctx, auditID, payload); err != nil {
			s.cleanupFailedStart(ctx, auditID, input.ProjectID, renderLocks)
			return StartAuditResult{}, fmt.Errorf("schedule audit: %w", err)
		}
	}
	_ = credential
	return StartAuditResult{AuditID: auditID}, nil
}

// postInsertGate enforces the org's running/capacity ceilings and holds the
// rendering credits for a rendered audit.
func (s *Service) postInsertGate(ctx context.Context, organizationID string, limits tierLimits, renderJavaScript bool, auditID string, maxPages int, renderLocks *[]RenderingLock) error {
	usage, err := s.repo.AuditUsageForOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	if usage.RunningCount > limits.MaxRunningAudits {
		return ErrAuditAlreadyRunning
	}
	if usage.CapacityUnits > limits.MaxCapacityUnits {
		return ErrAuditCapacityReached
	}
	if renderJavaScript && s.rendering != nil {
		locks, err := s.rendering.Lock(ctx, organizationID, auditID, maxPages)
		if err != nil {
			return err
		}
		*renderLocks = locks
	}
	return nil
}

// cleanupFailedStart terminates the scheduled work, releases rendering holds and
// removes the failed audit row.
func (s *Service) cleanupFailedStart(ctx context.Context, auditID, projectID string, locks []RenderingLock) {
	if s.scheduler != nil {
		if err := s.scheduler.TerminateAudit(ctx, auditID); err != nil {
			s.logger.WarnContext(ctx, "terminate failed audit start", "audit_id", auditID, "err", err)
		}
	}
	if len(locks) > 0 && s.rendering != nil {
		if err := s.rendering.Release(ctx, locks); err != nil {
			s.logger.WarnContext(ctx, "release rendering locks", "audit_id", auditID, "err", err)
		}
	}
	if err := s.repo.DeleteAuditForProject(ctx, auditID, projectID); err != nil {
		s.logger.WarnContext(ctx, "remove failed audit row", "audit_id", auditID, "err", err)
	}
}

// StatusResult is the audit status payload.
type StatusResult struct {
	ID                  string  `json:"id"`
	StartURL            string  `json:"startUrl"`
	Status              string  `json:"status"`
	PagesCrawled        int     `json:"pagesCrawled"`
	PagesTotal          int     `json:"pagesTotal"`
	LighthouseTotal     int     `json:"lighthouseTotal"`
	LighthouseCompleted int     `json:"lighthouseCompleted"`
	LighthouseFailed    int     `json:"lighthouseFailed"`
	CurrentPhase        *string `json:"currentPhase"`
	ErrorCode           *string `json:"errorCode"`
	StartedAt           string  `json:"startedAt"`
	CompletedAt         *string `json:"completedAt"`
}

// GetStatus returns an audit's status, self-healing a stale "running" row.
func (s *Service) GetStatus(ctx context.Context, auditID, projectID string) (StatusResult, error) {
	audit, err := s.repo.GetAuditForProject(ctx, auditID, projectID)
	if err != nil {
		return StatusResult{}, err
	}
	return statusFromRecord(audit), nil
}

// statusFromRecord maps a stored audit to its status payload.
func statusFromRecord(audit Record) StatusResult {
	return StatusResult{
		ID: audit.ID, StartURL: audit.StartURL, Status: audit.Status, PagesCrawled: audit.PagesCrawled,
		PagesTotal: audit.PagesTotal, LighthouseTotal: audit.LighthouseTotal,
		LighthouseCompleted: audit.LighthouseCompleted, LighthouseFailed: audit.LighthouseFailed,
		CurrentPhase: audit.CurrentPhase, ErrorCode: audit.ErrorCode,
		StartedAt: formatISO(audit.StartedAt), CompletedAt: formatISOMillisOpt(audit.CompletedAt),
	}
}

// GetLighthouseIssues returns the issue view of one stored Lighthouse result.
func (s *Service) GetLighthouseIssues(ctx context.Context, resultID, projectID string) (LighthouseIssuesResult, error) {
	detail, err := s.repo.GetLighthouseForProject(ctx, resultID, projectID)
	if err != nil {
		return LighthouseIssuesResult{}, err
	}
	return BuildLighthouseIssues(detail)
}

// ExportLighthouse builds an export file for one stored Lighthouse result.
func (s *Service) ExportLighthouse(ctx context.Context, resultID, projectID string, mode ExportMode, category LighthouseCategory) (LighthouseExportFile, error) {
	detail, err := s.repo.GetLighthouseForProject(ctx, resultID, projectID)
	if err != nil {
		return LighthouseExportFile{}, err
	}
	return BuildLighthouseExport(detail, mode, category)
}

// GetCrawlProgress returns the live crawl feed for a running audit.
func (s *Service) GetCrawlProgress(ctx context.Context, auditID, projectID string) ([]ProgressEntry, error) {
	if _, err := s.repo.GetAuditForProject(ctx, auditID, projectID); err != nil {
		return nil, err
	}
	if s.progress == nil {
		return []ProgressEntry{}, nil
	}
	return s.progress.CrawledURLs(ctx, auditID)
}

// ResultAudit is the audit header in the results payload.
type ResultAudit struct {
	ID           string  `json:"id"`
	StartURL     string  `json:"startUrl"`
	Status       string  `json:"status"`
	PagesCrawled int     `json:"pagesCrawled"`
	PagesTotal   int     `json:"pagesTotal"`
	StartedAt    string  `json:"startedAt"`
	CompletedAt  *string `json:"completedAt"`
	Config       Config  `json:"config"`
}

// ResultsPayload is the audit results payload.
type ResultsPayload struct {
	Audit      ResultAudit        `json:"audit"`
	Pages      []PageRecord       `json:"pages"`
	Lighthouse []LighthouseRecord `json:"lighthouse"`
	Issues     []IssueRecord      `json:"issues"`
}

// GetResults returns an audit's pages, issues and Lighthouse results.
func (s *Service) GetResults(ctx context.Context, auditID, projectID string) (ResultsPayload, error) {
	results, err := s.repo.GetAuditResultsForProject(ctx, auditID, projectID)
	if err != nil {
		return ResultsPayload{}, err
	}
	config, ok := ParseAuditConfig(results.Audit.Config)
	if !ok {
		return ResultsPayload{}, ErrInvalidConfig
	}
	return ResultsPayload{
		Audit: ResultAudit{
			ID: results.Audit.ID, StartURL: results.Audit.StartURL, Status: results.Audit.Status,
			PagesCrawled: results.Audit.PagesCrawled, PagesTotal: results.Audit.PagesTotal,
			StartedAt: formatISO(results.Audit.StartedAt), CompletedAt: formatISOMillisOpt(results.Audit.CompletedAt),
			Config: config,
		},
		Pages: results.Pages, Lighthouse: results.Lighthouse, Issues: results.Issues,
	}, nil
}

// HistoryItem is one row of the audit history list.
type HistoryItem struct {
	ID            string  `json:"id"`
	StartURL      string  `json:"startUrl"`
	Status        string  `json:"status"`
	PagesCrawled  int     `json:"pagesCrawled"`
	PagesTotal    int     `json:"pagesTotal"`
	RanLighthouse bool    `json:"ranLighthouse"`
	StartedAt     string  `json:"startedAt"`
	CompletedAt   *string `json:"completedAt"`
}

// GetHistory lists a project's audits, newest first.
func (s *Service) GetHistory(ctx context.Context, projectID string) ([]HistoryItem, error) {
	audits, err := s.repo.ListAuditsByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	items := make([]HistoryItem, 0, len(audits))
	for _, audit := range audits {
		config, _ := ParseAuditConfig(audit.Config)
		items = append(items, HistoryItem{
			ID: audit.ID, StartURL: audit.StartURL, Status: audit.Status, PagesCrawled: audit.PagesCrawled,
			PagesTotal: audit.PagesTotal, RanLighthouse: config.LighthouseStrategy != LighthouseNone,
			StartedAt: formatISO(audit.StartedAt), CompletedAt: formatISOMillisOpt(audit.CompletedAt),
		})
	}
	return items, nil
}

// Remove stops and deletes an audit scoped to its project.
func (s *Service) Remove(ctx context.Context, auditID, projectID string) error {
	audit, err := s.repo.GetAuditForProject(ctx, auditID, projectID)
	if err != nil {
		return err
	}
	if audit.Status == "running" {
		if audit.WorkflowInstanceID == nil || *audit.WorkflowInstanceID == "" {
			return fmt.Errorf("%w: cannot delete a running audit without workflow context", ErrAuditRunning)
		}
		if s.scheduler != nil {
			if err := s.scheduler.TerminateAudit(ctx, auditID); err != nil {
				return fmt.Errorf("%w: unable to stop the running audit", ErrAuditRunning)
			}
		}
	}
	if err := s.repo.DeleteAuditForProject(ctx, auditID, projectID); err != nil {
		return err
	}
	if s.progress != nil {
		if err := s.progress.Clear(ctx, auditID); err != nil {
			s.logger.WarnContext(ctx, "clear crawl progress", "audit_id", auditID, "err", err)
		}
	}
	return nil
}

// CapabilitiesResult reports which optional audit features this deployment has.
type CapabilitiesResult struct {
	CanRenderJavaScript bool `json:"canRenderJavaScript"`
}

// Capabilities reports the deployment's audit capabilities.
func (s *Service) Capabilities() CapabilitiesResult {
	return CapabilitiesResult{CanRenderJavaScript: IsAuditRenderingAllowed(s.env)}
}

// formatISO renders a time the way JavaScript's Date.toISOString does.
func formatISO(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000Z")
}

// formatISOMillisOpt renders an optional time as an ISO string.
func formatISOMillisOpt(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := formatISO(*value)
	return &formatted
}

// hostOf returns the hostname of an absolute URL.
func hostOf(rawURL string) string {
	parsed, err := parseURL(rawURL, "")
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
