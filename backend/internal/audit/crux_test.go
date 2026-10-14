package audit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const cruxRecordBody = `{
  "record": {
    "key": {"formFactor": "PHONE", "url": "https://example.com/"},
    "metrics": {
      "largest_contentful_paint": {"histogram": [{"start": 0, "end": 2500, "density": 0.8}, {"start": 2500, "end": 4000, "density": 0.15}, {"start": 4000, "density": 0.05}], "percentiles": {"p75": 2100}},
      "cumulative_layout_shift": {"histogram": [{"start": "0.00", "end": "0.10", "density": 0.9}, {"start": "0.10", "end": "0.25", "density": 0.06}, {"start": "0.25", "density": 0.04}], "percentiles": {"p75": "0.05"}},
      "interaction_to_next_paint": {"histogram": [{"start": 0, "end": 200, "density": 0.7}, {"start": 200, "end": 500, "density": 0.2}, {"start": 500, "density": 0.1}], "percentiles": {"p75": 260}}
    },
    "collectionPeriod": {"firstDate": {"year": 2026, "month": 9, "day": 13}, "lastDate": {"year": 2026, "month": 10, "day": 10}}
  }
}`

const cruxNotFoundBody = `{"error": {"code": 404, "message": "chrome ux report data not found", "status": "NOT_FOUND"}}`

// memoryCache is a FieldDataCache in a map.
type memoryCache struct {
	mu      sync.Mutex
	values  map[string][]byte
	failGet bool
}

func newMemoryCache() *memoryCache { return &memoryCache{values: map[string][]byte{}} }

func (c *memoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failGet {
		return nil, false, errors.New("cache down")
	}
	value, ok := c.values[key]
	return value, ok, nil
}

func (c *memoryCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = value
	return nil
}

// cruxRequest is one request the fake CrUX server received.
type cruxRequest struct {
	URL, Origin, FormFactor, Key, Query string
}

