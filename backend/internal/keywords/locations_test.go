package keywords

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

type memoryLocationCache struct {
	mu     sync.Mutex
	values map[string][]byte
	failed bool
}

func (c *memoryLocationCache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failed {
		return nil, errors.New("redis unavailable")
	}
	value, ok := c.values[key]
	if !ok {
		return nil, redis.Nil
	}
	return value, nil
}

func (c *memoryLocationCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.values == nil {
		c.values = map[string][]byte{}
	}
	c.values[key] = value
	return nil
}

func (c *memoryLocationCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.values)
}

type stubLimiter struct {
	allow bool
	err   error
}

func (l stubLimiter) Allow(context.Context, string) (bool, error) { return l.allow, l.err }

type noopRecorder struct{}

func (noopRecorder) RecordDataForSEO(context.Context, string, dataforseo.Cost) error { return nil }

// locationProvider stands in for DataForSEO's per-country locations endpoint.
type locationProvider struct {
	calls  atomic.Int32
	paths  []string
	mu     sync.Mutex
	status int
	rows   []map[string]any
	delay  time.Duration
}

func (p *locationProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.calls.Add(1)
	p.mu.Lock()
	p.paths = append(p.paths, r.URL.Path)
	p.mu.Unlock()
	time.Sleep(p.delay)
	status := p.status
	if status == 0 {
		status = 20000
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status_code": 20000,
		"tasks": []map[string]any{{
			"status_code": status, "status_message": "x", "path": []string{"v3", "serp"}, "cost": 0, "result": p.rows,
		}},
	})
}

func providerRows() []map[string]any {
	return []map[string]any{
		{"location_code": 1, "location_name": "Portland,Maine,United States", "location_type": "City"},
		{"location_code": 2, "location_name": "Portland,Oregon,United States", "location_type": "City"},
		{"location_code": 3, "location_name": "04101,Maine,United States", "location_type": "Postal Code"},
		{"location_code": 4, "location_name": "Maine,United States", "location_type": "State"},
		{"location_code": 5, "location_name": "Portland International Airport,Oregon,United States", "location_type": "Airport"},
		{"location_code": 6, "location_name": "Portland, OR,Oregon,United States", "location_type": "DMA Region"},
		{"location_code": 7, "location_name": "Broken", "location_type": nil},
	}
}

func newTestLocationService(t *testing.T, provider *locationProvider, cache *memoryLocationCache, limiter locationLimiter) *LocationService {
	t.Helper()
	server := httptest.NewServer(provider)
	t.Cleanup(server.Close)
	client, err := dataforseo.NewClient(dataforseo.Options{
		BaseURL: server.URL, APIKey: base64.StdEncoding.EncodeToString([]byte("test:test")),
		Recorder: noopRecorder{}, MaxRetries: 1, RetryBackoff: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return &LocationService{
		client: client, cache: cache, limiter: limiter, logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		inflight: map[string]*locationFill{},
	}
}

func TestLocationsFilterTypesAndCacheTheCountry(t *testing.T) {
	provider := &locationProvider{rows: providerRows()}
	cache := &memoryLocationCache{}
	service := newTestLocationService(t, provider, cache, stubLimiter{allow: true})

	for range 2 {
		rows, err := service.Locations(context.Background(), "org-1", "us")
		if err != nil {
			t.Fatalf("locations: %v", err)
		}
		var names []string
		for _, row := range rows {
			names = append(names, row.LocationName)
		}
		if got, want := strings.Join(names, "|"), "Portland,Maine,United States|Portland,Oregon,United States|Portland, OR,Oregon,United States"; got != want {
			t.Fatalf("kept rows = %s, want %s", got, want)
		}
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1 (second read comes from cache)", provider.calls.Load())
	}
	if len(provider.paths) != 1 || provider.paths[0] != "/v3/serp/google/locations/us" {
		t.Fatalf("paths = %v", provider.paths)
	}
}

func TestLocationsFailClosedWhenTheCacheIsDown(t *testing.T) {
	provider := &locationProvider{rows: providerRows()}
	service := newTestLocationService(t, provider, &memoryLocationCache{failed: true}, stubLimiter{allow: true})

	_, err := service.Locations(context.Background(), "org-1", "us")
	if !errors.Is(err, ErrLocationCache) {
		t.Fatalf("err = %v, want ErrLocationCache", err)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("a cache outage must not trigger billed provider calls, got %d", provider.calls.Load())
	}
}

func TestLocationsDoNotCacheAnEmptyOrFailedAnswer(t *testing.T) {
	cache := &memoryLocationCache{}
	empty := &locationProvider{}
	service := newTestLocationService(t, empty, cache, stubLimiter{allow: true})
	for range 2 {
		if rows, err := service.Locations(context.Background(), "org-1", "us"); err != nil || len(rows) != 0 {
			t.Fatalf("rows = %v err = %v", rows, err)
		}
	}
	if empty.calls.Load() != 2 || cache.size() != 0 {
		t.Fatalf("empty answer: calls = %d cached = %d, want 2 and 0", empty.calls.Load(), cache.size())
	}

	failing := &locationProvider{status: 50000, rows: providerRows()}
	service = newTestLocationService(t, failing, cache, stubLimiter{allow: true})
	if _, err := service.Locations(context.Background(), "org-1", "us"); err == nil {
		t.Fatal("a failed task must return an error")
	}
	if cache.size() != 0 {
		t.Fatal("a failed answer must not be cached")
	}
}

func TestLocationsCoalesceConcurrentColdFills(t *testing.T) {
	provider := &locationProvider{rows: providerRows(), delay: 150 * time.Millisecond}
	service := newTestLocationService(t, provider, &memoryLocationCache{}, stubLimiter{allow: true})

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Locations(context.Background(), "org-1", "us")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("locations: %v", err)
		}
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1 for 8 concurrent cold readers", provider.calls.Load())
	}
}

