package analytics

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"net/url"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
)

const desktopUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"

var day = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func TestEnsureSiteIsIdempotentUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	project := f.addProject(ctx, t, "")

	keys := make([]string, 8)
	var wg sync.WaitGroup
	for i := range keys {
		wg.Go(func() {
			key, err := f.svc.EnsureSite(ctx, project)
			if err != nil {
				t.Errorf("EnsureSite() error = %v", err)
			}
			keys[i] = key
		})
	}
	wg.Wait()
	if keys[0] == "" || slices.ContainsFunc(keys, func(k string) bool { return k != keys[0] }) {
		t.Fatalf("EnsureSite() keys = %q, want one key for every caller", keys)
	}
}

func TestCollect(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	withDomain := f.addSite(ctx, t, "acme.com")
	anyHost := f.addSite(ctx, t, "")
	archived := f.addSite(ctx, t, "")
	f.exec(ctx, t, `UPDATE projects SET archived_at = '2026-01-01T00:00:00.000Z' WHERE id = $1`, archived.project)

	tests := []struct {
		name      string
		site      testSite
		page      string
		ua        string
		wantErr   error
		wantSaved bool
	}{
		{name: "page on the project domain", site: withDomain, page: "https://acme.com/a", ua: desktopUA, wantSaved: true},
		{name: "www and subdomain of the project domain", site: withDomain, page: "https://WWW.Blog.Acme.com:8443/a", ua: desktopUA, wantSaved: true},
		{name: "page on another host", site: withDomain, page: "https://evil.com/a", ua: desktopUA, wantErr: ErrForeignHost},
		{name: "lookalike host", site: withDomain, page: "https://notacme.com/a", ua: desktopUA, wantErr: ErrForeignHost},
		{name: "project without a domain accepts any host", site: anyHost, page: "http://localhost:3000/", ua: desktopUA, wantSaved: true},
		{name: "unknown site key", site: testSite{key: "missing-" + rand.Text()}, page: "https://acme.com/", ua: desktopUA, wantErr: ErrUnknownSite},
		{name: "archived project", site: archived, page: "https://acme.com/", ua: desktopUA, wantErr: ErrUnknownSite},
		{name: "crawler is dropped silently", site: anyHost, page: "https://acme.com/", ua: "Googlebot/2.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := f.countEvents(ctx, t, tt.site.project)
			err := f.svc.Collect(ctx, hit(t, tt.site.key, tt.page, "", "198.51.100.7", tt.ua))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Collect() error = %v, want %v", err, tt.wantErr)
			}
			if saved := f.countEvents(ctx, t, tt.site.project) > before; saved != tt.wantSaved {
				t.Fatalf("event saved = %v, want %v", saved, tt.wantSaved)
			}
		})
	}
}

// The stored row carries the classified source, the path without its query
// string, and a hash, never the IP or the user agent.
func TestCollectStoresAPrivateEvent(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	s := f.addSite(ctx, t, "acme.com")
	const ip = "203.0.113.42"

	h := hit(t, s.key, "https://acme.com/pricing/caf%C3%A9?email=a@b.c&utm_source=chatgpt.com#plans", "", ip, desktopUA)
	if err := f.svc.Collect(ctx, h); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	var path, channel, aiSource, device string
	var refHost *string
	var visitor []byte
	f.queryRow(ctx, t, `SELECT path, referrer_host, channel, ai_source, device, visitor_hash FROM go_analytics_events e
		JOIN go_analytics_sites s ON s.id = e.site_id WHERE s.project_id = $1`, []any{s.project},
		&path, &refHost, &channel, &aiSource, &device, &visitor)
	if path != "/pricing/caf%C3%A9" || refHost != nil || channel != channelAI || aiSource != aiChatGPT || device != deviceDesktop {
		t.Errorf("event = %q %v %q %q %q", path, refHost, channel, aiSource, device)
	}
	if len(visitor) != 32 || bytes.Contains(visitor, []byte(ip)) {
		t.Errorf("visitor hash = %x, want 32 opaque bytes", visitor)
	}
}