// fakeCrux answers each request with respond and records what it was asked.
func fakeCrux(t *testing.T, respond func(cruxRequest) (int, string)) (*cruxClient, *[]cruxRequest, *memoryCache) {
	t.Helper()
	var mu sync.Mutex
	requests := []cruxRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL        string `json:"url"`
			Origin     string `json:"origin"`
			FormFactor string `json:"formFactor"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		request := cruxRequest{URL: body.URL, Origin: body.Origin, FormFactor: body.FormFactor, Key: r.Header.Get("X-Goog-Api-Key"), Query: r.URL.RawQuery}
		mu.Lock()
		requests = append(requests, request)
		mu.Unlock()
		status, payload := respond(request)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(server.Close)
	cache := newMemoryCache()
	client := &cruxClient{apiKey: "test-key", endpoint: server.URL, client: server.Client(), cache: cache, logger: discardLogger()}
	return client, &requests, cache
}

func TestCruxPageRecord(t *testing.T) {
	client, requests, _ := fakeCrux(t, func(cruxRequest) (int, string) { return http.StatusOK, cruxRecordBody })
	got := client.FieldData(t.Context(), "https://Example.com/#top", "mobile")

	if got.Status != FieldAvailable || got.Source != "crux" || got.Scope != "page" {
		t.Fatalf("got %+v, want available page-level CrUX data", got)
	}
	// INP 260 ms is "needs improvement", so the assessment fails.
	if got.CoreWebVitals != CoreWebVitalsFailed {
		t.Fatalf("coreWebVitals = %q, want failed", got.CoreWebVitals)
	}
	if got.CollectionPeriod == nil || got.CollectionPeriod.FirstDate != "2026-09-13" || got.CollectionPeriod.LastDate != "2026-10-10" {
		t.Fatalf("collection period = %+v", got.CollectionPeriod)
	}
	byKey := map[string]StoredFieldMetric{}
	for _, metric := range got.Metrics {
		byKey[metric.Key] = metric
	}
	if cls := byKey["cls"]; cls.Percentile != 0.05 || cls.Category != fieldGood || cls.Good != 0.9 {
		t.Fatalf("cls = %+v; CrUX sends CLS as a decimal string, not multiplied by 100", cls)
	}
	if inp := byKey["inp"]; inp.Percentile != 260 || inp.Category != fieldNeedsImprovement || inp.Poor != 0.1 {
		t.Fatalf("inp = %+v", inp)
	}
	first := (*requests)[0]
	if first.URL != "https://example.com/" || first.FormFactor != "PHONE" {
		t.Fatalf("request = %+v, want the lower-cased URL without its fragment, on PHONE", first)
	}
	if first.Key != "test-key" || strings.Contains(first.Query, "test-key") {
		t.Fatalf("request = %+v: the key must travel only in the header", first)
	}
}

func TestCruxFallsBackToOriginOnlyOnNoRecord(t *testing.T) {
	client, requests, _ := fakeCrux(t, func(r cruxRequest) (int, string) {
		if r.URL != "" {
			return http.StatusNotFound, cruxNotFoundBody
		}
		return http.StatusOK, cruxRecordBody
	})
	got := client.FieldData(t.Context(), "https://example.com/blog/post?id=1", "desktop")
	if got.Status != FieldAvailable || got.Scope != "origin" {
		t.Fatalf("got %+v, want origin-level data", got)
	}
	if len(*requests) != 2 || (*requests)[1].Origin != "https://example.com" || (*requests)[1].FormFactor != "DESKTOP" {
		t.Fatalf("requests = %+v, want a DESKTOP origin lookup after the page's no-record", *requests)
	}
}

func TestCruxNoRecordAnywhereIsInsufficientData(t *testing.T) {
	client, _, _ := fakeCrux(t, func(cruxRequest) (int, string) { return http.StatusNotFound, cruxNotFoundBody })
	got := client.FieldData(t.Context(), "https://example.com/", "mobile")
	if got.Status != FieldInsufficientData || len(got.Metrics) != 0 {
		t.Fatalf("got %+v, want insufficient_data", got)
	}
}

func TestCruxRecordWithoutCLSIsNotAssessed(t *testing.T) {
	body := `{"record": {"metrics": {"largest_contentful_paint": {"percentiles": {"p75": 1200}}}}}`
	client, _, _ := fakeCrux(t, func(cruxRequest) (int, string) { return http.StatusOK, body })
	got := client.FieldData(t.Context(), "https://example.com/", "mobile")
	if got.Status != FieldAvailable || got.CoreWebVitals != CoreWebVitalsNotAssessed || got.CollectionPeriod != nil {
		t.Fatalf("got %+v, want available data that cannot be assessed", got)
	}
}

func TestCruxFailuresAreUnavailableNotInsufficient(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"key rejected", http.StatusForbidden, `{"error": {"code": 403, "status": "PERMISSION_DENIED"}}`},
		{"bad key", http.StatusBadRequest, `{"error": {"code": 400, "status": "INVALID_ARGUMENT", "message": "API key not valid"}}`},
		{"quota", http.StatusTooManyRequests, `{"error": {"code": 429, "status": "RESOURCE_EXHAUSTED"}}`},
		{"outage", http.StatusServiceUnavailable, ``},
		{"404 that is not Google's no-record", http.StatusNotFound, `<html>not found</html>`},
		{"malformed record", http.StatusOK, `{"record": `},
		{"200 without a record", http.StatusOK, `{}`},
		{"bad percentile", http.StatusOK, `{"record": {"metrics": {"cumulative_layout_shift": {"percentiles": {"p75": "abc"}}}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, requests, cache := fakeCrux(t, func(cruxRequest) (int, string) { return tc.status, tc.body })
			got := client.FieldData(t.Context(), "https://example.com/", "mobile")
			if got.Status != FieldUnavailable {
				t.Fatalf("got %+v, want unavailable", got)
			}
			if len(*requests) != 1 {
				t.Fatalf("%d requests; a failed page lookup must not fall back to the origin", len(*requests))
			}
			if len(cache.values) != 0 {
				t.Fatalf("cached %v; failures must not be cached", cache.values)
			}
		})
	}
}

func TestCruxTimeoutIsUnavailable(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	client := &cruxClient{apiKey: "test-key", endpoint: server.URL, client: &http.Client{Timeout: 50 * time.Millisecond}, cache: newMemoryCache(), logger: discardLogger()}
	if got := client.FieldData(t.Context(), "https://example.com/", "mobile"); got.Status != FieldUnavailable {
		t.Fatalf("got %+v, want unavailable on timeout", got)
	}
}

