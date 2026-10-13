package google

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
)

type fakePlans struct {
	allowed bool
	err     error
	asked   []string
}

func (f *fakePlans) HasPaidPlan(_ context.Context, organizationID string) (bool, error) {
	f.asked = append(f.asked, organizationID)
	return f.allowed, f.err
}

func TestStartingAGoogleConnectionNeedsAPlanThatIncludesIt(t *testing.T) {
	start := func(plans PaidPlans) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		MountOAuth(mux, OAuthDeps{
			Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
			WithSession: func(next http.Handler) http.Handler { return next },
			Plans:       plans,
		})
		r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/google/oauth/gsc/start", strings.NewReader(`{"callbackURL":"/settings"}`))
		r = r.WithContext(auth.WithUser(r.Context(), auth.User{ID: "user_1", OrganizationID: "org_1", Role: "owner"}))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		return rec
	}

	refused := &fakePlans{}
	if rec := start(refused); rec.Code != http.StatusPaymentRequired || !strings.Contains(rec.Body.String(), "plan_required") {
		t.Errorf("free plan: status %d, body %s; want 402 plan_required", rec.Code, rec.Body)
	}
	if len(refused.asked) != 1 || refused.asked[0] != "org_1" {
		t.Errorf("plan was checked for %v, want the workspace of the user", refused.asked)
	}
	if rec := start(&fakePlans{err: errors.New("database down")}); rec.Code != http.StatusInternalServerError {
		t.Errorf("failing plan check: status %d, want 500", rec.Code)
	}
	// Allowed, and no plans at all, both reach the consent service, which
	// these tests leave unconfigured.
	if rec := start(&fakePlans{allowed: true}); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("included: status %d, want 503 from the unconfigured consent service", rec.Code)
	}
	if rec := start(nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no billing: status %d, want 503 from the unconfigured consent service", rec.Code)
	}
}
