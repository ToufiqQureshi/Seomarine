package billing

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
)

type creditBalanceStub struct {
	balance int64
	err error
	orgID string
}

func (s *creditBalanceStub) GetBalance(_ context.Context, orgID string) (int64, error) {
	s.orgID = orgID
	return s.balance, s.err
}

func TestCreditBalanceHandler(t *testing.T) {
	tests := []struct {
		name string
		credits CreditBalanceReader
		user auth.User
		wantStatus int
		wantBody string
		wantOrgID string
	}{
		{
			name: "returns active organization balance",
			credits: &creditBalanceStub{balance: 1234},
			user: auth.User{OrganizationID: "org-a"},
			wantStatus: http.StatusOK,
			wantBody: `"balanceCredits":1234`,
			wantOrgID: "org-a",
		},
		{
			name: "rejects missing organization",
			credits: &creditBalanceStub{balance: 1234},
			user: auth.User{},
			wantStatus: http.StatusForbidden,
			wantBody: "no_organization",
		},
		{
			name: "ledger unavailable",
			wantStatus: http.StatusServiceUnavailable,
			wantBody: "billing_unavailable",
		},
		{
			name: "database failure",
			credits: &creditBalanceStub{err: errors.New("database unavailable")},
			user: auth.User{OrganizationID: "org-a"},
			wantStatus: http.StatusInternalServerError,
			wantBody: "internal",
			wantOrgID: "org-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/credits", nil)
			req = req.WithContext(auth.WithUser(req.Context(), tt.user))
			rec := httptest.NewRecorder()
			creditBalanceHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), tt.credits).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body %q does not contain %q", rec.Body.String(), tt.wantBody)
			}
			if stub, ok := tt.credits.(*creditBalanceStub); ok && stub.orgID != tt.wantOrgID {
				t.Fatalf("balance lookup organization = %q, want %q", stub.orgID, tt.wantOrgID)
			}
		})
	}
}
