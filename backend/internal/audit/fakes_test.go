package audit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// failNetworkClient always fails, so a start-URL probe falls back to the
// requested URL without touching the network.
func failNetworkClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network disabled in tests")
	})}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// memoryStore is an in-memory Store for service, handler and runner tests.
type memoryStore struct {
	mu             sync.Mutex
	audits         map[string]AuditRecord
	projectOf      map[string]string
	pages          map[string][]CrawledPageResult
	issues         map[string][]DetectedIssue
	lighthouse     map[string][]LighthouseRecord
	failCreate     bool
	failInsert     bool
	completedCalls int
	failedCalls    int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		audits:     map[string]AuditRecord{},
		projectOf:  map[string]string{},
		pages:      map[string][]CrawledPageResult{},
		issues:     map[string][]DetectedIssue{},
		lighthouse: map[string][]LighthouseRecord{},
	}
}

func (m *memoryStore) CreateAudit(_ context.Context, input CreateAuditInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failCreate {
		return errors.New("create failed")
	}
	instance := input.WorkflowInstanceID
	m.audits[input.ID] = AuditRecord{
		ID: input.ID, ProjectID: input.ProjectID, StartedByUserID: input.StartedByUserID, StartURL: input.StartURL,
		Status: "running", WorkflowInstanceID: &instance, Config: input.Config, PagesTotal: input.PagesTotal,
		LighthouseTotal: input.LighthouseTotal, StartedAt: time.Unix(0, 0),
	}
	m.projectOf[input.ID] = input.ProjectID
	return nil
}

func (m *memoryStore) UpdateAuditProgress(_ context.Context, auditID, workflowInstanceID string, update ProgressUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	audit, ok := m.audits[auditID]
	if !ok || audit.WorkflowInstanceID == nil || *audit.WorkflowInstanceID != workflowInstanceID {
		return fmt.Errorf("%w: %s", ErrAuditNotFound, auditID)
	}
	if update.PagesCrawled != nil {
		audit.PagesCrawled = *update.PagesCrawled
	}
	if update.PagesTotal != nil {
		audit.PagesTotal = *update.PagesTotal
	}
	if update.LighthouseTotal != nil {
		audit.LighthouseTotal = *update.LighthouseTotal
	}
	if update.LighthouseCompleted != nil {
		audit.LighthouseCompleted = *update.LighthouseCompleted
	}
	if update.LighthouseFailed != nil {
		audit.LighthouseFailed = *update.LighthouseFailed
	}
	if update.CurrentPhase != nil {
		audit.CurrentPhase = update.CurrentPhase
	}
	m.audits[auditID] = audit
	return nil
}

func (m *memoryStore) CompleteAudit(_ context.Context, auditID, workflowInstanceID string, pagesCrawled, pagesTotal int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	audit, ok := m.audits[auditID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrAuditNotFound, auditID)
	}
	completed := time.Unix(1, 0)
	audit.Status = "completed"
	audit.PagesCrawled = pagesCrawled
	audit.PagesTotal = pagesTotal
	audit.CompletedAt = &completed
	m.audits[auditID] = audit
	m.completedCalls++
	return nil
}

func (m *memoryStore) FailAudit(_ context.Context, auditID, workflowInstanceID string, info AuditErrorInfo, failedPhase string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	audit, ok := m.audits[auditID]
	if !ok || audit.Status != "running" {
		return fmt.Errorf("%w: %s", ErrAuditNotFound, auditID)
	}
	code := string(info.ErrorCode)
	audit.Status = "failed"
	audit.ErrorCode = &code
	m.audits[auditID] = audit
	m.failedCalls++
	return nil
}

func (m *memoryStore) GetAuditForProject(_ context.Context, auditID, projectID string) (AuditRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	audit, ok := m.audits[auditID]
	if !ok || m.projectOf[auditID] != projectID {
		return AuditRecord{}, fmt.Errorf("%w: %s", ErrAuditNotFound, auditID)
	}
	return audit, nil
}

