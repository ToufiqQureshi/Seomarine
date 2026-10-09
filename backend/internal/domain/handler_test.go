package domain

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
)

type planFunc func(context.Context, string) (bool, error)

func (f planFunc) HasPaidPlan(ctx context.Context, org string) (bool, error) { return f(ctx, org) }

var (
	paid   = planFunc(func(context.Context, string) (bool, error) { return true, nil })
	unpaid = planFunc(func(context.Context, string) (bool, error) { return false, nil })
)

type serveOptions struct {
	provider *fakeProvider
	plans    PaidPlans
	noOrg    bool
	noSvc    bool
	ctx      context.Context
}

func serve(t *testing.T, opts serveOptions, endpoint, body string) *httptest.ResponseRecorder {
	t.Helper()
	if opts.provider == nil {
		opts.provider = &fakeProvider{result: overviewResult(10, 5)}
	}
	var svc *Service
	if !opts.noSvc {
		svc = newTestService(t, opts.provider, &memoryCache{})
	}
	setOrg := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !opts.noOrg {
				r = r.WithContext(auth.WithProjectOrganization(r.Context(), "org-a"))
			}
			next.ServeHTTP(w, r)
		})
	}
	identity := func(next http.Handler) http.Handler { return next }
	mux := http.NewServeMux()
	Mount(mux, Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Service: svc, Plans: opts.plans,
		WithSession: identity, WithProjectAccess: setOrg,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/domain/"+endpoint, strings.NewReader(body))
	if opts.ctx != nil {
		req = req.WithContext(opts.ctx)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not the error contract: %q", rec.Body)
	}
	return out.Error.Code
}