// One visitor is one hash within a UTC day, even when concurrent first
// events race to create the day's salt, and a different hash the next day.
func TestCollectVisitorHashRotatesDaily(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	s := f.addSite(ctx, t, "")
	// A day no other test uses, with its salt removed so the concurrent
	// first events below race to create it.
	first := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := f.rdb.Del(ctx, "analytics:salt:"+first.Format(time.DateOnly)).Err(); err != nil {
		t.Fatalf("delete salt: %v", err)
	}
	f.svc.now = func() time.Time { return first }

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := f.svc.Collect(ctx, hit(t, s.key, "https://acme.com/", "", "198.51.100.1", desktopUA)); err != nil {
				t.Errorf("Collect() error = %v", err)
			}
		})
	}
	wg.Wait()
	f.svc.now = func() time.Time { return first.Add(24 * time.Hour) }
	if err := f.svc.Collect(ctx, hit(t, s.key, "https://acme.com/", "", "198.51.100.1", desktopUA)); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if err := f.svc.Collect(ctx, hit(t, s.key, "https://acme.com/", "", "198.51.100.2", desktopUA)); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	var hashes int
	f.queryRow(ctx, t, `SELECT count(DISTINCT visitor_hash) FROM go_analytics_events e
		JOIN go_analytics_sites s ON s.id = e.site_id WHERE s.project_id = $1`, []any{s.project}, &hashes)
	if hashes != 3 {
		t.Fatalf("distinct visitor hashes = %d, want 3 (day one, day two, other IP)", hashes)
	}
	ttl := f.rdb.TTL(ctx, "analytics:salt:"+first.Format(time.DateOnly)).Val()
	if ttl <= 0 || ttl > saltTTL {
		t.Fatalf("salt TTL = %v, want an expiry within %v", ttl, saltTTL)
	}
}

func TestCollectRateLimitsPerIPAndSite(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	s := f.addSite(ctx, t, "")
	other := f.addSite(ctx, t, "")
	minute := time.Now().UTC().Truncate(time.Minute)
	f.svc.now = func() time.Time { return minute }

	for i := range rateLimit {
		if err := f.svc.Collect(ctx, hit(t, s.key, "https://acme.com/", "", "198.51.100.9", desktopUA)); err != nil {
			t.Fatalf("event %d: Collect() error = %v", i+1, err)
		}
	}
	if err := f.svc.Collect(ctx, hit(t, s.key, "https://acme.com/", "", "198.51.100.9", desktopUA)); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("event over the limit: error = %v, want ErrRateLimited", err)
	}
	if err := f.svc.Collect(ctx, hit(t, s.key, "https://acme.com/", "", "198.51.100.10", desktopUA)); err != nil {
		t.Fatalf("another IP: error = %v", err)
	}
	if err := f.svc.Collect(ctx, hit(t, other.key, "https://acme.com/", "", "198.51.100.9", desktopUA)); err != nil {
		t.Fatalf("another site: error = %v", err)
	}
	f.svc.now = func() time.Time { return minute.Add(rateWindow) }
	if err := f.svc.Collect(ctx, hit(t, s.key, "https://acme.com/", "", "198.51.100.9", desktopUA)); err != nil {
		t.Fatalf("next window: error = %v", err)
	}
}

func TestCollectReportsRedisErrors(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	s := f.addSite(ctx, t, "")
	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	t.Cleanup(func() { closeRedis(t, broken) })
	svc := NewService(f.pool, broken)

	err := svc.Collect(ctx, hit(t, s.key, "https://acme.com/", "", "198.51.100.1", desktopUA))
	if err == nil || errors.Is(err, ErrRateLimited) || errors.Is(err, ErrUnknownSite) {
		t.Fatalf("Collect() error = %v, want a Redis error", err)
	}
}

