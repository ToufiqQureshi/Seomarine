package backlinks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

type memoryCache struct {
	mu     sync.Mutex
	values map[string][]byte
	fail   bool
}

func (c *memoryCache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail {
		return nil, errors.New("redis unavailable")
	}
	value, ok := c.values[key]
	if !ok {
		return nil, errors.New("redis: nil")
	}
	return append([]byte(nil), value...), nil
}
func (c *memoryCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail {
		return errors.New("redis unavailable")
	}
	if c.values == nil {
		c.values = map[string][]byte{}
	}
	c.values[key] = append([]byte(nil), value...)
	return nil
}

type usageRecorder struct {
	mu            sync.Mutex
	organizations []string
}

func (r *usageRecorder) RecordDataForSEO(_ context.Context, org string, _ dataforseo.Cost) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.organizations = append(r.organizations, org)
	return nil
}

func testClient(t *testing.T, handler http.Handler, recorder *usageRecorder) *dataforseo.Client {
	return testClientWithTimeout(t, handler, recorder, 0)
}
func testClientWithTimeout(t *testing.T, handler http.Handler, recorder *usageRecorder, timeout time.Duration) *dataforseo.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := dataforseo.NewClient(dataforseo.Options{BaseURL: server.URL, APIKey: base64.StdEncoding.EncodeToString([]byte("test-user:test-pass")), Recorder: recorder, MaxRetries: 0, Timeout: timeout})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
func providerOK(w http.ResponseWriter, path string, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "tasks": []any{map[string]any{"status_code": 20000, "status_message": "Ok.", "path": strings.Split(strings.Trim(path, "/"), "/"), "cost": 0.01, "result": []any{result}}}})
}

func TestOverviewCacheIsOrganizationScopedAndRedisFailureIsSoft(t *testing.T) {
	t.Parallel()
	var calls int
	var mu sync.Mutex
	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/summary/live") {
			providerOK(w, r.URL.Path, map[string]any{"rank": 52, "backlinks": 90, "referring_pages": 20, "referring_domains": 10, "info": map[string]any{"target_spam_score": 5}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/history/live") {
			providerOK(w, r.URL.Path, map[string]any{"items": []any{map[string]any{"date": "2026-10-08T00:00:00Z", "backlinks": 90, "new_reffering_domains": 2}}})
			return
		}
		http.NotFound(w, r)
	})
	recorder := &usageRecorder{}
	cache := &memoryCache{values: map[string][]byte{}}
	svc := newService(testClient(t, server, recorder), cache, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return time.Date(2026, 10, 9, 11, 12, 13, 0, time.UTC) })
	in := lookupInput{Target: "example.com", Scope: string(ScopeDomain)}
	first, err := svc.Overview(context.Background(), "org-one", in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Overview(context.Background(), "org-one", in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Summary.Backlinks == nil || *first.Summary.Backlinks != 90 || len(second.Trends) != 1 || second.NewLostTrends[0].NewReferringDomains == nil || *second.NewLostTrends[0].NewReferringDomains != 2 {
		t.Fatalf("unexpected overview mapping: %#v", second)
	}
	if _, err := svc.Overview(context.Background(), "org-two", in); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls != 4 {
		t.Fatalf("provider calls = %d, want 4 (cached within org, isolated between orgs)", gotCalls)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.organizations) != 4 {
		t.Fatalf("usage rows = %d, want 4", len(recorder.organizations))
	}
	for i, org := range recorder.organizations {
		want := "org-one"
		if i >= 2 {
			want = "org-two"
		}
		if org != want {
			t.Errorf("usage organization[%d] = %q, want %q", i, org, want)
		}
	}
}

func TestCacheDownFallsThroughToProvider(t *testing.T) {
	t.Parallel()
	cache := &memoryCache{fail: true}
	recorder := &usageRecorder{}
	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerOK(w, r.URL.Path, map[string]any{"backlinks": 3, "info": map[string]any{}})
	})
	svc := newService(testClient(t, server, recorder), cache, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	got, err := svc.Overview(context.Background(), "org", lookupInput{Target: "example.com", Scope: "exact_url"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary.Backlinks == nil || *got.Summary.Backlinks != 3 {
		t.Fatalf("overview = %#v", got)
	}
}

func TestProviderRejectsBillingIssueWithoutRetry(t *testing.T) {
	t.Parallel()
	var calls int
	server := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "tasks": []any{map[string]any{"status_code": 40210, "status_message": "Insufficient funds"}}})
	})
	svc := newService(testClient(t, server, &usageRecorder{}), nil, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	_, err := svc.Overview(context.Background(), "org", lookupInput{Target: "example.com", Scope: "exact_url"})
	if !errors.Is(err, dataforseo.ErrBillingIssue) {
		t.Fatalf("error = %v, want billing issue", err)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want one non-retried request", calls)
	}
}

