package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
)

// goldenCookie was produced by better-call 1.3.7's own signCookieValue
// (the library better-auth signs session cookies with) for goldenToken and
// testSecret, so these tests pin the Go verifier to the real format,
// including the URL-encoded "+" and "=" in the base64 signature.
const (
	testSecret   = "test-secret-that-is-at-least-32-chars-long"
	goldenToken  = "Xk3nQ9wLr2TfVb8ZsY1aPeHu6JcMdGo4"                                                    //nolint:gosec // G101: a test-only session token, not a credential. ggignore
	goldenCookie = "Xk3nQ9wLr2TfVb8ZsY1aPeHu6JcMdGo4.zc6%2BkvqyJnR7Rf%2BkjANpojy00IheL4EjkbJukkoq89k%3D" // ggignore
)

func TestAuthenticate(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	expiredToken := "expired-" + rand.Text()
	f.addSession(ctx, t, expiredToken, time.Now().Add(-time.Minute))
	unknownToken := "unknown-" + rand.Text()

	tests := []struct {
		name   string
		cookie *http.Cookie
		wantOK bool
	}{
		{name: "golden cookie", cookie: &http.Cookie{Name: sessionCookie, Value: goldenCookie}, wantOK: true},
		{name: "golden cookie under the __Secure- name", cookie: &http.Cookie{Name: secureSessionCookie, Value: goldenCookie}, wantOK: true},
		{name: "cookie that was never URL-encoded", cookie: &http.Cookie{Name: sessionCookie, Value: goldenToken + "." + "zc6+kvqyJnR7Rf+kjANpojy00IheL4EjkbJukkoq89k="}, wantOK: true},
		{name: "no cookie"},
		{name: "other cookie name", cookie: &http.Cookie{Name: "session_token", Value: goldenCookie}},
		{name: "tampered signature", cookie: &http.Cookie{Name: sessionCookie, Value: strings.Replace(goldenCookie, "zc6", "zc7", 1)}},
		{name: "tampered token", cookie: &http.Cookie{Name: sessionCookie, Value: "Y" + goldenCookie[1:]}},
		{name: "signature without padding", cookie: &http.Cookie{Name: sessionCookie, Value: strings.TrimSuffix(goldenCookie, "%3D")}},
		{name: "signed with another secret", cookie: &http.Cookie{Name: sessionCookie, Value: sign(goldenToken, strings.Repeat("x", 40))}},
		{name: "token without signature", cookie: &http.Cookie{Name: sessionCookie, Value: goldenToken}},
		{name: "empty token", cookie: &http.Cookie{Name: sessionCookie, Value: sign("", testSecret)}},
		{name: "bad percent-encoding", cookie: &http.Cookie{Name: sessionCookie, Value: goldenCookie + "%zz"}},
		{name: "expired session", cookie: &http.Cookie{Name: sessionCookie, Value: sign(expiredToken, testSecret)}},
		{name: "validly signed token with no session", cookie: &http.Cookie{Name: sessionCookie, Value: sign(unknownToken, testSecret)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			user, err := f.svc.Authenticate(req)
			if !tt.wantOK {
				if !errors.Is(err, ErrUnauthenticated) {
					t.Fatalf("Authenticate() = %+v, %v; want ErrUnauthenticated", user, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Authenticate() error = %v", err)
			}
			if user != f.user {
				t.Fatalf("Authenticate() = %+v, want %+v", user, f.user)
			}
		})
	}
}

// A database failure is a server error, never "signed out": the caller must
// not be able to tell it apart from a 500.
func TestAuthenticateReportsDatabaseErrors(t *testing.T) {
	f := newFixture(context.Background(), t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: goldenCookie})

	_, err := f.svc.Authenticate(req)
	if err == nil || errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("Authenticate() error = %v, want a non-auth database error", err)
	}
}

// The session's active organization is only a hint: it counts when the user
// is still a member, and carries the member's role.
func TestAuthenticateResolvesTheActiveOrganization(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	exec(ctx, t, f.pool, `UPDATE member SET role = 'admin' WHERE organization_id = $1 AND user_id = $2`, f.ownOrg, f.user.ID)

	tests := []struct {
		name      string
		activeOrg any
		wantOrg   string
		wantRole  string
	}{
		{name: "member of the active organization", activeOrg: f.ownOrg, wantOrg: f.ownOrg, wantRole: "admin"},
		{name: "not a member of the active organization", activeOrg: f.otherOrg},
		{name: "active organization that no longer exists", activeOrg: "deleted-" + rand.Text()},
		{name: "no active organization", activeOrg: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := "org-" + rand.Text()
			f.addSession(ctx, t, token, time.Now().Add(time.Hour))
			exec(ctx, t, f.pool, `UPDATE session SET active_organization_id = $1 WHERE token = $2`, tt.activeOrg, token)
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sign(token, testSecret)})

			user, err := f.svc.Authenticate(req)
			if err != nil {
				t.Fatalf("Authenticate() error = %v", err)
			}
			if user.ID != f.user.ID || user.OrganizationID != tt.wantOrg || user.Role != tt.wantRole {
				t.Fatalf("Authenticate() = %+v, want organization %q with role %q", user, tt.wantOrg, tt.wantRole)
			}
		})
	}
}

