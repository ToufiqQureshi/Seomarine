package emailauth

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func verifyURL(e *authEnv, address, callback string) string {
	e.t.Helper()
	token, err := signJWT(testSecret, map[string]any{"email": address}, time.Hour, e.clock.Now())
	if err != nil {
		e.t.Fatal(err)
	}
	path := "/api/auth/verify-email?token=" + token
	if callback != "" {
		path += "&callbackURL=" + url.QueryEscape(callback)
	}
	return path
}

func TestVerifyEmailMarksTheAddressVerifiedAndSignsIn(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", false)
	resp := e.request("GET", verifyURL(e, "ada@example.com", "/onboarding"), nil, nil)

	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/onboarding" {
		t.Fatalf("got %d location=%q %s", resp.StatusCode, resp.Header.Get("Location"), resp.raw)
	}
	if n := e.count(`SELECT count(*) FROM "user" WHERE id = 'user-1' AND email_verified`); n != 1 {
		t.Fatal("the address was not marked verified")
	}
	user, err := e.authenticate(resp.sessionCookie())
	if err != nil || user.ID != "user-1" {
		t.Fatalf("verifying should sign the user in: %+v, %v", user, err)
	}
	// With verification done the user can sign in normally.
	if r := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "correct horse battery"), originHeaders()); r.StatusCode != 200 {
		t.Fatalf("sign-in after verify: %d %s", r.StatusCode, r.raw)
	}
}

func TestVerifyEmailWithoutCallbackReturnsJSON(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", false)
	resp := e.request("GET", verifyURL(e, "ada@example.com", ""), nil, nil)
	if resp.StatusCode != 200 || resp.json["status"] != true {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
}

func TestVerifyEmailBadLinks(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", false)
	expired, _ := signJWT(testSecret, map[string]any{"email": "ada@example.com"}, time.Hour, e.clock.Now().Add(-2*time.Hour))
	otherKey, _ := signJWT([]byte("another-secret-another-secret-123456"), map[string]any{"email": "ada@example.com"}, time.Hour, e.clock.Now())
	changeEmail, _ := signJWT(testSecret, map[string]any{"email": "ada@example.com", "updateTo": "evil@example.com"}, time.Hour, e.clock.Now())
	unknownUser, _ := signJWT(testSecret, map[string]any{"email": "ghost@example.com"}, time.Hour, e.clock.Now())
	notAnEmail, _ := signJWT(testSecret, map[string]any{"email": "nope"}, time.Hour, e.clock.Now())

	tests := []struct {
		name, token, code string
	}{
		{"expired", expired, "TOKEN_EXPIRED"},
		{"signed with another key", otherKey, "INVALID_TOKEN"},
		{"garbage", "abc.def.ghi", "INVALID_TOKEN"},
		{"change-email token", changeEmail, "INVALID_TOKEN"},
		{"unknown user", unknownUser, "USER_NOT_FOUND"},
		{"claim is not an address", notAnEmail, "INVALID_TOKEN"},
	}
	for _, tt := range tests {
		t.Run(tt.name+" with callback", func(t *testing.T) {
			resp := e.request("GET", "/api/auth/verify-email?token="+tt.token+"&callbackURL=%2Fsign-in", nil, nil)
			want := "/sign-in?error=" + tt.code
			if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != want {
				t.Fatalf("got %d location=%q, want %q", resp.StatusCode, resp.Header.Get("Location"), want)
			}
		})
		t.Run(tt.name+" without callback", func(t *testing.T) {
			resp := e.request("GET", "/api/auth/verify-email?token="+tt.token, nil, nil)
			if resp.StatusCode != http.StatusUnauthorized || resp.errCode() != tt.code {
				t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
			}
		})
	}
	if n := e.count(`SELECT count(*) FROM "user" WHERE email_verified`); n != 0 || e.count(`SELECT count(*) FROM session`) != 0 {
		t.Fatal("a bad link verified an address or created a session")
	}
}

func TestVerifyEmailRefusesForeignCallbacksAndMissingTokens(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", false)
	resp := e.request("GET", verifyURL(e, "ada@example.com", "https://evil.test/"), nil, nil)
	if resp.StatusCode != http.StatusForbidden || resp.errCode() != "INVALID_CALLBACKURL" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	if resp := e.request("GET", "/api/auth/verify-email", nil, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing token got %d", resp.StatusCode)
	}
}

func TestVerifyingAnAlreadyVerifiedAddressStartsNoSession(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	resp := e.request("GET", verifyURL(e, "ada@example.com", "/x"), nil, nil)
	if resp.StatusCode != http.StatusFound || resp.sessionCookie() != nil || e.count(`SELECT count(*) FROM session`) != 0 {
		t.Fatalf("an old verification link must not sign anyone in: %d", resp.StatusCode)
	}
}

func TestSendVerificationEmail(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("unverified", "new@example.com", "correct horse battery", false)
	e.seedUser("verified", "old@example.com", "correct horse battery", true)
	send := func(address string) *response {
		return e.request("POST", "/api/auth/send-verification-email", map[string]any{"email": address, "callbackURL": "/welcome"}, originHeaders())
	}

	started := time.Now()
	resp := send("new@example.com")
	if resp.StatusCode != 200 || resp.json["status"] != true || len(e.mailer.sent()) != 1 {
		t.Fatalf("got %d %s mails=%d", resp.StatusCode, resp.raw, len(e.mailer.sent()))
	}
	if elapsed := time.Since(started); elapsed < verificationFloor-50*time.Millisecond {
		t.Fatalf("took %v: a signed-out caller must wait out the floor", elapsed)
	}
	for _, address := range []string{"old@example.com", "nobody@example.com"} {
		started := time.Now()
		resp := send(address)
		if resp.StatusCode != 200 || resp.json["status"] != true {
			t.Fatalf("%s: got %d %s", address, resp.StatusCode, resp.raw)
		}
		if elapsed := time.Since(started); elapsed < verificationFloor-50*time.Millisecond {
			t.Fatalf("%s answered in %v: timing would reveal that the address is unknown or verified", address, elapsed)
		}
	}
	if len(e.mailer.sent()) != 1 {
		t.Fatal("no email may go to a verified or unknown address")
	}
	if resp := send("not-an-email"); resp.errCode() != "INVALID_EMAIL" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
}

func TestSendVerificationEmailForTheSignedInUser(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	cookie := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "correct horse battery"), originHeaders()).sessionCookie()
	send := func(address string) *response {
		return e.request("POST", "/api/auth/send-verification-email", map[string]any{"email": address}, originHeaders(), cookie)
	}
	if resp := send("someone-else@example.com"); resp.errCode() != "EMAIL_MISMATCH" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	if resp := send("ADA@example.com"); resp.errCode() != "EMAIL_ALREADY_VERIFIED" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
}

