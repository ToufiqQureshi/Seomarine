package dashboardoverview

import (
	"context"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerRejectsUnexpectedBodyAndScopesProject(t *testing.T) {
	store := &storeStub{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	withSession := func(next http.Handler) http.Handler { return next }
	withProject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.WithProjectOrganization(r.Context(), "org-1")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	mux := http.NewServeMux()
	Mount(mux, Deps{Logger: logger, Service: &Service{Store: store}, WithSession: withSession, WithProjectAccess: withProject})
	bad := httptest.NewRecorder()
	mux.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p/dashboard/overview", strings.NewReader(`{"unexpected":true}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid status=%d", bad.Code)
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p/dashboard/overview", strings.NewReader(`{}`)))
	if response.Code != http.StatusOK || store.project != "p" || store.organization != "org-1" {
		t.Fatalf("response=%d %s scope=%s/%s", response.Code, response.Body.String(), store.project, store.organization)
	}
}

var _ = context.Background
