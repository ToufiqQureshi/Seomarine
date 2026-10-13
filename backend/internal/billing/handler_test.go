package billing

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
)

func TestCancelHandlerRefusesBeforeTouchingBilling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// A Service with no database or Razorpay client: any use of it panics,
	// so a passing case proves the request was refused first.
	configured := &Service{}
	tests := []struct {
		name     string
		svc      *Service
		user     *auth.User
		wantCode int
		wantBody string
	}{
		{name: "billing not set up", svc: nil, user: &auth.User{OrganizationID: "org-1", Role: "owner"}, wantCode: http.StatusServiceUnavailable, wantBody: "billing_unavailable"},
		{name: "no workspace open", svc: configured, user: &auth.User{Role: "owner"}, wantCode: http.StatusForbidden, wantBody: "no_organization"},
		{name: "signed out", svc: configured, user: nil, wantCode: http.StatusForbidden, wantBody: "no_organization"},
		{name: "member", svc: configured, user: &auth.User{OrganizationID: "org-1", Role: "member"}, wantCode: http.StatusForbidden, wantBody: `"forbidden"`},
		{name: "no role", svc: configured, user: &auth.User{OrganizationID: "org-1"}, wantCode: http.StatusForbidden, wantBody: `"forbidden"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.user != nil {
				ctx = auth.WithUser(ctx, *tt.user)
			}
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/billing/cancel", nil)
			rec := httptest.NewRecorder()
			cancelHandler(logger, tt.svc).ServeHTTP(rec, req)
			if rec.Code != tt.wantCode || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("status %d body %s, want %d with %s", rec.Code, rec.Body.String(), tt.wantCode, tt.wantBody)
			}
		})
	}
}
