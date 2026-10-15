package contenteditor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (c *briefCacheFake) GetOrBuild(_ context.Context, key string, build func() (ContentBrief, error)) (ContentBrief, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.key = key
	if c.err != nil {
		return ContentBrief{}, c.err
	}
	if c.brief.FetchedAt.IsZero() {
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
	got, err := service.Get(context.Background(), "org-a", BriefInput{Keyword: "seo", LocationCode: 2840, LanguageCode: "en", OwnDomain: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || cache.built != 1 || len(got.Competitors) != 1 || got.Competitors[0].Title != "Competitor" {
		t.Fatalf("first result=%+v provider=%d builds=%d", got, provider.calls, cache.built)
	}
	got, err = service.Get(context.Background(), "org-b", BriefInput{Keyword: "SEO", LocationCode: 2840, LanguageCode: "en", OwnDomain: "different.test"})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || len(got.Competitors) != 3 {
		t.Fatalf("cache was not shared across orgs or filtering leaked: %+v calls=%d", got, provider.calls)
	}
}

type fakeBudgetClient struct {
	org          string
	feature      spend.Feature
	ceiling      float64
	requestCount int
	serp         []byte
	parsing      []byte
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
		switch path {
		case "/v3/serp/google/organic/live/advanced":
			if len(tasks) != 1 || tasks[0]["depth"] != float64(30) {
				return nil, fmt.Errorf("unexpected SERP request %s", body)
			}
			return c.serp, nil
		case "/v3/on_page/content_parsing/live":
			if len(tasks) > briefParses {
				return nil, fmt.Errorf("parsed %d pages", len(tasks))
			}
			return c.parsing, nil
		default:
			return nil, fmt.Errorf("unexpected provider path %q", path)
		}
	}
	if c.err != nil {
		return c.err
	}
	return work(request)
}

func TestDataForSEOBriefProviderUsesOneNineCreditHoldAndEightParses(t *testing.T) {
	organic := make([]map[string]any, 10)
	for i := range organic {
		organic[i] = map[string]any{"type": "organic", "url": fmt.Sprintf("https://site%d.test/page", i), "title": fmt.Sprintf("Title %d", i)}
	}
	serp, _ := json.Marshal(map[string]any{"status_code": 20000, "tasks": []any{map[string]any{"status_code": 20000, "result": []any{map[string]any{"items": append(organic, map[string]any{"type": "people_also_ask", "items": []any{map[string]any{"type": "people_also_ask_element", "title": "What is SEO?"}}})}}}}})
	parseTasks := make([]any, briefParses)
	for i := range parseTasks {
		parseTasks[i] = map[string]any{"status_code": 20000, "result": []any{map[string]any{"title": "Page", "items": []any{map[string]any{"type": "h2", "text": "Useful heading"}}}}}
	}
	parsing, _ := json.Marshal(map[string]any{"status_code": 20000, "tasks": parseTasks})
	client := &fakeBudgetClient{serp: serp, parsing: parsing}
	provider := DataForSEOBriefProvider{Client: client}
	brief, err := provider.Fetch(context.Background(), "org-9", BriefInput{Keyword: "seo", LocationCode: 2840, LanguageCode: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if client.org != "org-9" || client.feature != spend.KeywordResearch || client.ceiling != briefCeiling || client.requestCount != 2 {
		t.Fatalf("provider budget = %+v", client)
	}
	if len(brief.Competitors) != briefParses || len(brief.Questions) != 1 || brief.Questions[0] != "What is SEO?" || brief.Competitors[0].Headings[0] != "Useful heading" {
		t.Fatalf("unexpected brief response: %+v", brief)
	}
}

func TestDataForSEOBriefProviderRejectsMalformedTasksSoBudgetCanRelease(t *testing.T) {
	client := &fakeBudgetClient{
		serp:    []byte(`{"status_code":20000,"tasks":[{"status_code":20000,"result":[{"items":[{"type":"organic","url":"https://competitor.test"}]}]}]}`),
		parsing: []byte(`{"status_code":20000,"tasks":[{"status_code":50000,"result":[]}]}`),
	}
	err := func() error {
		_, err := (DataForSEOBriefProvider{Client: client}).Fetch(context.Background(), "org", BriefInput{Keyword: "seo", LocationCode: 2840, LanguageCode: "en"})
		return err
	}()
	if err == nil || !strings.Contains(err.Error(), "could not parse") {
		t.Fatalf("Fetch error = %v", err)
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
		go func() { defer wg.Done(); _, err := cache.GetOrBuild(ctx, key, build); errs <- err }()
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
	if _, err := pool.Exec(ctx, `UPDATE go_content_brief_cache SET expires_at=now() - interval '1 second' WHERE cache_key=$1`, key); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.GetOrBuild(ctx, key, build); err != nil {
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
	if _, err := cache.GetOrBuild(ctx, key, func() (ContentBrief, error) { return ContentBrief{}, want }); !errors.Is(err, want) {
		t.Fatalf("build error=%v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM go_content_brief_cache WHERE cache_key=$1`, key).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed cache rows=%d err=%v", count, err)
	}
}
