package backlinks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

type planResult bool

func (p planResult) HasPaidPlan(context.Context, string) (bool, error) { return bool(p), nil }

func requestHandler(t *testing.T, body string, plans PaidPlans, providerCalls *int) *httptest.ResponseRecorder {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*providerCalls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "tasks": []any{map[string]any{"status_code": 20000, "path": []string{"v3", "backlinks", "summary", "live"}, "cost": 0, "result": []any{map[string]any{"backlinks": 0, "info": map[string]any{}}}}}})
	}))
	t.Cleanup(server.Close)
	client, err := dataforseo.NewClient(dataforseo.Options{BaseURL: server.URL, APIKey: base64.StdEncoding.EncodeToString([]byte("fake-user:fake-pass")), Recorder: billingRecorder{}, MaxRetries: 0})
	if err != nil {
		t.Fatal(err)
	}
	svc := newService(client, &memoryCache{values: map[string][]byte{}}, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	d := Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Service: svc, Plans: plans}
	handler := overviewHandler(d)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj/backlinks/overview", strings.NewReader(body))
	req.SetPathValue("projectId", "proj")
	req = req.WithContext(auth.WithProjectOrganization(req.Context(), "org-a"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

type billingRecorder struct{}

func (billingRecorder) RecordDataForSEO(context.Context, string, dataforseo.Cost) error { return nil }

func TestPaidPlanCheckPrecedesValidationAndProviderSpend(t *testing.T) {
	t.Parallel()
	calls := 0
	rec := requestHandler(t, `not-json`, planResult(false), &calls)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if calls != 0 {
		t.Fatalf("provider calls = %d, want 0", calls)
	}
}
func TestHandlerStrictJSONAndTargetValidation(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`{"target":"example.com","unexpected":true}`, `{"target":"192.0.2.1"}`, `{"target":"example.com/a?x=1","scope":"exact_url"}`} {
		calls := 0
		rec := requestHandler(t, body, planResult(true), &calls)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status=%d response=%s", body, rec.Code, rec.Body)
		}
		if calls != 0 {
			t.Errorf("invalid body reached provider %d times", calls)
		}
	}
}
func TestRowsHandlerRejectsInvalidFiltersBeforeProvider(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { calls++ }))
	defer server.Close()
	client, err := dataforseo.NewClient(dataforseo.Options{BaseURL: server.URL, APIKey: base64.StdEncoding.EncodeToString([]byte("fake-user:fake-pass")), Recorder: billingRecorder{}, MaxRetries: 0})
	if err != nil {
		t.Fatal(err)
	}
	d := Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Service: newService(client, nil, nil, time.Now)}
	h := rowsHandler(d)
	for _, body := range []string{
		`{"target":"example.com","page":1,"pageSize":25,"sortField":"rank","filters":{"minLinkAuthority":100}}`,
		`{"target":"example.com","pageSize":50,"filters":{"include":"a,b,c,d","exclude":"e,f,g,h"}}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p/backlinks/rows", strings.NewReader(body))
		req = req.WithContext(auth.WithProjectOrganization(req.Context(), "org"))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, response=%s", body, rec.Code, rec.Body)
		}
		if calls != 0 {
			t.Errorf("invalid request reached provider %d times", calls)
		}
	}
}

func TestOversizedBodyIsRejectedBeforeProviderSpend(t *testing.T) {
	t.Parallel()
	calls := 0
	body := `{"target":"example.com","padding":"` + strings.Repeat("x", maxBody) + `"}`
	rec := requestHandler(t, body, planResult(true), &calls)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "too large") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if calls != 0 {
		t.Fatalf("oversized request reached provider %d times", calls)
	}
}
