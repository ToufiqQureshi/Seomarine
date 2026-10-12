package ahrefs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type ahrefsDoerFunc func(*http.Request) (*http.Response, error)

func (f ahrefsDoerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

type closeErrorBody struct{ io.Reader }

func (closeErrorBody) Close() error { return errors.New("close failed") }

func TestNormalizeDomain(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, input, want string
		wantErr           bool
	}{
		{name: "bare host", input: " Example.COM. ", want: "example.com"},
		{name: "URL with www and path", input: "https://www.example.com/path?q=1", want: "example.com"},
		{name: "international host", input: "bücher.example", want: "xn--bcher-kva.example"},
		{name: "empty", input: "  ", wantErr: true},
		{name: "credentials", input: (&url.URL{Scheme: "https", User: url.UserPassword("user", "pass"), Host: "example.com"}).String(), wantErr: true},
		{name: "not a host", input: "https://", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeDomain(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("normalizeDomain(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("normalizeDomain(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRatingsPreservesOriginalKeysAndUsesNormalizedHost(t *testing.T) {
	var requests int
	service := &Service{client: ahrefsDoerFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/v3/public/domain-rating-free" {
			return nil, fmt.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if got := r.URL.Query().Get("target"); got != "example.com" {
			return nil, fmt.Errorf("target = %q", got)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"domain_rating":{"domain_rating":42}}`))}, nil
	})}

	got, err := service.Ratings(context.Background(), []string{"Example.com", "https://www.example.com/path"})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || len(got) != 2 {
		t.Fatalf("requests=%d results=%v", requests, got)
	}
	for _, key := range []string{"Example.com", "https://www.example.com/path"} {
		if got[key] == nil || *got[key] != 42 {
			t.Fatalf("rating[%q] = %v", key, got[key])
		}
	}
}

func TestRatingsRejectsTooManyOrInvalidDomainsBeforeRequests(t *testing.T) {
	called := false
	service := &Service{client: ahrefsDoerFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected request")
	})}
	if _, err := service.Ratings(context.Background(), make([]string, maxDomains+1)); err == nil {
		t.Fatal("expected maximum-domain error")
	}
	if _, err := service.Ratings(context.Background(), []string{"valid.example", "https://user@example.com"}); err == nil {
		t.Fatal("expected invalid-domain error")
	}
	if called {
		t.Fatal("made a provider request for invalid input")
	}
}

func TestLookupUnavailableRatingsAndProviderErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantNil    bool
		wantErr    bool
	}{
		{name: "zero means unavailable", body: `{"domain_rating":{"domain_rating":0}}`, status: 200, wantNil: true},
		{name: "above provider range means unavailable", body: `{"domain_rating":{"domain_rating":101}}`, status: 200, wantNil: true},
		{name: "valid rating", body: `{"domain_rating":{"domain_rating":100}}`, status: 200},
		{name: "malformed response", body: `{`, status: 200, wantErr: true},
		{name: "provider failure", body: `{}`, status: 502, wantNil: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &Service{client: ahrefsDoerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Status: http.StatusText(tc.status), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			got, err := service.lookup(context.Background(), "example.com")
			if (err != nil) != tc.wantErr {
				t.Fatalf("lookup error=%v, wantErr=%v", err, tc.wantErr)
			}
			if tc.wantNil && got != nil {
				t.Fatalf("rating = %v, want nil", *got)
			}
			if tc.name == "valid rating" && (got == nil || *got != 100) {
				t.Fatalf("rating = %v, want 100", got)
			}
		})
	}
}

func TestLookupReturnsTransportAndBodyCloseErrors(t *testing.T) {
	service := &Service{client: ahrefsDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}
	if _, err := service.lookup(context.Background(), "example.com"); err == nil {
		t.Fatal("expected transport error")
	}
	service.client = ahrefsDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: closeErrorBody{strings.NewReader(`{"domain_rating":{"domain_rating":10}}`)}}, nil
	})
	if _, err := service.lookup(context.Background(), "example.com"); err == nil || !strings.Contains(err.Error(), "close Ahrefs response") {
		t.Fatalf("close error = %v", err)
	}
}

func TestNormalizeDomainRejectsOverlongInput(t *testing.T) {
	if _, err := normalizeDomain(strings.Repeat("a", 254)); err == nil {
		t.Fatal("expected overlong domain error")
	}
}
