package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONLimit(t *testing.T) {
	t.Run("decodes valid JSON within limit", func(t *testing.T) {
		var got struct {
			Name string `json:"name"`
		}
		body := bytes.NewReader([]byte(`{"name":"seomarine"}`))
		if err := DecodeJSONLimit(body, 1024, &got); err != nil {
			t.Fatalf("DecodeJSONLimit returned error: %v", err)
		}
		if got.Name != "seomarine" {
			t.Fatalf("name = %q, want %q", got.Name, "seomarine")
		}
	})

	t.Run("rejects payloads larger than max", func(t *testing.T) {
		var got struct{}
		body := bytes.NewReader(bytes.Repeat([]byte("a"), 2048))
		if err := DecodeJSONLimit(body, 1024, &got); err == nil {
			t.Fatal("DecodeJSONLimit() = nil, want error")
		}
	})
}

func TestParsePageLimit(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		page, limit, err := ParsePageLimit(httptest.NewRequest(http.MethodGet, "/?page=222&limit=50", http.NoBody).URL.Query())
		if err != nil || page != 222 || limit != 50 {
			t.Fatalf("ParsePageLimit() = (%d, %d), want (222, 50)", page, limit)
		}
	})

	t.Run("validation", func(t *testing.T) {
		_, _, err := ParsePageLimit(httptest.NewRequest(http.MethodGet, "/?page=0&limit=999", http.NoBody).URL.Query())
		if !errors.Is(err, ErrInvalidPage) {
			t.Fatalf("ParsePageLimit() error = %v, want ErrInvalidPage", err)
		}
	})

	t.Run("rejects duplicate page parameters", func(t *testing.T) {
		_, _, err := ParsePageLimit(httptest.NewRequest(http.MethodGet, "/?page=1&page=2", http.NoBody).URL.Query())
		if !errors.Is(err, ErrInvalidPage) {
			t.Fatalf("ParsePageLimit() error = %v, want ErrInvalidPage", err)
		}
	})
}

func TestWriteJSONAndError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusAccepted, map[string]string{"status": "ok"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("WriteJSON status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("WriteJSON body is not valid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %q, want %q", body["status"], "ok")
	}

	rec = httptest.NewRecorder()
	WriteError(rec, http.StatusBadRequest, "invalid_request", "bad payload")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("WriteError status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var errBody struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("WriteError body is not valid JSON: %v", err)
	}
	if errBody.Error.Code != "invalid_request" || errBody.Error.Message != "bad payload" {
		t.Fatalf("WriteError body = %+v, want code=%q message=%q", errBody.Error, "invalid_request", "bad payload")
	}
}

func TestRequestLoggingSetsSafeRequestID(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		keep bool
	}{
		{name: "single valid upstream ID", ids: []string{"edge-123_a.b"}, keep: true},
		{name: "invalid ID", ids: []string{"line\nbreak"}},
		{name: "duplicate IDs", ids: []string{"first", "second"}},
		{name: "oversized ID", ids: []string{strings.Repeat("a", 129)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := RequestLogging(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if w.Header().Get("X-Request-Id") == "" {
					t.Error("request ID header not set before handler")
				}
				w.WriteHeader(http.StatusAccepted)
			}))
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			for _, id := range tt.ids {
				req.Header.Add("X-Request-Id", id)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			got := rec.Header().Get("X-Request-Id")
			if got == "" || tt.keep && got != tt.ids[0] {
				t.Fatalf("response request ID = %q, keep input = %v", got, tt.keep)
			}
			if !validRequestID(got) {
				t.Errorf("response request ID %q is not safe", got)
			}
		})
	}
}
