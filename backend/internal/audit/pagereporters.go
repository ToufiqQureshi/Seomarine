package audit

// DetectedIssue is one issue found on a page. Cross-page checks (duplicates,
// redirect chains) also produce these.
type DetectedIssue struct {
	IssueType AuditIssueType `json:"issueType"`
	// PageID is nil for issues that are not tied to a page.
	PageID  *string        `json:"pageId"`
	PageURL string         `json:"pageUrl"`
	Details map[string]any `json:"details,omitempty"`
	// DedupeKey distinguishes multiple issues of the same type on the same page
	// (e.g. one broken-internal-link issue per target). It is part of the
	// deterministic row id, so step retries do not duplicate issues.
	DedupeKey string `json:"dedupeKey,omitempty"`
}

const (
	titleMaxChars           = 60
	titleMinChars           = 10
	metaDescriptionMaxChars = 160
	metaDescriptionMinChars = 70
	thinContentWords        = 150
	slowResponseReportMs    = 1500
	deepPageDepth           = 5
)

// runPageReporters runs the pure per-page checks over one crawled page and
// returns the issues found. Mirrors runPageReporters.
func runPageReporters(page CrawledPageResult) []DetectedIssue {
	issues := make([]DetectedIssue, 0, 6)
	report := func(issueType AuditIssueType, details map[string]any) {
		pageID := page.ID
		issues = append(issues, DetectedIssue{IssueType: issueType, PageID: &pageID, PageURL: page.URL, Details: details})
	}

	switch page.FetchClass {
	case FetchBlocked:
		report(IssueBlockedPage, map[string]any{"statusCode": page.StatusCode})
		return issues
	case FetchRateLimited:
		report(IssueRateLimitedPage, map[string]any{"statusCode": page.StatusCode})
		return issues
	case FetchError:
		return issues
	}

	if page.StatusCode >= 500 {
		report(IssueServerError, map[string]any{"statusCode": page.StatusCode})
		return issues
	}
	if page.StatusCode >= 400 {
		report(IssueBrokenPage, map[string]any{"statusCode": page.StatusCode})
		return issues
	}
	// Redirects are normal on their own; chains and loops are flagged in the
	// cross-page checks.
	if page.StatusCode >= 300 {
		return issues
	}

	if page.ResponseTimeMs > slowResponseReportMs {
		report(IssueSlowResponse, map[string]any{"responseTimeMs": page.ResponseTimeMs})
	}

	// Content checks only make sense for analyzed HTML documents (a PDF has no
	// title tag to miss).
	if !page.IsHtml {
		return issues
	}

	if !page.IsIndexable {
		report(IssueNoindexPage, map[string]any{"robotsMeta": nullable(page.RobotsMeta), "xRobotsTag": nullable(page.XRobotsTag)})
	}
	if page.CanonicalURL != "" && page.HeaderCanonicalURL != "" && page.CanonicalURL != page.HeaderCanonicalURL {
		report(IssueCanonicalConflict, map[string]any{"htmlCanonical": page.CanonicalURL, "headerCanonical": page.HeaderCanonicalURL})
	}
	effectiveCanonical := page.CanonicalURL
	if effectiveCanonical == "" {
		effectiveCanonical = page.HeaderCanonicalURL
	}
	if effectiveCanonical != "" && effectiveCanonical != page.URL {
		report(IssueCanonicalizedPage, map[string]any{"canonicalUrl": effectiveCanonical})
	}

	if page.CrawlDepth != nil && *page.CrawlDepth >= deepPageDepth {
		report(IssueDeepPage, map[string]any{"crawlDepth": *page.CrawlDepth})
	}

	// The initial app shell cannot establish what the rendered page is missing,
	// so report the coverage gap instead of thin-content/missing-heading claims.
	if page.JavaScriptShell {
		report(IssueJavaScriptRenderingSuspected, map[string]any{"wordCount": page.WordCount})
		return issues
	}

	switch {
	case page.Title == "":
		report(IssueMissingTitle, nil)
	case len(page.Title) > titleMaxChars:
		report(IssueTitleTooLong, map[string]any{"length": len(page.Title)})
	case len(page.Title) < titleMinChars:
		report(IssueTitleTooShort, map[string]any{"length": len(page.Title)})
	}

	switch {
	case page.MetaDescription == "":
		report(IssueMissingMetaDescription, nil)
	case len(page.MetaDescription) > metaDescriptionMaxChars:
		report(IssueMetaDescriptionTooLong, map[string]any{"length": len(page.MetaDescription)})
	case len(page.MetaDescription) < metaDescriptionMinChars:
		report(IssueMetaDescriptionTooShort, map[string]any{"length": len(page.MetaDescription)})
	}

	switch {
	case page.H1Count == 0:
		report(IssueMissingH1, nil)
	case page.H1Count > 1:
		report(IssueMultipleH1, map[string]any{"h1Count": page.H1Count})
	}
	if hasHeadingLevelSkip(page.HeadingOrder) {
		report(IssueHeadingOrderSkip, nil)
	}

	if page.IsIndexable && page.WordCount < thinContentWords {
		report(IssueThinContent, map[string]any{"wordCount": page.WordCount})
	}
	if page.ImagesMissingAlt > 0 {
		report(IssueImagesMissingAlt, map[string]any{"imagesMissingAlt": page.ImagesMissingAlt, "imagesTotal": page.ImagesTotal})
	}

	if page.IsIndexable && len(page.Links) == 0 {
		report(IssueNoOutgoingLinks, nil)
	}

	return issues
}

// hasHeadingLevelSkip reports whether the heading order skips a level.
func hasHeadingLevelSkip(headingOrder []int) bool {
	for i := 1; i < len(headingOrder); i++ {
		if headingOrder[i] > headingOrder[i-1]+1 {
			return true
		}
	}
	return false
}

// nullable returns nil for an empty string so the JSON details omit it, mirroring
// the TypeScript's nullable detail values.
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
