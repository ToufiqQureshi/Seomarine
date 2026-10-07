package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type pingerFunc func(ctx context.Context) error

func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestRoutes(t *testing.T) {
	healthy := pingerFunc(func(context.Context) error { return nil })
	down := pingerFunc(func(context.Context) error { return errors.New("connection refused") })

	tests := []struct {
		name       string
		db         Pinger
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "liveness is up", db: healthy, method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK, wantBody: `{"status":"ok"}`},
		{name: "liveness ignores the database", db: down, method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK, wantBody: `{"status":"ok"}`},
		{name: "ready when the database answers", db: healthy, method: http.MethodGet, path: "/readyz", wantStatus: http.StatusOK, wantBody: `{"status":"ok"}`},
		{name: "not ready when the database is down", db: down, method: http.MethodGet, path: "/readyz", wantStatus: http.StatusServiceUnavailable, wantBody: `{"status":"unavailable"}`},
		{name: "rejects other methods", db: healthy, method: http.MethodPost, path: "/healthz", wantStatus: http.StatusMethodNotAllowed},
		{name: "unknown path is 404", db: healthy, method: http.MethodGet, path: "/nope", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewHandler(discardLogger, tt.db).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody == "" {
				return
			}
			if got := rec.Body.String(); got != tt.wantBody+"\n" {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q", ct)
			}
		})
	}
}

// A hung database must not hang the readiness probe: the ping gets a deadline.
func TestReadyzBoundsTheDatabasePing(t *testing.T) {
	hung := pingerFunc(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("ping context has no deadline")
		} else if time.Until(deadline) > readinessTimeout {
			t.Errorf("deadline %v is longer than %v", time.Until(deadline), readinessTimeout)
		}
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	NewHandler(discardLogger, hung).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
