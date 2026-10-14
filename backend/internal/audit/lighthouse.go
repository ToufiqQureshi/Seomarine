package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// pageSpeedEndpoint is Google's PageSpeed Insights v5 API. It runs Lighthouse
// on Google's machines, so a check costs nothing but a share of the project's
// daily quota (25,000 queries a day across every workspace).
const pageSpeedEndpoint = "https://pagespeedonline.googleapis.com/pagespeedonline/v5/runPagespeed"

const (
	// pageSpeedTimeout bounds one check. Lighthouse itself gives up on a page
	// after about a minute, so this only cuts off a hung connection.
	pageSpeedTimeout = 90 * time.Second
	// pageSpeedMaxResponseBytes bounds the response read into memory. A full
	// report with screenshots is a few megabytes.
	pageSpeedMaxResponseBytes = 20 << 20
	// PageSpeedDailyChecksPerOrganization caps one workspace's checks a day
	// (UTC), so one busy workspace cannot spend the quota every other
	// workspace shares. It is 10 full audits (10 pages, mobile and desktop).
	PageSpeedDailyChecksPerOrganization = 200
)

// lighthouseLoadErrorPrefix starts the message of a check where Lighthouse
// could not load the customer's page. That is the customer's site, not ours.
const lighthouseLoadErrorPrefix = "Lighthouse encountered an error with the following code:"

// ErrPageSpeedDailyLimit is a check refused because the workspace used its
// daily share. The message is shown on the audit.
var ErrPageSpeedDailyLimit = fmt.Errorf("this workspace ran %d page speed checks today; the limit resets at midnight UTC", PageSpeedDailyChecksPerOrganization)

// lighthouseRequestCategories are the categories requested from the provider.
var lighthouseRequestCategories = []string{"PERFORMANCE", "ACCESSIBILITY", "BEST_PRACTICES", "SEO"}

// LighthouseProvider fetches a Lighthouse report for one URL and strategy.
type LighthouseProvider interface {
	// FetchLighthouse calls the provider and returns the compact stored payload.
	FetchLighthouse(ctx context.Context, organizationID string, input LighthouseInput) (StoredLighthousePayload, error)
}

// PageSpeedQuota counts one workspace's checks against its daily share.
// Implementations fail closed: an error means no check is sent.
type PageSpeedQuota interface {
	Allow(ctx context.Context, organizationID string) (bool, error)
}

// RedisPageSpeedQuota allows PageSpeedDailyChecksPerOrganization checks per
// organization per UTC day, shared across server instances.
type RedisPageSpeedQuota struct{ Client *redis.Client }

// Allow reports whether the organization may run another check today.
func (q RedisPageSpeedQuota) Allow(ctx context.Context, organizationID string) (bool, error) {
	if q.Client == nil {
		return false, errors.New("page speed quota is not configured")
	}
	key := "audit:pagespeed:" + organizationID + ":" + time.Now().UTC().Format("20060102")
	count, err := q.Client.Eval(ctx, `local count = redis.call('INCR', KEYS[1]); if count == 1 then redis.call('EXPIRE', KEYS[1], 172800) end; return count`, []string{key}).Int64()
	if err != nil {
		return false, fmt.Errorf("count page speed checks: %w", err)
	}
	return count <= PageSpeedDailyChecksPerOrganization, nil
}

// pageSpeedProvider implements LighthouseProvider over PageSpeed Insights.
type pageSpeedProvider struct {
	apiKey   string
	endpoint string
	client   *http.Client
	quota    PageSpeedQuota
	now      func() time.Time
	// retryDelay is the wait before the one retry of a 429 or 503.
	retryDelay time.Duration
}

// NewPageSpeedProvider builds a provider that sends apiKey with every check.
func NewPageSpeedProvider(apiKey string, quota PageSpeedQuota) LighthouseProvider {
	return &pageSpeedProvider{apiKey: apiKey, endpoint: pageSpeedEndpoint, client: &http.Client{}, quota: quota, now: time.Now, retryDelay: 5 * time.Second}
}

// FetchLighthouse runs one check. The key travels in a header, never in the
// URL, because a transport error repeats the URL and that text is stored on
// the audit for the customer to read.
func (p *pageSpeedProvider) FetchLighthouse(ctx context.Context, organizationID string, input LighthouseInput) (StoredLighthousePayload, error) {
	if p == nil || p.apiKey == "" || p.quota == nil {
		return StoredLighthousePayload{}, errors.New("lighthouse provider is not configured")
	}
	query := url.Values{"url": {input.URL}, "strategy": {strings.ToUpper(input.Strategy)}, "category": lighthouseRequestCategories}
	if err := p.reserve(ctx, organizationID); err != nil {
		return StoredLighthousePayload{}, err
	}
	status, body, err := p.call(ctx, query)
	// A 429 (per-minute quota, when several audits run at once) or a 503 is
	// usually gone a few seconds later, so it is tried once more. The retry
	// is a request like any other and needs its own slot in the workspace's
	// daily share; without one it is not sent. Nothing else is retried: a 500
	// is the customer's page failing to load, and repeating it only spends
	// quota.
	if err == nil && (status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable) {
		select {
		case <-ctx.Done():
			return StoredLighthousePayload{}, fmt.Errorf("call PageSpeed Insights: %w", ctx.Err())
		case <-time.After(p.retryDelay):
		}
		if p.reserve(ctx, organizationID) == nil {
			status, body, err = p.call(ctx, query)
		}
	}
	if err != nil {
		return StoredLighthousePayload{}, err
	}
	if status != http.StatusOK {
		return StoredLighthousePayload{}, pageSpeedError(status, body)
	}
	return ParsePageSpeedPayload(body, input, p.now())
}

