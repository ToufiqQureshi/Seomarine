package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// lighthousePath is the DataForSEO on-page Lighthouse endpoint.
const lighthousePath = "/v3/on_page/lighthouse/live/json"

// lighthouseRequestCategories are the categories requested from the provider.
var lighthouseRequestCategories = []string{"performance", "accessibility", "best_practices", "seo"}

// LighthouseProvider fetches a Lighthouse report for one URL and strategy.
type LighthouseProvider interface {
	// FetchLighthouse calls the provider and returns the compact stored payload.
	FetchLighthouse(ctx context.Context, organizationID string, input LighthouseInput) (StoredLighthousePayload, error)
}

// dataforseoLighthouseProvider implements LighthouseProvider over the shared
// DataForSEO client.
type dataforseoLighthouseProvider struct {
	client dataforseoLighthouseClient
	now    func() time.Time
}

// dataforseoLighthouseClient is the subset of the DataForSEO client the provider
// needs, so tests can fake it.
type dataforseoLighthouseClient interface {
	Do(ctx context.Context, organizationID, method, path string, body []byte, retrySafe bool) (json.RawMessage, error)
}

// NewDataForseoLighthouseProvider builds a provider over the shared client.
func NewDataForseoLighthouseProvider(client dataforseoLighthouseClient) LighthouseProvider {
	return &dataforseoLighthouseProvider{client: client, now: time.Now}
}

// FetchLighthouse posts the Lighthouse task. The call is billed and
// non-idempotent, so it is never replayed: retrySafe is false.
func (p *dataforseoLighthouseProvider) FetchLighthouse(ctx context.Context, organizationID string, input LighthouseInput) (StoredLighthousePayload, error) {
	if p == nil || p.client == nil {
		return StoredLighthousePayload{}, errors.New("lighthouse provider is not configured")
	}
	body, err := json.Marshal([]map[string]any{{
		"url":        input.URL,
		"for_mobile": input.Strategy == "mobile",
		"categories": lighthouseRequestCategories,
	}})
	if err != nil {
		return StoredLighthousePayload{}, fmt.Errorf("encode lighthouse request: %w", err)
	}
	payload, err := p.client.Do(ctx, organizationID, http.MethodPost, lighthousePath, body, false)
	if err != nil {
		return StoredLighthousePayload{}, err
	}
	now := time.Now()
	if p.now != nil {
		now = p.now()
	}
	return ParseDataForseoLighthousePayload(payload, input, now)
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
// the homepage plus one page per URL template, capped at ten. Mirrors
// selectLighthouseSample.
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
func FetchLighthouseResult(ctx context.Context, provider LighthouseProvider, logger *slog.Logger, organizationID, url, pageID, strategy string) LighthouseFetchResult {
	if provider == nil {
		return FailedLighthouseFetch(url, pageID, strategy, "lighthouse provider is not configured")
	}
	data, err := provider.FetchLighthouse(ctx, organizationID, LighthouseInput{URL: url, Strategy: strategy})
	if err != nil {
		message := err.Error()
		// Lighthouse runtime errors (ERRORED_DOCUMENT_REQUEST, NOT_HTML, NO_FCP)
		// mean the tenant's page did not load for the provider's Chrome. The
		// failure is surfaced on the audit row, so there is nothing to act on.
		if logger != nil {
			if strings.Contains(message, "Lighthouse encountered an error with the following code") {
				logger.WarnContext(ctx, "lighthouse check failed", "url", url, "strategy", strategy, "err", message)
			} else {
				logger.ErrorContext(ctx, "lighthouse check failed", "url", url, "strategy", strategy, "err", message)
			}
		}
		return FailedLighthouseFetch(url, pageID, strategy, message)
	}
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
