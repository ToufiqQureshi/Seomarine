package audit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"
)

const (
	sitemapFetchTimeout = 15 * time.Second
	// maxRobotsTxtBytes keeps a misbehaving server (e.g. HTML at /robots.txt)
	// from blowing a checkpoint. RFC 9309 requires parsers to handle at least
	// 500 KiB and permits ignoring anything beyond it.
	maxRobotsTxtBytes = 500 * 1024
	// maxSitemapDepth caps sitemap-index recursion by URL depth.
	maxSitemapDepth = 3
	// maxSitemapDocs caps how many sitemap documents one discovery fetches.
	maxSitemapDocs = 300
	// sitemapConcurrency is how many sitemap shards are read at once.
	sitemapConcurrency = 5
	// sitemapRetries is the number of retries after a timeout.
	sitemapRetries = 1
	// maxDiscoveryRedirectHops bounds the hand-followed redirect chain.
	maxDiscoveryRedirectHops = 5
)

// discoverer fetches robots.txt and sitemaps for one audit origin.
type discoverer struct {
	guard  *Guard
	client *http.Client
	access *CrawlerAccess
	now    func() time.Time
}

// discovered is one sitemap document's contribution.
type discoveredSitemap struct {
	nested   []string
	pages    []string
	timedOut bool
}

// fetchRobotsTxtText fetches the raw robots.txt body (nil = missing or
// unreachable). Kept separate from parsing so a workflow can checkpoint the text
// and re-derive the parsed result deterministically on replay.
func (d discoverer) fetchRobotsTxtText(ctx context.Context, origin string) *string {
	fetched, err := d.fetchFollowingRedirects(ctx, origin+"/robots.txt", 10*time.Second)
	if err != nil || fetched == nil || fetched.resp.StatusCode < 200 || fetched.resp.StatusCode >= 300 {
		if fetched != nil {
			_ = fetched.resp.Body.Close()
		}
		return nil
	}
	body, err := ReadTextUpTo(fetched.resp, maxRobotsTxtBytes)
	_ = fetched.resp.Body.Close()
	if err != nil {
		return nil
	}
	return &body
}

// fetchedResponse is a response whose body is still open for the caller.
type fetchedResponse struct {
	resp     *http.Response
	finalURL string
}

// fetchFollowingRedirects follows redirects by hand so every hop is
// revalidated against the crawl policy and crawler-access headers are re-matched
// against the hop's host instead of riding along to another site.
func (d discoverer) fetchFollowingRedirects(ctx context.Context, rawURL string, timeout time.Duration) (*fetchedResponse, error) {
	deadline := d.now().Add(timeout)
	current := rawURL
	for hop := 0; hop <= maxDiscoveryRedirectHops; hop++ {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			remaining = time.Millisecond
		}
		hopCtx, cancel := context.WithTimeout(ctx, remaining)
		request, err := http.NewRequestWithContext(hopCtx, http.MethodGet, current, nil)
		if err != nil {
			cancel()
			return nil, err
		}
		request.Header.Set("User-Agent", auditUserAgent)
		for key, value := range CrawlerHeadersFor(current, d.access) {
			request.Header.Set(key, value)
		}
		response, err := d.client.Do(request)
		if err != nil {
			cancel()
			if current != rawURL {
				// A redirect hop stole the deadline; treat as a hard failure.
				return nil, err
			}
			return nil, err
		}
		// The body of an intermediate redirect is discarded; the response body
		// for the final hop stays open for the caller, so its read must not be
		// governed by the per-hop context.
		if response.StatusCode < 300 || response.StatusCode >= 400 {
			response.Body = &detachedBody{ReadCloser: response.Body, cancel: cancel}
			return &fetchedResponse{resp: response, finalURL: current}, nil
		}
		location := response.Header.Get("Location")
		cancel()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
		if location == "" {
			return nil, nil
		}
		next, err := resolveLocation(location, current)
		if err != nil {
			return nil, nil
		}
		if !isCrawlableURL(next) {
			return nil, nil
		}
		current = next
	}
	return nil, nil
}

// detachedBody keeps an HTTP response body readable after its request context is
// cancelled by the next redirect hop.
type detachedBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *detachedBody) Close() error {
	err := b.ReadCloser.Close()
	if b.cancel != nil {
		b.cancel()
	}
	return err
}

// fetchSitemapDocumentWithRetry fetches and parses one sitemap document.
func (d discoverer) fetchSitemapDocumentWithRetry(ctx context.Context, sitemapURL string) discoveredSitemap {
	normalized, ok := normalizeURL(sitemapURL, "")
	if !ok {
		return discoveredSitemap{}
	}
	var lastTimeout bool
	for attempt := 0; attempt <= sitemapRetries; attempt++ {
		result, retryable, timedOut := d.fetchSitemapDocument(ctx, normalized)
		lastTimeout = timedOut
		if !retryable {
			return result
		}
	}
	return discoveredSitemap{timedOut: lastTimeout}
}

