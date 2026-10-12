package team

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
)

func TestInvitationHandlerValidatesCallerAndRequest(t *testing.T) {
	withUser := func(user auth.User) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), user)))
			})
		}
	}
	for _, tc := range []struct {
		name    string
		user    auth.User
		body    string
		service *Service
		want    int
	}{
		{name: "no organization", user: auth.User{ID: "u1", Role: "owner"}, body: `{"email":"a@example.com"}`, service: &Service{}, want: http.StatusForbidden},
		{name: "member cannot invite", user: auth.User{ID: "u1", OrganizationID: "o1", Role: "member"}, body: `{"email":"a@example.com"}`, service: &Service{}, want: http.StatusForbidden},
		{name: "invalid email", user: auth.User{ID: "u1", OrganizationID: "o1", Role: "admin"}, body: `{"email":"bad"}`, service: &Service{}, want: http.StatusBadRequest},
		{name: "unknown request field", user: auth.User{ID: "u1", OrganizationID: "o1", Role: "admin"}, body: `{"email":"a@example.com","admin":true}`, service: &Service{}, want: http.StatusBadRequest},
		{name: "trailing JSON", user: auth.User{ID: "u1", OrganizationID: "o1", Role: "admin"}, body: `{"email":"a@example.com"} {}`, service: &Service{}, want: http.StatusBadRequest},
		{name: "limiter unavailable", user: auth.User{ID: "u1", OrganizationID: "o1", Role: "admin"}, body: `{"email":"a@example.com"}`, service: &Service{}, want: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			Mount(mux, tc.service, withUser(tc.user))
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/organization/invitations", strings.NewReader(tc.body)))
			if recorder.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tc.want, recorder.Body.String())
			}
		})
	}

	mux := http.NewServeMux()
	Mount(mux, nil, withUser(auth.User{ID: "u1", OrganizationID: "o1", Role: "owner"}))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/organization/invitations", strings.NewReader(`{"email":"a@example.com"}`)))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil service status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
