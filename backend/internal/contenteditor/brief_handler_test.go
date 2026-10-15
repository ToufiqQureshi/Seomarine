package contenteditor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
)

type briefGetterFake struct {
	calls   int
	input   BriefInput
	refresh bool
	org     string
	err     error
}

func (f *briefGetterFake) Get(_ context.Context, org string, input BriefInput, refresh bool) (ContentBrief, error) {
	f.calls++
	f.org, f.input, f.refresh = org, input, refresh
	if f.err != nil {
		return ContentBrief{}, f.err
	}
	return ContentBrief{Keyword: input.Keyword, Questions: []string{"What is it?"}}, nil
}

type projectMarketFake struct{ err error }

func (p projectMarketFake) ProjectMarket(context.Context, string, string) (string, int, string, error) {
	return "example.com", 2356, "hi", p.err
}

type briefPlansFake struct {
	allowed bool
	err     error
}

func (p briefPlansFake) HasPaidPlan(context.Context, string) (bool, error) { return p.allowed, p.err }

func briefMux(getter BriefGetter, plans BriefPlans, market ProjectMarket) *http.ServeMux {
	mux := http.NewServeMux()
	pass := func(next http.Handler) http.Handler { return next }
	MountBrief(mux, BriefDeps{
		Briefs: getter, Projects: market, Plans: plans, WithSession: pass,
		WithProjectAccess: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org-1")))
			})
		},
	})
	return mux
}

func postBrief(mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/projects/p1/content/brief", strings.NewReader(body)))
	return rec
}

func briefErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Error.Code
}

func TestBriefRouteUsesTheProjectMarketAndOwnDomain(t *testing.T) {
	getter := &briefGetterFake{}
	rec := postBrief(briefMux(getter, briefPlansFake{allowed: true}, projectMarketFake{}), `{"keyword":"hotel revenue management","refresh":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	want := BriefInput{Keyword: "hotel revenue management", LocationCode: 2356, LanguageCode: "hi", OwnDomain: "example.com"}
	if getter.org != "org-1" || getter.input != want || !getter.refresh {
		t.Fatalf("Get(%q, %+v, %v)", getter.org, getter.input, getter.refresh)
	}
}

func TestBriefRouteRefusesBeforeAnyPaidCall(t *testing.T) {
	for _, tt := range []struct {
		name     string
		plans    BriefPlans
		market   ProjectMarket
		body     string
		status   int
		code     string
		getterIn BriefGetter
	}{
		{"free plan", briefPlansFake{allowed: false}, projectMarketFake{}, `{"keyword":"seo"}`, http.StatusPaymentRequired, "plan_required", &briefGetterFake{}},
		{"plan check down fails closed", briefPlansFake{err: errors.New("db down")}, projectMarketFake{}, `{"keyword":"seo"}`, http.StatusInternalServerError, "internal", &briefGetterFake{}},
		{"no plan service", nil, projectMarketFake{}, `{"keyword":"seo"}`, http.StatusServiceUnavailable, "brief_unavailable", &briefGetterFake{}},
		{"unknown field", briefPlansFake{allowed: true}, projectMarketFake{}, `{"keyword":"seo","locationCode":1}`, http.StatusBadRequest, "invalid_request", &briefGetterFake{}},
		{"project gone", briefPlansFake{allowed: true}, projectMarketFake{err: errors.New("not found")}, `{"keyword":"seo"}`, http.StatusNotFound, "project_not_found", &briefGetterFake{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			getter := tt.getterIn.(*briefGetterFake)
			rec := postBrief(briefMux(getter, tt.plans, tt.market), tt.body)
			if rec.Code != tt.status || briefErrorCode(t, rec) != tt.code || getter.calls != 0 {
				t.Fatalf("status = %d code %q calls %d; want %d %q and no fetch", rec.Code, briefErrorCode(t, rec), getter.calls, tt.status, tt.code)
			}
		})
	}
}

func TestBriefRouteMapsProviderAndCreditErrors(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{&ValidationError{Message: "bad keyword"}, http.StatusBadRequest, "validation_error"},
		{spend.ErrNoCredits, http.StatusPaymentRequired, "insufficient_credits"},
		{spend.ErrWorkspaceDailyLimit, http.StatusTooManyRequests, "daily_credit_limit"},
		{ErrBriefUnavailable, http.StatusBadGateway, "provider_error"},
		{errors.New("decode failed"), http.StatusBadGateway, "provider_error"},
	} {
		rec := postBrief(briefMux(&briefGetterFake{err: tt.err}, briefPlansFake{allowed: true}, projectMarketFake{}), `{"keyword":"seo"}`)
		if rec.Code != tt.status || briefErrorCode(t, rec) != tt.code {
			t.Errorf("%v: status = %d code %q, want %d %q", tt.err, rec.Code, briefErrorCode(t, rec), tt.status, tt.code)
		}
	}
}