// fetchSitemapDocument performs a single sitemap fetch; retryable=true means the
// attempt timed out and should be retried.
func (d discoverer) fetchSitemapDocument(ctx context.Context, normalized string) (discoveredSitemap, bool, bool) {
	fetched, err := d.fetchFollowingRedirects(ctx, normalized, sitemapFetchTimeout)
	if err != nil || fetched == nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return discoveredSitemap{}, true, true
		}
		return discoveredSitemap{}, false, false
	}
	defer func() { _ = fetched.resp.Body.Close() }()

	finalURL, ok := normalizeURL(fetched.finalURL, normalized)
	if !ok || !isSameOrigin(finalURL, normalized) {
		return discoveredSitemap{}, false, false
	}
	if fetched.resp.StatusCode < 200 || fetched.resp.StatusCode >= 300 {
		return discoveredSitemap{}, false, false
	}
	body, withinCap := readBodyCapped(fetched.resp, maxSitemapBytes)
	if !withinCap || !isProbablySitemapXML(fetched.resp.Header.Get("Content-Type"), body) {
		return discoveredSitemap{}, false, false
	}
	sections := parseSitemapXML(body)
	result := discoveredSitemap{}
	for _, loc := range sections.Sitemaps {
		if resolved, ok := normalizeURL(loc, finalURL); ok {
			result.nested = append(result.nested, resolved)
		}
	}
	for _, loc := range sections.URLs {
		if resolved, ok := normalizeURL(loc, finalURL); ok {
			result.pages = append(result.pages, resolved)
		}
	}
	return result, false, false
}

// DiscoverURLs discovers page URLs from robots.txt and sitemaps for an origin,
// also trying /sitemap.xml when robots.txt does not list it. It returns at most
// maxPages seeds (the crawl can never use more) plus the raw robots.txt body.
func (d discoverer) DiscoverURLs(ctx context.Context, origin string, maxPages int, access *CrawlerAccess) ([]string, *string) {
	d.access = access
	robotsText := d.fetchRobotsTxtText(ctx, origin)
	robots := parseRobotsTxt(origin, robotsText)

	sitemapSources := map[string]struct{}{origin + "/sitemap.xml": {}}
	for _, sitemapURL := range robots.SitemapURLs {
		sitemapSources[sitemapURL] = struct{}{}
	}

	maxDiscoveredURLs := min(max(maxPages*20, 500), 50_000)
	type queued struct {
		url   string
		depth int
	}
	queue := make([]queued, 0, len(sitemapSources))
	for rawURL := range sitemapSources {
		normalized, ok := normalizeURL(rawURL, origin)
		if !ok || !isSameOrigin(normalized, origin) {
			continue
		}
		queue = append(queue, queued{url: normalized, depth: maxSitemapDepth})
	}

	var mu sync.Mutex
	allURLs := map[string]struct{}{}
	seenDocs := map[string]struct{}{}
	fetchedDocs := 0

	for len(queue) > 0 {
		mu.Lock()
		enough := len(allURLs) >= maxDiscoveredURLs
		mu.Unlock()
		if enough || fetchedDocs >= maxSitemapDocs {
			break
		}
		batchSize := min(sitemapConcurrency, len(queue))
		batch := queue[:batchSize]
		queue = queue[batchSize:]

		var waitGroup sync.WaitGroup
		var nestedMu sync.Mutex
		for _, item := range batch {
			waitGroup.Go(func() {
				normalized, ok := normalizeURL(item.url, "")
				if !ok || !isSameOrigin(normalized, origin) || item.depth <= 0 {
					return
				}
				mu.Lock()
				if _, seen := seenDocs[normalized]; seen {
					mu.Unlock()
					return
				}
				seenDocs[normalized] = struct{}{}
				fetchedDocs++
				mu.Unlock()

				result := d.fetchSitemapDocumentWithRetry(ctx, normalized)
				if len(result.pages) == 0 && len(result.nested) == 0 {
					return
				}

				mu.Lock()
				for _, pageURL := range result.pages {
					if !isSameOrigin(pageURL, origin) {
						continue
					}
					if len(allURLs) >= maxDiscoveredURLs {
						break
					}
					allURLs[pageURL] = struct{}{}
				}
				mu.Unlock()

				if item.depth <= 1 {
					return
				}
				nestedMu.Lock()
				defer nestedMu.Unlock()
				for _, nestedURL := range result.nested {
					if !isSameOrigin(nestedURL, origin) {
						continue
					}
					mu.Lock()
					_, seen := seenDocs[nestedURL]
					mu.Unlock()
					if !seen {
						queue = append(queue, queued{url: nestedURL, depth: item.depth - 1})
					}
				}
			})
		}
		waitGroup.Wait()
	}

	urls := make([]string, 0, len(allURLs))
	for url := range allURLs {
		urls = append(urls, url)
	}
	slices.Sort(urls)
	if len(urls) > maxPages {
		urls = urls[:maxPages]
	}
	return urls, robotsText
}

// resolveLocation resolves a redirect Location header against its request URL.
func resolveLocation(location, base string) (string, error) {
	parsedLocation, err := parseURL(location, base)
	if err != nil {
		return "", err
	}
	return parsedLocation.String(), nil
}
