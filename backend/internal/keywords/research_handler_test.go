package keywords

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

type switchPlans struct{ paid bool }

func (p *switchPlans) HasPaidPlan(context.Context, string) (bool, error) { return p.paid, nil }

func newResearchServer(t *testing.T) (*httptest.Server, researchFixture, *switchPlans) {
	t.Helper()
	f := newResearchFixture(t)
	plans := &switchPlans{paid: true}
	mux := http.NewServeMux()
	MountResearch(mux, ResearchDeps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Service: f.svc, Saved: f.savedFixture.svc, ProjectMarkets: indiaMarket{}, Plans: plans,
		WithSession: func(next http.Handler) http.Handler { return next },
		WithProjectAccess: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org-test")))
			})
		},
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, f, plans
}

func postKeywords(t *testing.T, srv *httptest.Server, project, path, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/projects/"+project+"/keywords/"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s response: %v", path, err)
	}
	return resp.StatusCode, out
}

func TestResearchRoutesEndToEnd(t *testing.T) {
	srv, f, _ := newResearchServer(t)
	f.data.labsFn = func(LabsRequest) ([]LabsItem, error) { return items("kw", 6), nil }
	f.data.serpFn = func(SerpRequest) ([]SerpItem, error) { return []SerpItem{organic(1, "a.com")}, nil }
	f.data.overviewFn = func(k []string) ([]LabsItem, error) { return []LabsItem{labsItem(k[0], 5)}, nil }

	status, body := postKeywords(t, srv, f.project, "research", `{"keywords":["Seed"]}`)
	rows, _ := body["rows"].([]any)
	if status != 200 || body["source"] != "blended" || body["usedFallback"] != false || len(rows) == 0 {
		t.Fatalf("research = %d %v", status, body)
	}
	if first := rows[0].(map[string]any); first["intent"] != "commercial" || first["keywordDifficulty"] != float64(30) || first["trend"] == nil {
		t.Errorf("first row = %v", first)
	}
	// The project's market is India/Hindi; Labs serves its own language there.
	if call := f.data.labs[0]; call.LocationCode != 2356 {
		t.Errorf("provider location = %d, want the project's India", call.LocationCode)
	}

	status, body = postKeywords(t, srv, f.project, "serp", `{"keyword":"best seo"}`)
	items, _ := body["items"].([]any)
	if status != 200 || body["depth"] != float64(20) || len(items) != 1 || body["requestedKeyword"] != "best seo" {
		t.Errorf("serp = %d %v", status, body)
	}
	if status, body = postKeywords(t, srv, f.project, "serp", `{"keyword":"best seo","depth":100}`); status != 200 || body["depth"] != float64(100) {
		t.Errorf("deep serp = %d %v", status, body)
	}

	f.save(t, func(in *SaveInput) { in.LocationCode, in.LanguageCode = 2840, "en" })
	status, body = postKeywords(t, srv, f.project, "saved/refresh", `{}`)
	if status != 200 || body["updated"] != float64(1) {
		t.Errorf("refresh = %d %v", status, body)
	}
}