func TestVerificationEmailsAreOffWithoutAMailerOrWhenBypassed(t *testing.T) {
	for name, opts := range map[string]envOptions{"no mailer": {noMailer: true}, "bypass": {bypass: true}} {
		t.Run(name, func(t *testing.T) {
			e := newAuthEnv(t, opts)
			resp := e.request("POST", "/api/auth/send-verification-email", map[string]any{"email": "a@example.com"}, originHeaders())
			if resp.StatusCode != 400 || resp.errCode() != "VERIFICATION_EMAIL_NOT_ENABLED" {
				t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
			}
		})
	}
}

func resetToken(t *testing.T, e *authEnv) string {
	t.Helper()
	mails := e.mailer.sent()
	link, err := url.Parse(mails[len(mails)-1].url)
	if err != nil {
		t.Fatal(err)
	}
	return link.Path[strings.LastIndex(link.Path, "/")+1:]
}

func TestRequestPasswordReset(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	reset := func(address string) *response {
		return e.request("POST", "/api/auth/request-password-reset", map[string]any{"email": address, "redirectTo": "/reset-password"}, originHeaders())
	}

	unknown := reset("nobody@example.com")
	known := reset("Ada@Example.com")
	if unknown.StatusCode != 200 || known.StatusCode != 200 || string(unknown.raw) != string(known.raw) {
		t.Fatalf("answers differ:\n%s\n%s", unknown.raw, known.raw)
	}
	if known.json["message"] != genericResetMessage {
		t.Fatalf("message = %v", known.json["message"])
	}
	mails := e.mailer.sent()
	if len(mails) != 1 || mails[0].kind != "reset" || mails[0].address != "ada@example.com" {
		t.Fatalf("mails = %+v", mails)
	}
	link, _ := url.Parse(mails[0].url)
	token := resetToken(t, e)
	if link.Host != "app.uplumo.test" || !strings.HasPrefix(link.Path, "/api/auth/reset-password/") || len(token) != 24 || link.Query().Get("callbackURL") != "/reset-password" {
		t.Fatalf("link = %s", mails[0].url)
	}
	var value string
	var expires time.Time
	if err := e.pool.QueryRow(context.Background(), `SELECT value, expires_at FROM verification WHERE identifier = $1`, "reset-password:"+token).Scan(&value, &expires); err != nil {
		t.Fatalf("the token must be stored: %v", err)
	}
	if value != "user-1" || expires.Sub(e.clock.Now().Add(time.Hour)).Abs() > time.Second {
		t.Fatalf("stored value=%q expires=%v", value, expires)
	}
}