func TestSummarize(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	s := f.addSite(ctx, t, "acme.com")
	noise := f.addSite(ctx, t, "")
	const iPhone = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148"

	at := func(ts time.Time, site testSite, page, ref, ip, ua string) {
		t.Helper()
		f.svc.now = func() time.Time { return ts }
		if err := f.svc.Collect(ctx, hit(t, site.key, page, ref, ip, ua)); err != nil {
			t.Fatalf("Collect() error = %v", err)
		}
	}
	d1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) // first day of the range
	d3 := d1.AddDate(0, 0, 2)                         // last day of the range
	// Visitor A arrives from ChatGPT and clicks through, twice on day 1.
	at(d1, s, "https://acme.com/?utm_source=chatgpt.com", "", "10.0.0.1", desktopUA)
	at(d1.Add(time.Minute), s, "https://acme.com/pricing", "https://acme.com/", "10.0.0.1", desktopUA)
	// Visitor B on a phone from Google, day 1 and again day 3 (a new daily id).
	at(d1.Add(23*time.Hour+59*time.Minute+59*time.Second), s, "https://acme.com/pricing", "https://www.google.co.in/", "10.0.0.2", iPhone)
	at(d3.Add(10*time.Hour), s, "https://acme.com/", "https://www.perplexity.ai/", "10.0.0.2", iPhone)
	// Visitor C from a blog on day 3.
	at(d3.Add(23*time.Hour), s, "https://acme.com/blog", "https://blog.example.org/x", "10.0.0.3", desktopUA)
	// Outside the range, on either side, and another project's traffic.
	at(d1.Add(-time.Second), s, "https://acme.com/", "", "10.0.0.4", desktopUA)
	at(d3.AddDate(0, 0, 1), s, "https://acme.com/", "", "10.0.0.5", desktopUA)
	at(d1.Add(time.Hour), noise, "https://other.com/", "https://chatgpt.com/", "10.0.0.6", desktopUA)

	got, err := f.svc.Summarize(ctx, s.project, d1.Add(15*time.Hour), d3)
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	want := Summary{
		Visitors:  4, // A, B on day 1, B on day 3, C
		Pageviews: 5,
		Series: []DayCount{
			{Date: "2026-03-01", Visitors: 2, Pageviews: 3},
			{Date: "2026-03-02", Visitors: 0, Pageviews: 0},
			{Date: "2026-03-03", Visitors: 2, Pageviews: 2},
		},
		TopPages:  []PageCount{{"/", 2}, {"/pricing", 2}, {"/blog", 1}},
		Channels:  []ChannelCount{{channelAI, 2}, {channelReferral, 1}, {channelSearch, 1}},
		AISources: []AISourceCount{{aiChatGPT, 1}, {aiPerplexity, 1}},
		Referrers: []ReferrerCount{{"blog.example.org", 1}, {"google.co.in", 1}, {"perplexity.ai", 1}},
		Devices:   []DeviceCount{{deviceDesktop, 2}, {deviceMobile, 2}},
	}
	assertSummary(t, got, want)
}

func TestSummarizeWithoutTraffic(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	project := f.addProject(ctx, t, "") // no site yet

	got, err := f.svc.Summarize(ctx, project, day, day)
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	assertSummary(t, got, Summary{
		Series:    []DayCount{{Date: "2026-03-10"}},
		TopPages:  []PageCount{},
		Channels:  []ChannelCount{},
		AISources: []AISourceCount{},
		Referrers: []ReferrerCount{},
		Devices:   []DeviceCount{},
	})
}

func TestSummarizeValidatesTheRange(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	project := f.addProject(ctx, t, "")
	leapYear := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		from, to time.Time
		wantErr  error
	}{
		{name: "one day", from: day, to: day},
		{name: "a whole leap year", from: leapYear, to: leapYear.AddDate(0, 0, 365)},
		{name: "one day too many", from: leapYear, to: leapYear.AddDate(0, 0, 366), wantErr: ErrInvalidRange},
		{name: "reversed", from: day, to: day.AddDate(0, 0, -1), wantErr: ErrInvalidRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := f.svc.Summarize(ctx, project, tt.from, tt.to)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Summarize() error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && len(got.Series) != int(tt.to.Sub(tt.from).Hours()/24)+1 {
				t.Fatalf("series has %d days", len(got.Series))
			}
		})
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.svc.Summarize(canceled, project, day, day); err == nil {
		t.Fatal("Summarize() succeeded on a canceled context")
	}
}

