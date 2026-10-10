package google

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeTokenSource struct{ calls []string }

func (f *fakeTokenSource) AccessToken(_ context.Context, _, _, _, rejected string) (string, error) {
	f.calls = append(f.calls, rejected)
	if rejected != "" {
		return "fresh", nil
	}
	return "stale", nil
}

func TestAPIClientRefreshesOnceAfter401(t *testing.T) {
	tokens := &fakeTokenSource{}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") == "Bearer stale" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"value": "ok"})
	}))
	defer server.Close()
	client := &APIClient{Tokens: tokens, Client: server.Client()}
	var result struct {
		Value string `json:"value"`
	}
	if err := client.DoJSON(t.Context(), APIRequest{UserID: "user", Provider: "gsc", Method: http.MethodGet, URL: server.URL, Response: &result, Retryable: true}); err != nil {
		t.Fatal(err)
	}
	if result.Value != "ok" || calls != 2 || len(tokens.calls) != 2 || tokens.calls[1] != "stale" {
		t.Fatalf("result=%+v, calls=%d, tokens=%v", result, calls, tokens.calls)
	}
}

func TestAPIClientStopsAfterSecond401(t *testing.T) {
	tokens := &fakeTokenSource{}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	client := &APIClient{Tokens: tokens, Client: server.Client()}
	var result any
	err := client.DoJSON(t.Context(), APIRequest{UserID: "user", Provider: "gsc", Method: http.MethodGet, URL: server.URL, Response: &result, Retryable: true})
	if apiErr, ok := errors.AsType[APIError](err); !ok || apiErr.Status != http.StatusUnauthorized || calls != 2 {
		t.Fatalf("error=%v, calls=%d", err, calls)
	}
}

func TestAPIClientDoesNotRetryNonIdempotentRequest(t *testing.T) {
	serverCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		serverCalls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := &APIClient{Tokens: &fakeTokenSource{}, Client: server.Client()}
	var result any
	err := client.DoJSON(t.Context(), APIRequest{UserID: "user", Provider: "gsc", Method: http.MethodPost, URL: server.URL, Body: map[string]string{"x": "y"}, Response: &result})
	if _, ok := errors.AsType[APIError](err); !ok || serverCalls != 1 {
		t.Fatalf("error=%v, calls=%d", err, serverCalls)
	}
}

func TestAPIErrorExposesOnlyKnownReasonAndRetryDelay(t *testing.T) {
	for _, test := range []struct {
		name, body, retryAfter string
		status, wantRetry      int
		wantReason             string
	}{
		{name: "disabled service", body: `{"error":{"errors":[{"reason":"SERVICE_DISABLED"}]}}`, status: http.StatusForbidden, wantReason: "SERVICE_DISABLED"},
		{name: "quota retry", body: `{"error":{"message":"private provider text"}}`, retryAfter: "60", status: http.StatusTooManyRequests, wantRetry: 60},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.retryAfter != "" {
					w.Header().Set("Retry-After", test.retryAfter)
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			client := &APIClient{Tokens: &fakeTokenSource{}, Client: server.Client()}
			var response any
			err := client.DoJSON(t.Context(), APIRequest{UserID: "user", Provider: "ga4", Method: http.MethodGet, URL: server.URL, Response: &response})
			apiErr, ok := errors.AsType[APIError](err)
			if !ok || apiErr.Status != test.status || apiErr.Reason != test.wantReason || apiErr.RetryAfterSeconds != test.wantRetry {
				t.Fatalf("error = %#v, want status=%d reason=%q retry=%d", err, test.status, test.wantReason, test.wantRetry)
			}
		})
	}
}
