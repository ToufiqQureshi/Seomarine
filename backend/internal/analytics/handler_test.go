package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollectRejectsInvalidEvents(t *testing.T) {
	long := "https://acme.com/" + strings.Repeat("a", maxURL)
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "not JSON", body: "k=abc", wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "empty body", wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "wrong type", body: `{"k":"abc","u":"https://acme.com/","w":"wide"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "missing site key", body: `{"u":"https://acme.com/"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "site key too long", body: fmt.Sprintf(`{"k":%q,"u":"https://acme.com/"}`, strings.Repeat("k", maxSiteKey+1)), wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "relative URL", body: `{"k":"abc","u":"/pricing"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "non-http URL", body: `{"k":"abc","u":"file:///etc/passwd"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "URL without host", body: `{"k":"abc","u":"https:///x"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "URL too long", body: fmt.Sprintf(`{"k":"abc","u":%q}`, long[:maxURL+1]), wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "referrer too long", body: fmt.Sprintf(`{"k":"abc","u":"https://acme.com/","r":%q}`, long[:maxURL+1]), wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "negative width", body: `{"k":"abc","u":"https://acme.com/","w":-1}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "absurd width", body: `{"k":"abc","u":"https://acme.com/","w":100001}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "body over the limit", body: fmt.Sprintf(`{"k":"abc","u":"https://acme.com/","x":%q}`, strings.Repeat("x", maxCollectBody)), wantStatus: http.StatusRequestEntityTooLarge, wantCode: "payload_too_large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			collect(nil, nil, nil)(rec, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/collect", strings.NewReader(tt.body)))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			var got struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if got.Error.Code != tt.wantCode {
				t.Fatalf("error code = %q, want %q", got.Error.Code, tt.wantCode)
			}
			if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
				t.Errorf("CORS origin = %q, want *", rec.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

func TestCollectPreflight(t *testing.T) {
	rec := httptest.NewRecorder()
	collectPreflight(rec, httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/collect", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	for name, want := range map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "POST",
		"Access-Control-Allow-Headers": "Content-Type",
	} {
		if got := rec.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}
