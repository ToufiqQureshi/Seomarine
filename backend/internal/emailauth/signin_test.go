package emailauth

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func signInBody(address, password string) map[string]any {
	return map[string]any{"email": address, "password": password}
}

func TestSignInStartsASessionTheRestOfTheServerAccepts(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	resp := e.request("POST", "/api/auth/sign-in/email", signInBody("Ada@Example.com", "correct horse battery"), originHeaders())

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, resp.raw)
	}
	cookie := resp.sessionCookie()
	if cookie == nil || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != 7*24*3600 {
		t.Fatalf("session cookie = %+v", cookie)
	}
	user, err := e.authenticate(cookie)
	if err != nil || user.ID != "user-1" {
		t.Fatalf("authenticate = %+v, %v", user, err)
	}
	if resp.json["redirect"] != false || resp.json["url"] != nil {
		t.Fatalf("body = %s", resp.raw)
	}
	token, _ := resp.json["token"].(string)
	var ip, agent *string
	var expires time.Time
	if err := e.pool.QueryRow(context.Background(), `SELECT ip_address, user_agent, expires_at FROM session WHERE token = $1`, token).Scan(&ip, &agent, &expires); err != nil {
		t.Fatalf("the token in the body must be the stored session: %v", err)
	}
	if ip == nil || *ip != "203.0.113.9" || agent == nil || *agent != "test-agent/1.0" {
		t.Fatalf("session metadata = %v %v", ip, agent)
	}
	if want := e.clock.Now().Add(7 * 24 * time.Hour); expires.Sub(want).Abs() > time.Second {
		t.Fatalf("expires_at = %v, want about %v", expires, want)
	}
}

func TestSignInWorksForAccountsBetterAuthCreated(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.exec(`INSERT INTO "user" (id, name, email, email_verified) VALUES ('legacy-user', 'Legacy', 'legacy@example.com', true)`)
	e.exec(`INSERT INTO account (id, account_id, provider_id, user_id, password, updated_at) VALUES ('a1', 'legacy-user', 'credential', 'legacy-user', $1, now())`, betterAuthHash)
	if resp := e.request("POST", "/api/auth/sign-in/email", signInBody("legacy@example.com", "correct horse battery"), originHeaders()); resp.StatusCode != 200 {
		t.Fatalf("a hash made by Better Auth must still sign in: %d %s", resp.StatusCode, resp.raw)
	}
}

func TestSignInFailuresAreIndistinguishable(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	e.exec(`INSERT INTO "user" (id, name, email, email_verified) VALUES ('social-user', 'Social', 'social@example.com', true)`)
	e.exec(`INSERT INTO "user" (id, name, email, email_verified) VALUES ('broken-user', 'Broken', 'broken@example.com', true)`)
	e.exec(`INSERT INTO account (id, account_id, provider_id, user_id, password, updated_at) VALUES ('a-broken', 'broken-user', 'credential', 'broken-user', 'not-a-hash', now())`)

	attempts := map[string]map[string]any{
		"wrong password":   signInBody("ada@example.com", "wrong password!"),
		"unknown address":  signInBody("nobody@example.com", "correct horse battery"),
		"social-only user": signInBody("social@example.com", "correct horse battery"),
		"corrupt hash":     signInBody("broken@example.com", "anything at all"),
	}
	for name, body := range attempts {
		t.Run(name, func(t *testing.T) {
			resp := e.request("POST", "/api/auth/sign-in/email", body, originHeaders())
			if resp.StatusCode != http.StatusUnauthorized || resp.errCode() != "INVALID_EMAIL_OR_PASSWORD" || resp.sessionCookie() != nil {
				t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
			}
		})
	}
	if n := e.count(`SELECT count(*) FROM session`); n != 0 {
		t.Fatalf("failed sign-ins created %d sessions", n)
	}
}

func TestSignInValidation(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	if resp := e.request("POST", "/api/auth/sign-in/email", signInBody("nope", "x"), originHeaders()); resp.errCode() != "INVALID_EMAIL" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	body := signInBody("a@example.com", "x")
	body["callbackURL"] = "https://evil.test"
	if resp := e.request("POST", "/api/auth/sign-in/email", body, originHeaders()); resp.StatusCode != 403 || resp.errCode() != "INVALID_CALLBACKURL" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	if resp := e.request("POST", "/api/auth/sign-in/email", "{", originHeaders()); resp.StatusCode != 400 {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestSignInRequiresAVerifiedEmail(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", false)
	resp := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "correct horse battery"), originHeaders())
	if resp.StatusCode != http.StatusForbidden || resp.errCode() != "EMAIL_NOT_VERIFIED" || resp.sessionCookie() != nil {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	// The password is checked first: a wrong one must not reveal that the
	// account exists but is unverified.
	if resp := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "wrong password!"), originHeaders()); resp.errCode() != "INVALID_EMAIL_OR_PASSWORD" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	e.svc.Cfg.BypassEmailVerification = true
	if resp := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "correct horse battery"), originHeaders()); resp.StatusCode != 200 {
		t.Fatalf("bypass mode signs unverified users in: %d %s", resp.StatusCode, resp.raw)
	}
}

func TestSignInWithCallbackURLRedirects(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	body := signInBody("ada@example.com", "correct horse battery")
	body["callbackURL"] = "/projects"
	resp := e.request("POST", "/api/auth/sign-in/email", body, originHeaders())
	if resp.json["redirect"] != true || resp.json["url"] != "/projects" || resp.Header.Get("Location") != "/projects" {
		t.Fatalf("got %s location=%q", resp.raw, resp.Header.Get("Location"))
	}
}

