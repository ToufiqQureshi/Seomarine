package razorpay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreateSubscription(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		if r.Method != http.MethodPost || r.URL.Path != "/v1/subscriptions" || !ok || id != "rzp_test_key" || secret != "key-secret" {
			t.Errorf("request = %s %s, basic auth %q:%q", r.Method, r.URL.Path, id, secret)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		var body struct {
			PlanID         string            `json:"plan_id"`
			TotalCount     int               `json:"total_count"`
			CustomerNotify bool              `json:"customer_notify"`
			Notes          map[string]string `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.PlanID != "plan_pro" || body.TotalCount != 120 || !body.CustomerNotify || body.Notes["organization_id"] != "org-1" {
			t.Errorf("request body = %+v", body)
		}
		_, _ = io.WriteString(w, `{"id":"sub_123","entity":"subscription","status":"created","current_start":null,
			"current_end":null,"created_at":1767225600,"notes":{"organization_id":"org-1"},"short_url":"https://rzp.io/i/x"}`)
	}))
	defer srv.Close()

	sub, err := NewClient(srv.URL+"/v1", "rzp_test_key", "key-secret").
		CreateSubscription(context.Background(), "plan_pro", 120, map[string]string{"organization_id": "org-1"})
	if err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
	if sub.ID != "sub_123" || sub.Status != "created" || sub.CurrentEnd != nil || sub.CreatedAt != 1767225600 {
		t.Fatalf("CreateSubscription() = %+v", sub)
	}
}

func TestCreateSubscriptionFailures(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantAPIErr *APIError
	}{
		{
			name: "4xx with Razorpay's error body", status: http.StatusBadRequest,
			body:       `{"error":{"code":"BAD_REQUEST_ERROR","description":"The id provided does not exist","source":"business"}}`,
			wantAPIErr: &APIError{StatusCode: 400, Code: "BAD_REQUEST_ERROR", Description: "The id provided does not exist"},
		},
		{
			name: "wrong key pair", status: http.StatusUnauthorized,
			body:       `{"error":{"code":"BAD_REQUEST_ERROR","description":"Authentication failed"}}`,
			wantAPIErr: &APIError{StatusCode: 401, Code: "BAD_REQUEST_ERROR", Description: "Authentication failed"},
		},
		{
			name: "5xx with an HTML body", status: http.StatusBadGateway, body: "<html>bad gateway</html>",
			wantAPIErr: &APIError{StatusCode: 502, Description: "Bad Gateway"},
		},
		{name: "2xx that is not JSON", status: http.StatusOK, body: "ok"},
		{name: "2xx without a subscription id", status: http.StatusOK, body: `{"status":"created"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()

			sub, err := NewClient(srv.URL, "k", "s").CreateSubscription(context.Background(), "plan_pro", 120, nil)
			if err == nil {
				t.Fatalf("CreateSubscription() = %+v, want an error", sub)
			}
			var apiErr *APIError
			if got := errors.As(err, &apiErr); got != (tt.wantAPIErr != nil) {
				t.Fatalf("error = %v, is an APIError = %v", err, got)
			}
			if tt.wantAPIErr != nil && *apiErr != *tt.wantAPIErr {
				t.Errorf("APIError = %+v, want %+v", *apiErr, *tt.wantAPIErr)
			}
		})
	}
}

// A hung Razorpay must not hang checkout: the client gives up at its timeout.
func TestCreateSubscriptionTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	c := NewClient(srv.URL, "k", "s")
	if c.http.Timeout != requestTimeout || requestTimeout > 30*time.Second {
		t.Fatalf("client timeout = %v, want %v and under the server's write timeout", c.http.Timeout, requestTimeout)
	}
	c.http.Timeout = 50 * time.Millisecond
	start := time.Now()
	_, err := c.CreateSubscription(context.Background(), "plan_pro", 120, nil)
	if err == nil {
		t.Fatal("CreateSubscription() succeeded against a hung server")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("CreateSubscription() took %v, want it to stop at the timeout", elapsed)
	}
}

func TestValidSignature(t *testing.T) {
	const secret = "webhook-secret"
	body := []byte(`{"event":"subscription.activated","payload":{}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	valid := hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name      string
		body      []byte
		signature string
		secret    string
		want      bool
	}{
		{name: "valid", body: body, signature: valid, secret: secret, want: true},
		{name: "missing", body: body, signature: "", secret: secret},
		{name: "tampered body", body: append([]byte(" "), body...), signature: valid, secret: secret},
		{name: "other secret", body: body, signature: valid, secret: "other-secret"},
		{name: "truncated", body: body, signature: valid[:len(valid)-2], secret: secret},
		{name: "not hex", body: body, signature: strings.Repeat("z", len(valid)), secret: secret},
		{name: "base64 instead of hex", body: body, signature: "c2lnbmF0dXJl", secret: secret},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidSignature(tt.body, tt.signature, tt.secret); got != tt.want {
				t.Errorf("ValidSignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubscriptionNote(t *testing.T) {
	tests := []struct {
		name  string
		notes string
		want  string
	}{
		{name: "object", notes: `{"organization_id":"org-1"}`, want: "org-1"},
		{name: "other keys only", notes: `{"source":"web"}`},
		{name: "empty notes are an array", notes: `[]`},
		{name: "null", notes: `null`},
		{name: "missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Subscription{Notes: json.RawMessage(tt.notes)}).Note("organization_id"); got != tt.want {
				t.Errorf("Note() = %q, want %q", got, tt.want)
			}
		})
	}
}
