package dashboardoverview

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/backlinks"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
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
	mux.ServeHTTP(bad, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/projects/p/dashboard/overview", strings.NewReader(`{"unexpected":true}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid status=%d", bad.Code)
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/projects/p/dashboard/overview", strings.NewReader(`{}`)))
	if response.Code != http.StatusOK || store.project != "p" || store.organization != "org-1" {
		t.Fatalf("response=%d %s scope=%s/%s", response.Code, response.Body.String(), store.project, store.organization)
	}
}

type writableStore struct{ *storeStub }

func (writableStore) InsertBacklinkSnapshot(context.Context, string, BacklinkSummary) error {
	return nil
}

type refusingBacklinks struct{ calls int }

func (b *refusingBacklinks) DashboardSummary(context.Context, string, string) (backlinks.Summary, error) {
	b.calls++
	return backlinks.Summary{}, fmt.Errorf("DataForSEO: %w", spend.ErrNoCredits)
}

func TestRefreshAnswersARefusalAsOutOfCredits(t *testing.T) {
	domain := "example.com"
	provider := &refusingBacklinks{}
	withSession := func(next http.Handler) http.Handler { return next }
	withProject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org-1")))
		})
	}
	mux := http.NewServeMux()
	Mount(mux, Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Service: &Service{Store: writableStore{&storeStub{domain: &domain}}, Backlinks: provider}, WithSession: withSession, WithProjectAccess: withProject})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/projects/p/dashboard/backlinks/refresh", strings.NewReader(`{}`)))
	if provider.calls != 1 || rec.Code != http.StatusPaymentRequired || !strings.Contains(rec.Body.String(), "insufficient_credits") {
		t.Fatalf("refresh = %d %s after %d calls, want 402 insufficient_credits", rec.Code, rec.Body, provider.calls)
	}
}
