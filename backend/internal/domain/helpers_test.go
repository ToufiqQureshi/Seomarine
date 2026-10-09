package domain

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

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

var fixedNow = time.Date(2026, 10, 9, 11, 12, 13, 0, time.UTC)

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

func (c *memoryCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.values)
}

// fakeProvider stands in for DataForSEO: it records every request and answers
// with `result` inside a task whose status code is `taskStatus` (20000 = ok).
type fakeProvider struct {
	calls      atomic.Int32
	mu         sync.Mutex
	paths      []string
	bodies     []map[string]any
	taskStatus int
	result     func(path string) any
}

func (f *fakeProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	var body []map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	f.paths = append(f.paths, r.URL.Path)
	if len(body) > 0 {
		f.bodies = append(f.bodies, body[0])
	}
	f.mu.Unlock()
	status := f.taskStatus
	if status == 0 {
		status = 20000
	}
	task := map[string]any{"status_code": status, "status_message": "task message", "cost": 0.01, "path": strings.Split(strings.Trim(r.URL.Path, "/"), "/")}
	if status == 20000 && f.result != nil {
		task["result"] = []any{f.result(r.URL.Path)}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "tasks": []any{task}})
}

func (f *fakeProvider) lastBody(t *testing.T) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bodies) == 0 {
		t.Fatal("provider received no request")
	}
	return f.bodies[len(f.bodies)-1]
}

func newTestService(t *testing.T, f *fakeProvider, c cache) *Service {
	t.Helper()
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	client, err := dataforseo.NewClient(dataforseo.Options{
		BaseURL: server.URL,
		APIKey:  base64.StdEncoding.EncodeToString([]byte("fake-user:fake-pass")),
		Recorder: costRecorderFunc(func(context.Context, string, dataforseo.Cost) error {
			return nil
		}),
		MaxRetries: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewServiceWithCache(client, c, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return fixedNow })
}

func rankedKeywordsResult(total int, items ...any) func(string) any {
	return func(string) any { return map[string]any{"total_count": total, "items": items} }
}

func rankedItem(keyword string, rank, etv, volume, cpc, difficulty float64, url, relativeURL string) map[string]any {
	serp := map[string]any{"rank_absolute": rank, "etv": etv}
	if url != "" {
		serp["url"] = url
	}
	if relativeURL != "" {
		serp["relative_url"] = relativeURL
	}
	return map[string]any{
		"keyword_data": map[string]any{
			"keyword":            keyword,
			"keyword_info":       map[string]any{"search_volume": volume, "cpc": cpc},
			"keyword_properties": map[string]any{"keyword_difficulty": difficulty},
		},
		"ranked_serp_element": map[string]any{"serp_item": serp},
	}
}

func floatValue(t *testing.T, v *float64) float64 {
	t.Helper()
	if v == nil {
		t.Fatal("value is nil")
	}
	return *v
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(b.String())
}
