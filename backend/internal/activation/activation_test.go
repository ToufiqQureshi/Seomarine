package activation

import (
	"context"

	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
)

type storeFake struct {
	step, stamp, user string
	dismissed         bool
	calls             int
}

func (s *storeFake) MarkClicked(_ context.Context, _, step, stamp string) error {
	s.step = step
	s.stamp = stamp
	s.calls++
	return nil
}
func (s *storeFake) DismissGA4(_ context.Context, _, stamp string) error {
	s.step = "ga4"
	s.stamp = stamp
	s.calls++
	return nil
}
func (s *storeFake) SetDismissed(_ context.Context, user, _, step string, d bool) error {
	s.user = user
	s.step = step
	s.dismissed = d
	s.calls++
	return nil
}
func TestServiceValidatesStepsAndUsesUTC(t *testing.T) {
	store := &storeFake{}
	svc := &Service{Store: store, Now: func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.FixedZone("offset", 2*60*60)) }}
	if err := svc.MarkClicked(context.Background(), "p", "keywords"); err != nil {
		t.Fatal(err)
	}
	if store.stamp != "2026-10-10T10:00:00.000Z" {
		t.Fatalf("timestamp=%q", store.stamp)
	}
	if err := svc.MarkClicked(context.Background(), "p", "audit"); err == nil {
		t.Fatal("unsupported click accepted")
	}
	if err := svc.SetDismissed(context.Background(), "u", "p", "unknown", true); err == nil {
		t.Fatal("unsupported setup step accepted")
	}
}
func TestDismissRouteScopesCaller(t *testing.T) {
	store := &storeFake{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	withSession := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: "user-1"})))
		})
	}
	withProject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org-1")))
		})
	}
	mux := http.NewServeMux()
	Mount(mux, Deps{Logger: logger, Service: &Service{Store: store}, WithSession: withSession, WithProjectAccess: withProject})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p/activation/dismiss", strings.NewReader(`{"step":"audit","dismissed":true}`)))
	if rec.Code != http.StatusOK || store.user != "user-1" || store.step != "audit" || !store.dismissed {
		t.Fatalf("status=%d store=%+v", rec.Code, store)
	}
	bad := httptest.NewRecorder()
	mux.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p/activation/dismiss", strings.NewReader(`{"step":"bad","dismissed":true}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid step status=%d", bad.Code)
	}
}
