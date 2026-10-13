package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestValidOAuthRedirectURI(t *testing.T) {
	for _, tc := range []struct {
		uri  string
		want bool
	}{
		{"https://client.example/callback", true},
		{"http://localhost:5173/callback", true},
		{"http://127.0.0.1/callback", true},
		{"http://[::1]:8080/callback", true},
		{"http://client.example/callback", false},
		{"https://user@client.example/callback", false},
		{"https://client.example/callback#fragment", false},
		{"javascript:alert(1)", false},
		{"//client.example/callback", false},
		{"not a URL", false},
	} {
		t.Run(tc.uri, func(t *testing.T) {
			if got := validOAuthRedirectURI(tc.uri); got != tc.want {
				t.Errorf("validOAuthRedirectURI(%q) = %t, want %t", tc.uri, got, tc.want)
			}
		})
	}
}

func TestOAuthAuthorizationParametersAreAllowlisted(t *testing.T) {
	for _, key := range []string{"response_type", "client_id", "redirect_uri", "scope", "state", "code_challenge", "code_challenge_method", "resource"} {
		if !allowedOAuthAuthorizationParam(key) {
			t.Errorf("expected %q to be allowed", key)
		}
	}
	for _, key := range []string{"", "prompt", "login_hint", "redirect", "__proto__"} {
		if allowedOAuthAuthorizationParam(key) {
			t.Errorf("unexpectedly allowed %q", key)
		}
	}
}

func TestOAuthRedirectsPreserveClientQueryAndEncodeState(t *testing.T) {
	req := authorizationRequest{redirectURI: "https://client.example/callback?existing=1", state: "a&b=c"}
	got, err := url.Parse(oauthRedirectURL(req, "https://uplumo.example"))
	if err != nil {
		t.Fatal(err)
	}
	q := got.Query()
	if q.Get("existing") != "1" || q.Get("state") != req.state || q.Get("iss") != "https://uplumo.example" {
		t.Fatalf("redirect query = %v", q)
	}
	errorURL, err := url.Parse(oauthErrorRedirect(req, "access_denied", "user declined", "https://uplumo.example"))
	if err != nil {
		t.Fatal(err)
	}
	if errorURL.Query().Get("error") != "access_denied" || errorURL.Query().Get("error_description") != "user declined" || errorURL.Query().Get("state") != req.state {
		t.Fatalf("error redirect query = %v", errorURL.Query())
	}
}

func TestReadOAuthBodyRejectsOversizedRequest(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/oauth", strings.NewReader(strings.Repeat("x", maxOAuthBody+1)))
	rec := httptest.NewRecorder()
	if _, ok := readOAuthBody(rec, req); ok {
		t.Fatal("readOAuthBody accepted an oversized request")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || response["error"] != "invalid_request" {
		t.Fatalf("error response = %s (decode error: %v)", rec.Body.String(), err)
	}
}

func TestOAuthValueHashAndConstantTimeComparison(t *testing.T) {
	value := "secret value"
	if !constantTimeHashEqual(hashOAuthValue(value), value) {
		t.Fatal("expected hash to match raw value")
	}
	if constantTimeHashEqual(hashOAuthValue(value), "different value") {
		t.Fatal("different raw value matched")
	}
	if got := hashOAuthValue(value); got == value || got == "" {
		t.Fatalf("hashOAuthValue returned invalid hash %q", got)
	}
}

func TestValidPKCEVerifier(t *testing.T) {
	valid := strings.Repeat("a", 43)
	for _, value := range []string{valid, strings.Repeat("~._-Z9", 21) + "~"} {
		if !validPKCEVerifier(value) {
			t.Errorf("valid verifier rejected: %q", value)
		}
	}
	for _, value := range []string{"", strings.Repeat("a", 42), strings.Repeat("a", 129), strings.Repeat("a", 42) + " ", strings.Repeat("a", 42) + "/"} {
		if validPKCEVerifier(value) {
			t.Errorf("invalid verifier accepted: %q", value)
		}
	}
}

func TestSameOriginRequiresExactWebOrigin(t *testing.T) {
	for _, tc := range []struct {
		origin, expected string
		want             bool
	}{
		{"https://UPLUMO.example", "https://uplumo.example", true},
		{"https://uplumo.example", "https://uplumo.example", true},
		{"", "https://uplumo.example", false},
		{"null", "https://uplumo.example", false},
		{"https://uplumo.example.evil", "https://uplumo.example", false},
		{"http://uplumo.example", "https://uplumo.example", false},
		{"https://user@uplumo.example", "https://uplumo.example", false},
		{"https://uplumo.example/path", "https://uplumo.example", false},
	} {
		if got := sameOrigin(tc.origin, tc.expected); got != tc.want {
			t.Errorf("sameOrigin(%q, %q) = %t, want %t", tc.origin, tc.expected, got, tc.want)
		}
	}
}

func TestRequestTargetIncludesRawQueryOnlyWhenPresent(t *testing.T) {
	for _, tc := range []struct{ target, want string }{
		{"https://example.test/path", "/path"},
		{"https://example.test/path?a=1%2F2", "/path?a=1%2F2"},
	} {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, tc.target, nil)
		if got := requestTarget(r); got != tc.want {
			t.Errorf("requestTarget(%q) = %q, want %q", tc.target, got, tc.want)
		}
	}
}

func TestRandomOAuthValueProducesURLSafeEntropy(t *testing.T) {
	value, err := randomOAuthValue(32)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 || bytes.ContainsAny([]byte(value), "+/=") {
		t.Fatalf("random OAuth value is not 32 URL-safe bytes: %q (%v)", value, err)
	}
}

func TestPublicBaseURLRemovesPathQueryAndFragment(t *testing.T) {
	parsed, err := url.Parse("https://uplumo.example/proxy/path?token=secret#fragment")
	if err != nil {
		t.Fatal(err)
	}
	h := newHandler(Deps{PublicURL: parsed})
	if got, want := h.publicBaseURL(), "https://uplumo.example"; got != want {
		t.Fatalf("publicBaseURL() = %q, want %q", got, want)
	}
	if got := newHandler(Deps{}).publicBaseURL(); got != "" {
		t.Fatalf("nil public URL produced %q", got)
	}
}
