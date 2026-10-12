package ahrefs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMountValidatesRequestAndReturnsRatings(t *testing.T) {
	service := &Service{client: ahrefsDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"domain_rating":{"domain_rating":60}}`)),
		}, nil
	})}
	identity := func(next http.Handler) http.Handler { return next }
	mux := http.NewServeMux()
	Mount(mux, service, identity, identity)
	path := "/api/v1/projects/p1/backlinks/domain-ratings"

	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{name: "valid", body: `{"domains":["Example.com"]}`, want: http.StatusOK},
		{name: "malformed JSON", body: `{`, want: http.StatusBadRequest},
		{name: "unknown field", body: `{"domains":[],"other":true}`, want: http.StatusBadRequest},
		{name: "second JSON value", body: `{"domains":[]} {}`, want: http.StatusBadRequest},
		{name: "invalid domain", body: `{"domains":["https://user@example.com"]}`, want: http.StatusBadRequest},
		{name: "too many domains", body: `{"domains":[` + strings.TrimRight(strings.Repeat(`"example.com",`, maxDomains+1), ",") + `]}`, want: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(tc.body))
			mux.ServeHTTP(recorder, request)
			if recorder.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tc.want, recorder.Body.String())
			}
			if tc.name == "valid" && !strings.Contains(recorder.Body.String(), `"Example.com":60`) {
				t.Fatalf("rating response = %s", recorder.Body.String())
			}
		})
	}

	large := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"domains":["`+strings.Repeat("a", maxBody)+`"]}`))
	mux.ServeHTTP(large, request)
	if large.Code != http.StatusBadRequest {
		t.Fatalf("oversized request status=%d body=%s", large.Code, large.Body.String())
	}
}

func TestMountReturnsUnavailableWithoutService(t *testing.T) {
	identity := func(next http.Handler) http.Handler { return next }
	mux := http.NewServeMux()
	Mount(mux, nil, identity, identity)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/projects/p1/backlinks/domain-ratings", strings.NewReader(`{"domains":[]}`)))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
