package audit

import (
	"strings"
	"testing"
)

// okPage returns a healthy HTML page that produces no issues.
func okPage() CrawledPageResult {
	return CrawledPageResult{
		ID: "p1", URL: "https://example.com/page", StatusCode: 200, FetchClass: FetchOK,
		Title: "A perfectly reasonable title", MetaDescription: strings.Repeat("d", 100),
		WordCount: 500, IsIndexable: true, IsHtml: true, ResponseTimeMs: 100,
		Links:   []PageLink{{TargetURL: "https://example.com/a", IsInternal: true}},
		H1Count: 1, HeadingOrder: []int{1, 2},
	}
}

// issueTypes returns the set of issue types in a result.
func issueTypes(issues []DetectedIssue) map[AuditIssueType]bool {
	set := map[AuditIssueType]bool{}
	for _, issue := range issues {
		set[issue.IssueType] = true
	}
	return set
}

func TestRunPageReportersHealthyPage(t *testing.T) {
	if issues := runPageReporters(okPage()); len(issues) != 0 {
		t.Fatalf("healthy page produced %v", issues)
	}
}

func TestRunPageReportersFetchClassesShortCircuit(t *testing.T) {
	blocked := okPage()
	blocked.FetchClass = FetchBlocked
	blocked.StatusCode = 403
	issues := runPageReporters(blocked)
	if len(issues) != 1 || issues[0].IssueType != IssueBlockedPage {
		t.Fatalf("blocked issues = %v", issues)
	}

	limited := okPage()
	limited.FetchClass = FetchRateLimited
	limited.StatusCode = 429
	issues = runPageReporters(limited)
	if len(issues) != 1 || issues[0].IssueType != IssueRateLimitedPage {
		t.Fatalf("rate limited issues = %v", issues)
	}

	failed := okPage()
	failed.FetchClass = FetchError
	if issues := runPageReporters(failed); len(issues) != 0 {
		t.Fatalf("transport error produced %v", issues)
	}
}

func TestRunPageReportersStatusCodes(t *testing.T) {
	serverError := okPage()
	serverError.StatusCode = 503
	issues := runPageReporters(serverError)
	if len(issues) != 1 || issues[0].IssueType != IssueServerError {
		t.Fatalf("server error issues = %v", issues)
	}

	broken := okPage()
	broken.StatusCode = 404
	issues = runPageReporters(broken)
	if len(issues) != 1 || issues[0].IssueType != IssueBrokenPage {
		t.Fatalf("broken issues = %v", issues)
	}

	redirect := okPage()
	redirect.StatusCode = 301
	if issues := runPageReporters(redirect); len(issues) != 0 {
		t.Fatalf("redirect produced %v", issues)
	}
}

func TestRunPageReportersContentChecks(t *testing.T) {
	page := okPage()
	page.Title = ""
	page.MetaDescription = ""
	page.H1Count = 2
	page.HeadingOrder = []int{1, 1, 4}
	page.WordCount = 10
	page.ImagesMissingAlt = 2
	page.ImagesTotal = 3
	page.ResponseTimeMs = 4000
	page.Links = nil
	page.CanonicalURL = "https://example.com/other"
	page.HeaderCanonicalURL = "https://example.com/header"
	depth := 7
	page.CrawlDepth = &depth

	set := issueTypes(runPageReporters(page))
	for _, want := range []AuditIssueType{
		IssueMissingTitle, IssueMissingMetaDescription, IssueMultipleH1, IssueHeadingOrderSkip,
		IssueThinContent, IssueImagesMissingAlt, IssueSlowResponse,
		IssueNoOutgoingLinks, IssueCanonicalConflict, IssueCanonicalizedPage, IssueDeepPage,
	} {
		if !set[want] {
			t.Errorf("missing issue %s", want)
		}
	}
}

func TestRunPageReportersNoindex(t *testing.T) {
	page := okPage()
	page.IsIndexable = false
	page.RobotsMeta = "noindex"
	set := issueTypes(runPageReporters(page))
	if !set[IssueNoindexPage] {
		t.Fatalf("expected noindex-page, got %v", set)
	}
	// A noindex page is not flagged for thin content or dead ends.
	page.WordCount = 10
	page.Links = nil
	set = issueTypes(runPageReporters(page))
	if set[IssueThinContent] || set[IssueNoOutgoingLinks] {
		t.Fatalf("noindex page should skip indexable-only checks: %v", set)
	}
}

func TestRunPageReportersJavaScriptShellSkipsContentChecks(t *testing.T) {
	page := okPage()
	page.JavaScriptShell = true
	page.WordCount = 3
	set := issueTypes(runPageReporters(page))
	if !set[IssueJavaScriptRenderingSuspected] {
		t.Fatalf("expected javascript-rendering-suspected, got %v", set)
	}
	if set[IssueThinContent] || set[IssueMissingH1] {
		t.Fatalf("content checks must be skipped for a shell: %v", set)
	}
}

func TestRunPageReportersNonHTMLSkipsContentChecks(t *testing.T) {
	page := okPage()
	page.IsHtml = false
	page.Title = ""
	set := issueTypes(runPageReporters(page))
	if set[IssueMissingTitle] {
		t.Fatalf("a non-HTML page must not report content issues: %v", set)
	}
}

