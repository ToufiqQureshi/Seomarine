package activation

import (
	"context"
	"errors"

	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
)

type storeFake struct {
	step, stamp, user string
	dismissed         bool
	calls             int
	activation        DashboardActivation
	err               error
}

func (s *storeFake) GetDashboardActivation(_ context.Context, user, project, organization string) (DashboardActivation, error) {
	if user != "user-1" || project != "p" || organization != "org-1" {
		return DashboardActivation{}, context.Canceled
	}
	return s.activation, s.err
}

func (s *storeFake) MarkClicked(_ context.Context, _, step, stamp string) error {
	s.step = step
	s.stamp = stamp
	s.calls++
	return s.err
}
func (s *storeFake) DismissGA4(_ context.Context, _, stamp string) error {
	s.step = "ga4"
	s.stamp = stamp
	s.calls++
	return s.err
}
func (s *storeFake) SetDismissed(_ context.Context, user, _, step string, d bool) error {
	s.user = user
	s.step = step
	s.dismissed = d
	s.calls++
	return s.err
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
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/projects/p/activation/dismiss", strings.NewReader(`{"step":"audit","dismissed":true}`)))
	if rec.Code != http.StatusOK || store.user != "user-1" || store.step != "audit" || !store.dismissed {
		t.Fatalf("status=%d store=%+v", rec.Code, store)
	}
	bad := httptest.NewRecorder()
	mux.ServeHTTP(bad, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/projects/p/activation/dismiss", strings.NewReader(`{"step":"bad","dismissed":true}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid step status=%d", bad.Code)
	}
}

func TestDashboardActivationRouteReturnsProjectScopedState(t *testing.T) {
	store := &storeFake{activation: DashboardActivation{Domain: stringPtr("example.com"), GA4: DashboardGA4{Connected: true}, DismissedSteps: []string{"audit"}}}
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
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/projects/p/activation/get", strings.NewReader(`{}`)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"hasAudit":false`) || !strings.Contains(response.Body.String(), `"domain":"example.com"`) || !strings.Contains(response.Body.String(), `"dismissedSteps":["audit"]`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}
func stringPtr(value string) *string { return &value }

func TestActivationMutationRoutesValidateInputAndReturnServerErrors(t *testing.T) {
	store := &storeFake{}
	withSession := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: "user-1"})))
		})
	}
	withProject := func(next http.Handler) http.Handler { return next }
	mux := http.NewServeMux()
	Mount(mux, Deps{Service: &Service{Store: store}, WithSession: withSession, WithProjectAccess: withProject})

	for _, tc := range []struct {
		body string
		want int
	}{
		{body: `{"step":"competitor"}`, want: http.StatusOK},
		{body: `{"step":"audit"}`, want: http.StatusBadRequest},
		{body: `{"step":"keywords","extra":true}`, want: http.StatusBadRequest},
		{body: `{"step":"keywords"} {}`, want: http.StatusBadRequest},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p/activation/click", strings.NewReader(tc.body))
		mux.ServeHTTP(recorder, request)
		if recorder.Code != tc.want {
			t.Fatalf("click body %q: status=%d, want %d body=%s", tc.body, recorder.Code, tc.want, recorder.Body.String())
		}
	}
	if store.step != "competitor" {
		t.Fatalf("stored click step = %q", store.step)
	}

	store.err = errors.New("database unavailable")
	for _, routeAndBody := range [][2]string{
		{"/api/v1/projects/p/activation/click", `{"step":"keywords"}`},
		{"/api/v1/projects/p/activation/dismiss", `{"step":"audit","dismissed":true}`},
		{"/api/v1/projects/p/activation/ga4-dismiss", `{}`},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, routeAndBody[0], strings.NewReader(routeAndBody[1]))
		mux.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("%s status=%d, want 500 body=%s", routeAndBody[0], recorder.Code, recorder.Body.String())
		}
	}
}

func TestActivationMutationRoutesReturnUnavailableWithoutService(t *testing.T) {
	withSession := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: "user-1"})))
		})
	}
	mux := http.NewServeMux()
	Mount(mux, Deps{WithSession: withSession, WithProjectAccess: func(next http.Handler) http.Handler { return next }})
	for _, routeAndBody := range [][2]string{
		{"/api/v1/projects/p/activation/click", `{"step":"competitor"}`},
		{"/api/v1/projects/p/activation/dismiss", `{"step":"audit","dismissed":true}`},
		{"/api/v1/projects/p/activation/ga4-dismiss", `{}`},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, routeAndBody[0], strings.NewReader(routeAndBody[1]))
		mux.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status=%d, want 503 body=%s", routeAndBody[0], recorder.Code, recorder.Body.String())
		}
	}
}

func TestActivationServiceMethodsHandleMissingStore(t *testing.T) {
	service := &Service{}
	if err := service.MarkClicked(context.Background(), "p", "competitor"); err == nil {
		t.Fatal("MarkClicked accepted missing store")
	}
	if err := service.DismissGA4(context.Background(), "p"); err == nil {
		t.Fatal("DismissGA4 accepted missing store")
	}
	if err := service.SetDismissed(context.Background(), "u", "p", "audit", true); err == nil {
		t.Fatal("SetDismissed accepted missing store")
	}
}