func TestResearchRoutesRejectBadRequests(t *testing.T) {
	srv, f, _ := newResearchServer(t)
	many := `"` + strings.Repeat(`a","`, 200) + `a"`
	tests := []struct{ name, path, body string }{
		{"unknown field", "research", `{"keywords":["a"],"nope":1}`},
		{"no keywords", "research", `{"keywords":[]}`},
		{"empty keyword", "research", `{"keywords":[""]}`},
		{"too many seeds", "research", `{"keywords":[` + many + `]}`},
		{"result limit", "research", `{"keywords":["a"],"resultLimit":200}`},
		{"mode", "research", `{"keywords":["a"],"mode":"all"}`},
		{"language", "research", `{"keywords":["a"],"languageCode":"x"}`},
		{"unsupported location", "research", `{"keywords":["a"],"locationCode":999999}`},
		{"blank city", "research", `{"keywords":["a"],"locationName":"  "}`},
		{"long city", "research", `{"keywords":["a"],"locationName":"` + strings.Repeat("c", 201) + `"}`},
		{"serp without keyword", "serp", `{"keyword":" "}`},
		{"serp depth", "serp", `{"keyword":"a","depth":50}`},
		{"serp unknown field", "serp", `{"keyword":"a","limit":5}`},
		{"refresh takes nothing", "saved/refresh", `{"all":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if status, body := postKeywords(t, srv, f.project, tt.path, tt.body); status != 400 || errCode(body) != "invalid_request" {
				t.Errorf("%s %s = %d %v, want 400 invalid_request", tt.path, tt.body, status, body)
			}
		})
	}
	if len(f.data.labs)+len(f.data.serps)+len(f.data.overviews) != 0 {
		t.Error("a rejected request reached the paid provider")
	}
}

func TestResearchRoutesGateAndMapErrors(t *testing.T) {
	srv, f, plans := newResearchServer(t)
	plans.paid = false
	for _, path := range []string{"research", "serp", "saved/refresh"} {
		body := map[string]string{"research": `{"keywords":["a"]}`, "serp": `{"keyword":"a"}`, "saved/refresh": `{}`}[path]
		if status, resp := postKeywords(t, srv, f.project, path, body); status != 402 || errCode(resp) != "payment_required" {
			t.Errorf("%s on a free plan = %d %v, want 402", path, status, resp)
		}
	}
	if len(f.data.labs)+len(f.data.serps) != 0 {
		t.Error("a free plan reached the paid provider")
	}
	plans.paid = true

	cases := map[string]struct {
		err    error
		status int
		code   string
	}{
		"billing":  {fmt.Errorf("DataForSEO: %w", dataforseo.ErrBillingIssue), 503, "provider_billing_issue"},
		"upstream": {fmt.Errorf("x: %w", dataforseo.ErrUpstreamUnavailable), 502, "provider_error"},
		"task":     {&dataforseo.TaskError{StatusCode: 40000, Message: "bad"}, 502, "provider_error"},
		"http":     {&dataforseo.HTTPError{StatusCode: 500}, 502, "provider_error"},
		"other":    {errors.New("bug"), 500, "internal"},
	}
	for name, c := range cases {
		f.cache.values = nil
		f.data.labsFn = func(LabsRequest) ([]LabsItem, error) { return nil, c.err }
		if status, resp := postKeywords(t, srv, f.project, "research", `{"keywords":["a"]}`); status != c.status || errCode(resp) != c.code {
			t.Errorf("%s = %d %v, want %d %s", name, status, resp, c.status, c.code)
		}
	}

	city := `{"keywords":["a"],"locationCode":2356,"locationName":"Nowhere"}`
	if status, resp := postKeywords(t, srv, f.project, "research", city); status != 400 || errCode(resp) != "unknown_location" || !strings.Contains(fmt.Sprint(resp), "India") {
		t.Errorf("unknown place = %d %v, want 400 unknown_location", status, resp)
	}
	f.registry.err = ErrLocationCache
	if status, resp := postKeywords(t, srv, f.project, "research", `{"keywords":["a"],"locationCode":2356,"locationName":"Pune,Maharashtra,India"}`); status != 503 || errCode(resp) != "locations_unavailable" {
		t.Errorf("registry down = %d %v, want 503", status, resp)
	}
}

func TestResearchRoutesWithoutAServiceAreUnavailable(t *testing.T) {
	mux := http.NewServeMux()
	pass := func(next http.Handler) http.Handler { return next }
	MountResearch(mux, ResearchDeps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), ProjectMarkets: indiaMarket{}, WithSession: pass,
		WithProjectAccess: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org")))
			})
		}})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p/keywords/research", strings.NewReader(`{"keywords":["a"]}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