func TestRequestPasswordResetEdgeCases(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	e.mailer.fail = errBoom
	resp := e.request("POST", "/api/auth/request-password-reset", map[string]any{"email": "ada@example.com"}, originHeaders())
	if resp.StatusCode != 200 || resp.json["status"] != true {
		t.Fatalf("a mail failure must not reveal that the address exists: %d %s", resp.StatusCode, resp.raw)
	}
	if resp := e.request("POST", "/api/auth/request-password-reset", map[string]any{"email": "ada@example.com", "redirectTo": "https://evil.test"}, originHeaders()); resp.StatusCode != 403 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if resp := e.request("POST", "/api/auth/request-password-reset", map[string]any{"email": "bad"}, originHeaders()); resp.errCode() != "INVALID_EMAIL" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	off := newAuthEnv(t, envOptions{noMailer: true})
	if resp := off.request("POST", "/api/auth/request-password-reset", map[string]any{"email": "ada@example.com"}, originHeaders()); resp.errCode() != "RESET_PASSWORD_DISABLED" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
}

func TestResetPasswordCallbackRedirectsWithTheTokenOrAnError(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	e.request("POST", "/api/auth/request-password-reset", map[string]any{"email": "ada@example.com", "redirectTo": "/reset-password"}, originHeaders())
	token := resetToken(t, e)

	resp := e.request("GET", "/api/auth/reset-password/"+token+"?callbackURL=%2Freset-password", nil, nil)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/reset-password?token="+token {
		t.Fatalf("got %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	// Following the link must not use the token up.
	if n := e.count(`SELECT count(*) FROM verification WHERE identifier = $1`, "reset-password:"+token); n != 1 {
		t.Fatal("the callback consumed the token")
	}
	bad := e.request("GET", "/api/auth/reset-password/nope?callbackURL=%2Freset-password%3Ffrom%3Demail", nil, nil)
	if bad.Header.Get("Location") != "/reset-password?from=email&error=INVALID_TOKEN" {
		t.Fatalf("location = %q", bad.Header.Get("Location"))
	}
	if resp := e.request("GET", "/api/auth/reset-password/"+token, nil, nil); resp.StatusCode != 400 {
		t.Fatalf("a missing callbackURL got %d", resp.StatusCode)
	}
	if resp := e.request("GET", "/api/auth/reset-password/"+token+"?callbackURL=https%3A%2F%2Fevil.test", nil, nil); resp.StatusCode != 403 {
		t.Fatalf("a foreign callbackURL got %d", resp.StatusCode)
	}
	e.clock.advance(2 * time.Hour)
	expired := e.request("GET", "/api/auth/reset-password/"+token+"?callbackURL=%2Freset-password", nil, nil)
	if !strings.HasSuffix(expired.Header.Get("Location"), "error=INVALID_TOKEN") {
		t.Fatalf("an expired token got %q", expired.Header.Get("Location"))
	}
}

func TestResetPasswordChangesThePasswordOnceAndSignsOutEverywhere(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	cookie := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "correct horse battery"), originHeaders()).sessionCookie()
	e.request("POST", "/api/auth/request-password-reset", map[string]any{"email": "ada@example.com"}, originHeaders())
	token := resetToken(t, e)

	short := e.request("POST", "/api/auth/reset-password", map[string]any{"newPassword": "short", "token": token}, originHeaders())
	if short.errCode() != "PASSWORD_TOO_SHORT" {
		t.Fatalf("got %d %s", short.StatusCode, short.raw)
	}
	if n := e.count(`SELECT count(*) FROM verification WHERE identifier = $1`, "reset-password:"+token); n != 1 {
		t.Fatal("a rejected password must not use the token up")
	}

	ok := e.request("POST", "/api/auth/reset-password", map[string]any{"newPassword": "a brand new password", "token": token}, originHeaders())
	if ok.StatusCode != 200 || ok.json["status"] != true {
		t.Fatalf("got %d %s", ok.StatusCode, ok.raw)
	}
	if r := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "correct horse battery"), originHeaders()); r.StatusCode != 401 {
		t.Fatal("the old password still works")
	}
	if r := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "a brand new password"), originHeaders()); r.StatusCode != 200 {
		t.Fatalf("the new password does not work: %d %s", r.StatusCode, r.raw)
	}
	if _, err := e.authenticate(cookie); err == nil {
		t.Fatal("a session from before the reset must be revoked")
	}
	again := e.request("POST", "/api/auth/reset-password", map[string]any{"newPassword": "yet another password", "token": token}, originHeaders())
	if again.StatusCode != 400 || again.errCode() != "INVALID_TOKEN" {
		t.Fatalf("a reset link is single-use: %d %s", again.StatusCode, again.raw)
	}
}

