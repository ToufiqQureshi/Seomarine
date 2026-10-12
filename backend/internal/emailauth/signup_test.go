package emailauth

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func signUpBody(address string) map[string]any {
	return map[string]any{"name": "Ada Lovelace", "email": address, "password": "correct horse battery", "callbackURL": "/onboarding"}
}

func TestSignUpCreatesAnUnverifiedAccountAndEmailsALink(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	resp := e.request("POST", "/api/auth/sign-up/email", signUpBody("Ada@Example.com"), originHeaders())

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, resp.raw)
	}
	if resp.json["token"] != nil {
		t.Fatalf("token = %v: a sign-up that needs verification must not sign anyone in", resp.json["token"])
	}
	user, _ := resp.json["user"].(map[string]any)
	if user["email"] != "ada@example.com" || user["emailVerified"] != false || user["name"] != "Ada Lovelace" || user["id"] == "" {
		t.Fatalf("user = %v", user)
	}
	if created, _ := user["createdAt"].(string); !strings.HasSuffix(created, "Z") || len(created) != len("2026-01-01T00:00:00.000Z") {
		t.Fatalf("createdAt = %q, want an ISO timestamp with milliseconds", created)
	}
	if resp.sessionCookie() != nil || e.count(`SELECT count(*) FROM session`) != 0 {
		t.Fatal("no session may exist before the email is verified")
	}

	var hash string
	var verified bool
	if err := e.pool.QueryRow(context.Background(), `SELECT a.password, u.email_verified FROM account a JOIN "user" u ON u.id = a.user_id WHERE u.email = 'ada@example.com' AND a.provider_id = 'credential'`).Scan(&hash, &verified); err != nil {
		t.Fatal(err)
	}
	if ok, _ := e.svc.Hasher.Verify(context.Background(), hash, "correct horse battery"); !ok || verified {
		t.Fatalf("stored hash verifies = %v, verified = %v", ok, verified)
	}

	mails := e.mailer.sent()
	if len(mails) != 1 || mails[0].kind != "verify" || mails[0].address != "ada@example.com" {
		t.Fatalf("mails = %+v", mails)
	}
	link, err := url.Parse(mails[0].url)
	if err != nil || link.Host != "app.uplumo.test" || link.Path != "/api/auth/verify-email" {
		t.Fatalf("verification link = %s", mails[0].url)
	}
	if got := link.Query().Get("callbackURL"); got != "/onboarding" {
		t.Fatalf("callbackURL = %q", got)
	}
	claims, err := verifyJWT(testSecret, link.Query().Get("token"), e.clock.Now())
	if err != nil || claims["email"] != "ada@example.com" {
		t.Fatalf("token claims = %v err = %v", claims, err)
	}
}

func TestSignUpOfAnExistingAddressLooksIdentical(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("existing-user", "taken@example.com", "some password", true)
	resp := e.request("POST", "/api/auth/sign-up/email", signUpBody("taken@example.com"), originHeaders())

	if resp.StatusCode != http.StatusOK || resp.json["token"] != nil {
		t.Fatalf("status = %d body = %s", resp.StatusCode, resp.raw)
	}
	user, _ := resp.json["user"].(map[string]any)
	if user["id"] == "existing-user" || user["emailVerified"] != false {
		t.Fatalf("the response must not expose the real account: %v", user)
	}
	if n := e.count(`SELECT count(*) FROM "user" WHERE email = 'taken@example.com'`); n != 1 {
		t.Fatalf("users = %d", n)
	}
	if len(e.mailer.sent()) != 0 {
		t.Fatal("no email may go out for an address that already has an account")
	}
	var hash string
	_ = e.pool.QueryRow(context.Background(), `SELECT password FROM account WHERE user_id = 'existing-user'`).Scan(&hash)
	if ok, _ := e.svc.Hasher.Verify(context.Background(), hash, "some password"); !ok {
		t.Fatal("the existing password was overwritten")
	}
}

