package audit

import (
	"context"
	"log/slog"
	"time"
)

// maxCrawlDepth bounds link-following depth.
const maxCrawlDepth = 6

// Runner executes one audit job end to end: discovery, crawl, cross-page
// checks, Lighthouse, then finalize. It is idempotent: a retry re-derives
// deterministic row ids and upserts, so a partially written audit completes
// instead of duplicating.
type Runner struct {
	repo       Store
	progress   *Progress
	guard      *Guard
	crawler    *Crawler
	lighthouse LighthouseProvider
	rendering  RenderingMeter
	logger     *slog.Logger
	now        func() time.Time
}

// RunnerConfig configures a Runner.
type RunnerConfig struct {
	Repository Store
	Progress   *Progress
	Guard      *Guard
	Crawler    *Crawler
	Lighthouse LighthouseProvider
	Rendering  RenderingMeter
	Logger     *slog.Logger
	Now        func() time.Time
}

// NewRunner builds a Runner.
func NewRunner(config RunnerConfig) *Runner {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	guard := config.Guard
	if guard == nil {
		guard = NewGuard()
	}
	crawler := config.Crawler
	if crawler == nil {
		crawler = NewCrawler(CrawlerOptions{Guard: guard})
	}
	return &Runner{
		repo: config.Repository, progress: config.Progress, guard: guard, crawler: crawler,
		lighthouse: config.Lighthouse, rendering: config.Rendering, logger: logger, now: now,
	}
}

// auditDeadline bounds one audit's crawl.
const auditDeadline = 30 * time.Minute

// Run executes the audit. A returned error means the audit failed and was
// marked failed; a nil error means it completed (possibly with partial data).
func (r *Runner) Run(ctx context.Context, job JobPayload) error {
	runCtx, cancel := context.WithTimeout(ctx, auditDeadline)
	defer cancel()

	if err := r.setPhase(runCtx, job, "discovery"); err != nil {
		return err
	}
	if err := r.run(runCtx, job); err != nil {
		r.failAudit(ctx, job, err)
		return err
	}
	return nil
}

// run performs the phases and completes the audit.
func (r *Runner) run(ctx context.Context, job JobPayload) error {
	maxPages := job.Config.MaxPages
	if maxPages <= 0 {
		maxPages = DefaultAuditPages
	}
	origin := getOrigin(job.StartURL)
	if origin == "" {
		return ErrStartURLInvalid
	}

	discoverer := discoverer{guard: r.guard, client: r.guard.NewClient(0), now: r.now}
	seedURLs, robotsText := discoverer.DiscoverURLs(ctx, origin, maxPages, nil)
	robots := parseRobotsTxt(origin, robotsText)

	pagesCrawled, err := r.crawl(ctx, job, origin, seedURLs, robots, maxPages)
	if err != nil {
		return err
	}

	if err := r.finalizeChecks(ctx, job); err != nil {
		return err
	}
	if err := r.lighthousePhase(ctx, job); err != nil {
		return err
	}
	if err := r.repo.CompleteAudit(ctx, job.AuditID, job.AuditID, pagesCrawled, maxPages); err != nil {
		return err
	}
	if r.progress != nil {
		_ = r.progress.Clear(ctx, job.AuditID)
	}
	if r.rendering != nil && len(job.RenderingLocks) > 0 {
		_ = r.rendering.Settle(ctx, job.OrganizationID, job.AuditID, job.RenderingLocks, RenderUsage{})
	}
	return nil
}

// crawlQueueItem is one URL waiting to be fetched.
type crawlQueueItem struct {
	url   string
	depth int
}