// reserve takes one slot of the workspace's daily share for one request. The
// Redis counter is incremented atomically, so concurrent checks cannot both
// take the last slot.
func (p *pageSpeedProvider) reserve(ctx context.Context, organizationID string) error {
	allowed, err := p.quota.Allow(ctx, organizationID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPageSpeedDailyLimit
	}
	return nil
}

// call sends one request and returns the status and the (bounded) body.
func (p *pageSpeedProvider) call(ctx context.Context, query url.Values) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, pageSpeedTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return 0, nil, fmt.Errorf("build PageSpeed Insights request: %w", err)
	}
	request.Header.Set("X-Goog-Api-Key", p.apiKey)
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return 0, nil, fmt.Errorf("call PageSpeed Insights: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, pageSpeedMaxResponseBytes))
	if err != nil {
		return 0, nil, fmt.Errorf("read PageSpeed Insights response: %w", err)
	}
	return response.StatusCode, body, nil
}

// pageSpeedError turns an API error into a message for the audit. Google
// answers a page that would not load with a 500 whose message names the
// Lighthouse error code; that is reported as a load error.
func pageSpeedError(status int, body []byte) error {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	message := envelope.Error.Message
	if before, rest, ok := strings.Cut(message, "Lighthouse returned error: "); ok && before == "" {
		return fmt.Errorf("%s %s", lighthouseLoadErrorPrefix, rest)
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return fmt.Errorf("PageSpeed Insights answered %d: %s", status, message)
}

// LighthouseSamplePage is the page subset Lighthouse sampling needs.
type LighthouseSamplePage struct {
	URL        string
	StatusCode int
	FetchClass PageFetchClass
}

// canonicalURLKeyWithoutTrailingSlash is canonicalURLKey with a trailing slash
// removed from a non-root path.
func canonicalURLKeyWithoutTrailingSlash(raw string) string {
	key := canonicalURLKey(raw)
	parsed, err := parseURL(key, "")
	if err != nil {
		return key
	}
	if parsed.Path != "/" {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}
	return serialize(parsed)
}

// SelectLighthouseSample chooses which pages to check, based on the strategy:
// the homepage plus one page per URL template, capped at ten, or the homepage
// alone. Mirrors selectLighthouseSample.
func SelectLighthouseSample(pages []LighthouseSamplePage, startURL string, strategy LighthouseStrategy) []string {
	if strategy == LighthouseNone {
		return []string{}
	}
	validPages := make([]LighthouseSamplePage, 0, len(pages))
	for _, page := range pages {
		// A bot challenge can answer 200; a paid check of it measures the
		// challenge, so only successful fetches qualify.
		if page.StatusCode >= 200 && page.StatusCode < 300 && page.FetchClass == FetchOK {
			validPages = append(validPages, page)
		}
	}

	selected := []string{}
	selectedSet := map[string]struct{}{}
	add := func(url string) {
		if _, ok := selectedSet[url]; ok {
			return
		}
		selectedSet[url] = struct{}{}
		selected = append(selected, url)
	}

	// Always include the start URL / homepage. Prefer an exact canonical match
	// so distinct 2xx /path and /path/ pages stay distinct, then tolerate a
	// trailing-slash redirect when the exact start URL was not crawled as 2xx.
	startKey := canonicalURLKey(startURL)
	startPage := -1
	for i, page := range validPages {
		if canonicalURLKey(page.URL) == startKey {
			startPage = i
			break
		}
	}
	if startPage < 0 {
		startKeyNoSlash := canonicalURLKeyWithoutTrailingSlash(startURL)
		for i, page := range validPages {
			if canonicalURLKeyWithoutTrailingSlash(page.URL) == startKeyNoSlash {
				startPage = i
				break
			}
		}
	}

	// Group by URL template pattern, preserving first-appearance order.
	templateGroups := []LighthouseSamplePage{}
	templateSeen := map[string]struct{}{}
	if strategy == LighthouseHomepage {
		if startPage < 0 {
			return selected
		}
		return []string{validPages[startPage].URL}
	}
	if startPage >= 0 {
		add(validPages[startPage].URL)
		template := templateForURL(validPages[startPage].URL)
		templateSeen[template] = struct{}{}
		templateGroups = append(templateGroups, validPages[startPage])
	}
	for _, page := range validPages {
		if _, ok := selectedSet[page.URL]; ok {
			continue
		}
		template := templateForURL(page.URL)
		if _, ok := templateSeen[template]; ok {
			continue
		}
		templateSeen[template] = struct{}{}
		templateGroups = append(templateGroups, page)
	}

	for _, page := range templateGroups {
		if len(selected) >= 10 {
			break
		}
		add(page.URL)
	}
	return selected
}

// templateForURL derives a URL template from an absolute URL's path.
func templateForURL(rawURL string) string {
	parsed, err := parseURL(rawURL, "")
	if err != nil {
		return ""
	}
	return detectURLTemplate(parsed.Path)
}

// LighthouseFetchResult pairs a Lighthouse result with its stored payload JSON.
type LighthouseFetchResult struct {
	Result      LighthouseResult
	PayloadJSON string
}

// FailedLighthouseFetch builds the result for a check that produced no payload.
func FailedLighthouseFetch(url, pageID, strategy, errorMessage string) LighthouseFetchResult {
	return LighthouseFetchResult{Result: LighthouseResult{
		URL: url, PageID: pageID, Strategy: strategy, ErrorMessage: &errorMessage,
	}}
}

// FetchLighthouseResult runs one Lighthouse check, mapping provider scores onto
// the persisted result. Provider errors become a result carrying errorMessage,
// matching the TypeScript's behaviour.
func FetchLighthouseResult(ctx context.Context, provider LighthouseProvider, fields FieldDataProvider, logger *slog.Logger, organizationID, url, pageID, strategy string) LighthouseFetchResult {
	if provider == nil {
		return FailedLighthouseFetch(url, pageID, strategy, "lighthouse provider is not configured")
	}
	data, err := provider.FetchLighthouse(ctx, organizationID, LighthouseInput{URL: url, Strategy: strategy})
	if err != nil {
		message := err.Error()
		// Lighthouse runtime errors (ERRORED_DOCUMENT_REQUEST, NOT_HTML, NO_FCP)
		// mean the tenant's page did not load for the provider's Chrome, and a
		// used-up daily share is the workspace's own. Both are surfaced on the
		// audit row, so there is nothing for on-call to act on.
		if logger != nil {
			if strings.HasPrefix(message, lighthouseLoadErrorPrefix) || errors.Is(err, ErrPageSpeedDailyLimit) {
				logger.WarnContext(ctx, "lighthouse check failed", "url", url, "strategy", strategy, "err", message)
			} else {
				logger.ErrorContext(ctx, "lighthouse check failed", "url", url, "strategy", strategy, "err", message)
			}
		}
		return FailedLighthouseFetch(url, pageID, strategy, message)
	}
	data.FieldData = resolveFieldData(ctx, fields, data, strategy)
	payloadJSON, err := json.Marshal(data)
	if err != nil {
		return FailedLighthouseFetch(url, pageID, strategy, fmt.Sprintf("encode lighthouse payload: %v", err))
	}
	return LighthouseFetchResult{
		Result: LighthouseResult{
			URL:                url,
			PageID:             pageID,
			Strategy:           strategy,
			PerformanceScore:   floatFromInt(data.Scores.Performance),
			AccessibilityScore: floatFromInt(data.Scores.Accessibility),
			BestPracticesScore: floatFromInt(data.Scores.BestPractices),
			SeoScore:           floatFromInt(data.Scores.SEO),
			LcpMs:              data.Metrics.LargestContentfulPaint.NumericValue,
			CLS:                data.Metrics.CumulativeLayoutShift.NumericValue,
			InpMs:              data.Metrics.InteractionToNextPaint.NumericValue,
			TtfbMs:             data.Metrics.ServerResponseTime.NumericValue,
		},
		PayloadJSON: string(payloadJSON),
	}
}

// floatFromInt converts an optional int score to an optional float.
func floatFromInt(value *int) *float64 {
	if value == nil {
		return nil
	}
	result := float64(*value)
	return &result
}

// resolveFieldData picks the field data stored with a check. The CrUX API is
// the source when configured; PageSpeed Insights' embedded copy is kept when
// CrUX is not configured or its lookup failed, since a failed lookup says
// nothing about the site.
func resolveFieldData(ctx context.Context, fields FieldDataProvider, data StoredLighthousePayload, strategy string) *StoredFieldData {
	embedded := data.FieldData
	if fields == nil {
		if embedded != nil {
			return embedded
		}
		return &StoredFieldData{Status: FieldNotConfigured, Metrics: []StoredFieldMetric{}}
	}
	looked := fields.FieldData(ctx, data.Metadata.FinalURL, strategy)
	if looked.Status == FieldUnavailable && embedded != nil {
		return embedded
	}
	return &looked
}
