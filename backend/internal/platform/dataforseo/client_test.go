package dataforseo

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordedCost struct {
	organizationID string
	cost           Cost
}

type testRecorder struct {
	mu    sync.Mutex
	costs []recordedCost
	err   error
}

func (r *testRecorder) RecordDataForSEO(_ context.Context, organizationID string, cost Cost) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.costs = append(r.costs, recordedCost{organizationID: organizationID, cost: cost})
	return r.err
}

func (r *testRecorder) snapshot() []recordedCost {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedCost(nil), r.costs...)
}

func testClient(t *testing.T, server *httptest.Server, recorder CostRecorder, retries int) *Client {
	t.Helper()
	client, err := NewClient(Options{
		BaseURL: server.URL, APIKey: base64.StdEncoding.EncodeToString([]byte("user:secret")),
		Recorder: recorder, HTTPClient: server.Client(), Timeout: 2 * time.Second,
		MaxRetries: retries, RetryBackoff: time.Nanosecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestDoAuthenticatesRecordsPerOrganizationAndReturnsTaskFailures(t *testing.T) {
	var authorization, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"tasks":[{"path":["v3","serp","google","organic","live","advanced"],"cost":0.002,"status_code":40501}]}`)
	}))
	defer server.Close()
	recorder := &testRecorder{}
	client := testClient(t, server, recorder, 0)
	response, err := client.Do(context.Background(), "org-a", http.MethodPost, "/v3/serp/live/advanced", []byte(`[{"keyword":"café"}]`), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(response) == 0 || authorization != "Basic "+base64.StdEncoding.EncodeToString([]byte("user:secret")) {
		t.Fatalf("response=%s authorization=%q", response, authorization)
	}
	if body != `[{"keyword":"café"}]` {
		t.Fatalf("request body = %q", body)
	}
	costs := recorder.snapshot()
	if len(costs) != 1 || costs[0].organizationID != "org-a" || costs[0].cost.USD != 0.002 {
		t.Fatalf("recorded costs = %#v", costs)
	}
}

func TestDoRetriesOnlyRetrySafeRequests(t *testing.T) {
	for _, retrySafe := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry-safe=%t", retrySafe), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				if requests == 1 {
					http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
					return
				}
				_, _ = io.WriteString(w, `{}`)
			}))
			defer server.Close()
			client := testClient(t, server, &testRecorder{}, 1)
			_, err := client.Do(context.Background(), "org", http.MethodGet, "/v3/appendix/status", nil, retrySafe)
			if retrySafe && err != nil {
				t.Fatalf("retry-safe Do() error = %v", err)
			}
			if !retrySafe {
				if apiErr, ok := errors.AsType[*HTTPError](err); !ok || apiErr.StatusCode != http.StatusServiceUnavailable {
					t.Fatalf("non-retry-safe error = %v", err)
				}
			}
			wantRequests := 1
			if retrySafe {
				wantRequests = 2
			}
			if requests != wantRequests {
				t.Fatalf("requests = %d, want %d", requests, wantRequests)
			}
		})
	}
}

func TestDoValidatesPathAndOrganization(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := testClient(t, server, &testRecorder{}, 0)
	for _, input := range []struct{ organizationID, path string }{
		{"", "/v3/serp"}, {"org", "https://example.com/steal"}, {"org", "//example.com/steal"},
	} {
		if _, err := client.Do(context.Background(), input.organizationID, http.MethodGet, input.path, nil, true); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("Do(%q, %q) error = %v", input.organizationID, input.path, err)
		}
	}
}

func TestNewClientRejectsMissingCostRecorder(t *testing.T) {
	_, err := NewClient(Options{APIKey: base64.StdEncoding.EncodeToString([]byte("user:secret"))})
	if err == nil || !strings.Contains(err.Error(), "cost recorder") {
		t.Fatalf("NewClient() error = %v, want cost recorder error", err)
	}
}

func TestDoDoesNotFollowRedirects(t *testing.T) {
	requests := 0
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", destination.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client := testClient(t, redirect, &testRecorder{}, 0)
	_, err := client.Do(context.Background(), "org", http.MethodGet, "/v3/appendix/status", nil, false)
	if apiErr, ok := errors.AsType[*HTTPError](err); !ok || apiErr.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("Do() error = %v, want HTTP 307", err)
	}
	if requests != 0 {
		t.Fatalf("redirect destination received %d requests, want 0", requests)
	}
}

func TestDoReturnsCostRecordingFailureWithReconciliationData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tasks":[{"path":["v3","serp"],"cost":0.01}]}`)
	}))
	defer server.Close()
	recorder := &testRecorder{err: errors.New("ledger unavailable")}
	client := testClient(t, server, recorder, 0)
	_, err := client.Do(context.Background(), "org-123", http.MethodPost, "/v3/serp/live", []byte(`[]`), false)
	if recordingErr, ok := errors.AsType[*CostRecordingError](err); !ok || recordingErr.OrganizationID != "org-123" || recordingErr.Cost.USD != 0.01 {
		t.Fatalf("Do() error = %#v, want cost recording context", err)
	}
}
