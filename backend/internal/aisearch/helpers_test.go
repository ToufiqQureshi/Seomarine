package aisearch

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/toufiqqureshi/seomarine/backend/internal/kv"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

// taskReply is what the fake provider answers with, as a task of an HTTP 200
// response, which is how DataForSEO reports task failures too.
type taskReply struct {
	status  int
	message string
	result  []any
}

func okReply(result ...any) taskReply {
	return taskReply{status: 20000, message: "Ok.", result: result}
}

func failReply(status int, message string) taskReply {
	return taskReply{status: status, message: message}
}

type recordedRequest struct {
	method string
	path   string
	body   string
}

// fakeAPI is a DataForSEO stand-in that records every request.
type fakeAPI struct {
	server *httptest.Server

	mu       sync.Mutex
	requests []recordedRequest
	// replies overrides the default reply of a path; call counts from 1.
	replies  map[string]func(call int, body string) taskReply
	recorder *countingRecorder
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	api := &fakeAPI{replies: map[string]func(int, string) taskReply{}, recorder: &countingRecorder{}}
	api.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read fake API request: %v", err)
		}
		api.mu.Lock()
		api.requests = append(api.requests, recordedRequest{r.Method, r.URL.Path, string(body)})
		call := 0
		for _, req := range api.requests {
			if req.path == r.URL.Path {
				call++
			}
		}
		reply := defaultReply(r.URL.Path)
		if custom, ok := api.replies[r.URL.Path]; ok {
			reply = custom(call, string(body))
		}
		api.mu.Unlock()

		task := map[string]any{
			"status_code": reply.status, "status_message": reply.message,
			"path": strings.Split(strings.Trim(r.URL.Path, "/"), "/"), "cost": 0.0025, "result": reply.result,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "status_message": "Ok.", "tasks": []any{task}}); err != nil {
			t.Errorf("write fake API response: %v", err)
		}
	}))
	t.Cleanup(api.server.Close)
	return api
}

// on overrides the reply of path.
func (a *fakeAPI) on(path string, reply func(call int, body string) taskReply) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.replies[path] = reply
}

func (a *fakeAPI) requestsTo(path string) []recordedRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	var matched []recordedRequest
	for _, r := range a.requests {
		if r.path == path {
			matched = append(matched, r)
		}
	}
	return matched
}

func (a *fakeAPI) total() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.requests)
}

// defaultReply answers every endpoint with a small, valid payload.
func defaultReply(path string) taskReply {
	switch {
	case path == mentionsSearchPath:
		return okReply(map[string]any{"items": []any{
			map[string]any{
				"question": "best seo tool", "ai_search_volume": 1200.4,
				"sources":          []any{map[string]any{"url": "https://acme.com/blog/post", "title": "Post"}},
				"monthly_searches": []any{map[string]any{"year": 2026, "month": 9, "search_volume": 1000}},
				"brand_entities":   []any{map[string]any{"title": "Acme"}},
			},
		}})
	case path == aggregatedPath:
		return okReply(map[string]any{"total": map[string]any{"platform": []any{
			map[string]any{"key": platformChatGPT, "mentions": 10, "ai_search_volume": 100},
			map[string]any{"key": platformGoogle, "mentions": 5, "ai_search_volume": 50},
		}}})
	case path == topPagesPath:
		return okReply(map[string]any{"items": []any{map[string]any{
			"key": "https://acme.com/blog/post",
			"platform": []any{
				map[string]any{"key": platformChatGPT, "mentions": 3, "ai_search_volume": 30},
				map[string]any{"key": platformGoogle, "mentions": 4, "ai_search_volume": 40},
			},
		}}})
	case path == crossAggregatedPath:
		return okReply(map[string]any{"items": []any{
			map[string]any{"key": "acme.com", "platform": []any{map[string]any{"key": "google", "mentions": 30}}},
			map[string]any{"key": "rival.com", "platform": []any{map[string]any{"key": "google", "mentions": 10}}},
		}})
	case strings.HasSuffix(path, "/llm_responses/models"):
		return okReply(
			map[string]any{"model_name": "gpt-5.6-luna"}, map[string]any{"model_name": "gpt-5.5"},
			map[string]any{"model_name": "claude-sonnet-5"}, map[string]any{"model_name": "gemini-2.5-pro"},
			map[string]any{"model_name": "sonar-reasoning-pro"})
	case strings.HasSuffix(path, "/llm_responses/live"):
		return okReply(answer(true, "Use Acme. It is great.", "https://acme.com/review"))
	}
	return failReply(40400, "Not Found.")
}

// answer is a model response; withSources adds one cited page.
func answer(webSearch bool, text string, sources ...string) map[string]any {
	annotations := []any{}
	for _, s := range sources {
		annotations = append(annotations, map[string]any{"title": "Source", "url": s})
	}
	return map[string]any{
		"model_name": "gpt-5", "web_search": webSearch, "output_tokens": 12.0,
		"items": []any{map[string]any{"type": "message", "sections": []any{map[string]any{"type": "text", "text": text, "annotations": annotations}}}},
	}
}

type countingRecorder struct {
	mu    sync.Mutex
	costs []dataforseo.Cost
}

func (r *countingRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.costs)
}

func (r *countingRecorder) RecordDataForSEO(_ context.Context, _ string, cost dataforseo.Cost) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.costs = append(r.costs, cost)
	return nil
}

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_REDIS_URL must be set in CI")
		}
		t.Skip("TEST_REDIS_URL not set; skipping Redis integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := kv.Open(ctx, url)
	if err != nil {
		t.Fatalf("open redis: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close redis: %v", err)
		}
	})
	return client
}

// newTestService returns a Service wired to api and a real Redis. Every call
// should use a fresh organization id (see newOrg) so cache entries never leak
// between tests.
func newTestService(t *testing.T, api *fakeAPI) *Service {
	t.Helper()
	client, err := dataforseo.NewClient(dataforseo.Options{
		BaseURL:  api.server.URL,
		APIKey:   base64.StdEncoding.EncodeToString([]byte("login:password")),
		Recorder: api.recorder,
	})
	if err != nil {
		t.Fatalf("create DataForSEO client: %v", err)
	}
	return NewService(client, testRedis(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func newOrg() string { return "org-" + rand.Text() }