func locationRequestTo(t *testing.T, service *LocationService, path, body string, user *auth.User) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	MountLocations(mux, LocationDeps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Service: service,
		WithSession: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if user != nil {
					r = r.WithContext(auth.WithUser(r.Context(), *user))
				}
				next.ServeHTTP(w, r)
			})
		},
	})
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	return recorder
}

func TestLocationHandler(t *testing.T) {
	member := &auth.User{ID: "user-1", OrganizationID: "org-1"}
	cases := []struct {
		name   string
		path   string
		body   string
		user   *auth.User
		allow  bool
		status int
	}{
		{"search ranks cities", "/api/v1/serp-locations/search", `{"countryCode":"US","query":"portland"}`, member, true, http.StatusOK},
		{"prewarm succeeds", "/api/v1/serp-locations/prewarm", `{"countryCode":"us"}`, member, true, http.StatusOK},
		{"no organization", "/api/v1/serp-locations/search", `{"countryCode":"us","query":"a"}`, &auth.User{ID: "user-1"}, true, http.StatusForbidden},
		{"bad country", "/api/v1/serp-locations/search", `{"countryCode":"usa","query":"a"}`, member, true, http.StatusBadRequest},
		{"empty query", "/api/v1/serp-locations/search", `{"countryCode":"us","query":""}`, member, true, http.StatusBadRequest},
		{"query over 100 chars", "/api/v1/serp-locations/search", `{"countryCode":"us","query":"` + strings.Repeat("a", 101) + `"}`, member, true, http.StatusBadRequest},
		{"prewarm with a query", "/api/v1/serp-locations/prewarm", `{"countryCode":"us","query":"a"}`, member, true, http.StatusBadRequest},
		{"unknown field", "/api/v1/serp-locations/search", `{"countryCode":"us","query":"a","x":1}`, member, true, http.StatusBadRequest},
		{"two json values", "/api/v1/serp-locations/search", `{"countryCode":"us","query":"a"}{}`, member, true, http.StatusBadRequest},
		{"rate limited", "/api/v1/serp-locations/search", `{"countryCode":"us","query":"a"}`, member, false, http.StatusTooManyRequests},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := &locationProvider{rows: providerRows()}
			service := newTestLocationService(t, provider, &memoryLocationCache{}, stubLimiter{allow: tc.allow})
			recorder := locationRequestTo(t, service, tc.path, tc.body, tc.user)
			if recorder.Code != tc.status {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, tc.status, recorder.Body)
			}
			if tc.status != http.StatusOK && provider.calls.Load() != 0 {
				t.Fatalf("a rejected request must not reach the provider, got %d calls", provider.calls.Load())
			}
		})
	}
}

func TestLocationHandlerSearchReturnsRankedCities(t *testing.T) {
	provider := &locationProvider{rows: providerRows()}
	service := newTestLocationService(t, provider, &memoryLocationCache{}, stubLimiter{allow: true})
	recorder := locationRequestTo(t, service, "/api/v1/serp-locations/search", `{"countryCode":"us","query":"Portland, Maine"}`, &auth.User{ID: "u", OrganizationID: "o"})
	var rows []SerpLocation
	if err := json.Unmarshal(recorder.Body.Bytes(), &rows); err != nil || len(rows) == 0 || rows[0].LocationName != "Portland,Maine,United States" {
		t.Fatalf("rows = %+v err = %v body = %s", rows, err, recorder.Body)
	}
	for _, row := range rows {
		if row.LocationType == "Postal Code" || row.LocationType == "State" || row.LocationType == "Airport" {
			t.Fatalf("excluded location type leaked: %+v", row)
		}
	}
}

func TestLocationHandlerMapsProviderFailures(t *testing.T) {
	member := &auth.User{ID: "user-1", OrganizationID: "org-1"}
	body := `{"countryCode":"us","query":"a"}`

	billing := &locationProvider{status: 40200, rows: providerRows()}
	service := newTestLocationService(t, billing, &memoryLocationCache{}, stubLimiter{allow: true})
	if recorder := locationRequestTo(t, service, "/api/v1/serp-locations/search", body, member); recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("billing issue status = %d, want 503 (%s)", recorder.Code, recorder.Body)
	}

	service = newTestLocationService(t, &locationProvider{rows: providerRows()}, &memoryLocationCache{failed: true}, stubLimiter{allow: true})
	if recorder := locationRequestTo(t, service, "/api/v1/serp-locations/search", body, member); recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("cache outage status = %d, want 503", recorder.Code)
	}

	service = newTestLocationService(t, &locationProvider{rows: providerRows()}, &memoryLocationCache{}, stubLimiter{err: errors.New("redis down")})
	if recorder := locationRequestTo(t, service, "/api/v1/serp-locations/search", body, member); recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("limiter outage status = %d, want 503", recorder.Code)
	}

	service = newTestLocationService(t, &locationProvider{status: 50000, rows: providerRows()}, &memoryLocationCache{}, stubLimiter{allow: true})
	if recorder := locationRequestTo(t, service, "/api/v1/serp-locations/search", body, member); recorder.Code != http.StatusBadGateway {
		t.Fatalf("provider failure status = %d, want 502", recorder.Code)
	}
}

func TestLocationHandlerWithoutAServiceAnswers503(t *testing.T) {
	recorder := locationRequestTo(t, nil, "/api/v1/serp-locations/search", `{"countryCode":"us","query":"a"}`, &auth.User{ID: "u", OrganizationID: "o"})
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}
