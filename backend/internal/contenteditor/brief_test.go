package contenteditor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
)

type briefCacheFake struct {
	mu    sync.Mutex
	brief ContentBrief
	key   string
	built int
	err   error
}

func (c *briefCacheFake) GetOrBuild(_ context.Context, key string, refresh bool, build func() (ContentBrief, error)) (ContentBrief, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.key = key
	if c.err != nil {
		return ContentBrief{}, c.err
	}
	if c.brief.FetchedAt.IsZero() || refresh {
		c.built++
		brief, err := build()
		if err != nil {
			return ContentBrief{}, err
		}
		c.brief = brief
	}
	return c.brief, nil
}

type briefProviderFake struct {
	brief ContentBrief
	calls int
	err   error
}

func (p *briefProviderFake) Fetch(_ context.Context, _ string, input BriefInput) (ContentBrief, error) {
	p.calls++
	if p.err != nil {
		return ContentBrief{}, p.err
	}
	brief := p.brief
	brief.Keyword, brief.LocationCode, brief.LanguageCode = input.Keyword, input.LocationCode, input.LanguageCode
	return brief, nil
}

func TestBriefInputValidationAndCacheKeyExcludesOwnDomain(t *testing.T) {
	base := BriefInput{Keyword: "  SEO   tools ", LocationCode: 2840, LanguageCode: "en", OwnDomain: "https://www.example.com/"}
	clean, keyA, domain, err := validateBriefInput(base)
	if err != nil {
		t.Fatal(err)
	}
	base.OwnDomain = "other.example"
	_, keyB, _, err := validateBriefInput(base)
	if err != nil {
		t.Fatal(err)
	}
	if keyA != keyB || domain != "example.com" || clean.Keyword != "SEO tools" {
		t.Fatalf("normalization/key = %+v %q %q; own domain must not affect cache key", clean, keyA, keyB)
	}
	for _, input := range []BriefInput{
		{Keyword: "site:example.com", LocationCode: 2840, LanguageCode: "en"},
		{Keyword: "x", LocationCode: 2840, LanguageCode: "zz"},
		{Keyword: "x", LocationCode: 999999, LanguageCode: "en"},
		{Keyword: "x", LocationCode: 2840, LanguageCode: "en", OwnDomain: "https://bad.example/path"},
	} {
		if _, _, _, err := validateBriefInput(input); err == nil {
			t.Errorf("accepted invalid input: %+v", input)
		}
	}
}