func TestSignUpWithoutVerificationSignsTheUserIn(t *testing.T) {
	e := newAuthEnv(t, envOptions{bypass: true})
	resp := e.request("POST", "/api/auth/sign-up/email", signUpBody("dev@example.com"), originHeaders())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, resp.raw)
	}
	cookie := resp.sessionCookie()
	token, _ := resp.json["token"].(string)
	if cookie == nil || token == "" {
		t.Fatalf("cookie = %v token = %q", cookie, token)
	}
	user, err := e.authenticate(cookie)
	if err != nil || user.Email != "dev@example.com" {
		t.Fatalf("authenticate = %+v, %v", user, err)
	}
	if user.OrganizationID == "" || user.Role != "owner" {
		t.Fatalf("a new user must land in their own workspace as owner: %+v", user)
	}
	if len(e.mailer.sent()) != 0 {
		t.Fatal("bypass mode sends no verification email")
	}

	dup := e.request("POST", "/api/auth/sign-up/email", signUpBody("dev@example.com"), originHeaders())
	if dup.StatusCode != http.StatusUnprocessableEntity || dup.errCode() != "USER_ALREADY_EXISTS_USE_ANOTHER_EMAIL" {
		t.Fatalf("duplicate = %d %s", dup.StatusCode, dup.raw)
	}
}

func TestSignUpValidation(t *testing.T) {
	e := newAuthEnv(t, envOptions{disposable: true})
	tests := []struct {
		name   string
		body   any
		status int
		code   string
	}{
		{"missing email", map[string]any{"name": "A", "password": "correct horse"}, 400, "INVALID_EMAIL"},
		{"malformed email", map[string]any{"name": "A", "email": "not-an-email", "password": "correct horse"}, 400, "INVALID_EMAIL"},
		{"email with consecutive dots", map[string]any{"name": "A", "email": "a..b@example.com", "password": "correct horse"}, 400, "INVALID_EMAIL"},
		{"no password", map[string]any{"name": "A", "email": "a@example.com"}, 400, "INVALID_PASSWORD"},
		{"password too short", map[string]any{"name": "A", "email": "a@example.com", "password": "short"}, 400, "PASSWORD_TOO_SHORT"},
		{"password too long", map[string]any{"name": "A", "email": "a@example.com", "password": strings.Repeat("x", 129)}, 400, "PASSWORD_TOO_LONG"},
		{"disposable domain", map[string]any{"name": "A", "email": "Throwaway@Mailinator.com", "password": "correct horse"}, 400, "BAD_REQUEST"},
		{"callback to another site", map[string]any{"name": "A", "email": "a@example.com", "password": "correct horse", "callbackURL": "https://evil.test/"}, 403, "INVALID_CALLBACKURL"},
		{"not json", "{nope", 400, "BAD_REQUEST"},
		{"array body", "[1,2]", 400, "BAD_REQUEST"},
		{"oversized body", strings.Repeat("a", maxBodyBytes+10), 400, "BAD_REQUEST"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := e.request("POST", "/api/auth/sign-up/email", tt.body, originHeaders())
			if resp.StatusCode != tt.status || resp.errCode() != tt.code {
				t.Fatalf("got %d %s, want %d %s", resp.StatusCode, resp.raw, tt.status, tt.code)
			}
		})
	}
	if n := e.count(`SELECT count(*) FROM "user"`); n != 0 {
		t.Fatalf("a rejected sign-up created %d users", n)
	}
}

func TestPasswordLengthCountsUTF16Units(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	// Four emoji are four characters but eight UTF-16 units: long enough, as
	// the JavaScript length check the limits were written against sees it.
	body := map[string]any{"name": "A", "email": "emoji@example.com", "password": "😀😀😀😀"}
	if resp := e.request("POST", "/api/auth/sign-up/email", body, originHeaders()); resp.StatusCode != 200 {
		t.Fatalf("status = %d %s", resp.StatusCode, resp.raw)
	}
	body["email"] = "emoji2@example.com"
	body["password"] = "😀😀😀"
	if resp := e.request("POST", "/api/auth/sign-up/email", body, originHeaders()); resp.errCode() != "PASSWORD_TOO_SHORT" {
		t.Fatalf("three emoji are six units: %d %s", resp.StatusCode, resp.raw)
	}
}

