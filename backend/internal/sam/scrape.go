package sam

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Lightweight, dependency-free site reading for the chat agent: list a site's
// URLs from its sitemap (falling back to the homepage) and extract readable
// text from pages with a plain fetch. Enough for the model to infer what a site
// does; JavaScript-heavy sites degrade to less text.
const (
	perPageChars     = 4000
	fetchTimeout     = 10 * time.Second
	maxResponseBytes = 2_000_000
	scrapeUserAgent  = "Uplumo-Onboarding/1.0"
)

// URLGuard validates URLs against the SSRF policy and supplies an HTTP client
// that refuses private addresses at dial time. *audit.Guard implements it.
type URLGuard interface {
	NormalizeAndValidateStartURL(ctx context.Context, input string) (string, error)
	NewClient(timeout time.Duration) *http.Client
}

type scrapedPage struct {
	URL   string  `json:"url"`
	Title *string `json:"title"`
	Text  string  `json:"text"`
}

type scraper struct{ guard URLGuard }

// fetchText returns the response body, or false when the page cannot be read.
// A redirect is followed one hop, and only after its destination passes the
// same SSRF validation as the original URL.
func (s scraper) fetchText(ctx context.Context, rawURL string) (string, bool) {
	client := s.guard.NewClient(fetchTimeout)
	target := rawURL
	for hop := 0; hop < 2; hop++ {
		hopCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
		body, location, ok := fetchOnce(hopCtx, client, target)
		cancel()
		if location == "" {
			return body, ok
		}
		if hop == 1 {
			return "", false
		}
		next, err := resolveReference(target, location)
		if err != nil {
			return "", false
		}
		validated, err := s.guard.NormalizeAndValidateStartURL(ctx, next)
		if err != nil {
			return "", false
		}
		target = validated
	}
	return "", false
}

func fetchOnce(ctx context.Context, client *http.Client, target string) (body, redirect string, ok bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", "", false
	}
	req.Header.Set("User-Agent", scrapeUserAgent)
	req.Header.Set("Accept", "text/html,*/*")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", false
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			body, redirect, ok = "", "", false
		}
	}()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return "", resp.Header.Get("Location"), false
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", "", false
	}
	// Read one byte past the limit so an oversized body is rejected rather than
	// silently cut, whether or not Content-Length was sent.
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return "", "", false
	}
	return string(data), "", true
}

func resolveReference(base, ref string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return baseURL.ResolveReference(refURL).String(), nil
}

var (
	locPattern      = regexp.MustCompile(`(?i)<loc>\s*([^<\s]+)\s*</loc>`)
	titlePattern    = regexp.MustCompile(`(?is)<title[^>]*>([^<]*)</title>`)
	scriptPattern   = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	stylePattern    = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	noscriptPattern = regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</noscript>`)
	commentPattern  = regexp.MustCompile(`(?s)<!--.*?-->`)
	tagPattern      = regexp.MustCompile(`<[^>]+>`)
	spacePattern    = regexp.MustCompile(`\s+`)
)

// parseSitemapURLs keeps same-origin <loc> entries that are not nested sitemap
// files. A sitemap index is skipped rather than fetched, which is enough here;
// callers fall back to the homepage when nothing usable is found.
func parseSitemapURLs(xml, origin string) []string {
	var urls []string
	for _, match := range locPattern.FindAllStringSubmatch(xml, -1) {
		resolved, err := resolveReference(origin, match[1])
		if err != nil {
			continue
		}
		if strings.HasPrefix(resolved, origin) && !strings.HasSuffix(resolved, ".xml") {
			urls = append(urls, resolved)
		}
	}
	return urls
}

func extractTitle(page string) *string {
	match := titlePattern.FindStringSubmatch(page)
	if match == nil {
		return nil
	}
	title := html.UnescapeString(strings.TrimSpace(match[1]))
	return &title
}

// htmlToText strips scripts, styles and tags and collapses whitespace.
func htmlToText(page string) string {
	for _, pattern := range []*regexp.Regexp{scriptPattern, stylePattern, noscriptPattern, commentPattern} {
		page = pattern.ReplaceAllString(page, " ")
	}
	page = tagPattern.ReplaceAllString(page, " ")
	return html.UnescapeString(strings.TrimSpace(spacePattern.ReplaceAllString(page, " ")))
}

func (s scraper) scrapePage(ctx context.Context, pageURL string) (scrapedPage, bool) {
	body, ok := s.fetchText(ctx, pageURL)
	if !ok {
		return scrapedPage{}, false
	}
	text := []rune(htmlToText(body))
	if len(text) == 0 {
		return scrapedPage{}, false
	}
	if len(text) > perPageChars {
		text = text[:perPageChars]
	}
	return scrapedPage{URL: pageURL, Title: extractTitle(body), Text: string(text)}, true
}

// readPages reads specific URLs as plain text. Each URL is validated on its
// own, so a blocked or unreachable one is skipped instead of failing the batch.
func (s scraper) readPages(ctx context.Context, urls []string, limit int) (pages []scrapedPage, blocked bool) {
	for _, raw := range urls[:min(len(urls), limit)] {
		validated, err := s.guard.NormalizeAndValidateStartURL(ctx, raw)
		if err != nil {
			continue
		}
		if page, ok := s.scrapePage(ctx, validated); ok {
			pages = append(pages, page)
		}
	}
	return pages, len(pages) == 0
}

// mapLinks lists a site's homepage plus what its sitemap declares, up to limit.
func (s scraper) mapLinks(ctx context.Context, domain string, limit int) (urls []string, blocked bool) {
	root, err := s.guard.NormalizeAndValidateStartURL(ctx, domain)
	if err != nil {
		return nil, true
	}
	parsed, err := url.Parse(root)
	if err != nil {
		return nil, true
	}
	origin := parsed.Scheme + "://" + parsed.Host
	urls = []string{root}
	if sitemap, ok := s.fetchText(ctx, origin+"/sitemap.xml"); ok {
		for _, u := range parseSitemapURLs(sitemap, origin) {
			// The homepage appears with and without a trailing slash.
			if strings.TrimSuffix(u, "/") != strings.TrimSuffix(root, "/") {
				urls = append(urls, u)
			}
		}
	}
	return urls[:min(len(urls), limit)], false
}

// readSite maps a site and reads its first pages.
func (s scraper) readSite(ctx context.Context, domain string, limit int) (pages []scrapedPage, blocked bool) {
	urls, blocked := s.mapLinks(ctx, domain, limit)
	if blocked {
		return nil, true
	}
	return s.readPages(ctx, urls, limit)
}
