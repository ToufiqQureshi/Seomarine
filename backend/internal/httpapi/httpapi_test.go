package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/database"
	"github.com/toufiqqureshi/uplumo/backend/internal/mcp"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
	"github.com/toufiqqureshi/uplumo/backend/internal/site"
)

const testSecret = "test-secret-that-is-at-least-32-chars-long"

var (
	discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	healthy       = PingFunc(func(context.Context) error { return nil })
	down          = PingFunc(func(context.Context) error { return errors.New("connection refused") })
)

// newTestHandler builds the root handler. db may be nil for requests that
// never reach the database.
func newTestHandler(t *testing.T, db *pgxpool.Pool, pgPing, redisPing Pinger) http.Handler {
	t.Helper()
	pages, err := site.New(&url.URL{Scheme: "https", Host: "uplumo.com"})
	if err != nil {
		t.Fatalf("site.New: %v", err)
	}
	return NewHandler(Deps{
		Logger:    discardLogger,
		DB:        pgPing,
		Redis:     redisPing,
		Auth:      auth.NewService(db, testSecret),
		Site:      pages,
		StaticDir: testStaticDir(t),
	})
}

const appShell = "<!doctype html><title>app shell</title>"

// testStaticDir is a built frontend with an index and one hashed asset.
func testStaticDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"index.html": appShell, "assets/app-abc123.js": "console.log(1)"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestHealthRoutes(t *testing.T) {
	tests := []struct {
		name       string
		db, redis  Pinger
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "liveness is up", db: healthy, redis: healthy, path: "/healthz", wantStatus: http.StatusOK, wantBody: `{"status":"ok"}`},
		{name: "liveness ignores dependencies", db: down, redis: down, path: "/healthz", wantStatus: http.StatusOK, wantBody: `{"status":"ok"}`},
		{name: "ready when both answer", db: healthy, redis: healthy, path: "/readyz", wantStatus: http.StatusOK, wantBody: `{"status":"ok"}`},
		{name: "not ready when postgres is down", db: down, redis: healthy, path: "/readyz", wantStatus: http.StatusServiceUnavailable, wantBody: `{"status":"unavailable"}`},
		{name: "not ready when redis is down", db: healthy, redis: down, path: "/readyz", wantStatus: http.StatusServiceUnavailable, wantBody: `{"status":"unavailable"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newTestHandler(t, nil, tt.db, tt.redis).
				ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Body.String(); got != tt.wantBody+"\n" {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q", ct)
			}
		})
	}
}

// A hung dependency must not hang the readiness probe: the pings get a deadline.
func TestReadyzBoundsThePings(t *testing.T) {
	hung := PingFunc(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("ping context has no deadline")
		} else if time.Until(deadline) > readinessTimeout {
			t.Errorf("deadline %v is longer than %v", time.Until(deadline), readinessTimeout)
		}
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	newTestHandler(t, nil, hung, hung).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestSPAServesClientRoutesAndAssets(t *testing.T) {
	handler := newTestHandler(t, nil, healthy, healthy)
	tests := []struct {
		name, method, target string
		wantStatus           int
		wantBody, wantCache  string
	}{
		{name: "client route gets the index", method: http.MethodGet, target: "/dashboard", wantStatus: 200, wantBody: appShell, wantCache: "no-cache"},
		{name: "nested client route", method: http.MethodGet, target: "/p/abc/keywords", wantStatus: 200, wantBody: appShell, wantCache: "no-cache"},
		{name: "hashed asset is immutable", method: http.MethodGet, target: "/assets/app-abc123.js", wantStatus: 200, wantBody: "console.log(1)", wantCache: "public, max-age=31536000, immutable"},
		{name: "missing asset stays 404", method: http.MethodGet, target: "/assets/gone-1.js", wantStatus: 404},
		{name: "traversal is redirected to the clean path", method: http.MethodGet, target: "/../../etc/passwd", wantStatus: 307},
		{name: "unknown api path is JSON 404", method: http.MethodGet, target: "/api/v1beta/x", wantStatus: 404},
		{name: "POST is refused", method: http.MethodPost, target: "/dashboard", wantStatus: 405},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), tt.method, tt.target, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
			if tt.wantCache != "" && rec.Header().Get("Cache-Control") != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", rec.Header().Get("Cache-Control"), tt.wantCache)
			}
		})
	}
}

func TestSPAIndexCannotBeFramedByOtherSites(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler(t, nil, healthy, healthy).ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/dashboard", nil))
	if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'self'" {
		t.Errorf("Content-Security-Policy = %q", got)
	}
}

func TestSPAWithoutABuildAnswers503(t *testing.T) {
	pages, err := site.New(&url.URL{Scheme: "https", Host: "uplumo.com"})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(Deps{Logger: discardLogger, DB: healthy, Redis: healthy, Auth: auth.NewService(nil, testSecret), Site: pages, StaticDir: t.TempDir()})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/dashboard", nil))
	assertError(t, rec, http.StatusServiceUnavailable, "frontend_unavailable")
}

func TestMCPRouteIsServedByGo(t *testing.T) {
	pages, err := site.New(&url.URL{Scheme: "https", Host: "uplumo.com"})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(Deps{
		Logger: discardLogger, DB: healthy, Redis: healthy, Auth: auth.NewService(nil, testSecret), Site: pages,
		StaticDir: testStaticDir(t),
		MCP:       &mcp.Deps{Logger: discardLogger},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d from the Go dispatcher", rec.Code, http.StatusUnauthorized)
	}
}

func TestLandingPageIsForVisitorsWithoutASession(t *testing.T) {
	ctx := context.Background()
	pool := openTestDB(ctx, t)
	id := rand.Text()
	userID, liveToken, expiredToken := "user-"+id, "live-"+id, "expired-"+id
	t.Cleanup(func() {
		exec(context.WithoutCancel(ctx), t, pool, `DELETE FROM "user" WHERE id = $1`, userID)
	})
	exec(ctx, t, pool, `INSERT INTO "user" (id, name, email) VALUES ($1, 'Asha', $1 || '@example.com')`, userID)
	exec(ctx, t, pool, `INSERT INTO session (id, token, user_id, expires_at, updated_at) VALUES
		($1, $2, $5, now() + interval '1 hour', now()), ($3, $4, $5, now() - interval '1 second', now())`,
		"s-live-"+id, liveToken, "s-expired-"+id, expiredToken, userID)

	handler := newTestHandler(t, pool, healthy, healthy)
	closedPool := openTestDB(ctx, t)
	closedPool.Close()
	brokenDBHandler := newTestHandler(t, closedPool, healthy, healthy)

	const landing, app, pricing = "landing", "app", "pricing"
	tests := []struct {
		name     string
		brokenDB bool
		method   string
		target   string
		cookie   string
		want     string
	}{
		{name: "no cookie", target: "/", want: landing},
		{name: "query string", target: "/?utm_source=chatgpt.com", want: landing},
		{name: "HEAD", method: http.MethodHead, target: "/", want: landing},
		{name: "forged signature", target: "/", cookie: liveToken + ".Zm9yZ2Vk", want: landing},
		{name: "expired session", target: "/", cookie: sign(expiredToken), want: landing},
		{name: "unknown session", target: "/", cookie: sign("missing-" + id), want: landing},
		{name: "live session", target: "/", cookie: sign(liveToken), want: app},
		{name: "session lookup fails", brokenDB: true, target: "/", cookie: sign(liveToken), want: app},
		{name: "pricing without a session", target: "/pricing", want: pricing},
		{name: "pricing with a live session", target: "/pricing", cookie: sign(liveToken), want: pricing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, h := http.MethodGet, handler
			if tt.method != "" {
				method = tt.method
			}
			if tt.brokenDB {
				h = brokenDBHandler
			}
			req := httptest.NewRequestWithContext(ctx, method, tt.target, nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "__Secure-better-auth.session_token", Value: tt.cookie, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			got := app
			if rec.Body.String() != appShell {
				got = landing
				if strings.Contains(rec.Header().Get("Cache-Control"), "max-age") {
					got = pricing
				}
			}
			if got != tt.want {
				t.Errorf("served the %s, want the %s", got, tt.want)
			}
			if tt.target != "/pricing" && rec.Header().Get("Vary") != "Cookie" {
				t.Errorf("Vary = %q, want Cookie: / depends on the session", rec.Header().Get("Vary"))
			}
		})
	}
}

func TestSiteAssetsAreServed(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler(t, nil, healthy, healthy).
		ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/site/logo.svg", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/svg+xml" {
		t.Errorf("GET /site/logo.svg: status %d, Content-Type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestAPIRequiresASessionBeforeTouchingTheDatabase(t *testing.T) {
	for _, target := range []string{"/api/v1/billing/status", "/api/v1/projects/p1/analytics/summary", "/api/v1/"} {
		rec := httptest.NewRecorder()
		// A nil pool would panic if the handler reached the database.
		newTestHandler(t, nil, healthy, healthy).
			ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, nil))
		assertError(t, rec, http.StatusUnauthorized, "unauthenticated")
	}
}

func TestAPIAuthorization(t *testing.T) {
	ctx := context.Background()
	pool := openTestDB(ctx, t)
	id := rand.Text()
	userID, stranger, ownOrg, otherOrg := "user-"+id, "stranger-"+id, "org-own-"+id, "org-other-"+id
	ownProject, otherProject := "own-"+id, "other-"+id
	liveToken, expiredToken := "live-"+id, "expired-"+id

	t.Cleanup(func() {
		cleanupCtx := context.WithoutCancel(ctx)
		exec(cleanupCtx, t, pool, `DELETE FROM "user" WHERE id IN ($1, $2)`, userID, stranger)
		exec(cleanupCtx, t, pool, `DELETE FROM organization WHERE id IN ($1, $2)`, ownOrg, otherOrg)
	})
	exec(ctx, t, pool, `INSERT INTO "user" (id, name, email) VALUES ($1, 'Asha', $1 || '@example.com'), ($2, 'Stranger', $2 || '@example.com')`, userID, stranger)
	exec(ctx, t, pool, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Own', $1, now()), ($2, 'Other', $2, now())`, ownOrg, otherOrg)
	exec(ctx, t, pool, `INSERT INTO member (id, organization_id, user_id, created_at) VALUES ($1, $2, $3, now()), ($4, $5, $6, now())`,
		"member-"+id, ownOrg, userID, "member-stranger-"+id, otherOrg, stranger)
	exec(ctx, t, pool, `INSERT INTO projects (id, organization_id, name) VALUES ($1, $3, 'Own'), ($2, $4, 'Other')`, ownProject, otherProject, ownOrg, otherOrg)
	exec(ctx, t, pool, `INSERT INTO session (id, token, user_id, expires_at, updated_at) VALUES
		($1, $2, $5, now() + interval '1 hour', now()), ($3, $4, $5, now() - interval '1 second', now())`,
		"s-live-"+id, liveToken, "s-expired-"+id, expiredToken, userID)

	tests := []struct {
		name     string
		token    string
		path     string
		wantCode int
		wantErr  string
	}{
		{name: "own project reaches the project routes", token: liveToken, path: "/api/v1/projects/" + ownProject + "/nope", wantCode: http.StatusNotFound, wantErr: "not_found"},
		{name: "another organization's project is hidden", token: liveToken, path: "/api/v1/projects/" + otherProject + "/analytics/summary", wantCode: http.StatusNotFound, wantErr: "project_not_found"},
		{name: "unknown project is hidden", token: liveToken, path: "/api/v1/projects/missing-" + id + "/analytics/summary", wantCode: http.StatusNotFound, wantErr: "project_not_found"},
		{name: "signed-in user reaches the API", token: liveToken, path: "/api/v1/nope", wantCode: http.StatusNotFound, wantErr: "not_found"},
		{name: "expired session", token: expiredToken, path: "/api/v1/projects/" + ownProject + "/nope", wantCode: http.StatusUnauthorized, wantErr: "unauthenticated"},
	}
	handler := newTestHandler(t, pool, healthy, healthy)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, tt.path, nil)
			req.AddCookie(&http.Cookie{Name: "__Secure-better-auth.session_token", Value: sign(tt.token), HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assertError(t, rec, tt.wantCode, tt.wantErr)
		})
	}
}