func TestResetPasswordTokenInTheQueryAndForSocialOnlyUsers(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.exec(`INSERT INTO "user" (id, name, email, email_verified) VALUES ('social', 'Social', 'social@example.com', true)`)
	e.request("POST", "/api/auth/request-password-reset", map[string]any{"email": "social@example.com"}, originHeaders())
	token := resetToken(t, e)
	resp := e.request("POST", "/api/auth/reset-password?token="+token, map[string]any{"newPassword": "a brand new password"}, originHeaders())
	if resp.StatusCode != 200 {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	if r := e.request("POST", "/api/auth/sign-in/email", signInBody("social@example.com", "a brand new password"), originHeaders()); r.StatusCode != 200 {
		t.Fatalf("a user who only had a provider login gets a password: %d %s", r.StatusCode, r.raw)
	}
}

func TestResetPasswordRejectsUnknownAndExpiredTokens(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	e.request("POST", "/api/auth/request-password-reset", map[string]any{"email": "ada@example.com"}, originHeaders())
	token := resetToken(t, e)
	for name, body := range map[string]map[string]any{
		"no token":      {"newPassword": "a brand new password"},
		"unknown token": {"newPassword": "a brand new password", "token": "nope"},
	} {
		if resp := e.request("POST", "/api/auth/reset-password", body, originHeaders()); resp.StatusCode != 400 || resp.errCode() != "INVALID_TOKEN" {
			t.Errorf("%s: got %d %s", name, resp.StatusCode, resp.raw)
		}
	}
	e.clock.advance(61 * time.Minute)
	if resp := e.request("POST", "/api/auth/reset-password", map[string]any{"newPassword": "a brand new password", "token": token}, originHeaders()); resp.errCode() != "INVALID_TOKEN" {
		t.Fatalf("an expired token got %d %s", resp.StatusCode, resp.raw)
	}
}

func TestConcurrentResetsOfOneTokenSucceedOnce(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	e.request("POST", "/api/auth/request-password-reset", map[string]any{"email": "ada@example.com"}, originHeaders())
	token := resetToken(t, e)

	var wg sync.WaitGroup
	statuses := make(chan int, 8)
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := e.request("POST", "/api/auth/reset-password", map[string]any{"newPassword": "a brand new password " + string(rune('a'+i)), "token": token}, originHeaders())
			statuses <- resp.StatusCode
		}()
	}
	wg.Wait()
	close(statuses)
	succeeded := 0
	for status := range statuses {
		if status == 200 {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d of 8 racing resets succeeded, want exactly 1", succeeded)
	}
}

func TestUpdateUser(t *testing.T) {
	e := newAuthEnv(t, envOptions{})
	e.seedUser("user-1", "ada@example.com", "correct horse battery", true)
	cookie := e.request("POST", "/api/auth/sign-in/email", signInBody("ada@example.com", "correct horse battery"), originHeaders()).sessionCookie()
	update := func(body any, cookies ...*http.Cookie) *response {
		return e.request("POST", "/api/auth/update-user", body, originHeaders(), cookies...)
	}

	if resp := update(map[string]any{"name": "Hacker"}); resp.StatusCode != 401 || resp.errCode() != "UNAUTHORIZED" {
		t.Fatalf("anonymous update got %d %s", resp.StatusCode, resp.raw)
	}
	if resp := update(map[string]any{"name": "Ada Byron"}, cookie); resp.StatusCode != 200 || resp.json["status"] != true {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	var name string
	_ = e.pool.QueryRow(context.Background(), `SELECT name FROM "user" WHERE id = 'user-1'`).Scan(&name)
	if name != "Ada Byron" {
		t.Fatalf("name = %q", name)
	}
	// false is a real value, not "unset".
	e.exec(`UPDATE "user" SET analytics_opted_out = true WHERE id = 'user-1'`)
	if resp := update(map[string]any{"analyticsOptedOut": false}, cookie); resp.StatusCode != 200 {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	if n := e.count(`SELECT count(*) FROM "user" WHERE id = 'user-1' AND analytics_opted_out = false`); n != 1 {
		t.Fatal("analyticsOptedOut=false was ignored")
	}
	if resp := update(map[string]any{"email": "new@example.com"}, cookie); resp.errCode() != "EMAIL_CAN_NOT_BE_UPDATED" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	if resp := update(map[string]any{}, cookie); resp.StatusCode != 400 || resp.json["message"] != "No fields to update" {
		t.Fatalf("got %d %s", resp.StatusCode, resp.raw)
	}
	// One user's session can only change that user's own row.
	e.seedUser("user-2", "bob@example.com", "correct horse battery", true)
	update(map[string]any{"name": "Changed"}, cookie)
	_ = e.pool.QueryRow(context.Background(), `SELECT name FROM "user" WHERE id = 'user-2'`).Scan(&name)
	if name != "Seeded User" {
		t.Fatalf("another user's profile was changed: %q", name)
	}
}