func TestBriefServiceFiltersOwnDomainAfterCacheAndOnlyFetchesOnMiss(t *testing.T) {
	provider := &briefProviderFake{brief: ContentBrief{
		Questions: []string{"What is technical SEO?"},
		Competitors: []BriefCompetitor{
			{URL: "https://www.example.com/article", Title: "Own page"},
			{URL: "https://blog.example.com/guide", Title: "Own subdomain"},
			{URL: "https://competitor.test/guide", Title: "Competitor", Headings: []string{"Overview"}},
		},
	}}
	cache := &briefCacheFake{}
	service := BriefService{Cache: cache, Provider: provider}
	got, err := service.Get(context.Background(), "org-a", BriefInput{Keyword: "seo", LocationCode: 2840, LanguageCode: "en", OwnDomain: "example.com"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || cache.built != 1 || len(got.Competitors) != 1 || got.Competitors[0].Title != "Competitor" {
		t.Fatalf("first result=%+v provider=%d builds=%d", got, provider.calls, cache.built)
	}
	got, err = service.Get(context.Background(), "org-b", BriefInput{Keyword: "SEO", LocationCode: 2840, LanguageCode: "en", OwnDomain: "different.test"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || len(got.Competitors) != 3 {
		t.Fatalf("cache was not shared across orgs or filtering leaked: %+v calls=%d", got, provider.calls)
	}
	if _, err := service.Get(context.Background(), "org-b", BriefInput{Keyword: "seo", LocationCode: 2840, LanguageCode: "en"}, true); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 {
		t.Fatalf("refresh did not fetch again: calls=%d", provider.calls)
	}
}

// fakeBudgetClient answers like DataForSEO: one SERP, then one content
// parsing task per call (the live endpoint refuses more), per URL.
type fakeBudgetClient struct {
	org          string
	feature      spend.Feature
	ceiling      float64
	requestCount int
	serp         []byte
	pages        map[string][]byte // URL -> parsing payload; missing = provider error
	err          error
}

func (c *fakeBudgetClient) DoBudgeted(_ context.Context, organizationID string, feature spend.Feature, ceiling float64, work func(dataforseo.DoBudgetedRequest) error) error {
	c.org, c.feature, c.ceiling = organizationID, feature, ceiling
	request := func(_ string, path string, body []byte, _ bool) (json.RawMessage, error) {
		c.requestCount++
		var tasks []map[string]any
		if err := json.Unmarshal(body, &tasks); err != nil {
			return nil, err
		}
		if len(tasks) != 1 {
			return nil, fmt.Errorf("live endpoints take one task per call, got %d", len(tasks))
		}
		switch path {
		case "/v3/serp/google/organic/live/advanced":
			if tasks[0]["depth"] != float64(30) {
				return nil, fmt.Errorf("unexpected SERP request %s", body)
			}
			return c.serp, nil
		case "/v3/on_page/content_parsing/live":
			link, _ := tasks[0]["url"].(string)
			if payload, ok := c.pages[link]; ok {
				return payload, nil
			}
			return nil, errors.New("DataForSEO task error 40400")
		default:
			return nil, fmt.Errorf("unexpected provider path %q", path)
		}
	}
	if c.err != nil {
		return c.err
	}
	return work(request)
}

func serpWithOrganic(count int) []byte {
	items := make([]any, 0, count+1)
	for i := range count {
		items = append(items, map[string]any{"type": "organic", "url": fmt.Sprintf("https://site%d.test/page", i), "title": fmt.Sprintf("Title %d", i)})
	}
	items = append(items, map[string]any{"type": "people_also_ask", "items": []any{map[string]any{"type": "people_also_ask_element", "title": "What is technical SEO?"}}})
	serp, _ := json.Marshal(map[string]any{"status_code": 20000, "tasks": []any{map[string]any{"status_code": 20000, "result": []any{map[string]any{"items": items}}}}})
	return serp
}

func parsedPage(heading, text string) []byte {
	payload, _ := json.Marshal(map[string]any{"status_code": 20000, "tasks": []any{map[string]any{"status_code": 20000, "result": []any{map[string]any{
		"items": []any{map[string]any{"page_content": map[string]any{
			"header":     map[string]any{"primary_content": []any{map[string]any{"text": "Menu Login Pricing"}}},
			"main_topic": []any{map[string]any{"h_title": heading, "main_title": "Guide", "primary_content": []any{map[string]any{"text": text}}}},
		}}},
	}}}}})
	return payload
}

func TestDataForSEOBriefProviderParsesEachPageSeparatelyAndBuildsTerms(t *testing.T) {
	client := &fakeBudgetClient{serp: serpWithOrganic(10), pages: map[string][]byte{}}
	for i := range briefParses {
		client.pages[fmt.Sprintf("https://site%d.test/page", i)] = parsedPage("Crawl budget basics", "Technical SEO helps search engines crawl pages. Crawl budget matters for large sites. Use a sitemap.")
	}
	brief, err := (DataForSEOBriefProvider{Client: client}).Fetch(context.Background(), "org-9", BriefInput{Keyword: "technical seo", LocationCode: 2840, LanguageCode: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if client.org != "org-9" || client.feature != spend.KeywordResearch || client.ceiling != briefCeiling || client.requestCount != 1+briefParses {
		t.Fatalf("provider budget = %+v", client)
	}
	if len(brief.Competitors) != briefParses || len(brief.Questions) != 1 || brief.Competitors[0].Headings[0] != "Crawl budget basics" {
		t.Fatalf("unexpected brief: %+v", brief)
	}
	if brief.Competitors[0].WordCount == 0 || strings.Contains(brief.Competitors[0].Text, "Menu Login") {
		t.Fatalf("page text must be the main content only: %+v", brief.Competitors[0])
	}
	if !slices.ContainsFunc(brief.Terms, func(term BriefTerm) bool { return term.Term == "crawl budget" && term.Domains == briefParses }) {
		t.Fatalf("terms = %+v, want crawl budget used by every page", brief.Terms)
	}
	if !slices.Contains(brief.Subtopics, "crawl budget") {
		t.Fatalf("subtopics = %v", brief.Subtopics)
	}
}

func TestDataForSEOBriefProviderSkipsUnreadablePagesButNeedsThree(t *testing.T) {
	text := "Technical SEO helps search engines crawl pages."
	client := &fakeBudgetClient{serp: serpWithOrganic(10), pages: map[string][]byte{
		"https://site0.test/page": parsedPage("One", text),
		"https://site3.test/page": parsedPage("Two", text),
		"https://site5.test/page": parsedPage("Three", text),
	}}
	brief, err := (DataForSEOBriefProvider{Client: client}).Fetch(context.Background(), "org", BriefInput{Keyword: "seo", LocationCode: 2840, LanguageCode: "en"})
	if err != nil || len(brief.Competitors) != 3 {
		t.Fatalf("Fetch = %d competitors, err %v; want the 3 readable pages", len(brief.Competitors), err)
	}

	delete(client.pages, "https://site5.test/page")
	if _, err := (DataForSEOBriefProvider{Client: client}).Fetch(context.Background(), "org", BriefInput{Keyword: "seo", LocationCode: 2840, LanguageCode: "en"}); !errors.Is(err, ErrBriefUnavailable) {
		t.Fatalf("two readable pages: err = %v, want ErrBriefUnavailable so the hold is released", err)
	}
}

func TestPostgresBriefCacheCoalescesConcurrentMissesAndExpires(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := contentTestPool(ctx, t)
	key := strings.Repeat("a", 64)
	if _, err := pool.Exec(ctx, `DELETE FROM go_content_brief_cache WHERE cache_key=$1`, key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM go_content_brief_cache WHERE cache_key=$1`, key)
	})
	cache := PostgresBriefCache{DB: pool}
	var calls atomic.Int32
	build := func() (ContentBrief, error) {
		calls.Add(1)
		time.Sleep(80 * time.Millisecond)
		return ContentBrief{Keyword: "shared", LocationCode: 2840, LanguageCode: "en", FetchedAt: time.Now().UTC(), Questions: []string{"Q?"}}, nil
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := cache.GetOrBuild(ctx, key, false, build); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("build calls=%d, want 1", got)
	}
	if _, err := pool.Exec(ctx, `UPDATE go_content_brief_cache SET fetched_at=now() - interval '31 days', expires_at=now() - interval '1 second' WHERE cache_key=$1`, key); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.GetOrBuild(ctx, key, false, build); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("after expiry build calls=%d, want 2", got)
	}
}

func TestBriefCacheDoesNotPersistFailedBuild(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := contentTestPool(ctx, t)
	key := strings.Repeat("b", 64)
	if _, err := pool.Exec(ctx, `DELETE FROM go_content_brief_cache WHERE cache_key=$1`, key); err != nil {
		t.Fatal(err)
	}
	cache := PostgresBriefCache{DB: pool}
	want := errors.New("provider rejected query")
	if _, err := cache.GetOrBuild(ctx, key, false, func() (ContentBrief, error) { return ContentBrief{}, want }); !errors.Is(err, want) {
		t.Fatalf("build error=%v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM go_content_brief_cache WHERE cache_key=$1`, key).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed cache rows=%d err=%v", count, err)
	}
}
