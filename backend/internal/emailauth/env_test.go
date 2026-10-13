package emailauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
)

const testBase = "https://app.uplumo.test"

type sentMail struct{ kind, address, url string }

type fakeMailer struct {
	mu       sync.Mutex
	mails    []sentMail
	contacts []string
	fail     error
}

func (m *fakeMailer) SendVerification(_ context.Context, address, link string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	m.mails = append(m.mails, sentMail{"verify", address, link})
	return nil
}

func (m *fakeMailer) SendPasswordReset(_ context.Context, address, link string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	m.mails = append(m.mails, sentMail{"reset", address, link})
	return nil
}

func (m *fakeMailer) UpsertSignupContact(_ context.Context, _, address, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.contacts = append(m.contacts, address)
	return nil
}

func (m *fakeMailer) sent() []sentMail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]sentMail(nil), m.mails...)
}

type fakeCaptcha struct {
	err      error
	response string
	ip       string
}

func (c *fakeCaptcha) Verify(_ context.Context, response, ip string) error {
	c.response, c.ip = response, ip
	return c.err
}

type fakeLimiter struct {
	mu   sync.Mutex
	deny bool
	err  error
	keys []string
}

func (l *fakeLimiter) Allow(_ context.Context, key string, _ int, _ time.Duration) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.keys = append(l.keys, key)
	if l.err != nil {
		return true, 0, l.err
	}
	return !l.deny, 30 * time.Second, nil
}

type authEnv struct {
	t       *testing.T
	pool    *pgxpool.Pool
	svc     *Service
	server  *httptest.Server
	mailer  *fakeMailer
	captcha *fakeCaptcha
	limiter *fakeLimiter
	authSvc *auth.Service
	clock   *testClock
}

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type envOptions struct {
	bypass     bool
	captcha    bool
	noMailer   bool
	disposable bool
}

// newAuthEnv builds the routes on a private schema of a real Postgres, using
// the shared legacy table definitions.
func newAuthEnv(t *testing.T, opts envOptions) *authEnv {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("emailauth_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(stop, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
		admin.Close()
	})
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ddl, err := os.ReadFile("../database/testdata/legacy_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(ddl)); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	base, _ := url2parse(testBase)
	e := &authEnv{t: t, pool: pool, mailer: &fakeMailer{}, captcha: &fakeCaptcha{}, limiter: &fakeLimiter{},
		clock: &testClock{now: time.Now().UTC().Truncate(time.Millisecond)}}
	e.authSvc = auth.NewService(pool, string(testSecret))
	svc := Service{
		DB: pool, Hasher: NewHasher(4), Sessions: AuthSessions{Auth: e.authSvc},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: e.clock.Now,
		Cfg:      Config{Secret: string(testSecret), BaseURL: base, BypassEmailVerification: opts.bypass, BlockDisposableEmail: opts.disposable},
		ClientIP: func(_ *http.Request) string { return "203.0.113.9" },
		Limiter:  e.limiter,
	}
	if !opts.noMailer {
		svc.Mailer = e.mailer
	}
	if opts.captcha {
		svc.Captcha = e.captcha
	}
	e.svc = New(svc)
	mux := http.NewServeMux()
	Mount(mux, e.svc)
	e.server = httptest.NewServer(mux)
	t.Cleanup(e.server.Close)
	return e
}

func url2parse(raw string) (*url.URL, error) { return url.Parse(raw) }

// request sends a JSON request without following redirects.
func (e *authEnv) request(method, path string, body any, headers map[string]string, cookies ...*http.Cookie) *response {
	e.t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	default:
		encoded, err := json.Marshal(b)
		if err != nil {
			e.t.Fatal(err)
		}
		reader = strings.NewReader(string(encoded))
	}
	req, err := http.NewRequestWithContext(context.Background(), method, e.server.URL+path, reader)
	if err != nil {
		e.t.Fatal(err)
	}
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "test-agent/1.0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			e.t.Errorf("close response: %v", err)
		}
	}()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("read response: %v", err)
	}
	r := &response{Response: resp, raw: raw}
	_ = json.Unmarshal(raw, &r.json)
	return r
}

type response struct {
	*http.Response
	raw  []byte
	json map[string]any
}

func (r *response) cookie(name string) *http.Cookie {
	for _, c := range r.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (r *response) errCode() string {
	code, _ := r.json["code"].(string)
	return code
}

// sessionCookie returns the session cookie of a response, named for the https
// base URL.
func (r *response) sessionCookie() *http.Cookie {
	return r.cookie("__Secure-better-auth.session_token")
}

// authenticate resolves a session cookie through the Go API's own reader, which
// proves the cookies this package writes are the ones the rest of the server
// accepts.
func (e *authEnv) authenticate(c *http.Cookie) (auth.User, error) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.AddCookie(c)
	return e.authSvc.Authenticate(req)
}

func (e *authEnv) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

func (e *authEnv) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// seedUser inserts a verified user with a known password, as an existing
// Better Auth account would look.
func (e *authEnv) seedUser(id, address, password string, verified bool) {
	e.t.Helper()
	hash, err := e.svc.Hasher.Hash(context.Background(), password)
	if err != nil {
		e.t.Fatal(err)
	}
	e.exec(`INSERT INTO "user" (id, name, email, email_verified) VALUES ($1, 'Seeded User', $2, $3)`, id, address, verified)
	e.exec(`INSERT INTO account (id, account_id, provider_id, user_id, password, updated_at) VALUES ($1, $2, 'credential', $2, $3, now())`, "acct-"+id, id, hash)
}

var errBoom = errors.New("boom")

func originHeaders() map[string]string { return map[string]string{"Origin": testBase} }

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}
