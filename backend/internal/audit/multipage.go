package audit

// SlimPage is the subset of a persisted page row the cross-page checks need.
type SlimPage struct {
	ID                 string
	URL                string
	StatusCode         *int
	FetchClass         PageFetchClass
	Title              string
	MetaDescription    string
	ContentHash        string
	RedirectURL        string
	WordCount          int
	IsIndexable        bool
	CanonicalURL       string
	HeaderCanonicalURL string
}

// duplicateGroupSample caps the other URLs listed per duplicate issue.
const duplicateGroupSample = 3

// isOKHTMLPage reports whether a page is a successful HTML document.
func isOKHTMLPage(page SlimPage) bool {
	return page.FetchClass == FetchOK &&
		page.StatusCode != nil &&
		*page.StatusCode >= 200 &&
		*page.StatusCode < 300
}

// isDuplicateCandidate reports whether a page can join a duplicate group. Pages
// the owner already de-duplicated (noindex, or canonicalized to another URL) are
// excluded, because flagging them tells the user to fix something they fixed.
func isDuplicateCandidate(page SlimPage) bool {
	if !isOKHTMLPage(page) || !page.IsIndexable {
		return false
	}
	effectiveCanonical := page.CanonicalURL
	if effectiveCanonical == "" {
		effectiveCanonical = page.HeaderCanonicalURL
	}
	return effectiveCanonical == "" || effectiveCanonical == page.URL
}

// FindDuplicates groups pages by title, meta description and content hash and
// emits one issue per page in every group of two or more. Mirrors findDuplicates.
func FindDuplicates(pages []SlimPage) []DetectedIssue {
	candidates := make([]SlimPage, 0, len(pages))
	for _, page := range pages {
		if isDuplicateCandidate(page) {
			candidates = append(candidates, page)
		}
	}

	groupBy := func(keyOf func(SlimPage) string) map[string][]SlimPage {
		groups := map[string][]SlimPage{}
		for _, page := range candidates {
			key := keyOf(page)
			if key == "" {
				continue
			}
			groups[key] = append(groups[key], page)
		}
		return groups
	}

	issues := make([]DetectedIssue, 0)
	emitGroups := func(groups map[string][]SlimPage, issueType AuditIssueType) {
		for _, group := range groups {
			if len(group) < 2 {
				continue
			}
			for _, page := range group {
				others := make([]string, 0, duplicateGroupSample)
				for _, other := range group {
					if other.ID == page.ID {
						continue
					}
					others = append(others, other.URL)
					if len(others) == duplicateGroupSample {
						break
					}
				}
				pageID := page.ID
				issues = append(issues, DetectedIssue{
					IssueType: issueType,
					PageID:    &pageID,
					PageURL:   page.URL,
					Details:   map[string]any{"groupSize": len(group), "otherUrls": others},
				})
			}
		}
	}

	emitGroups(groupBy(func(page SlimPage) string { return page.Title }), IssueDuplicateTitle)
	emitGroups(groupBy(func(page SlimPage) string { return page.MetaDescription }), IssueDuplicateMetaDescription)
	emitGroups(groupBy(func(page SlimPage) string {
		if page.WordCount > 0 {
			return page.ContentHash
		}
		return ""
	}), IssueDuplicateContent)
	return issues
}

// FindRedirectChainsAndLoops walks the redirect graph and emits a chain issue
// for two or more hops and a loop issue for a cycle. Mirrors
// findRedirectChainsAndLoops.
func FindRedirectChainsAndLoops(pages []SlimPage) []DetectedIssue {
	redirects := map[string]SlimPage{}
	order := make([]string, 0)
	for _, page := range pages {
		isRedirect := page.StatusCode != nil && *page.StatusCode >= 300 && *page.StatusCode < 400 && page.RedirectURL != ""
		if isRedirect {
			if _, exists := redirects[page.URL]; !exists {
				order = append(order, page.URL)
			}
			redirects[page.URL] = page
		}
	}
	redirectTargets := map[string]struct{}{}
	for _, page := range redirects {
		redirectTargets[page.RedirectURL] = struct{}{}
	}

	issues := make([]DetectedIssue, 0)
	walked := map[string]struct{}{}

	// Walk from chain heads (redirects nothing else redirects to), so a 5-hop
	// chain yields one issue, not five.
	for _, url := range order {
		if _, ok := redirectTargets[url]; ok {
			continue
		}
		head := redirects[url]
		hops := []string{url}
		seen := map[string]struct{}{url: {}}
		walked[url] = struct{}{}
		current := head.RedirectURL
		isLoop := false
		for current != "" {
			if _, ok := seen[current]; ok {
				isLoop = true
				hops = append(hops, current)
				break
			}
			hops = append(hops, current)
			seen[current] = struct{}{}
			if _, ok := redirects[current]; ok {
				walked[current] = struct{}{}
			}
			next, ok := redirects[current]
			if !ok {
				current = ""
			} else {
				current = next.RedirectURL
			}
		}

		pageID := head.ID
		switch {
		case isLoop:
			issues = append(issues, DetectedIssue{IssueType: IssueRedirectLoop, PageID: &pageID, PageURL: url, Details: map[string]any{"hops": hops}})
		case len(hops) > 2:
			// url -> a -> b: two redirects before content is a chain.
			issues = append(issues, DetectedIssue{IssueType: IssueRedirectChain, PageID: &pageID, PageURL: url, Details: map[string]any{"hops": hops, "finalUrl": hops[len(hops)-1]}})
		}
	}

	// Headless cycles (every member is also a target, e.g. a<->b, or a->a) are
	// never reached from a head; emit one loop issue per cycle.
	for _, url := range order {
		if _, ok := walked[url]; ok {
			continue
		}
		page := redirects[url]
		cycle := []string{}
		current := url
		for current != "" {
			if _, ok := walked[current]; ok {
				break
			}
			walked[current] = struct{}{}
			cycle = append(cycle, current)
			next, ok := redirects[current]
			if !ok {
				break
			}
			current = next.RedirectURL
		}
		pageID := page.ID
		issues = append(issues, DetectedIssue{
			IssueType: IssueRedirectLoop,
			PageID:    &pageID,
			PageURL:   url,
			Details:   map[string]any{"hops": append(cycle, url)},
		})
	}
	return issues
}
