package ranktracking

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

type fakeMarkets struct{}

func (fakeMarkets) Get(context.Context, string, string) (market.Pair, error) { return usMarket, nil }

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	return newTestServerWith(t, nil)
}

// newTestServerWith lets a test add a Checks service built on the same project.
func newTestServerWith(t *testing.T, checks func(*Service) *Checks) (*httptest.Server, string) {
	t.Helper()
	svc, project := newTestService(t, nil)
	var c *Checks
	if checks != nil {
		c = checks(svc)
	}
	mux := http.NewServeMux()
	Mount(mux, Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Service: svc, Checks: c, ProjectMarkets: fakeMarkets{},
		WithSession: func(next http.Handler) http.Handler { return next },
		WithProjectAccess: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org-test")))
			})
		},
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, project
}

func post(t *testing.T, srv *httptest.Server, project, path, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/projects/"+project+"/rank-tracking/"+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
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

func errorCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func TestHandlerConfigAndKeywordFlow(t *testing.T) {
	srv, project := newTestServer(t)

	if status, body := post(t, srv, project, "configs/list", `{}`); status != 200 || len(body["configs"].([]any)) != 0 {
		t.Fatalf("empty list = %d %v, want 200 and an empty array", status, body)
	}
	status, body := post(t, srv, project, "configs/create", `{"domain":"https://www.Example.com/","serpDepth":20,"scheduleInterval":"manual"}`)
	if status != 200 {
		t.Fatalf("create = %d %v", status, body)
	}
	cfg := body["config"].(map[string]any)
	id := cfg["id"].(string)
	if cfg["domain"] != "example.com" || cfg["devices"] != "both" || cfg["nextCheckAt"] != nil || cfg["locationCode"] != float64(2840) {
		t.Errorf("created config = %v", cfg)
	}
	if status, body = post(t, srv, project, "configs/create", `{"domain":"example.com","serpDepth":20}`); status != 409 || errorCode(body) != "already_tracked" {
		t.Errorf("duplicate = %d %v, want 409 already_tracked", status, body)
	}

	status, body = post(t, srv, project, "keywords/add", `{"configId":"`+id+`","keywords":["One","two"]}`)
	if status != 200 || body["added"] != float64(2) {
		t.Fatalf("add = %d %v", status, body)
	}
	ids := body["addedIds"].([]any)
	if status, body = post(t, srv, project, "keywords/list", `{"configId":"`+id+`"}`); status != 200 || len(body["keywords"].([]any)) != 2 {
		t.Errorf("list = %d %v", status, body)
	}
	if status, body = post(t, srv, project, "estimate-cost", `{"configId":"`+id+`","additionalKeywords":["three"]}`); status != 200 || body["keywordCount"] != float64(3) {
		t.Errorf("estimate = %d %v", status, body)
	}
	if status, body = post(t, srv, project, "keywords/remove", `{"configId":"`+id+`","keywordIds":["`+ids[0].(string)+`"]}`); status != 200 || body["removed"] != float64(1) {
		t.Errorf("remove = %d %v", status, body)
	}
	cid := `"configId":"` + id + `"`
	for route, req := range map[string]string{
		"results/latest":  `{` + cid + `}`,
		"runs/latest":     `{` + cid + `}`,
		"results/history": `{` + cid + `,"trackingKeywordId":"` + ids[1].(string) + `"}`,
		"results/trend":   `{` + cid + `,"device":"desktop"}`,
		"results/matrix":  `{` + cid + `,"device":"mobile","runLimit":5}`,
	} {
		if status, body = post(t, srv, project, route, req); status != 200 {
			t.Errorf("%s = %d %v, want 200 with no data yet", route, status, body)
		}
	}
	if _, body = post(t, srv, project, "runs/latest", `{"configId":"`+id+`"}`); body["run"] != nil {
		t.Errorf("runs/latest = %v, want a null run before any check", body)
	}
	status, body = post(t, srv, project, "configs/update", `{"configId":"`+id+`","isActive":false}`)
	if status != 200 || body["config"].(map[string]any)["isActive"] != false {
		t.Errorf("archive = %d %v", status, body)
	}
}

func TestHandlerRefreshesConfigKeywordMetrics(t *testing.T) {
	provider := &fakeRankMetricProvider{metrics: []KeywordMetric{{Keyword: "nodex"}}}
	srv, project := newTestServerWith(t, func(service *Service) *Checks {
		service.Metrics = provider
		service.Plans = fakePlans{paid: true}
		return nil
	})
	status, body := post(t, srv, project, "configs/create", `{"domain":"metrics.example.com","serpDepth":10,"scheduleInterval":"manual"}`)
	if status != http.StatusOK {
		t.Fatalf("create config = %d %v", status, body)
	}
	config := body["config"].(map[string]any)
	configID := config["id"].(string)
	status, body = post(t, srv, project, "keywords/add", `{"configId":"`+configID+`","keywords":["Nodex","nodex"],"matchCase":true}`)
	if status != http.StatusOK {
		t.Fatalf("add keywords = %d %v", status, body)
	}
	status, body = post(t, srv, project, "keywords/metrics/refresh", `{"configId":"`+configID+`"}`)
	if status != http.StatusOK || body["updated"] != float64(2) {
		t.Fatalf("refresh metrics = %d %v; want 200 updated=2", status, body)
	}
	status, body = post(t, srv, project, "keywords/metrics/refresh", `{"configId":"`+configID+`","unexpected":true}`)
	if status != http.StatusBadRequest || errorCode(body) != "invalid_request" {
		t.Fatalf("unknown field response = %d %v; want 400 invalid_request", status, body)
	}
}

func TestHandlerRejectsBadRequests(t *testing.T) {
	srv, project := newTestServer(t)
	missing := "00000000-0000-4000-8000-000000000000"
	tests := []struct {
		name, path, body string
		status           int
		code             string
	}{
		{"unknown field", "configs/create", `{"domain":"a.example.com","serpDepth":10,"extra":1}`, 400, "invalid_request"},
		{"not json", "configs/create", `nope`, 400, "invalid_request"},
		{"two json values", "configs/list", `{}{}`, 400, "invalid_request"},
		{"empty body", "configs/list", ``, 400, "invalid_request"},
		{"too large", "configs/create", `{"domain":"` + strings.Repeat("a", maxBody) + `"}`, 400, "invalid_request"},
		{"bad domain", "configs/create", `{"domain":"localhost","serpDepth":10}`, 400, "invalid_request"},
		{"bad depth", "configs/create", `{"domain":"a.example.com","serpDepth":15}`, 400, "invalid_request"},
		{"unknown timezone", "configs/create", `{"domain":"a.example.com","serpDepth":10,"scheduleTime":{"weekday":1,"hour":5,"minute":0,"timeZone":"Mars/Base"}}`, 400, "invalid_request"},
		{"timezone Local is the server's, not the user's", "configs/create", `{"domain":"a.example.com","serpDepth":10,"scheduleTime":{"weekday":1,"hour":5,"minute":0,"timeZone":"Local"}}`, 400, "invalid_request"},
		{"empty timezone", "configs/create", `{"domain":"a.example.com","serpDepth":10,"scheduleTime":{"weekday":1,"hour":5,"minute":0,"timeZone":""}}`, 400, "invalid_request"},
		{"hour out of range", "configs/create", `{"domain":"a.example.com","serpDepth":10,"scheduleTime":{"weekday":1,"hour":25,"minute":0}}`, 400, "invalid_request"},
		{"city without a checker", "configs/create", `{"domain":"a.example.com","serpDepth":10,"locationName":"Mumbai"}`, 503, "location_check_unavailable"},
		{"short config id", "keywords/list", `{"configId":"x"}`, 400, "invalid_request"},
		{"unknown config", "keywords/list", `{"configId":"` + missing + `"}`, 404, "config_not_found"},
		{"no keywords", "keywords/add", `{"configId":"` + missing + `","keywords":[]}`, 400, "invalid_request"},
		{"too many keywords", "keywords/add", `{"configId":"` + missing + `","keywords":["` + strings.Repeat(`a","`, maxKeywordBatch) + `a"]}`, 400, "invalid_request"},
		{"no ids to remove", "keywords/remove", `{"configId":"` + missing + `","keywordIds":[]}`, 400, "invalid_request"},
		{"bad compare period", "results/latest", `{"configId":"` + missing + `","comparePeriod":"2d"}`, 400, "invalid_request"},
		{"trend needs a device", "results/trend", `{"configId":"` + missing + `"}`, 400, "invalid_request"},
		{"history needs a keyword id", "results/history", `{"configId":"` + missing + `","trackingKeywordId":"x"}`, 400, "invalid_request"},
		{"update unknown config", "configs/update", `{"configId":"` + missing + `","devices":"mobile"}`, 404, "config_not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := post(t, srv, project, tt.path, tt.body)
			if status != tt.status || errorCode(body) != tt.code {
				t.Errorf("%s = %d %v, want %d %s", tt.path, status, body, tt.status, tt.code)
			}
		})
	}
}

