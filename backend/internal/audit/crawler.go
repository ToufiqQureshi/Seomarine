package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Crawler fetches and analyzes pages for one audit, honoring robots.txt,
// politeness and memory limits.
type Crawler struct {
	guard  *Guard
	client *http.Client
	access *CrawlerAccess
	// maxHTMLBytes bounds the HTML read per page.
	maxHTMLBytes int
}

// CrawlerOptions configures a Crawler. Zero values select safe defaults.
type CrawlerOptions struct {
	Guard        *Guard
	Client       *http.Client
	Access       *CrawlerAccess
	MaxHTMLBytes int
}

// NewCrawler builds a crawler. A nil guard uses the default SSRF guard.
func NewCrawler(options CrawlerOptions) *Crawler {
	guard := options.Guard
	if guard == nil {
		guard = NewGuard()
	}
	client := options.Client
	if client == nil {
		client = guard.NewClient(30 * time.Second)
	}
	maxHTMLBytes := options.MaxHTMLBytes
	if maxHTMLBytes <= 0 {
		maxHTMLBytes = MaxHTMLBytes
	}
	return &Crawler{guard: guard, client: client, access: options.Access, maxHTMLBytes: maxHTMLBytes}
}

// CrawlResult is one page's fetch outcome.
type CrawlResult struct {
	// Page is the analyzed page. Only URL, StatusCode, FetchClass and the
	// timing/height signals are set for non-HTML responses.
	CrawledPageResult
	// HTMLBytesRead is how many bytes of HTML were read (approximate).
	HTMLBytesRead int
}

// FetchPage fetches and analyzes one page. A non-2xx status is still analyzed
// when the body is HTML, so the report can explain it. The returned result is
// always usable: a transport failure becomes an error-class page.
func (c *Crawler) FetchPage(ctx context.Context, rawURL string) CrawlResult {
	started := time.Now()
	result := CrawlResult{CrawledPageResult: CrawledPageResult{URL: rawURL, FetchClass: FetchError}}

	if !isCrawlableURL(rawURL) {
		result.HTMLBytesRead = 0
		return result
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return result
	}
	request.Header.Set("User-Agent", auditUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	for key, value := range CrawlerHeadersFor(rawURL, c.access) {
		request.Header.Set(key, value)
	}
	// The crawler follows redirects one hop at a time so it can record the
	// redirect target and classify the final response.
	response, err := c.client.Do(request)
	if err != nil {
		result.ResponseTimeMs = time.Since(started).Milliseconds()
		return result
	}
	defer func() { _ = response.Body.Close() }()

	result.ResponseTimeMs = time.Since(started).Milliseconds()
	result.StatusCode = response.StatusCode
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		if location := response.Header.Get("Location"); location != "" {
			base, baseErr := url.Parse(rawURL)
			if baseErr == nil {
				if next, nextErr := url.Parse(location); nextErr == nil {
					result.RedirectURL = base.ResolveReference(next).String()
				}
			}
		}
	}

	body, err := ReadTextUpTo(response, c.maxHTMLBytes)
	if err != nil && body == "" {
		return result
	}
	result.HTMLBytesRead = len(body)
	mitigated := strings.TrimSpace(response.Header.Get("Cf-Mitigated")) != ""
	result.FetchClass = classifyFetch(response.StatusCode, mitigated, prefixSnippet(body, 4_000))

	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	isHTML := strings.Contains(contentType, "html") || looksLikeHTML(body)
	if !isHTML {
		result.IsHtml = false
		return result
	}
	result.IsHtml = true

	analysis := AnalyzeHTML(body, rawURL, response.StatusCode, result.ResponseTimeMs, result.RedirectURL)
	applyAnalysis(&result.CrawledPageResult, analysis, response.Header)
	return result
}

