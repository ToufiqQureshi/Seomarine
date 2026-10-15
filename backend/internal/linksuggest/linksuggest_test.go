package linksuggest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/database"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
)

type fakeSource struct {
	pages []SourcePage
	err   error
	calls int
}

func (f *fakeSource) Pages(_ context.Context, _, _ string) ([]SourcePage, error) {
	f.calls++
	return f.pages, f.err
}

func TestMatchRanksAndExcludes(t *testing.T) {
	pages := []SourcePage{
		{URL: "https://site.com/services", Title: "Technical SEO services in India", Clicks: 40, Impressions: 900, Queries: []string{"technical seo services"}},
		{URL: "https://site.com/blog/technical-seo-guide", Title: "Technical SEO guide", Clicks: 5, Impressions: 120},
		{URL: "https://site.com/contact", Title: "Reach our team", Clicks: 30, Impressions: 9000},
	}
	got := match(pages, Input{
		Content: "## Choosing a provider\n\nTechnical SEO services help sites get crawled.\n\nOur team is friendly.\n",
		Slug:    "technical-seo-guide",
	})
	if len(got) == 0 {
		t.Fatal("expected suggestions")
	}
	// The draft's own future URL (/technical-seo-guide) must never appear.
	for _, suggestion := range got {
		if strings.HasSuffix(suggestion.URL, "/technical-seo-guide") {
			t.Fatalf("own URL suggested: %+v", suggestion)
		}
		if suggestion.Anchor == "" || len([]rune(suggestion.Anchor)) < minAnchorRunes {
			t.Fatalf("weak anchor: %+v", suggestion)
		}
	}
	// Traffic ranks first: the services page (40 clicks) goes before a
	// clicks-free match, and the contact page has no title-word match.
	if got[0].URL != "https://site.com/services" {
		t.Fatalf("order: %+v", got)
	}
	if !strings.Contains(got[0].Reason, "technical seo services") {
		t.Fatalf("reason should name the query: `%s`", got[0].Reason)
	}
}

func TestMatchKeepsUnicodeAnchors(t *testing.T) {
	pages := []SourcePage{{URL: "https://site.com/hindi-seo", Title: "हिंदी SEO गाइड"}}
	got := match(pages, Input{Content: "## हिंदी SEO गाइड समझें\n\nपूरी जानकारी यहाँ है।\n", Topic: "हिंदी SEO"})
	if len(got) != 1 {
		t.Fatalf("unicode anchor dropped: %+v", got)
	}
	if got[0].Anchor == "" || !strings.Contains(got[0].Anchor, "हिंदी SEO गाइड") {
		t.Fatalf("anchor lost its Devanagari text: %+v", got[0])
	}
}

func TestUniqueWordsKeepsHindiVowelMarks(t *testing.T) {
	got := uniqueWords("हिंदी गाइड")
	want := []string{"हिंदी", "गाइड"}
	if !slices.Equal(got, want) {
		t.Fatalf("Hindi words split at vowel marks: got %q, want %q", got, want)
	}
}

func TestMatchBoundedForHugeDraft(t *testing.T) {
	// A draft at the editor's limit (and beyond) must stay bounded in size
	// and time: the matcher scans at most MaxDraftRunes runes and returns.
	pages := []SourcePage{{URL: "https://site.com/p", Title: "Technical SEO services"}}
	draft := strings.Repeat("Technical SEO paragraph one.\n", 8_000) // ~232k runes
	start := time.Now()
	got := match(pages, Input{Content: draft})
	if time.Since(start) > 2*time.Second {
		t.Fatalf("matching took %s on a huge draft", time.Since(start))
	}
	t.Logf("huge draft: %d anchors in %s", len(got), time.Since(start))
}

func TestServiceEmptyWhenNoSourceAnswers(t *testing.T) {
	dead := &fakeSource{err: errors.New("no google, no audit")}
	empty := &fakeSource{pages: nil}
	service := &Service{Sources: []Source{dead, empty}}
	got, err := service.Suggestions(context.Background(), "org", "project", Input{Content: "Some text here."})
	if err != nil || len(got) != 0 {
		t.Fatalf("empty sources must return an empty list, not %v %v", got, err)
	}
}

type cacheClient struct {
	store map[string]string
}

func (c *cacheClient) Get(_ context.Context, key string) (string, error) {
	value, ok := c.store[key]
	if !ok {
		return "", errCacheMiss
	}
	return value, nil
}

func (c *cacheClient) Set(_ context.Context, key, value string, _ time.Duration) error {
	c.store[key] = value
	return nil
}

func TestServiceCachesPerDraft(t *testing.T) {
	source := &fakeSource{pages: []SourcePage{{URL: "https://site.com/services", Title: "Technical SEO services in India"}}}
	service := &Service{Sources: []Source{source}, Redis: &cacheClient{store: map[string]string{}}}
	first, err := service.Suggestions(context.Background(), "org", "p", Input{Content: "Technical SEO services are useful here."})
	if err != nil || len(first) == 0 {
		t.Fatalf("first call: %v", err)
	}
	_, err = service.Suggestions(context.Background(), "org", "p", Input{Content: "Technical SEO services are useful here."})
	if err != nil {
		t.Fatal(err)
	}
	if source.calls != 1 {
		t.Fatalf("second call should be cached, source called %d times", source.calls)
	}
}

func TestLinkSuggestionsNeverCrossWorkspaces(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := linkTestPool(ctx, t)
	suffix := strings.ReplaceAll(fmt.Sprint(time.Now().UnixNano()), ".", "")
	orgA, orgB := "ls-org-a"+suffix, "ls-org-b"+suffix
	projectA, projectB := "ls-p-a"+suffix, "ls-p-b"+suffix
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	exec(`INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'L', $1, now())`, orgA)
	exec(`INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'L', $1, now())`, orgB)
	exec(`INSERT INTO projects (id, organization_id, name) VALUES ($1, $2, $1)`, projectA, orgA)
	exec(`INSERT INTO projects (id, organization_id, name) VALUES ($1, $2, $1)`, projectB, orgB)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		for _, org := range []string{orgA, orgB} {
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM organization WHERE id=$1`, org)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM organization WHERE id=$1`, org)
		}
	})
	// Only workspace B has a completed audit with pages.
	var auditID string
	if err := pool.QueryRow(ctx, `INSERT INTO go_audits (id, project_id, started_by_user_id, start_url, status)
		VALUES ($1, $2, 'u', 'https://b.com', 'completed') RETURNING id`, "ls-audit-"+suffix, projectB).Scan(&auditID); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO go_audit_pages (id, audit_id, url, title)
		VALUES ($1, $2, 'https://b.com/technical-seo-services', 'Technical SEO services in India')`, "ls-page-"+suffix, auditID)

	source := AuditSource{DB: pool}
	pages, err := source.Pages(ctx, orgB, projectB)
	if err != nil || len(pages) != 1 {
		t.Fatalf("own audit pages: %v %v", pages, err)
	}
	// Workspace A, and unknown projects, never see B's audit.
	for _, probe := range []struct{ org, project string }{{orgA, projectA}, {orgA, projectB}, {orgB, projectA}} {
		got, err := source.Pages(ctx, probe.org, probe.project)
		if err != nil || len(got) != 0 {
			t.Fatalf("cross-workspace pages leaked for %+v: %v %v", probe, got, err)
		}
	}
}

func linkTestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	pool, err := pgdb.Open(ctx, url)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}
