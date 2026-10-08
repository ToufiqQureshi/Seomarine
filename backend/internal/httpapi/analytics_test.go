package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/seomarine/backend/internal/analytics"
	"github.com/toufiqqureshi/seomarine/backend/internal/analytics/geo"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
)

func TestTrackerScriptIsCacheable(t *testing.T) {
	handler := newTestHandler(t, nil, healthy, healthy, "http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t.js", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != string(trackerJS) {
		t.Fatalf("status = %d, body matches script = %v", rec.Code, rec.Body.String() == string(trackerJS))
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "public") || !strings.Contains(cc, "max-age=") {
		t.Errorf("Cache-Control = %q", cc)
	}

	req := httptest.NewRequest(http.MethodGet, "/t.js", nil)
	req.Header.Set("If-None-Match", rec.Header().Get("ETag"))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("revalidation status = %d, want %d", rec.Code, http.StatusNotModified)
	}
}

// Bad events are refused before the service runs; the nil service would
// panic if one got through.
func TestCollectRejectsInvalidEvents(t *testing.T) {
	long := "https://acme.com/" + strings.Repeat("a", maxURL)
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "not JSON", body: "k=abc", wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "empty body", body: "", wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "wrong type", body: `{"k":"abc","u":"https://acme.com/","w":"wide"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "missing site key", body: `{"u":"https://acme.com/"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "site key too long", body: fmt.Sprintf(`{"k":%q,"u":"https://acme.com/"}`, strings.Repeat("k", maxSiteKey+1)), wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "relative URL", body: `{"k":"abc","u":"/pricing"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "non-http URL", body: `{"k":"abc","u":"file:///etc/passwd"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "URL without host", body: `{"k":"abc","u":"https:///x"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "URL too long", body: fmt.Sprintf(`{"k":"abc","u":%q}`, long[:maxURL+1]), wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "referrer too long", body: fmt.Sprintf(`{"k":"abc","u":"https://acme.com/","r":%q}`, long[:maxURL+1]), wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "negative width", body: `{"k":"abc","u":"https://acme.com/","w":-1}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "absurd width", body: `{"k":"abc","u":"https://acme.com/","w":100001}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_event"},
		{name: "body over the limit", body: fmt.Sprintf(`{"k":"abc","u":"https://acme.com/","x":%q}`, strings.Repeat("x", maxCollectBody)), wantStatus: http.StatusRequestEntityTooLarge, wantCode: "payload_too_large"},
	}
	handler := newTestHandler(t, nil, healthy, healthy, "http://127.0.0.1:1")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(tt.body)))
			assertError(t, rec, tt.wantStatus, tt.wantCode)
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
			}
		})
	}
}

func TestCollectPreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/collect", nil)
	req.Header.Set("Origin", "https://acme.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	newTestHandler(t, nil, healthy, healthy, "http://127.0.0.1:1").ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	for header, want := range map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "POST",
		"Access-Control-Allow-Headers": "Content-Type",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name    string
		remote  string
		xff     []string
		cf      string
		trusted []netip.Prefix
		want    string
	}{
		{name: "no proxy header", remote: "198.51.100.1:443", want: "198.51.100.1"},
		{name: "untrusted peer cannot set forwarded ip", remote: "10.0.0.2:80", xff: []string{"1.2.3.4"}, want: "10.0.0.2"},
		{name: "proxy-appended entry wins over a forged one", remote: "10.0.0.2:80", xff: []string{"1.2.3.4, 203.0.113.5"}, trusted: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, want: "203.0.113.5"},
		{name: "last of several headers", remote: "10.0.0.2:80", xff: []string{"1.2.3.4", "203.0.113.5"}, trusted: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, want: "203.0.113.5"},
		{name: "ipv6", remote: "10.0.0.2:80", xff: []string{" 2001:db8::1 "}, trusted: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, want: "2001:db8::1"},
		{name: "trusted cloudflare header", remote: "10.0.0.2:80", cf: "8.8.8.8", trusted: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, want: "8.8.8.8"},
		{name: "bad cloudflare header uses forwarded peer", remote: "10.0.0.2:80", cf: "not-an-ip", xff: []string{"1.1.1.1"}, trusted: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, want: "1.1.1.1"},
		{name: "untrusted cloudflare header", remote: "10.0.0.2:80", cf: "8.8.8.8", want: "10.0.0.2"},
		{name: "garbage header falls back to the peer", remote: "[2001:db8::2]:443", xff: []string{"unknown"}, trusted: []netip.Prefix{netip.MustParsePrefix("2001:db8::/32")}, want: "2001:db8::2"},
		{name: "peer without a port", remote: "198.51.100.3", want: "198.51.100.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/collect", nil)
			req.RemoteAddr = tt.remote
			for _, v := range tt.xff {
				req.Header.Add("X-Forwarded-For", v)
			}
			if tt.cf != "" {
				req.Header.Set("CF-Connecting-IP", tt.cf)
			}
			if got := clientIP(req, tt.trusted); got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAnalyticsEndToEnd(t *testing.T) {
	ctx := context.Background()
	pool := openTestDB(ctx, t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	opts, err := redis.ParseURL(testRedisURL(t))
	if err != nil {
		t.Fatalf("parse redis url: %v", err)
	}
	rdb := redis.NewClient(opts)
	t.Cleanup(func() {
		if err := rdb.Close(); err != nil {
			t.Errorf("close redis: %v", err)
		}
	})
	lookup, err := geo.New()
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(Deps{
		Logger:    discardLogger,
		DB:        healthy,
		Redis:     healthy,
		Auth:      auth.NewService(pool, testSecret),
		Analytics: analytics.NewService(pool, rdb, lookup),
		Upstream:  &url.URL{Scheme: "http", Host: "127.0.0.1:1"},
	})

	id := rand.Text()
	userID, org, project, token := "user-"+id, "org-"+id, "project-"+id, "token-"+id
	t.Cleanup(func() {
		cleanupCtx := context.WithoutCancel(ctx)
		exec(cleanupCtx, t, pool, `DELETE FROM "user" WHERE id = $1`, userID)
		exec(cleanupCtx, t, pool, `DELETE FROM organization WHERE id = $1`, org)
	})
	exec(ctx, t, pool, `INSERT INTO "user" (id, name, email) VALUES ($1, 'Asha', $1 || '@example.com')`, userID)
	exec(ctx, t, pool, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Own', $1, now())`, org)
	exec(ctx, t, pool, `INSERT INTO member (id, organization_id, user_id, created_at) VALUES ($1, $2, $3, now())`, "member-"+id, org, userID)
	exec(ctx, t, pool, `INSERT INTO projects (id, organization_id, name, domain) VALUES ($1, $2, 'Acme', 'acme.com')`, project, org)
	exec(ctx, t, pool, `INSERT INTO session (id, token, user_id, expires_at) VALUES ($1, $2, $3, now() + interval '1 hour')`, "session-"+id, token, userID)

	api := func(method, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, method, "http://app.seomarine.com"+target, nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		req.AddCookie(&http.Cookie{Name: "__Secure-better-auth.session_token", Value: sign(token)})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	sitePath := "/api/v1/projects/" + project + "/analytics/site"

	var site siteResponse
	rec := api(http.MethodPost, sitePath)
	decode(t, rec, http.StatusOK, &site)
	wantSnippet := `<script defer data-site="` + site.SiteKey + `" src="https://app.seomarine.com/t.js"></script>`
	if site.SiteKey == "" || site.Snippet != wantSnippet {
		t.Fatalf("site = %+v, want snippet %s", site, wantSnippet)
	}
	var again siteResponse
	decode(t, api(http.MethodPost, sitePath), http.StatusOK, &again)
	if again != site {
		t.Fatalf("second call = %+v, want the same site %+v", again, site)
	}

	post := func(page, siteKey string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"k":%q,"u":%q,"r":"https://chatgpt.com/","w":1440}`, siteKey, page)
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/collect", strings.NewReader(body))
		req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15")
		req.Header.Set("X-Forwarded-For", "forged, 203.0.113.5")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if rec := post("https://acme.com/pricing", site.SiteKey); rec.Code != http.StatusAccepted || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("collect status = %d, CORS = %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	assertError(t, post("https://evil.com/", site.SiteKey), http.StatusForbidden, "foreign_host")
	assertError(t, post("https://acme.com/", "missing-"+id), http.StatusNotFound, "unknown_site")

	today := time.Now().UTC().Format(time.DateOnly)
	var sum analytics.Summary
	decode(t, api(http.MethodGet, "/api/v1/projects/"+project+"/analytics/summary?from="+today+"&to="+today), http.StatusOK, &sum)
	if sum.Visitors != 1 || sum.Pageviews != 1 || len(sum.AISources) != 1 || sum.AISources[0].Source != "chatgpt" {
		t.Errorf("summary = %+v, want one ChatGPT visit", sum)
	}
	var countryRows []analytics.CountryCount
	decode(t, api(http.MethodGet, fmt.Sprintf("/api/v1/analytics/%d/countries?from=%s&to=%s", site.ID, today, today)), http.StatusOK, &countryRows)
	if len(countryRows) != 0 {
		t.Errorf("unlocated test visitor has countries = %+v", countryRows)
	}
	assertError(t, api(http.MethodGet, fmt.Sprintf("/api/v1/analytics/%d/countries?from=%s&to=%s&page=0", site.ID, today, today)), http.StatusBadRequest, "invalid_page")
	assertError(t, api(http.MethodGet, fmt.Sprintf("/api/v1/analytics/%d/countries?from=%s&to=%s", site.ID+9999, today, today)), http.StatusNotFound, "unknown_site")

	for _, query := range []string{
		"",
		"?from=" + today,
		"?from=2026-02-30&to=2026-03-01",
		"?from=03/01/2026&to=2026-03-01",
		"?from=2026-03-02&to=2026-03-01",
		"?from=2026-01-01&to=2027-01-02",
	} {
		assertError(t, api(http.MethodGet, "/api/v1/projects/"+project+"/analytics/summary"+query), http.StatusBadRequest, "invalid_range")
	}

	// The limit is 120 events a minute per IP and site. Allow for one
	// window rollover during the loop.
	limited := false
	for range 250 {
		if rec := post("https://acme.com/", site.SiteKey); rec.Code == http.StatusTooManyRequests {
			assertError(t, rec, http.StatusTooManyRequests, "rate_limited")
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("250 events from one IP were never rate limited")
	}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, dst any) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, wantStatus, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
}

// testRedisURL returns TEST_REDIS_URL. CI always sets it, so a missing
// value there fails instead of silently skipping.
func testRedisURL(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("TEST_REDIS_URL"); v != "" {
		return v
	}
	if os.Getenv("CI") != "" {
		t.Fatal("TEST_REDIS_URL must be set in CI")
	}
	t.Skip("TEST_REDIS_URL not set; skipping Redis integration test")
	return ""
}