func TestHandlerUpdateCanClearCityWithNull(t *testing.T) {
	var body optionalString
	if err := json.Unmarshal([]byte(`null`), &body); err != nil || !body.Set || body.Value != nil {
		t.Fatalf("null -> %+v, %v; want Set with no value", body, err)
	}
	var absent struct {
		Name optionalString `json:"name"`
	}
	if err := json.Unmarshal([]byte(`{}`), &absent); err != nil || absent.Name.Set {
		t.Fatalf("absent -> %+v, %v; want not Set", absent, err)
	}
}

func TestHandlerWithoutServiceIsUnavailable(t *testing.T) {
	mux := http.NewServeMux()
	passthrough := func(next http.Handler) http.Handler { return next }
	Mount(mux, Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), WithSession: passthrough, WithProjectAccess: passthrough})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p/rank-tracking/configs/list", strings.NewReader(`{}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestHandlerStartCheck(t *testing.T) {
	sched, plans := &fakeScheduler{}, &fakePlans{paid: true}
	var checks *Checks
	srv, project := newTestServerWith(t, func(svc *Service) *Checks {
		store := svc.Repo.(Store)
		checks = &Checks{
			Repo: store, Results: store, Runs: store, Serp: &fakeSerp{}, Scheduler: sched, Plans: plans,
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: svc.Schedule.Now,
		}
		return checks
	})
	_, body := post(t, srv, project, "configs/create", `{"domain":"example.com","serpDepth":10,"scheduleInterval":"manual"}`)
	id := body["config"].(map[string]any)["id"].(string)
	cid := `"configId":"` + id + `"`

	if status, body := post(t, srv, project, "checks/start", `{`+cid+`}`); status != 400 || errorCode(body) != "invalid_request" {
		t.Errorf("start without keywords = %d %v, want 400", status, body)
	}
	post(t, srv, project, "keywords/add", `{`+cid+`,"keywords":["one"]}`)
	for name, tc := range map[string]struct{ body, code string }{
		"negative ceiling": {`{` + cid + `,"maxCostCredits":-1}`, "invalid_request"},
		"unknown field":    {`{` + cid + `,"force":true}`, "invalid_request"},
		"short id":         {`{"configId":"x"}`, "invalid_request"},
	} {
		if status, body := post(t, srv, project, "checks/start", tc.body); status != 400 || errorCode(body) != tc.code {
			t.Errorf("%s = %d %v, want 400 %s", name, status, body, tc.code)
		}
	}
	plans.paid = false
	if status, body := post(t, srv, project, "checks/start", `{`+cid+`}`); status != 402 || errorCode(body) != "payment_required" {
		t.Errorf("free plan = %d %v, want 402", status, body)
	}
	plans.paid = true

	status, body := post(t, srv, project, "checks/start", `{`+cid+`,"maxCostCredits":1000}`)
	runID, _ := body["runId"].(string)
	if status != 200 || body["ok"] != true || runID == "" || len(sched.jobs) != 1 || sched.jobs[0].OrganizationID != "org-test" {
		t.Fatalf("start = %d %v, jobs %+v", status, body, sched.jobs)
	}
	status, body = post(t, srv, project, "checks/start", `{`+cid+`}`)
	if status != 409 || errorCode(body) != "already_running" || body["blockingRunId"] != runID {
		t.Errorf("second start = %d %v, want 409 already_running blocked by %s", status, body, runID)
	}
	if status, body = post(t, srv, project, "runs/latest", `{`+cid+`}`); status != 200 || body["run"].(map[string]any)["status"] != "pending" {
		t.Errorf("runs/latest = %d %v, want the pending run", status, body)
	}
}

func TestHandlerStartCheckWithoutChecksIsUnavailable(t *testing.T) {
	srv, project := newTestServer(t)
	_, body := post(t, srv, project, "configs/create", `{"domain":"example.com","serpDepth":10}`)
	id := body["config"].(map[string]any)["id"].(string)
	if status, body := post(t, srv, project, "checks/start", `{"configId":"`+id+`"}`); status != 503 || errorCode(body) != "checks_unavailable" {
		t.Errorf("start = %d %v, want 503 checks_unavailable", status, body)
	}
}
