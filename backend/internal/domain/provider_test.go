package domain

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

func TestProviderOverviewMapsDataForSEOResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/dataforseo_labs/google/domain_rank_overview/live" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		var body []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(body) != 1 || body[0]["target"] != "example.com" {
			t.Errorf("request body = %#v", body)
		}
		_, _ = w.Write([]byte(`{"status_code":20000,"tasks":[{"status_code":20000,"result":[{"items":[{"metrics":{"organic":{"etv":1234.6,"count":99.2}}}]}]}]}`))
	}))
	t.Cleanup(server.Close)

	client, err := dataforseo.NewClient(dataforseo.Options{
		BaseURL: server.URL,
		APIKey:  base64.StdEncoding.EncodeToString([]byte("user:pass")),
		Recorder: costRecorderFunc(func(context.Context, string, dataforseo.Cost) error {
			return nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := (&provider{client: client}).overview(t.Context(), "org-a", "example.com", 2840, "en")
	if err != nil {
		t.Fatal(err)
	}
	if got.Organic == nil || got.Organic.ETV == nil || *got.Organic.ETV != 1234.6 || got.Organic.Count == nil || *got.Organic.Count != 99.2 {
		t.Fatalf("overview = %+v", got)
	}
}

type costRecorderFunc func(context.Context, string, dataforseo.Cost) error

func (f costRecorderFunc) RecordDataForSEO(ctx context.Context, org string, cost dataforseo.Cost) error {
	return f(ctx, org, cost)
}

func TestProviderDoesNotRetryPaidCall(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"status_code":20000,"tasks":[{"status_code":40500,"status_message":"failed"}]} `))
	}))
	t.Cleanup(server.Close)
	client, err := dataforseo.NewClient(dataforseo.Options{
		BaseURL: server.URL,
		APIKey:  base64.StdEncoding.EncodeToString([]byte("user:pass")),
		Recorder: costRecorderFunc(func(context.Context, string, dataforseo.Cost) error {
			return nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&provider{client: client}).overview(t.Context(), "org-a", "example.com", 2840, "en")
	if err == nil || !strings.Contains(err.Error(), "40500") {
		t.Fatalf("overview error = %v, want task error", err)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
}
