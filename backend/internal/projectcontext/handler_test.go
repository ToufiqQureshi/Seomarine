package projectcontext

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type apiFake struct {
	project string
	updates []json.RawMessage
	author  string
}

func (f *apiFake) Get(_ context.Context, p string) (ProjectContext, error) {
	f.project = p
	return ProjectContext{MissingSections: []string{"business_overview"}}, nil
}
func (f *apiFake) Apply(_ context.Context, p string, u []json.RawMessage, a string) (ProjectContext, error) {
	f.project = p
	f.updates = u
	f.author = a
	return ProjectContext{MissingSections: []string{}}, nil
}
func TestContextAPIReadsAndAppliesUserUpdatesProjectScoped(t *testing.T) {
	service := &apiFake{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	withSession := func(next http.Handler) http.Handler { return next }
	withProject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { next.ServeHTTP(w, r) })
	}
	mux := http.NewServeMux()
	Mount(mux, HandlerDeps{Logger: logger, Service: service, WithSession: withSession, WithProjectAccess: withProject})
	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/context/get", strings.NewReader(`{"projectId":"p1"}`)))
	if get.Code != http.StatusOK || service.project != "p1" {
		t.Fatalf("get status=%d project=%q body=%s", get.Code, service.project, get.Body.String())
	}
	update := httptest.NewRecorder()
	mux.ServeHTTP(update, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/context/update", strings.NewReader(`{"projectId":"p1","updates":[{"section":"current_goal","content":"Grow"}]}`)))
	if update.Code != http.StatusOK || service.project != "p1" || service.author != "user" || len(service.updates) != 1 {
		t.Fatalf("update status=%d service=%+v body=%s", update.Code, service, update.Body.String())
	}
	mismatch := httptest.NewRecorder()
	mux.ServeHTTP(mismatch, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/context/get", strings.NewReader(`{"projectId":"p2"}`)))
	if mismatch.Code != http.StatusBadRequest {
		t.Fatalf("cross-project body status=%d", mismatch.Code)
	}
}