// crawl runs the BFS crawl, persisting each sub-batch as it lands.
func (r *Runner) crawl(ctx context.Context, job JobPayload, _ string, seedURLs []string, robots RobotsResult, maxPages int) (int, error) {
	startNormalized, ok := normalizeURL(job.StartURL, "")
	if !ok {
		return 0, ErrStartURLInvalid
	}
	visited := map[string]struct{}{}
	queue := []crawlQueueItem{{url: startNormalized, depth: 0}}
	for _, seed := range seedURLs {
		if normalized, ok := normalizeURL(seed, ""); ok {
			queue = append(queue, crawlQueueItem{url: normalized})
		}
	}

	throttle := NewCrawlThrottle(r.now().Add(auditDeadline), nil, nil)
	window := CrawlWindow.Initial
	pagesCrawled := 0
	stoppedEarly := false

	for len(queue) > 0 && pagesCrawled < maxPages {
		if ctx.Err() != nil {
			return pagesCrawled, ctx.Err()
		}
		batchSize := min(window, len(queue))
		batchSize = min(batchSize, maxPages-pagesCrawled)
		batch := queue[:batchSize]
		queue = queue[batchSize:]

		ready, err := throttle.Ready(ctx)
		if err != nil {
			return pagesCrawled, err
		}
		if !ready {
			stoppedEarly = true
			break
		}

		results := make([]CrawlResult, len(batch))
		for i, item := range batch {
			if _, seen := visited[item.url]; seen {
				continue
			}
			visited[item.url] = struct{}{}
			results[i] = r.fetchWithRetry(ctx, throttle, item.url)
		}

		pages := make([]CrawledPageResult, 0, len(batch))
		issues := make([]DetectedIssue, 0, len(batch))
		progressEntries := make([]ProgressEntry, 0, len(batch))
		for i, item := range batch {
			result := results[i]
			if result.URL == "" {
				continue // already visited
			}
			page := result.CrawledPageResult
			page.ID = DeterministicAuditRowID(job.AuditID, page.URL)
			depth := item.depth
			if item.depth != 0 || i != 0 {
				page.CrawlDepth = &depth
			}
			page.HTMLBytes = result.HTMLBytesRead
			pages = append(pages, page)
			issues = append(issues, runPageReporters(page)...)
			progressEntries = append(progressEntries, ProgressEntry{
				URL: page.URL, StatusCode: page.StatusCode, Title: page.Title, CrawledAt: r.now().UnixMilli(),
			})
		}

		if err := r.repo.InsertCrawledBatch(ctx, job.AuditID, pages, issues); err != nil {
			return pagesCrawled, err
		}
		pagesCrawled += len(pages)
		if r.progress != nil && len(progressEntries) > 0 {
			if err := r.progress.PushCrawledURLs(ctx, job.AuditID, progressEntries); err != nil {
				r.logger.WarnContext(ctx, "push crawl progress", "audit_id", job.AuditID, "err", err)
			}
		}
		crawled := pagesCrawled
		if err := r.repo.UpdateAuditProgress(ctx, job.AuditID, job.AuditID, ProgressUpdate{
			PagesCrawled: &crawled, CurrentPhase: phasePtr("crawling"),
		}); err != nil {
			return pagesCrawled, err
		}

		window = AdjustCrawlWindow(window, pages, CrawlWindow)

		// Enqueue discovered internal links.
		for _, page := range pages {
			if page.CrawlDepth != nil && *page.CrawlDepth >= maxCrawlDepth {
				continue
			}
			for _, link := range page.Links {
				if !link.IsInternal {
					continue
				}
				next, ok := normalizeURL(link.TargetURL, "")
				if !ok {
					continue
				}
				if _, seen := visited[next]; seen {
					continue
				}
				if !isCrawlableURL(next) || !robots.IsAllowed(next) {
					continue
				}
				depth := 0
				if page.CrawlDepth != nil {
					depth = *page.CrawlDepth + 1
				} else {
					depth = 1
				}
				queue = append(queue, crawlQueueItem{url: next, depth: depth})
			}
		}
	}

	if stoppedEarly || throttle.Stopped() {
		issue := DetectedIssue{IssueType: IssueCrawlRateLimited, PageURL: job.StartURL}
		if err := r.repo.InsertIssues(ctx, job.AuditID, []DetectedIssue{issue}); err != nil {
			return pagesCrawled, err
		}
	}
	return pagesCrawled, nil
}

// fetchWithRetry fetches a URL, retrying a 429 while the throttle allows.
func (r *Runner) fetchWithRetry(ctx context.Context, throttle *CrawlThrottle, rawURL string) CrawlResult {
	attempt := 0
	for {
		result := r.crawler.FetchPage(ctx, rawURL)
		if result.FetchClass != FetchRateLimited {
			if result.FetchClass != FetchError {
				_ = throttle.Recovered(ctx)
			}
			result.RateLimited = attempt > 0
			return result
		}
		retryAfter := ""
		allowed, err := throttle.Backoff(ctx, attempt, retryAfter)
		if err != nil || !allowed {
			result.RateLimited = true
			return result
		}
		ready, err := throttle.Ready(ctx)
		if err != nil || !ready {
			result.RateLimited = true
			return result
		}
		attempt++
		result.RateLimited = true
	}
}