func TestHandlerRoutesAndOverviewContract(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{result: overviewResult(1234.6, 99)}
	rec := serve(t, serveOptions{provider: provider, plans: paid}, "overview", `{"domain":"https://www.example.com/x","scope":"domain","locationCode":2826,"languageCode":" fr "}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"domain", "scope", "displayTarget", "organicTraffic", "organicKeywords", "backlinks", "referringDomains", "hasData", "fetchedAt"} {
		if _, ok := got[key]; !ok {
			t.Errorf("overview response lacks %q: %s", key, rec.Body)
		}
	}
	if got["organicTraffic"] != 1235.0 || got["backlinks"] != nil {
		t.Errorf("overview values = %v", got)
	}
	body := provider.lastBody(t)
	if body["location_code"] != 2826.0 || body["language_code"] != "fr" {
		t.Fatalf("explicit market not forwarded: %v", body)
	}
}

func TestHandlerOtherEndpointsReturnTheirContracts(t *testing.T) {
	t.Parallel()
	keywords := &fakeProvider{result: rankedKeywordsResult(1, rankedItem("seo", 1, 2, 3, 4, 5, "https://example.com/p", ""))}
	rec := serve(t, serveOptions{provider: keywords}, "keyword-suggestions", `{"domain":"example.com"}`)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), `[{"keyword":"seo"`) {
		t.Fatalf("suggestions: %d %s", rec.Code, rec.Body)
	}
	rec = serve(t, serveOptions{provider: keywords}, "keywords", `{"domain":"example.com","page":1,"pageSize":100,"sortMode":"rank","sortOrder":"asc","filters":{"minVol":1}}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"relativeUrl":"/p"`) || !strings.Contains(rec.Body.String(), `"hasMore":false`) {
		t.Fatalf("keywords: %d %s", rec.Code, rec.Body)
	}
	pages := &fakeProvider{result: func(string) any {
		return map[string]any{"total_count": 1, "items": []any{map[string]any{"page_address": "https://example.com/a"}}}
	}}
	rec = serve(t, serveOptions{provider: pages}, "pages", `{"domain":"example.com"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"relativePath":"/a"`) {
		t.Fatalf("pages: %d %s", rec.Code, rec.Body)
	}
}

func TestHandlerRejectsInvalidInputBeforeSpending(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 2049)
	emoji := strings.Repeat("😀", 1025) // 2050 UTF-16 units but only 1025 runes
	tests := []struct {
		name     string
		endpoint string
		body     string
	}{
		{"empty object", "overview", `{}`},
		{"blank domain", "overview", `{"domain":"   "}`},
		{"domain over 2048 units", "overview", `{"domain":"` + long + `"}`},
		{"domain over 2048 UTF-16 units", "overview", `{"domain":"` + emoji + `"}`},
		{"bad scope", "overview", `{"domain":"example.com","scope":"everything"}`},
		{"zero location", "overview", `{"domain":"example.com","locationCode":0}`},
		{"negative location", "keyword-suggestions", `{"domain":"example.com","locationCode":-5}`},
		{"one letter language", "overview", `{"domain":"example.com","languageCode":"e"}`},
		{"nine letter language", "overview", `{"domain":"example.com","languageCode":"abcdefghi"}`},
		{"unknown field", "overview", `{"domain":"example.com","extra":true}`},
		{"not json", "overview", `domain=example.com`},
		{"two json values", "overview", `{"domain":"example.com"} {"domain":"b.com"}`},
		{"body too large", "overview", `{"domain":"example.com","search":"` + strings.Repeat("x", 40<<10) + `"}`},
		{"string where number expected", "keywords", `{"domain":"example.com","filters":{"minVol":"100"}}`},
		{"page too large", "keywords", `{"domain":"example.com","page":1000001}`},
		{"negative page", "pages", `{"domain":"example.com","page":-1}`},
		{"page size 75", "pages", `{"domain":"example.com","pageSize":75}`},
		{"negative filter", "keywords", `{"domain":"example.com","filters":{"minCpc":-1}}`},
		{"min above max", "keywords", `{"domain":"example.com","filters":{"minKd":50,"maxKd":10}}`},
		{"include over 500", "keywords", `{"domain":"example.com","filters":{"include":"` + strings.Repeat("a", 501) + `"}}`},
		{"include with nine terms", "pages", `{"domain":"example.com","filters":{"include":"a,b,c,d,e,f,g,h,i"}}`},
		{"unknown sort mode", "keywords", `{"domain":"example.com","sortMode":"revenue"}`},
		{"pages cannot sort by rank", "pages", `{"domain":"example.com","sortMode":"rank"}`},
		{"private address", "overview", `{"domain":"192.168.0.1"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := &fakeProvider{result: overviewResult(1, 1)}
			rec := serve(t, serveOptions{provider: provider, plans: paid}, tc.endpoint, tc.body)
			if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_request" {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			if provider.calls.Load() != 0 {
				t.Fatalf("invalid input reached the paid provider %d times", provider.calls.Load())
			}
		})
	}
}

func TestHandlerPlanGate(t *testing.T) {
	t.Parallel()
	t.Run("free plan is refused before the body is read", func(t *testing.T) {
		t.Parallel()
		provider := &fakeProvider{result: overviewResult(1, 1)}
		rec := serve(t, serveOptions{provider: provider, plans: unpaid}, "overview", `not json at all`)
		if rec.Code != http.StatusPaymentRequired || errorCode(t, rec) != "payment_required" {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		if provider.calls.Load() != 0 {
			t.Fatal("a free plan reached the provider")
		}
	})
	t.Run("plan lookup failure is a server error", func(t *testing.T) {
		t.Parallel()
		failing := planFunc(func(context.Context, string) (bool, error) { return false, errors.New("db down") })
		rec := serve(t, serveOptions{plans: failing}, "overview", `{"domain":"example.com"}`)
		if rec.Code != http.StatusInternalServerError || errorCode(t, rec) != "internal" {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
	})
	t.Run("the plan is checked for the project organization", func(t *testing.T) {
		t.Parallel()
		var checked string
		plans := planFunc(func(_ context.Context, org string) (bool, error) { checked = org; return true, nil })
		if rec := serve(t, serveOptions{plans: plans}, "overview", `{"domain":"example.com"}`); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		if checked != "org-a" {
			t.Fatalf("plan checked for %q, want org-a", checked)
		}
	})
	t.Run("billing not configured leaves it open", func(t *testing.T) {
		t.Parallel()
		if rec := serve(t, serveOptions{}, "overview", `{"domain":"example.com"}`); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
	})
}

func TestHandlerServerSideFailures(t *testing.T) {
	t.Parallel()
	t.Run("no DataForSEO key", func(t *testing.T) {
		t.Parallel()
		rec := serve(t, serveOptions{noSvc: true}, "overview", `{"domain":"example.com"}`)
		if rec.Code != http.StatusServiceUnavailable || errorCode(t, rec) != "domain_unavailable" {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
	})
	t.Run("request without a project organization", func(t *testing.T) {
		t.Parallel()
		rec := serve(t, serveOptions{noOrg: true}, "overview", `{"domain":"example.com"}`)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
	})
	t.Run("provider balance problem", func(t *testing.T) {
		t.Parallel()
		rec := serve(t, serveOptions{provider: &fakeProvider{taskStatus: 40210}}, "overview", `{"domain":"example.com"}`)
		if rec.Code != http.StatusServiceUnavailable || errorCode(t, rec) != "provider_billing_issue" {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "task message") {
			t.Fatalf("provider text leaked to the client: %s", rec.Body)
		}
	})
	t.Run("provider failure", func(t *testing.T) {
		t.Parallel()
		rec := serve(t, serveOptions{provider: &fakeProvider{taskStatus: 50000}}, "pages", `{"domain":"example.com"}`)
		if rec.Code != http.StatusBadGateway || errorCode(t, rec) != "provider_error" {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
	})
	t.Run("caller went away", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		rec := serve(t, serveOptions{ctx: ctx}, "overview", `{"domain":"example.com"}`)
		if rec.Body.Len() != 0 {
			t.Fatalf("wrote a response to a cancelled request: %s", rec.Body)
		}
	})
	t.Run("only POST is routed", func(t *testing.T) {
		t.Parallel()
		mux := http.NewServeMux()
		identity := func(next http.Handler) http.Handler { return next }
		Mount(mux, Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), WithSession: identity, WithProjectAccess: identity})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/projects/p/domain/overview", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET status = %d, want 405", rec.Code)
		}
	})
}