// applyAnalysis folds a PageAnalysis and the response headers into a page row.
func applyAnalysis(page *CrawledPageResult, analysis PageAnalysis, headers http.Header) {
	page.Title = analysis.Title
	page.MetaDescription = analysis.MetaDescription
	page.CanonicalURL = analysis.Canonical
	page.RobotsMeta = analysis.RobotsMeta
	page.HeaderCanonicalURL = parseHeaderCanonical(headers)
	page.XRobotsTag = strings.TrimSpace(headers.Get("X-Robots-Tag"))
	page.OgTitle = analysis.OgTitle
	page.OgDescription = analysis.OgDescription
	page.OgImage = analysis.OgImage
	page.HeadingOrder = analysis.HeadingOrder
	page.WordCount = analysis.WordCount
	page.Images = analysis.Images
	page.Links = analysis.Links
	page.HasStructuredData = analysis.HasStructuredData
	page.HreflangTags = analysis.HreflangTags
	page.JavaScriptShell = analysis.JavaScriptShell
	page.IsIndexable = computeIsIndexable(analysis.RobotsMeta, page.XRobotsTag)

	for _, heading := range analysis.HeadingOrder {
		switch heading {
		case 1:
			page.H1Count++
		case 2:
			page.H2Count++
		case 3:
			page.H3Count++
		case 4:
			page.H4Count++
		case 5:
			page.H5Count++
		case 6:
			page.H6Count++
		}
	}
	page.ImagesTotal = len(analysis.Images)
	missingAlt := 0
	for _, image := range analysis.Images {
		if image.Alt == "" {
			missingAlt++
		}
	}
	page.ImagesMissingAlt = missingAlt
	page.ContentHash = contentHash(analysis.BodyText)
}

// computeIsIndexable reports whether the page allows indexing, from the robots
// meta tag and the X-Robots-Tag header. An unset pair is indexable.
func computeIsIndexable(robotsMeta, xRobotsTag string) bool {
	for _, directives := range []string{robotsMeta, xRobotsTag} {
		for _, token := range strings.FieldsFunc(strings.ToLower(directives), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			if token == "noindex" || token == "none" {
				return false
			}
		}
	}
	return true
}

// parseHeaderCanonical reads an RFC 8288 Link header's rel=canonical target.
func parseHeaderCanonical(headers http.Header) string {
	for _, value := range headers.Values("Link") {
		for _, part := range strings.Split(value, ",") {
			lower := strings.ToLower(part)
			if !strings.Contains(lower, `rel="canonical"`) && !strings.Contains(lower, "rel=canonical") {
				continue
			}
			start := strings.Index(part, "<")
			end := strings.Index(part, ">")
			if start >= 0 && end > start {
				return strings.TrimSpace(part[start+1 : end])
			}
		}
	}
	return ""
}

// contentHash is the SHA-256 of a page's visible text, used for duplicate
// detection. Empty text yields an empty hash.
func contentHash(bodyText string) string {
	if strings.TrimSpace(bodyText) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(bodyText))
	return hex.EncodeToString(sum[:])
}

// looksLikeHTML is a conservative sniff for a body served without a useful
// content type.
func looksLikeHTML(body string) bool {
	trimmed := strings.ToLower(strings.TrimLeft(body, " \t\r\n"))
	return strings.HasPrefix(trimmed, "<!doctype html") || strings.HasPrefix(trimmed, "<html")
}

// prefixSnippet returns at most maxBytes of body.
func prefixSnippet(body string, maxBytes int) string {
	if len(body) <= maxBytes {
		return body
	}
	return body[:maxBytes]
}

// CrawlPageURLs fetches a batch of URLs concurrently, bounded by the crawl
// window, and returns the results in input order.
func (c *Crawler) CrawlPageURLs(ctx context.Context, urls []string, window int) ([]CrawlResult, error) {
	if window < 1 {
		window = 1
	}
	results := make([]CrawlResult, len(urls))
	semaphore := make(chan struct{}, window)
	var waitGroup sync.WaitGroup
	for index, rawURL := range urls {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		semaphore <- struct{}{}
		waitGroup.Go(func() {
			defer func() { <-semaphore }()
			if ctx.Err() != nil {
				return
			}
			results[index] = c.FetchPage(ctx, rawURL)
		})
	}
	waitGroup.Wait()
	if err := ctx.Err(); err != nil {
		return results, err
	}
	return results, nil
}
