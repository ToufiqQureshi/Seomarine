package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// cruxEndpoint is the Chrome UX Report API (developer.chrome.com/docs/crux/api).
const cruxEndpoint = "https://chromeuxreport.googleapis.com/v1/records:queryRecord"

const (
	// cruxTimeout bounds one lookup; the API answers from a precomputed table.
	cruxTimeout = 15 * time.Second
	// cruxCacheTTL keeps a lookup for 12 hours. Google recomputes the 28-day
	// window once a day (around 04:00 UTC), so a fresher answer changes at
	// most one day of 28, and repeat audits of a site cost no quota.
	cruxCacheTTL = 12 * time.Hour
	// cruxMaxResponseBytes bounds a response read into memory.
	cruxMaxResponseBytes = 1 << 20
)

// cruxRequestMetrics are the metrics asked for, in the API's names.
var cruxRequestMetrics = func() []string {
	names := make([]string, len(fieldMetrics))
	for i, metric := range fieldMetrics {
		names[i] = metric.crux
	}
	return names
}()

// errCruxUnavailable wraps every lookup failure so callers cannot mistake it
// for "no record".
var errCruxUnavailable = errors.New("field data lookup failed")

// FieldDataProvider looks up field data for one checked page. It never fails:
// a failure is a FieldUnavailable result.
type FieldDataProvider interface {
	FieldData(ctx context.Context, pageURL, strategy string) StoredFieldData
}

// FieldDataCache stores CrUX answers across server instances.
type FieldDataCache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

// RedisFieldDataCache is a FieldDataCache in Redis.
type RedisFieldDataCache struct{ Client *redis.Client }

// Get returns a cached value, or false when there is none.
func (c RedisFieldDataCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := c.Client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read field data cache: %w", err)
	}
	return value, true, nil
}

// Set stores a value until ttl passes.
func (c RedisFieldDataCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.Client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("write field data cache: %w", err)
	}
	return nil
}

// cruxClient implements FieldDataProvider over the CrUX API.
type cruxClient struct {
	apiKey   string
	endpoint string
	client   *http.Client
	cache    FieldDataCache
	logger   *slog.Logger
	// group shares one lookup between concurrent identical requests on this
	// instance; the cache shares it across instances afterwards.
	group singleflight.Group
}

// NewCrUXProvider builds a provider that sends apiKey with every lookup.
func NewCrUXProvider(apiKey string, cache FieldDataCache, logger *slog.Logger) FieldDataProvider {
	return &cruxClient{apiKey: apiKey, endpoint: cruxEndpoint, client: &http.Client{}, cache: cache, logger: logger}
}

// cruxLookup is one cached answer: a record, or Google's "no record".
type cruxLookup struct {
	Found   bool                   `json:"found"`
	Metrics []StoredFieldMetric    `json:"metrics,omitempty"`
	Period  *FieldCollectionPeriod `json:"period,omitempty"`
}

// FieldData asks for the page's own record, then the site's. Only a "no
// record" answer for the page moves on to the site, and only "no record" for
// both means insufficient data; any failure is reported as unavailable.
func (c *cruxClient) FieldData(ctx context.Context, pageURL, strategy string) StoredFieldData {
	formFactor := "PHONE"
	if strategy == "desktop" {
		formFactor = "DESKTOP"
	}
	pageKey, originKey, err := cruxIdentifiers(pageURL)
	if err != nil {
		return StoredFieldData{Status: FieldUnavailable, Metrics: []StoredFieldMetric{}}
	}
	for _, try := range []struct{ field, value, scope string }{
		{"url", pageKey, "page"},
		{"origin", originKey, "origin"},
	} {
		lookup, err := c.lookup(ctx, try.field, try.value, formFactor)
		if err != nil {
			c.logger.WarnContext(ctx, "field data lookup failed", "identifier", try.field, "err", err)
			return StoredFieldData{Status: FieldUnavailable, Metrics: []StoredFieldMetric{}}
		}
		if lookup.Found {
			if data := availableFieldData("crux", try.scope, lookup.Metrics, lookup.Period); data != nil {
				return *data
			}
			// A record without any of our metrics is no better than none.
		}
	}
	return StoredFieldData{Status: FieldInsufficientData, Source: "crux", Metrics: []StoredFieldMetric{}}
}

// cruxIdentifiers returns the URL CrUX is asked about (no fragment) and its
// origin, both with a lower-case scheme and host.
func cruxIdentifiers(raw string) (string, string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", "", fmt.Errorf("not an http(s) URL: %q", raw)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment, parsed.RawFragment = "", ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), parsed.Scheme + "://" + parsed.Host, nil
}