func TestCruxCacheIsKeyedByIdentifierAndDevice(t *testing.T) {
	client, requests, _ := fakeCrux(t, func(r cruxRequest) (int, string) {
		if r.URL == "https://example.com/b" {
			return http.StatusNotFound, cruxNotFoundBody
		}
		return http.StatusOK, cruxRecordBody
	})
	ctx := t.Context()
	client.FieldData(ctx, "https://example.com/a", "mobile")
	client.FieldData(ctx, "https://example.com/a", "mobile")  // cached
	client.FieldData(ctx, "https://example.com/a", "desktop") // other device
	client.FieldData(ctx, "https://example.com/b", "mobile")  // other page, then origin
	if len(*requests) != 4 {
		t.Fatalf("%d requests, want 4: %+v", len(*requests), *requests)
	}
	client.FieldData(ctx, "https://example.com/b", "mobile") // both no-record and origin answers cached
	if len(*requests) != 4 {
		t.Fatalf("%d requests after a repeat; cached answers must be reused", len(*requests))
	}
}

func TestCruxCacheReadFailureStillLooksUp(t *testing.T) {
	client, requests, cache := fakeCrux(t, func(cruxRequest) (int, string) { return http.StatusOK, cruxRecordBody })
	cache.failGet = true
	if got := client.FieldData(t.Context(), "https://example.com/", "mobile"); got.Status != FieldAvailable || len(*requests) != 1 {
		t.Fatalf("got %+v after %d requests, want a live lookup when the cache is down", got, len(*requests))
	}
}

func TestCruxConcurrentIdenticalLookupsShareOneRequest(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		<-release
		_, _ = w.Write([]byte(cruxRecordBody))
	}))
	t.Cleanup(server.Close)
	client := &cruxClient{apiKey: "test-key", endpoint: server.URL, client: server.Client(), cache: newMemoryCache(), logger: discardLogger()}

	var wg sync.WaitGroup
	results := make([]StoredFieldData, 5)
	for i := range results {
		wg.Go(func() { results[i] = client.FieldData(context.Background(), "https://example.com/", "mobile") })
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("%d requests for 5 identical concurrent lookups, want 1", calls.Load())
	}
	for _, result := range results {
		if result.Status != FieldAvailable {
			t.Fatalf("result = %+v", result)
		}
	}
}

func TestCruxRejectsNonHTTPURL(t *testing.T) {
	client, requests, _ := fakeCrux(t, func(cruxRequest) (int, string) { return http.StatusOK, cruxRecordBody })
	if got := client.FieldData(t.Context(), "ftp://example.com/", "mobile"); got.Status != FieldUnavailable || len(*requests) != 0 {
		t.Fatalf("got %+v after %d requests, want unavailable without a request", got, len(*requests))
	}
}

type fixedFieldData StoredFieldData

func (f fixedFieldData) FieldData(context.Context, string, string) StoredFieldData {
	return StoredFieldData(f)
}

func TestResolveFieldData(t *testing.T) {
	embedded := &StoredFieldData{Status: FieldAvailable, Source: "pagespeed"}
	withEmbedded := StoredLighthousePayload{FieldData: embedded}
	cases := []struct {
		name    string
		fields  FieldDataProvider
		payload StoredLighthousePayload
		want    FieldDataStatus
		source  string
	}{
		{"no CrUX key, PageSpeed has data", nil, withEmbedded, FieldAvailable, "pagespeed"},
		{"no CrUX key, no PageSpeed data", nil, StoredLighthousePayload{}, FieldNotConfigured, ""},
		{"CrUX answers", fixedFieldData{Status: FieldAvailable, Source: "crux"}, withEmbedded, FieldAvailable, "crux"},
		{"CrUX has no record, overriding PageSpeed", fixedFieldData{Status: FieldInsufficientData, Source: "crux"}, withEmbedded, FieldInsufficientData, "crux"},
		{"CrUX failed, PageSpeed copy kept", fixedFieldData{Status: FieldUnavailable}, withEmbedded, FieldAvailable, "pagespeed"},
		{"CrUX failed, nothing else", fixedFieldData{Status: FieldUnavailable}, StoredLighthousePayload{}, FieldUnavailable, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveFieldData(t.Context(), tc.fields, tc.payload, "mobile")
			if got == nil || got.Status != tc.want || got.Source != tc.source {
				t.Fatalf("got %+v, want status %s source %q", got, tc.want, tc.source)
			}
		})
	}
}
