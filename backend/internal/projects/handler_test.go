package projects

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
)

func TestProjectRoutesUseSessionAndProjectScope(t *testing.T) {
	store := &fakeStore{role: "owner", rows: []Project{{ID: "p1", Name: "Site"}}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	withSession := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := auth.User{ID: "u1", OrganizationID: "org1"}
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
		})
	}
	withProject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org1")))
		})
	}
	mux := http.NewServeMux()
	Mount(mux, Deps{Logger: logger, Service: &Service{Store: store}, WithSession: withSession, WithProjectAccess: withProject})
	created := httptest.NewRecorder()
	mux.ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/v1/projects/create", strings.NewReader(`{"name":" New "}`)))
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"name":"New"`) {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	got := httptest.NewRecorder()
	mux.ServeHTTP(got, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/get", strings.NewReader(`{}`)))
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"id":"p1"`) {
		t.Fatalf("get status=%d body=%s", got.Code, got.Body.String())
	}
	invalid := httptest.NewRecorder()
	mux.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/get", strings.NewReader(`{"unknown":true}`)))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d", invalid.Code)
	}
}