func (m *memoryStore) GetAuditResultsForProject(ctx context.Context, auditID, projectID string) (Results, error) {
	audit, err := m.GetAuditForProject(ctx, auditID, projectID)
	if err != nil {
		return Results{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return Results{
		Audit: audit, Pages: []PageRecord{}, Issues: issuesToRecords(auditID, m.issues[auditID]),
		Lighthouse: m.lighthouse[auditID],
	}, nil
}

func issuesToRecords(auditID string, issues []DetectedIssue) []IssueRecord {
	records := []IssueRecord{}
	for _, issue := range issues {
		records = append(records, IssueRecord{
			ID:     DeterministicAuditRowID(auditID, issue.PageURL, string(issue.IssueType), issue.DedupeKey),
			PageID: issue.PageID, PageURL: issue.PageURL, IssueType: string(issue.IssueType),
			Severity: severityOf(issue.IssueType), Details: issue.Details,
		})
	}
	return records
}

func (m *memoryStore) ListAuditsByProject(_ context.Context, projectID string) ([]AuditRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	records := []AuditRecord{}
	for id, audit := range m.audits {
		if m.projectOf[id] == projectID {
			records = append(records, audit)
		}
	}
	return records, nil
}

func (m *memoryStore) AuditUsageForOrganization(context.Context, string) (AuditUsage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	usage := AuditUsage{}
	for _, audit := range m.audits {
		usage.CapacityUnits += audit.PagesTotal + audit.LighthouseTotal
		if audit.Status == "running" {
			usage.RunningCount++
		}
	}
	return usage, nil
}

func (m *memoryStore) DeleteAuditForProject(_ context.Context, auditID, projectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.audits[auditID]; !ok || m.projectOf[auditID] != projectID {
		return fmt.Errorf("%w: %s", ErrAuditNotFound, auditID)
	}
	delete(m.audits, auditID)
	delete(m.projectOf, auditID)
	delete(m.pages, auditID)
	delete(m.issues, auditID)
	delete(m.lighthouse, auditID)
	return nil
}

func (m *memoryStore) InsertCrawledBatch(_ context.Context, auditID string, pages []CrawledPageResult, issues []DetectedIssue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failInsert {
		return errors.New("insert failed")
	}
	m.pages[auditID] = append(m.pages[auditID], pages...)
	m.issues[auditID] = append(m.issues[auditID], issues...)
	return nil
}

func (m *memoryStore) InsertIssues(_ context.Context, auditID string, issues []DetectedIssue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.issues[auditID] = append(m.issues[auditID], issues...)
	return nil
}

func (m *memoryStore) InsertLighthouseResults(_ context.Context, auditID string, results []LighthouseRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lighthouse[auditID] = append(m.lighthouse[auditID], results...)
	return nil
}

func (m *memoryStore) GetLighthouseForProject(_ context.Context, resultID, projectID string) (LighthouseDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for auditID, records := range m.lighthouse {
		if m.projectOf[auditID] != projectID {
			continue
		}
		for _, record := range records {
			if record.ID == resultID && record.PayloadJSON != nil {
				return LighthouseDetail{
					ID: record.ID, Strategy: record.Strategy, PageURL: record.URL,
					StartedAt: m.audits[auditID].StartedAt, PayloadJSON: *record.PayloadJSON,
				}, nil
			}
		}
	}
	return LighthouseDetail{}, ErrLighthouseNotFound
}

func (m *memoryStore) ListSlimPages(_ context.Context, auditID string) ([]SlimPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	slim := []SlimPage{}
	for _, page := range m.pages[auditID] {
		status := page.StatusCode
		slim = append(slim, SlimPage{
			ID: page.ID, URL: page.URL, StatusCode: &status, FetchClass: page.FetchClass,
			Title: page.Title, MetaDescription: page.MetaDescription, ContentHash: page.ContentHash,
			RedirectURL: page.RedirectURL, WordCount: page.WordCount, IsIndexable: page.IsIndexable,
			CanonicalURL: page.CanonicalURL, HeaderCanonicalURL: page.HeaderCanonicalURL,
		})
	}
	return slim, nil
}

func (m *memoryStore) ShellPageIDs(_ context.Context, auditID string) (map[string]struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := map[string]struct{}{}
	for _, issue := range m.issues[auditID] {
		if issue.IssueType == IssueJavaScriptRenderingSuspected && issue.PageID != nil {
			ids[*issue.PageID] = struct{}{}
		}
	}
	return ids, nil
}

func (m *memoryStore) RunMultipageChecks(ctx context.Context, auditID string) ([]DetectedIssue, bool, error) {
	shellIDs, err := m.ShellPageIDs(ctx, auditID)
	if err != nil {
		return nil, false, err
	}
	pages, err := m.ListSlimPages(ctx, auditID)
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

func (m *memoryStore) CountPagesByFetchClass(_ context.Context, auditID string, class PageFetchClass) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, page := range m.pages[auditID] {
		if page.FetchClass == class {
			count++
		}
	}
	return count, nil
}

// recordingScheduler records enqueued and terminated audits.
type recordingScheduler struct {
	enqueued    []AuditJobPayload
	terminated  []string
	failEnqueue bool
	enqueueErr  error
}

func (s *recordingScheduler) EnqueueAudit(_ context.Context, _ string, payload AuditJobPayload) error {
	if s.failEnqueue {
		if s.enqueueErr != nil {
			return s.enqueueErr
		}
		return errors.New("enqueue failed")
	}
	s.enqueued = append(s.enqueued, payload)
	return nil
}

func (s *recordingScheduler) TerminateAudit(_ context.Context, auditID string) error {
	s.terminated = append(s.terminated, auditID)
	return nil
}

// fakePlans and fakeAccess drive the tier resolution tests.
type fakePlans struct {
	paid bool
	err  error
}

func (f fakePlans) HasPaidPlan(context.Context, string) (bool, error) { return f.paid, f.err }

type fakeAccess struct {
	allowed bool
	err     error
}

func (f fakeAccess) HasManagedAccess(context.Context, string) (bool, error) {
	return f.allowed, f.err
}

// fakeRenderingMeter records lock/release/settle calls.
type fakeRenderingMeter struct {
	locked       int
	released     int
	settled      int
	lockErr      error
	lastMaxPages int
}

func (f *fakeRenderingMeter) Lock(_ context.Context, _, _ string, maxPages int) ([]RenderingLock, error) {
	if f.lockErr != nil {
		return nil, f.lockErr
	}
	f.locked++
	f.lastMaxPages = maxPages
	return []RenderingLock{{LockID: "lock-1", FeatureID: "usage_credits", EstimatedCredits: 100}}, nil
}

func (f *fakeRenderingMeter) Release(context.Context, []RenderingLock) error {
	f.released++
	return nil
}

func (f *fakeRenderingMeter) Settle(context.Context, string, string, []RenderingLock, RenderUsage) error {
	f.settled++
	return nil
}

// fakeLighthouseProvider returns a canned payload.
type fakeLighthouseProvider struct {
	calls int
	err   error
}

func (f *fakeLighthouseProvider) FetchLighthouse(_ context.Context, _ string, input LighthouseInput) (StoredLighthousePayload, error) {
	f.calls++
	if f.err != nil {
		return StoredLighthousePayload{}, f.err
	}
	score := 90
	return StoredLighthousePayload{
		Version: 2, Source: "dataforseo-lighthouse",
		Metadata: StoredLighthouseMetadata{RequestedURL: input.URL, FinalURL: input.URL, Strategy: input.Strategy, FetchedAt: "2024-01-01T00:00:00.000Z"},
		Scores:   StoredLighthouseScores{Performance: &score},
	}, nil
}
