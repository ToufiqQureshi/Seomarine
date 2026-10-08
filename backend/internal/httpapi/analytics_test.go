package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	var site struct {
		ID      int64  `json:"id"`
		SiteKey string `json:"siteKey"`
		Snippet string `json:"snippet"`
	}
	rec := api(http.MethodPost, sitePath)
	decode(t, rec, http.StatusOK, &site)
	wantSnippet := `<script defer data-site="` + site.SiteKey + `" src="https://app.seomarine.com/t.js"></script>`
	if site.SiteKey == "" || site.Snippet != wantSnippet {
		t.Fatalf("site = %+v, want snippet %s", site, wantSnippet)
	}
	var again struct {
		ID      int64  `json:"id"`
		SiteKey string `json:"siteKey"`
		Snippet string `json:"snippet"`
	}
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