func TestAuthorizeProject(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)

	tests := []struct {
		name      string
		projectID string
		wantErr   error
	}{
		{name: "own organization's project", projectID: f.ownProject},
		{name: "another organization's project", projectID: f.otherProject, wantErr: ErrProjectNotFound},
		{name: "archived project", projectID: f.archivedProject, wantErr: ErrProjectNotFound},
		{name: "project that does not exist", projectID: "missing-" + rand.Text(), wantErr: ErrProjectNotFound},
		{name: "empty project id", projectID: "", wantErr: ErrProjectNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := f.svc.AuthorizeProject(ctx, f.user.ID, tt.projectID); !errors.Is(err, tt.wantErr) {
				t.Fatalf("AuthorizeProject() error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := f.svc.AuthorizeProject(canceled, f.user.ID, f.ownProject); err == nil || errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("AuthorizeProject() on a failing database = %v, want a database error", err)
	}
}

// sign mirrors better-call's signCookieValue; TestAuthenticate's golden case
// proves the two agree.
func sign(token, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token))
	return url.PathEscape(token + "." + base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

type fixture struct {
	svc             *Service
	pool            *pgxpool.Pool
	user            User
	ownProject      string
	otherProject    string
	archivedProject string
	ownOrg          string
	otherOrg        string
}

// newFixture creates a user in one organization with an active and an
// archived project, a second organization with its own project and member,
// and a live session for goldenToken.
func newFixture(ctx context.Context, t *testing.T) fixture {
	t.Helper()
	pool := openTestDB(ctx, t)
	id := rand.Text()
	f := fixture{
		svc:             NewService(pool, testSecret),
		pool:            pool,
		user:            User{ID: "user-" + id, Name: "Asha Rao", Email: id + "@example.com"},
		ownProject:      "own-" + id,
		otherProject:    "other-" + id,
		archivedProject: "archived-" + id,
		ownOrg:          "org-own-" + id,
		otherOrg:        "org-other-" + id,
	}
	ownOrg, otherOrg, stranger := f.ownOrg, f.otherOrg, "stranger-"+id
	t.Cleanup(func() {
		cleanupCtx := context.WithoutCancel(ctx)
		exec(cleanupCtx, t, pool, `DELETE FROM "user" WHERE id IN ($1, $2)`, f.user.ID, stranger)
		exec(cleanupCtx, t, pool, `DELETE FROM organization WHERE id IN ($1, $2)`, ownOrg, otherOrg)
	})

	exec(ctx, t, pool, `DELETE FROM session WHERE token = $1`, goldenToken)
	exec(ctx, t, pool, `INSERT INTO "user" (id, name, email) VALUES ($1, $2, $3), ($4, 'Stranger', $4 || '@example.com')`,
		f.user.ID, f.user.Name, f.user.Email, stranger)
	exec(ctx, t, pool, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Own', $1, now()), ($2, 'Other', $2, now())`, ownOrg, otherOrg)
	exec(ctx, t, pool, `INSERT INTO member (id, organization_id, user_id, created_at) VALUES ($1, $2, $3, now()), ($4, $5, $6, now())`,
		"member-"+id, ownOrg, f.user.ID, "member-stranger-"+id, otherOrg, stranger)
	exec(ctx, t, pool, `INSERT INTO projects (id, organization_id, name, archived_at) VALUES
		($1, $4, 'Own', NULL), ($2, $5, 'Other', NULL), ($3, $4, 'Archived', '2026-01-01T00:00:00.000Z')`,
		f.ownProject, f.otherProject, f.archivedProject, ownOrg, otherOrg)
	f.addSession(ctx, t, goldenToken, time.Now().Add(time.Hour))
	return f
}

func (f fixture) addSession(ctx context.Context, t *testing.T, token string, expiresAt time.Time) {
	t.Helper()
	exec(ctx, t, f.pool, `INSERT INTO session (id, token, user_id, expires_at) VALUES ($1, $2, $3, $4)`,
		"session-"+rand.Text(), token, f.user.ID, expiresAt)
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
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	pool, err := pgdb.Open(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	schema, err := os.ReadFile("../database/testdata/legacy_schema.sql")
	if err != nil {
		t.Fatalf("read legacy schema: %v", err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	return pool
}