// lookup answers from the cache, else asks Google once per identical key.
// Errors are never cached.
func (c *cruxClient) lookup(ctx context.Context, field, value, formFactor string) (cruxLookup, error) {
	key := "audit:crux:" + formFactor + ":" + field + ":" + value
	if cached, ok, err := c.cache.Get(ctx, key); err != nil {
		c.logger.WarnContext(ctx, "field data cache read failed", "err", err)
	} else if ok {
		var lookup cruxLookup
		if json.Unmarshal(cached, &lookup) == nil {
			return lookup, nil
		}
	}
	result, err, _ := c.group.Do(key, func() (any, error) {
		// Detached from the first caller, so one caller giving up does not
		// fail the others waiting on the same lookup.
		callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cruxTimeout)
		defer cancel()
		lookup, err := c.query(callCtx, field, value, formFactor)
		if err != nil {
			return cruxLookup{}, err
		}
		if encoded, err := json.Marshal(lookup); err == nil {
			if err := c.cache.Set(callCtx, key, encoded, cruxCacheTTL); err != nil {
				c.logger.WarnContext(callCtx, "field data cache write failed", "err", err)
			}
		}
		return lookup, nil
	})
	if err != nil {
		return cruxLookup{}, err
	}
	return result.(cruxLookup), nil
}

// query sends one queryRecord request. The key travels in the
// X-Goog-Api-Key header, never the URL, so it cannot reach a log or an error.
func (c *cruxClient) query(ctx context.Context, field, value, formFactor string) (cruxLookup, error) {
	body, err := json.Marshal(map[string]any{field: value, "formFactor": formFactor, "metrics": cruxRequestMetrics})
	if err != nil {
		return cruxLookup{}, fmt.Errorf("encode CrUX request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return cruxLookup{}, fmt.Errorf("build CrUX request: %w", err)
	}
	request.Header.Set("X-Goog-Api-Key", c.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return cruxLookup{}, fmt.Errorf("%w: %w", errCruxUnavailable, err)
	}
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(response.Body, cruxMaxResponseBytes))
	if err != nil {
		return cruxLookup{}, fmt.Errorf("%w: read response: %w", errCruxUnavailable, err)
	}
	switch {
	case response.StatusCode == http.StatusNotFound && cruxNotFound(payload):
		return cruxLookup{Found: false}, nil
	case response.StatusCode != http.StatusOK:
		return cruxLookup{}, fmt.Errorf("%w: CrUX answered %d", errCruxUnavailable, response.StatusCode)
	}
	return parseCruxRecord(payload)
}

// cruxNotFound reports whether a 404 body is Google's documented "no record"
// error, not a 404 from something else on the path (a proxy, a wrong URL).
func cruxNotFound(payload []byte) bool {
	var envelope struct {
		Error struct {
			Status string `json:"status"`
		} `json:"error"`
	}
	return json.Unmarshal(payload, &envelope) == nil && envelope.Error.Status == "NOT_FOUND"
}

// cruxValue is a CrUX number: an int for times, a decimal string for CLS.
type cruxValue struct {
	value float64
	set   bool
}

func (v *cruxValue) UnmarshalJSON(data []byte) error {
	text := strings.Trim(string(data), `"`)
	if text == "null" || text == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fmt.Errorf("CrUX value %s: %w", data, err)
	}
	v.value, v.set = parsed, true
	return nil
}

type cruxDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

func (d cruxDate) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// parseCruxRecord reads a 200 response into a lookup.
func parseCruxRecord(payload []byte) (cruxLookup, error) {
	var response struct {
		Record *struct {
			Metrics map[string]struct {
				Histogram []struct {
					Density float64 `json:"density"`
				} `json:"histogram"`
				Percentiles struct {
					P75 cruxValue `json:"p75"`
				} `json:"percentiles"`
			} `json:"metrics"`
			CollectionPeriod *struct {
				FirstDate cruxDate `json:"firstDate"`
				LastDate  cruxDate `json:"lastDate"`
			} `json:"collectionPeriod"`
		} `json:"record"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return cruxLookup{}, fmt.Errorf("%w: invalid response: %w", errCruxUnavailable, err)
	}
	if response.Record == nil {
		return cruxLookup{}, fmt.Errorf("%w: response has no record", errCruxUnavailable)
	}
	lookup := cruxLookup{Found: true, Metrics: []StoredFieldMetric{}}
	for _, name := range fieldMetrics {
		raw, ok := response.Record.Metrics[name.crux]
		if !ok || !raw.Percentiles.P75.set {
			continue
		}
		value := raw.Percentiles.P75.value
		metric := StoredFieldMetric{Key: name.key, Percentile: value, Category: name.threshold.categorize(value)}
		// CrUX bins match the good / needs improvement / poor thresholds.
		if len(raw.Histogram) == 3 {
			metric.Good = raw.Histogram[0].Density
			metric.NeedsImprovement = raw.Histogram[1].Density
			metric.Poor = raw.Histogram[2].Density
		}
		lookup.Metrics = append(lookup.Metrics, metric)
	}
	if period := response.Record.CollectionPeriod; period != nil {
		lookup.Period = &FieldCollectionPeriod{FirstDate: period.FirstDate.String(), LastDate: period.LastDate.String()}
	}
	return lookup, nil
}
