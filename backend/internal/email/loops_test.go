package email

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSendTransactionalSendsPrivateTransactionalPayload(t *testing.T) {
	var gotAuthorization, gotContentType string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/transactional" {
			t.Errorf("request = %s %s, want POST /api/v1/transactional", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		forwarded := req.Clone(req.Context())
		forwarded.URL = mustParseURL(t, server.URL+req.URL.Path)
		return http.DefaultTransport.RoundTrip(forwarded)
	})}
	err := (LoopsClient{APIKey: "test-api-key", HTTP: client}).SendTransactional(
		context.Background(), "person@example.com", "invite-template", map[string]string{"name": "Asha"},
	)
	if err != nil {
		t.Fatalf("SendTransactional() error = %v", err)
	}
	if gotAuthorization != "Bearer test-api-key" || gotContentType != "application/json" {
		t.Fatalf("request headers = authorization %q, content type %q", gotAuthorization, gotContentType)
	}
	if gotBody["transactionalId"] != "invite-template" || gotBody["email"] != "person@example.com" || gotBody["addToAudience"] != false {
		t.Fatalf("transactional request body = %#v", gotBody)
	}
	variables, ok := gotBody["dataVariables"].(map[string]any)
	if !ok || variables["name"] != "Asha" {
		t.Fatalf("dataVariables = %#v", gotBody["dataVariables"])
	}
}

func TestSendTransactionalReturnsUnavailableForMissingConfigOrProviderFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		client LoopsClient
	}{
		{name: "missing API key", client: LoopsClient{APIKey: "  ", HTTP: http.DefaultClient}},
		{name: "missing template", client: LoopsClient{APIKey: "key", HTTP: http.DefaultClient}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.client.SendTransactional(context.Background(), "person@example.com", " ", nil)
			if !errors.Is(err, ErrLoopsUnavailable) {
				t.Fatalf("SendTransactional() error = %v, want ErrLoopsUnavailable", err)
			}
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "private provider detail", http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		forwarded := req.Clone(req.Context())
		forwarded.URL = mustParseURL(t, server.URL+req.URL.Path)
		return http.DefaultTransport.RoundTrip(forwarded)
	})}
	err := (LoopsClient{APIKey: "key", HTTP: client}).SendTransactional(context.Background(), "person@example.com", "template", nil)
	if !errors.Is(err, ErrLoopsUnavailable) || strings.Contains(err.Error(), "private provider detail") {
		t.Fatalf("provider error = %v, want sanitized ErrLoopsUnavailable", err)
	}
}

func TestSendTransactionalHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		t.Fatal("canceled request reached the provider with a live context")
		return nil, errors.New("unexpected live request")
	})}
	err := (LoopsClient{APIKey: "key", HTTP: client}).SendTransactional(ctx, "person@example.com", "template", nil)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrLoopsUnavailable) {
		t.Fatalf("SendTransactional() error = %v, want cancellation and ErrLoopsUnavailable", err)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