func TestRememberMeFalseGivesAShortBrowserSession(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	body := signInBody("ada@example.com", "correct horse battery")
	body["rememberMe"] = false
	resp := e.request("POST", "/api/auth/sign-in/email", body, originHeaders())
	cookie := resp.sessionCookie()
	if cookie == nil || cookie.MaxAge != 0 {
		t.Fatalf("a session the user did not want remembered must not outlive the browser: %+v", cookie)
	}
	if resp.cookie("__Secure-better-auth.dont_remember") == nil {
		t.Fatal("dont_remember marker missing")
	}
	var expires time.Time
	_ = e.pool.QueryRow(context.Background(), `SELECT expires_at FROM session`).Scan(&expires)
	if want := e.clock.Now().Add(24 * time.Hour); expires.Sub(want).Abs() > time.Second {
		t.Fatalf("expires_at = %v, want about %v", expires, want)
	}
}

func TestSignInExpiresTheCachedSessionSnapshot(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	resp := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "correct horse battery"), originHeaders())
	if c := resp.cookie("__Secure-better-auth.session_data"); c == nil || c.MaxAge >= 0 {
		t.Fatal("a previous user's cached session must not survive a new sign-in")
	}
}

func TestSignInPicksTheRightWorkspace(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	signIn := func() string {
		resp := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "correct horse battery"), originHeaders())
		user, err := e.authenticate(resp.sessionCookie())
		if err != nil {
			t.Fatalf("authenticate: %v (%s)", err, resp.raw)
		}
		return user.OrganizationID
	}

	first := signIn()
	if first == "" || e.count(`SELECT count(*) FROM organization`) != 1 {
		t.Fatalf("a user with no workspace gets one: org=%q", first)
	}
	var role, name, slug string
	_ = e.pool.QueryRow(context.Background(), `SELECT m.role, o.name, o.slug FROM member m JOIN organization o ON o.id = m.organization_id WHERE m.user_id = 'user-1'`).Scan(&role, &name, &slug)
	if role != "owner" || name != "Seeded User's organization" || !strings.HasPrefix(slug, "seeded-user-") {
		t.Fatalf("workspace = %s %q %q", role, name, slug)
	}
	if second := signIn(); second != first || e.count(`SELECT count(*) FROM organization`) != 1 {
		t.Fatalf("a second sign-in must reuse the workspace, got %q", second)
	}

	// The most recently joined workspace wins over the personal one.
	e.exec(`INSERT INTO organization (id, name, slug, created_at) VALUES ('org-team', 'Team', 'team', now())`)
	e.exec(`INSERT INTO member (id, organization_id, user_id, role, created_at) VALUES ('m-team', 'org-team', 'user-1', 'admin', now() + interval '1 minute')`)
	if got := signIn(); got != "org-team" {
		t.Fatalf("newest membership should win, got %q", got)
	}
	// The last active workspace wins over both, while the user still belongs to it.
	e.exec(`UPDATE "user" SET last_active_organization_id = $1 WHERE id = 'user-1'`, first)
	if got := signIn(); got != first {
		t.Fatalf("last active workspace should win, got %q", got)
	}
	// A workspace the user left is not used.
	e.exec(`DELETE FROM member WHERE organization_id = $1`, first)
	if got := signIn(); got != "org-team" {
		t.Fatalf("after leaving, fall back to the newest remaining membership, got %q", got)
	}
}

func TestSignInWithAPendingInvitationMintsNoWorkspace(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("inviter", "owner@example.com", "correct horse battery", true)
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	e.exec(`INSERT INTO organization (id, name, slug, created_at) VALUES ('org-a', 'A', 'a', now())`)
	e.exec(`INSERT INTO invitation (id, organization_id, email, role, status, expires_at, inviter_id) VALUES ('inv-1', 'org-a', 'ADA@example.com', 'admin', 'pending', now() + interval '1 day', 'inviter')`)

	resp := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "correct horse battery"), originHeaders())
	if resp.StatusCode != 200 {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	if n := e.count(`SELECT count(*) FROM member WHERE user_id = 'user-1'`); n != 0 {
		t.Fatalf("an invitee must end up only in the inviter's workspace, memberships = %d", n)
	}
	if n := e.count(`SELECT count(*) FROM session WHERE user_id = 'user-1' AND active_organization_id IS NULL`); n != 1 {
		t.Fatal("the session should carry no active workspace until the invitation is accepted")
	}

	// An expired invitation does not hold the workspace back.
	e.exec(`UPDATE invitation SET expires_at = now() - interval '1 day'`)
	if resp := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "correct horse battery"), originHeaders()); resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if n := e.count(`SELECT count(*) FROM member WHERE user_id = 'user-1'`); n != 1 {
		t.Fatalf("memberships = %d", n)
	}
}

func TestConcurrentFirstSignInsCreateOneWorkspace(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	results := make(chan int, 6)
	for range 6 {
		go func() {
			resp := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "correct horse battery"), originHeaders())
			results <- resp.StatusCode
		}()
	}
	for range 6 {
		if status := <-results; status != 200 {
			t.Fatalf("a sign-in lost the workspace race with status %d", status)
		}
	}
	if n := e.count(`SELECT count(*) FROM member WHERE user_id = 'user-1'`); n != 1 {
		t.Fatalf("memberships = %d, want exactly one workspace", n)
	}
}