// A database outage during the session lookup is a 500, not a 401 that would
// sign the user out.
func TestAPIReportsDatabaseErrorsAsInternal(t *testing.T) {
	pool := openTestDB(context.Background(), t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/billing/status", nil)
	req.AddCookie(&http.Cookie{Name: "better-auth.session_token", Value: sign("any-token"), HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	rec := httptest.NewRecorder()
	newTestHandler(t, pool, healthy, healthy).ServeHTTP(rec, req)
	assertError(t, rec, http.StatusInternalServerError, "internal")
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, wantStatus, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body, err)
	}
	if body.Error.Code != wantCode || body.Error.Message == "" {
		t.Errorf("error = %+v, want code %q and a message", body.Error, wantCode)
	}
}

// sign produces a better-auth session cookie value for token; the auth
// package pins this format against better-call's own output.
func sign(token string) string {
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(token))
	return url.QueryEscape(token + "." + base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

func exec(ctx context.Context, t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// openTestDB connects to TEST_DATABASE_URL and creates the legacy tables.
// CI always sets the URL, so a missing value there fails instead of skipping.
func openTestDB(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	pool, err := pgdb.Open(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	schema, err := os.ReadFile("../database/testdata/legacy_schema.sql")
	if err != nil {
		t.Fatalf("read legacy schema: %v", err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	return pool
}
