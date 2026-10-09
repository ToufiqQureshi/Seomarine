package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
)

// testMux mounts the audit routes with middleware that injects a session user
// and project organization, then returns the mux.
func testMux(service *Service) *http.ServeMux {
	mux := http.NewServeMux()
	Mount(mux, Deps{
		Logger:  discardLogger(),
		Service: service,
		WithSession: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: "user-1"})))
			})
		},
		WithProjectAccess: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org-1")))
			})
		},
	})
	return mux
}

func doJSON(t *testing.T, mux http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	return recorder
}

func seedRunningAudit(t *testing.T, store *memoryStore) {
	t.Helper()
	config, _ := MarshalAuditConfig(AuditConfig{MaxPages: 50, LighthouseStrategy: LighthouseAuto})
	if err := store.CreateAudit(context.Background(), CreateAuditInput{
		ID: "audit-1", ProjectID: "proj-1", StartedByUserID: "user-1", StartURL: "https://example.com/",
		WorkflowInstanceID: "audit-1", Config: config, PagesTotal: 50, LighthouseTotal: 20,
	}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}
}

func TestHandlerStartAndReadEndpoints(t *testing.T) {
	store := newMemoryStore()
	scheduler := &recordingScheduler{}
	service := newTestService(store, scheduler, nil, nil, nil, false, nil)
	mux := testMux(service)

	recorder := doJSON(t, mux, "/api/v1/projects/proj-1/audit/start", `{"startUrl":"example.com","maxPages":50,"lighthouseStrategy":"auto"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("start status = %d body = %s", recorder.Code, recorder.Body)
	}
	var startResult StartAuditResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &startResult); err != nil || startResult.AuditID != "audit-1" {
		t.Fatalf("start body = %s err = %v", recorder.Body, err)
	}

	for _, endpoint := range []string{"status", "results", "progress"} {
		recorder = doJSON(t, mux, "/api/v1/projects/proj-1/audit/"+endpoint, `{"auditId":"audit-1"}`)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d body = %s", endpoint, recorder.Code, recorder.Body)
		}
	}

	recorder = doJSON(t, mux, "/api/v1/projects/proj-1/audit/history", `{}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("history status = %d", recorder.Code)
	}

	recorder = doJSON(t, mux, "/api/v1/projects/proj-1/audit/capabilities", `{}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("capabilities status = %d", recorder.Code)
	}

	recorder = doJSON(t, mux, "/api/v1/projects/proj-1/audit/delete", `{"auditId":"audit-1"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", recorder.Code, recorder.Body)
	}
	var deleted map[string]bool
	if err := json.Unmarshal(recorder.Body.Bytes(), &deleted); err != nil || !deleted["success"] {
		t.Fatalf("delete body = %s", recorder.Body)
	}
}

func TestHandlerValidationErrors(t *testing.T) {
	service := newTestService(newMemoryStore(), &recordingScheduler{}, nil, nil, nil, false, nil)
	mux := testMux(service)

	cases := []struct {
		name     string
		endpoint string
		body     string
		want     int
	}{
		{"unknown field", "start", `{"startUrl":"example.com","nope":1}`, http.StatusBadRequest},
		{"not json", "start", `{`, http.StatusBadRequest},
		{"missing url", "start", `{}`, http.StatusBadRequest},
		{"bad strategy", "start", `{"startUrl":"example.com","lighthouseStrategy":"weird"}`, http.StatusBadRequest},
		{"blocked target", "start", `{"startUrl":"http://127.0.0.1/"}`, http.StatusBadRequest},
		{"missing auditId", "status", `{}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := doJSON(t, mux, "/api/v1/projects/proj-1/audit/"+tc.endpoint, tc.body)
			if recorder.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", recorder.Code, tc.want, recorder.Body)
			}
		})
	}
}

func TestHandlerNotFound(t *testing.T) {
	service := newTestService(newMemoryStore(), nil, nil, nil, nil, false, nil)
	mux := testMux(service)
	recorder := doJSON(t, mux, "/api/v1/projects/proj-1/audit/status", `{"auditId":"missing"}`)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != "audit_not_found" {
		t.Fatalf("error body = %s", recorder.Body)
	}
}

func TestHandlerServiceUnavailable(t *testing.T) {
	mux := testMux(nil)
	recorder := doJSON(t, mux, "/api/v1/projects/proj-1/audit/status", `{"auditId":"audit-1"}`)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestHandlerPaymentRequired(t *testing.T) {
	store := newMemoryStore()
	// Hosted mode with no access denies the start with 402.
	service := newTestService(store, &recordingScheduler{}, nil, fakeAccess{allowed: false}, nil, true, nil)
	mux := testMux(service)
	recorder := doJSON(t, mux, "/api/v1/projects/proj-1/audit/start", `{"startUrl":"example.com"}`)
	if recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
	}
}

func TestHandlerDeleteRunningAudit(t *testing.T) {
	store := newMemoryStore()
	seedRunningAudit(t, store)
	service := newTestService(store, &recordingScheduler{}, nil, nil, nil, false, nil)
	mux := testMux(service)
	recorder := doJSON(t, mux, "/api/v1/projects/proj-1/audit/delete", `{"auditId":"audit-1"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
	}
	if len(store.audits) != 0 {
		t.Fatal("the audit should be deleted")
	}
}

func TestUTF16Length(t *testing.T) {
	if got := utf16Length("🎉"); got != 2 {
		t.Fatalf("emoji length = %d, want 2", got)
	}
}

func TestHandlerLighthouseIssuesAndExport(t *testing.T) {
	store := newMemoryStore()
	seedRunningAudit(t, store)
	payload := storedPayloadFixture(t, testIssue("seo", "meta-description", ptr(0), nil, nil))
	if err := store.InsertLighthouseResults(context.Background(), "audit-1", []LighthouseRecord{
		{ID: "lh-1", PageID: "page-1", URL: "https://example.com/", Strategy: "mobile", PayloadJSON: &payload},
		{ID: "lh-failed", PageID: "page-2", URL: "https://example.com/x", Strategy: "mobile"},
	}); err != nil {
		t.Fatal(err)
	}
	mux := testMux(newTestService(store, &recordingScheduler{}, nil, nil, nil, false, nil))

	recorder := doJSON(t, mux, "/api/v1/projects/proj-1/audit/lighthouse/issues", `{"resultId":"lh-1"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("issues status = %d body = %s", recorder.Code, recorder.Body)
	}
	var issues LighthouseIssuesResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &issues); err != nil || issues.ID != "lh-1" || len(issues.Issues) != 1 {
		t.Fatalf("issues body = %s err = %v", recorder.Body, err)
	}

	recorder = doJSON(t, mux, "/api/v1/projects/proj-1/audit/lighthouse/export", `{"resultId":"lh-1","mode":"category","category":"seo"}`)
	var exported LighthouseExportFile
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &exported) != nil || !strings.HasSuffix(exported.Filename, "-seo-issues.json") {
		t.Fatalf("export status = %d body = %s", recorder.Code, recorder.Body)
	}

	for _, tc := range []struct {
		name, path, body string
		status           int
	}{
		{"result without a stored payload", "issues", `{"resultId":"lh-failed"}`, http.StatusNotFound},
		{"unknown result", "issues", `{"resultId":"nope"}`, http.StatusNotFound},
		{"another project's result", "issues", `{"resultId":"lh-1"}`, http.StatusNotFound},
		{"missing result id", "issues", `{}`, http.StatusBadRequest},
		{"bad export mode", "export", `{"resultId":"lh-1","mode":"zip"}`, http.StatusBadRequest},
		{"bad export category", "export", `{"resultId":"lh-1","mode":"category","category":"speed"}`, http.StatusBadRequest},
		{"unknown field", "issues", `{"resultId":"lh-1","extra":1}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := "proj-1"
			if tc.name == "another project's result" {
				project = "proj-2"
			}
			recorder := doJSON(t, mux, "/api/v1/projects/"+project+"/audit/lighthouse/"+tc.path, tc.body)
			if recorder.Code != tc.status {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, tc.status, recorder.Body)
			}
		})
	}
}