func TestProviderErrorIsNotRetried(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "tasks": []any{map[string]any{"status_code": 50000, "status_message": "provider unavailable"}}})
	})
	svc := newService(testClient(t, server, &usageRecorder{}), nil, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	_, err := svc.Overview(context.Background(), "org", lookupInput{Target: "example.com", Scope: "exact_url"})
	if !errors.Is(err, dataforseo.ErrUpstreamUnavailable) {
		t.Fatalf("error = %v, want upstream unavailable", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want one non-retried paid request", calls.Load())
	}
}

func TestEmptyProviderListReturnsEmptyArray(t *testing.T) {
	t.Parallel()
	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerOK(w, r.URL.Path, map[string]any{"items": nil, "total_count": 0})
	})
	svc := newService(testClient(t, server, &usageRecorder{}), nil, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	got, err := svc.Rows(context.Background(), "org", lookupInput{Target: "example.com"}, pageInput{Page: 1, PageSize: 50, Sort: "rank,desc", Mode: "as_is"}, rowsFilters{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rows == nil || len(got.Rows) != 0 || got.TotalCount == nil || *got.TotalCount != 0 || got.HasMore {
		t.Fatalf("empty list = %#v", got)
	}
}

func TestProviderTimeoutIsReturnedToCaller(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { calls.Add(1); <-r.Context().Done() })
	svc := newService(testClientWithTimeout(t, server, &usageRecorder{}, time.Second), nil, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	_, err := svc.Overview(context.Background(), "org", lookupInput{Target: "example.com", Scope: "exact_url"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", calls.Load())
	}
}

func TestMapBacklinkLegacyFieldFallbacks(t *testing.T) {
	t.Parallel()
	domain := "source.example"
	url := "https://source.example/a"
	lostDate := "2026-01-01"
	score := float64(12)
	row := mapBacklink(providerBacklink{DomainFrom: &domain, URLFrom: &url, LostDate: &lostDate, SpamScoreLegacy: &score, Attributes: []string{"nofollow"}})
	if row.SpamScore == nil || *row.SpamScore != score || row.LastSeen == nil || *row.LastSeen != lostDate || !row.IsLost || len(row.RelAttributes) != 1 || row.RelAttributes[0] != "nofollow" {
		t.Fatalf("mapped row = %#v", row)
	}
}

func TestEmptyLostDateDoesNotMarkLinkLost(t *testing.T) {
	t.Parallel()
	empty := ""
	if got := mapBacklink(providerBacklink{LostDate: &empty}); got.IsLost {
		t.Fatalf("empty lost_date marked row lost: %#v", got)
	}
}

func TestSubfolderOverviewCountsFilteredRowsSequentially(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var modes []string
	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode provider request: %v", err)
			return
		}
		mode, _ := request[0]["mode"].(string)
		mu.Lock()
		modes = append(modes, mode)
		mu.Unlock()
		total := 120
		if mode == "one_per_domain" {
			total = 17
		}
		providerOK(w, r.URL.Path, map[string]any{"items": []any{}, "total_count": total})
	})
	recorder := &usageRecorder{}
	svc := newService(testClient(t, server, recorder), nil, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	got, err := svc.Overview(context.Background(), "org", lookupInput{Target: "example.com/blog", Scope: "subfolder"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary.Backlinks == nil || *got.Summary.Backlinks != 120 || got.Summary.ReferringDomains == nil || *got.Summary.ReferringDomains != 17 || got.Summary.Rank != nil || len(got.Trends) != 0 {
		t.Fatalf("subfolder overview = %#v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(modes, ",") != "as_is,one_per_domain" {
		t.Fatalf("modes = %v", modes)
	}
}

func TestCancelledRequestStopsProviderCall(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		providerOK(w, r.URL.Path, map[string]any{})
	})
	svc := newService(testClient(t, server, &usageRecorder{}), nil, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.Overview(ctx, "org", lookupInput{Target: "example.com", Scope: "exact_url"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("provider server was reached %d times", calls.Load())
	}
}

func TestConcurrentOrganizationRequestsAreRaceSafe(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		providerOK(w, r.URL.Path, map[string]any{"items": []any{}, "total_count": 0})
	})
	svc := newService(testClient(t, server, &usageRecorder{}), &memoryCache{values: map[string][]byte{}}, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	const workers = 8
	var wg sync.WaitGroup
	for i := range workers {
		org := fmt.Sprintf("org-%d", i)
		wg.Go(func() {
			_, err := svc.Rows(context.Background(), org, lookupInput{Target: "example.com"}, pageInput{Page: 1, PageSize: 50, Sort: "rank,desc", Mode: "as_is"}, rowsFilters{}, false)
			if err != nil {
				t.Errorf("Rows() for %s: %v", org, err)
			}
		})
	}
	wg.Wait()
	if got := calls.Load(); got != workers {
		t.Fatalf("provider calls = %d, want %d distinct tenant requests", got, workers)
	}
}

func TestFilterConditionsCountNestedGroups(t *testing.T) {
	t.Parallel()
	filters := prependInclude(buildRowsFilters(rowsFilters{Exclude: "a,b", MinLinkAuthority: floatPtr(1)}), "url_from", "x,y")
	if got := countFilters(filters); got != 5 {
		t.Fatalf("filter count = %d, want 5: %#v", got, filters)
	}
}

//go:fix inline
func floatPtr(v float64) *float64 { return new(v) }