func TestSignUpRejectsCrossSiteRequestsThatCarryCookies(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	cookie := &http.Cookie{Name: "better-auth.session_token", Value: "x"}
	resp := e.request("POST", "/api/auth/sign-up/email", signUpBody("csrf@example.com"), map[string]string{"Origin": "https://evil.test"}, cookie)
	if resp.StatusCode != http.StatusForbidden || resp.errCode() != "INVALID_ORIGIN" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	// Without cookies there is no session to ride, so a script may call the API.
	if resp := e.request("POST", "/api/auth/sign-up/email", signUpBody("script@example.com"), map[string]string{"Origin": "https://evil.test"}); resp.StatusCode != 200 {
		t.Fatalf("cookie-less request got %d %s", resp.StatusCode, resp.raw)
	}
}

func TestSignUpCaptcha(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		err     error
		status  int
		code    string
		created bool
	}{
		{"missing response", "", nil, 400, "MISSING_RESPONSE", false},
		{"rejected", "bad", errCaptchaRejected, 403, "VERIFICATION_FAILED", false},
		{"service down", "tok", errBoom, 500, "UNKNOWN_ERROR", false},
		{"solved", "good", nil, 200, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAuthEnv(t, envOptions{captcha: true})
			e.captcha.err = tt.err
			headers := originHeaders()
			if tt.header != "" {
				headers["x-captcha-response"] = tt.header
			}
			resp := e.request("POST", "/api/auth/sign-up/email", signUpBody("captcha@example.com"), headers)
			if resp.StatusCode != tt.status || resp.errCode() != tt.code {
				t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
			}
			if created := e.count(`SELECT count(*) FROM "user"`) == 1; created != tt.created {
				t.Fatalf("account created = %v, want %v", created, tt.created)
			}
			if tt.header != "" && (e.captcha.response != tt.header || e.captcha.ip != "203.0.113.9") {
				t.Fatalf("captcha saw response %q ip %q", e.captcha.response, e.captcha.ip)
			}
		})
	}
}

func TestSignUpFailsVisiblyWhenTheEmailCannotBeSent(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.mailer.fail = errBoom
	resp := e.request("POST", "/api/auth/sign-up/email", signUpBody("mailfail@example.com"), originHeaders())
	if resp.StatusCode != http.StatusInternalServerError || resp.errCode() != "INTERNAL_SERVER_ERROR" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	if strings.Contains(string(resp.raw), "boom") {
		t.Fatal("internal error text leaked to the client")
	}
}

func TestSignUpSyncsTheMarketingContactInTheBackground(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.request("POST", "/api/auth/sign-up/email", signUpBody("contact@example.com"), originHeaders())
	waitFor(t, func() bool {
		e.mailer.mu.Lock()
		defer e.mailer.mu.Unlock()
		return len(e.mailer.contacts) == 1 && e.mailer.contacts[0] == "contact@example.com"
	})
}

func TestRateLimitedRequestsAreRefusedAndRedisOutagesFailOpen(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.limiter.deny = true
	resp := e.request("POST", "/api/auth/sign-in/email", map[string]any{"email": "a@example.com", "password": "x"}, originHeaders())
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") != "31" || resp.errCode() != "TOO_MANY_REQUESTS" {
		t.Fatalf("got %d retry=%q %s", resp.StatusCode, resp.Header.Get("Retry-After"), resp.raw)
	}
	if len(e.limiter.keys) != 1 || e.limiter.keys[0] != "sign-in:203.0.113.9" {
		t.Fatalf("limiter keys = %v", e.limiter.keys)
	}

	e.limiter.deny, e.limiter.err = false, errBoom
	resp = e.request("POST", "/api/auth/sign-in/email", map[string]any{"email": "a@example.com", "password": "x"}, originHeaders())
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a limiter outage must not block sign-in: got %d", resp.StatusCode)
	}
}