func assertSummary(t *testing.T, got, want Summary) {
	t.Helper()
	if got.Visitors != want.Visitors || got.Pageviews != want.Pageviews {
		t.Errorf("totals = %d visitors, %d pageviews; want %d, %d", got.Visitors, got.Pageviews, want.Visitors, want.Pageviews)
	}
	check := func(name string, equal bool, got, want any) {
		if !equal {
			t.Errorf("%s = %+v\nwant %+v", name, got, want)
		}
	}
	// slices.Equal treats nil and empty alike; the JSON contract needs [], so
	// nil is checked separately.
	check("series", slices.Equal(got.Series, want.Series), got.Series, want.Series)
	check("topPages", slices.Equal(got.TopPages, want.TopPages) && got.TopPages != nil, got.TopPages, want.TopPages)
	check("channels", slices.Equal(got.Channels, want.Channels) && got.Channels != nil, got.Channels, want.Channels)
	check("aiSources", slices.Equal(got.AISources, want.AISources) && got.AISources != nil, got.AISources, want.AISources)
	check("referrers", slices.Equal(got.Referrers, want.Referrers) && got.Referrers != nil, got.Referrers, want.Referrers)
	check("devices", slices.Equal(got.Devices, want.Devices) && got.Devices != nil, got.Devices, want.Devices)
}

func hit(t *testing.T, siteKey, page, referrer, ip, ua string) Hit {
	t.Helper()
	u, err := url.Parse(page)
	if err != nil {
		t.Fatalf("parse %q: %v", page, err)
	}
	return Hit{SiteKey: siteKey, Page: u, Referrer: referrer, IP: ip, UserAgent: ua}
}

type fixture struct {
	svc  *Service
	pool *pgxpool.Pool
	rdb  *redis.Client
	org  string
}

type testSite struct {
	project, key string
}

// newFixture migrates the test database and creates an organization that
// the test's projects belong to; deleting it cascades to everything else.
func newFixture(ctx context.Context, t *testing.T) *fixture {
	t.Helper()
	pool, err := database.Open(ctx, testEnv(t, "TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(pool.Close)
	schema, err := os.ReadFile("../database/testdata/legacy_schema.sql")
	if err != nil {
		t.Fatalf("read legacy schema: %v", err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	opts, err := redis.ParseURL(testEnv(t, "TEST_REDIS_URL"))
	if err != nil {
		t.Fatalf("parse redis url: %v", err)
	}
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { closeRedis(t, rdb) })

	f := &fixture{svc: NewService(pool, rdb), pool: pool, rdb: rdb, org: "org-" + rand.Text()}
	f.exec(ctx, t, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Acme', $1, now())`, f.org)
	t.Cleanup(func() {
		f.exec(context.WithoutCancel(ctx), t, `DELETE FROM organization WHERE id = $1`, f.org)
	})
	return f
}

func (f *fixture) addProject(ctx context.Context, t *testing.T, domain string) string {
	t.Helper()
	id := "project-" + rand.Text()
	f.exec(ctx, t, `INSERT INTO projects (id, organization_id, name, domain) VALUES ($1, $2, 'Acme', nullif($3, ''))`, id, f.org, domain)
	return id
}

func (f *fixture) addSite(ctx context.Context, t *testing.T, domain string) testSite {
	t.Helper()
	project := f.addProject(ctx, t, domain)
	key, err := f.svc.EnsureSite(ctx, project)
	if err != nil {
		t.Fatalf("EnsureSite() error = %v", err)
	}
	return testSite{project: project, key: key}
}

func (f *fixture) countEvents(ctx context.Context, t *testing.T, project string) int {
	t.Helper()
	var n int
	f.queryRow(ctx, t, `SELECT count(*) FROM go_analytics_events e
		JOIN go_analytics_sites s ON s.id = e.site_id WHERE s.project_id = $1`, []any{project}, &n)
	return n
}

func (f *fixture) exec(ctx context.Context, t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func (f *fixture) queryRow(ctx context.Context, t *testing.T, sql string, args []any, dest ...any) {
	t.Helper()
	if err := f.pool.QueryRow(ctx, sql, args...).Scan(dest...); err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
}

func closeRedis(t *testing.T, c *redis.Client) {
	t.Helper()
	if err := c.Close(); err != nil {
		t.Errorf("close redis: %v", err)
	}
}

// testEnv returns a test service URL. CI always sets them, so a missing
// value there fails instead of silently skipping.
func testEnv(t *testing.T, name string) string {
	t.Helper()
	if v := os.Getenv(name); v != "" {
		return v
	}
	if os.Getenv("CI") != "" {
		t.Fatalf("%s must be set in CI", name)
	}
	t.Skipf("%s not set; skipping integration test", name)
	return ""
}
