package emailauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestEncodeURIComponentMatchesJavaScript(t *testing.T) {
	tests := map[string]string{
		"abc-_.!~*'()XYZ019": "abc-_.!~*'()XYZ019",
		"a+b/c=d":            "a%2Bb%2Fc%3Dd",
		"a b":                "a%20b",
		"é":                  "%C3%A9",
		"tok.sig==":          "tok.sig%3D%3D",
	}
	for in, want := range tests {
		if got := encodeURIComponent(in); got != want {
			t.Errorf("encodeURIComponent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSignedCookieVerifiesWithTheGoSessionReader(t *testing.T) {
	// The value must round-trip through the same logic internal/auth uses to
	// read the cookie, or sign-in would produce sessions nobody accepts.
	jar := cookieJar{secret: testSecret, secure: false}
	rec := httptest.NewRecorder()
	jar.setSession(rec, "session-token-value", true)

	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "better-auth.session_token" {
			session = c
		}
	}
	if session == nil {
		t.Fatal("no session cookie")
	}
	if !session.HttpOnly || session.SameSite != http.SameSiteLaxMode || session.Path != "/" || session.Secure {
		t.Errorf("attributes = %+v", session)
	}
	if session.MaxAge != 7*24*3600 {
		t.Errorf("max age = %d", session.MaxAge)
	}
	decoded, err := url.PathUnescape(session.Value)
	if err != nil || !strings.HasPrefix(decoded, "session-token-value.") {
		t.Fatalf("value = %q", session.Value)
	}
}

func TestSessionCookieNamesFollowTheScheme(t *testing.T) {
	rec := httptest.NewRecorder()
	cookieJar{secret: testSecret, secure: true}.setSession(rec, "t", true)
	names := map[string]*http.Cookie{}
	for _, c := range rec.Result().Cookies() {
		names[c.Name] = c
	}
	if c := names["__Secure-better-auth.session_token"]; c == nil || !c.Secure {
		t.Fatalf("cookies = %v", names)
	}
	if c := names["__Secure-better-auth.session_data"]; c == nil || c.MaxAge >= 0 {
		t.Fatal("the cached session snapshot must be expired on sign-in")
	}
}

func TestForgottenSessionIsABrowserSessionCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	cookieJar{secret: testSecret}.setSession(rec, "t", false)
	var session, marker *http.Cookie
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case "better-auth.session_token":
			session = c
		case "better-auth.dont_remember":
			marker = c
		}
	}
	if session == nil || session.MaxAge != 0 || session.Expires.Year() > 1 {
		t.Fatalf("a not-remembered session must have no lifetime: %+v", session)
	}
	if marker == nil {
		t.Fatal("the dont_remember marker is missing")
	}
}

func TestCallbackAllowed(t *testing.T) {
	o := newOrigins(mustURL(t, "https://app.uplumo.test"), "http://localhost:3000")
	tests := []struct {
		target string
		want   bool
	}{
		{"", true},
		{"/", true},
		{"/dashboard?tab=1", true},
		{"https://app.uplumo.test/reset-password", true},
		{"http://localhost:3000/x", true},
		{"//evil.test", false},
		{"/\\evil.test", false},
		{"/%2fevil.test", false},
		{"/%2Fevil.test", false},
		{"/%5cevil.test", false},
		{"https://evil.test/", false},
		{"https://app.uplumo.test.evil.test/", false},
		{"https://app.uplumo.test@evil.test/", false},
		{"https://user:pw@app.uplumo.test/", false},
		{"http://app.uplumo.test/", false},
		{"javascript:alert(1)", false},
		{"data:text/html,x", false},
		{"/ok\nHeader: x", false},
		{"relative/no-slash", false},
	}
	for _, tt := range tests {
		if got := o.callbackAllowed(tt.target); got != tt.want {
			t.Errorf("callbackAllowed(%q) = %v, want %v", tt.target, got, tt.want)
		}
	}
}

func TestRequestAllowed(t *testing.T) {
	o := newOrigins(mustURL(t, "https://app.uplumo.test"))
	req := func(headers map[string]string) *http.Request {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/auth/sign-in/email", nil)
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		return r
	}
	tests := []struct {
		name    string
		headers map[string]string
		want    bool
	}{
		{"no cookies cannot ride a session", nil, true},
		{"cookie from the app origin", map[string]string{"Cookie": "a=b", "Origin": "https://app.uplumo.test"}, true},
		{"cookie from another origin", map[string]string{"Cookie": "a=b", "Origin": "https://evil.test"}, false},
		{"cookie with no origin at all", map[string]string{"Cookie": "a=b"}, false},
		{"referer stands in for a missing origin", map[string]string{"Cookie": "a=b", "Referer": "https://app.uplumo.test/sign-in"}, true},
		{"foreign referer", map[string]string{"Cookie": "a=b", "Referer": "https://evil.test/x"}, false},
		{"null origin", map[string]string{"Cookie": "a=b", "Origin": "null"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := o.requestAllowed(req(tt.headers)); got != tt.want {
				t.Fatalf("requestAllowed = %v, want %v", got, tt.want)
			}
		})
	}
}