func TestRunPageReportersTitleLengths(t *testing.T) {
	long := okPage()
	long.Title = strings.Repeat("t", 70)
	if set := issueTypes(runPageReporters(long)); !set[IssueTitleTooLong] {
		t.Fatal("expected title-too-long")
	}
	short := okPage()
	short.Title = "tiny"
	if set := issueTypes(runPageReporters(short)); !set[IssueTitleTooShort] {
		t.Fatal("expected title-too-short")
	}
}

func TestHasHeadingLevelSkip(t *testing.T) {
	if !hasHeadingLevelSkip([]int{1, 3}) {
		t.Fatal("expected a skip")
	}
	if hasHeadingLevelSkip([]int{1, 2, 3}) {
		t.Fatal("no skip expected")
	}
	if hasHeadingLevelSkip(nil) {
		t.Fatal("empty order has no skip")
	}
}

func TestFindDuplicates(t *testing.T) {
	status := 200
	title := "Shared Title"
	pages := []SlimPage{
		{ID: "a", URL: "https://example.com/a", StatusCode: &status, FetchClass: FetchOK, Title: title, IsIndexable: true, WordCount: 10, ContentHash: "hash"},
		{ID: "b", URL: "https://example.com/b", StatusCode: &status, FetchClass: FetchOK, Title: title, IsIndexable: true, WordCount: 10, ContentHash: "hash"},
		{ID: "c", URL: "https://example.com/c", StatusCode: &status, FetchClass: FetchOK, Title: "Unique", IsIndexable: true, WordCount: 10, ContentHash: "other"},
	}
	issues := FindDuplicates(pages)
	counts := map[AuditIssueType]int{}
	for _, issue := range issues {
		counts[issue.IssueType]++
	}
	if counts[IssueDuplicateTitle] != 2 {
		t.Fatalf("duplicate title count = %d", counts[IssueDuplicateTitle])
	}
	if counts[IssueDuplicateContent] != 2 {
		t.Fatalf("duplicate content count = %d", counts[IssueDuplicateContent])
	}
}

func TestFindDuplicatesExcludesNoindexAndCanonicalized(t *testing.T) {
	status := 200
	title := "Shared"
	pages := []SlimPage{
		{ID: "a", URL: "https://example.com/a", StatusCode: &status, FetchClass: FetchOK, Title: title, IsIndexable: true},
		{ID: "b", URL: "https://example.com/b", StatusCode: &status, FetchClass: FetchOK, Title: title, IsIndexable: false},
		{ID: "c", URL: "https://example.com/c", StatusCode: &status, FetchClass: FetchOK, Title: title, IsIndexable: true, CanonicalURL: "https://example.com/other"},
	}
	if issues := FindDuplicates(pages); len(issues) != 0 {
		t.Fatalf("expected no duplicates, got %v", issues)
	}
}

func TestFindRedirectChainsAndLoops(t *testing.T) {
	status := func(code int) *int { return &code }
	pages := []SlimPage{
		{ID: "a", URL: "https://example.com/a", StatusCode: status(301), FetchClass: FetchOK, RedirectURL: "https://example.com/b"},
		{ID: "b", URL: "https://example.com/b", StatusCode: status(302), FetchClass: FetchOK, RedirectURL: "https://example.com/c"},
		{ID: "c", URL: "https://example.com/c", StatusCode: status(200), FetchClass: FetchOK},
		{ID: "x", URL: "https://example.com/x", StatusCode: status(301), FetchClass: FetchOK, RedirectURL: "https://example.com/y"},
		{ID: "y", URL: "https://example.com/y", StatusCode: status(301), FetchClass: FetchOK, RedirectURL: "https://example.com/x"},
	}
	set := map[AuditIssueType]int{}
	for _, issue := range FindRedirectChainsAndLoops(pages) {
		set[issue.IssueType]++
	}
	if set[IssueRedirectChain] != 1 {
		t.Errorf("chain count = %d, want 1", set[IssueRedirectChain])
	}
	if set[IssueRedirectLoop] != 1 {
		t.Errorf("loop count = %d, want 1", set[IssueRedirectLoop])
	}
}

func TestFindRedirectSelfLoop(t *testing.T) {
	status := 301
	pages := []SlimPage{{ID: "a", URL: "https://example.com/a", StatusCode: &status, FetchClass: FetchOK, RedirectURL: "https://example.com/a"}}
	issues := FindRedirectChainsAndLoops(pages)
	if len(issues) != 1 || issues[0].IssueType != IssueRedirectLoop {
		t.Fatalf("issues = %v", issues)
	}
}

func TestIssueSeverityDefaultsToWarning(t *testing.T) {
	if severityOf("unknown-type") != SeverityWarning {
		t.Fatal("unknown issue types should default to warning")
	}
	if severityOf(IssueMissingTitle) != SeverityCritical {
		t.Fatal("missing-title should be critical")
	}
	if _, ok := getIssueDescriptor(IssueDeepPage); !ok {
		t.Fatal("deep-page descriptor missing")
	}
}