// finalizeChecks runs the cross-page checks and persists their issues.
func (r *Runner) finalizeChecks(ctx context.Context, job JobPayload) error {
	issues, _, err := r.repo.RunMultipageChecks(ctx, job.AuditID)
	if err != nil {
		return err
	}
	if len(issues) > 0 {
		if err := r.repo.InsertIssues(ctx, job.AuditID, issues); err != nil {
			return err
		}
	}
	return nil
}

// lighthousePhase runs the sampled Lighthouse checks.
func (r *Runner) lighthousePhase(ctx context.Context, job JobPayload) error {
	if job.Config.LighthouseStrategy == LighthouseNone || r.lighthouse == nil {
		return nil
	}
	if err := r.setPhase(ctx, job, "lighthouse"); err != nil {
		return err
	}
	pages, err := r.repo.ListSlimPages(ctx, job.AuditID)
	if err != nil {
		return err
	}
	samples := make([]LighthouseSamplePage, 0, len(pages))
	for _, page := range pages {
		status := 0
		if page.StatusCode != nil {
			status = *page.StatusCode
		}
		samples = append(samples, LighthouseSamplePage{URL: page.URL, StatusCode: status, FetchClass: page.FetchClass})
	}
	selected := SelectLighthouseSample(samples, job.StartURL, job.Config.LighthouseStrategy)
	results := make([]LighthouseRecord, 0, len(selected)*2)
	completed, failed := 0, 0
	for _, pageURL := range selected {
		pageID := DeterministicAuditRowID(job.AuditID, pageURL)
		for _, strategy := range []string{"mobile", "desktop"} {
			fetched := FetchLighthouseResult(ctx, r.lighthouse, r.logger, job.OrganizationID, pageURL, pageID, strategy)
			record := LighthouseRecord{
				PageID: pageID, URL: pageURL, Strategy: strategy,
				PerformanceScore:   intFromFloat(fetched.Result.PerformanceScore),
				AccessibilityScore: intFromFloat(fetched.Result.AccessibilityScore),
				BestPracticesScore: intFromFloat(fetched.Result.BestPracticesScore),
				SeoScore:           intFromFloat(fetched.Result.SeoScore),
				LcpMs:              fetched.Result.LcpMs, CLS: fetched.Result.CLS, InpMs: fetched.Result.InpMs, TtfbMs: fetched.Result.TtfbMs,
				ErrorMessage: fetched.Result.ErrorMessage,
			}
			if fetched.PayloadJSON != "" {
				payload := fetched.PayloadJSON
				record.PayloadJSON = &payload
			}
			results = append(results, record)
			if fetched.Result.ErrorMessage == nil {
				completed++
			} else {
				failed++
			}
		}
	}
	if err := r.repo.InsertLighthouseResults(ctx, job.AuditID, results); err != nil {
		return err
	}
	return r.repo.UpdateAuditProgress(ctx, job.AuditID, job.AuditID, ProgressUpdate{
		LighthouseCompleted: &completed, LighthouseFailed: &failed, CurrentPhase: phasePtr("finalizing"),
	})
}

// setPhase records the audit's current phase.
func (r *Runner) setPhase(ctx context.Context, job JobPayload, phase string) error {
	return r.repo.UpdateAuditProgress(ctx, job.AuditID, job.AuditID, ProgressUpdate{CurrentPhase: &phase})
}

// failAudit marks the audit failed, classifying the error.
func (r *Runner) failAudit(ctx context.Context, job JobPayload, cause error) {
	failCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	info := ClassifyAuditError(cause)
	if err := r.repo.FailAudit(failCtx, job.AuditID, job.AuditID, info, ""); err != nil {
		r.logger.ErrorContext(failCtx, "mark audit failed", "audit_id", job.AuditID, "err", err)
	}
	if r.rendering != nil && len(job.RenderingLocks) > 0 {
		if err := r.rendering.Release(failCtx, job.RenderingLocks); err != nil {
			r.logger.WarnContext(failCtx, "release rendering locks", "audit_id", job.AuditID, "err", err)
		}
	}
	if r.progress != nil {
		_ = r.progress.Clear(failCtx, job.AuditID)
	}
}

// phasePtr returns a pointer to a phase string.
func phasePtr(phase string) *string { return &phase }

// intFromFloat rounds an optional float score to an optional int.
func intFromFloat(value *float64) *int {
	if value == nil {
		return nil
	}
	result := int(*value + 0.5)
	return &result
}
