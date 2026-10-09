package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

const (
	cacheTTL            = 12 * time.Hour
	cacheWriteTimeout   = 2 * time.Second
	defaultLocationCode = 2840
	defaultLanguageCode = "en"
	defaultPage         = 1
	defaultPageSize     = 50
	maxPage             = 1_000_000
)

type cache interface {
	Get(context.Context, string) ([]byte, error)
	Set(context.Context, string, []byte, time.Duration) error
}

type redisCache struct{ client *redis.Client }

func (c redisCache) Get(ctx context.Context, key string) ([]byte, error) {
	return c.client.Get(ctx, key).Bytes()
}
func (c redisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

// PaidPlans gates paid domain lookups when billing is configured.
type PaidPlans interface {
	HasPaidPlan(context.Context, string) (bool, error)
}

// Service retrieves and caches domain analysis reports for a project.
type Service struct {
	provider provider
	cache    cache
	logger   *slog.Logger
	now      func() time.Time
}

// NewService creates a service backed by the shared DataForSEO client and Redis.
func NewService(client *dataforseo.Client, rdb *redis.Client, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{provider: provider{client: client}, cache: redisCache{rdb}, logger: logger, now: time.Now}
}

// NewServiceWithCache supports a cache implementation owned by the application.
func NewServiceWithCache(client *dataforseo.Client, c cache, logger *slog.Logger, now func() time.Time) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	return &Service{provider: provider{client: client}, cache: c, logger: logger, now: now}
}

// OverviewResult is the Domain Overview card: organic traffic and keyword count.
type OverviewResult struct {
	Domain           string   `json:"domain"`
	Scope            Scope    `json:"scope"`
	DisplayTarget    string   `json:"displayTarget"`
	OrganicTraffic   *float64 `json:"organicTraffic"`
	OrganicKeywords  *float64 `json:"organicKeywords"`
	Backlinks        *float64 `json:"backlinks"`
	ReferringDomains *float64 `json:"referringDomains"`
	HasData          bool     `json:"hasData"`
	FetchedAt        string   `json:"fetchedAt"`
}

// KeywordSuggestion is a ranking keyword with its metrics.
type KeywordSuggestion struct {
	Keyword           string   `json:"keyword"`
	Position          *float64 `json:"position"`
	SearchVolume      *float64 `json:"searchVolume"`
	Traffic           *float64 `json:"traffic"`
	CPC               *float64 `json:"cpc"`
	KeywordDifficulty *float64 `json:"keywordDifficulty"`
}

// KeywordRow is one ranked keyword on the keywords tab, including the ranking URL.
type KeywordRow struct {
	KeywordSuggestion
	URL         *string `json:"url"`
	RelativeURL *string `json:"relativeUrl"`
}

// KeywordsPageInput is a validated request for one page of ranking keywords.
type KeywordsPageInput struct {
	ProjectID    string
	Domain       string
	Scope        Scope
	LocationCode int
	LanguageCode string
	Page         int
	PageSize     int
	SortMode     string
	SortOrder    string
	Filters      keywordFilters
	Search       string
}

// KeywordsPageResult is one page of ranking keywords.
type KeywordsPageResult struct {
	Domain     string       `json:"domain"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	TotalCount *int         `json:"totalCount"`
	HasMore    bool         `json:"hasMore"`
	Keywords   []KeywordRow `json:"keywords"`
	FetchedAt  string       `json:"fetchedAt"`
}

// PageResult is a ranking page with its organic traffic and keyword count.
type PageResult struct {
	Page           string   `json:"page"`
	RelativePath   *string  `json:"relativePath"`
	OrganicTraffic *float64 `json:"organicTraffic"`
	Keywords       *float64 `json:"keywords"`
}

// PagesPageInput is a validated request for one page of ranking pages.
type PagesPageInput struct {
	ProjectID    string
	Domain       string
	Scope        Scope
	LocationCode int
	LanguageCode string
	Page         int
	PageSize     int
	SortMode     string
	SortOrder    string
	Filters      keywordFilters
	Search       string
}

// PagesPageResult is one page of ranking pages.
type PagesPageResult struct {
	Domain     string       `json:"domain"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	TotalCount *int         `json:"totalCount"`
	HasMore    bool         `json:"hasMore"`
	Pages      []PageResult `json:"pages"`
	FetchedAt  string       `json:"fetchedAt"`
}

// Overview returns the Labs domain rank metrics. This endpoint reports the
// hostname and its subdomains regardless of the selected display scope.
func (s *Service) Overview(ctx context.Context, organizationID, projectID, rawDomain string, scope Scope, locationCode int, languageCode string) (OverviewResult, error) {
	target, err := parseResearchTarget(rawDomain, scope)
	if err != nil {
		return OverviewResult{}, badRequest(err.Error())
	}
	locationCode = normalizeLocationCode(locationCode)
	languageCode = normalizeLanguageCode(languageCode)
	key, err := makeCacheKey("overview", organizationID, projectID, struct {
		Domain       string
		LocationCode int
		LanguageCode string
	}{target.Hostname, locationCode, languageCode})
	if err != nil {
		return OverviewResult{}, err
	}
	var cached OverviewResult
	if s.readCache(ctx, key, &cached) && cached.HasData {
		cached.Scope, cached.DisplayTarget = target.Scope, target.Display
		return cached, nil
	}
	metrics, err := s.provider.overview(ctx, organizationID, target.Hostname, locationCode, languageCode)
	if err != nil {
		return OverviewResult{}, err
	}
	var traffic, keywords *float64
	if metrics.Organic != nil {
		traffic = roundNullable(metrics.Organic.ETV)
		keywords = roundNullable(metrics.Organic.Count)
	}
	result := OverviewResult{
		Domain: target.Hostname, Scope: target.Scope, DisplayTarget: target.Display,
		OrganicTraffic: traffic, OrganicKeywords: keywords,
		Backlinks: nil, ReferringDomains: nil,
		HasData:   keywords != nil && *keywords > 0,
		FetchedAt: s.now().UTC().Format(time.RFC3339Nano),
	}
	if result.HasData {
		s.writeCache(ctx, key, result)
	}
	return result, nil
}

// KeywordSuggestions returns the top 100 ranked keywords for a domain.
func (s *Service) KeywordSuggestions(ctx context.Context, organizationID, projectID, rawDomain string, scope Scope, locationCode int, languageCode string) ([]KeywordSuggestion, error) {
	target, err := parseResearchTarget(rawDomain, scope)
	if err != nil {
		return nil, badRequest(err.Error())
	}
	locationCode = normalizeLocationCode(locationCode)
	languageCode = normalizeLanguageCode(languageCode)
	key, err := makeCacheKey("keyword-suggestions", organizationID, projectID, struct {
		Domain       string
		Scope        Scope
		Path         string
		LocationCode int
		LanguageCode string
	}{target.Hostname, target.Scope, target.Path, locationCode, languageCode})
	if err != nil {
		return nil, err
	}
	var cached []KeywordSuggestion
	if s.readCache(ctx, key, &cached) && len(cached) > 0 {
		return cached, nil
	}
	filters := buildRankedKeywordsScopeFilter(target)
	request := map[string]any{
		"target": target.Hostname, "location_code": locationCode,
		"language_code": languageCode, "limit": 100,
		"order_by": []string{"ranked_serp_element.serp_item.etv,desc"},
	}
	if len(filters.clauses) != 0 {
		request["filters"] = join(filters.clauses, "and")
	}
	response, err := s.provider.rankedKeywords(ctx, organizationID, request)
	if err != nil {
		return nil, err
	}
	out := make([]KeywordSuggestion, 0, len(response.Items))
	for _, item := range response.Items {
		mapped, ok := mapKeyword(item)
		if !ok {
			continue
		}
		out = append(out, mapped.KeywordSuggestion)
	}
	if len(out) > 0 {
		s.writeCache(ctx, key, out)
	}
	return out, nil
}

// KeywordsPage returns sorted, filtered, paginated ranked keyword rows.
func (s *Service) KeywordsPage(ctx context.Context, organizationID string, input KeywordsPageInput) (KeywordsPageResult, error) {
	target, err := parseResearchTarget(input.Domain, input.Scope)
	if err != nil {
		return KeywordsPageResult{}, badRequest(err.Error())
	}
	page, pageSize, offset, err := normalizePaging(input.Page, input.PageSize)
	if err != nil {
		return KeywordsPageResult{}, err
	}
	orderBy, err := keywordOrderBy(input.SortMode, input.SortOrder)
	if err != nil {
		return KeywordsPageResult{}, err
	}
	locationCode, languageCode := normalizeLocationCode(input.LocationCode), normalizeLanguageCode(input.LanguageCode)
	scopeFilter := buildRankedKeywordsScopeFilter(target)
	filters, err := buildKeywordFilters(input.Filters, input.Search, scopeFilter)
	if err != nil {
		return KeywordsPageResult{}, badRequest(err.Error())
	}
	key, err := makeCacheKey("keywords-page", organizationID, input.ProjectID, struct {
		Domain       string
		Scope        Scope
		Path         string
		LocationCode int
		LanguageCode string
		Page         int
		PageSize     int
		SortMode     string
		SortOrder    string
		Filters      keywordFilters
		Search       string
	}{target.Hostname, target.Scope, target.Path, locationCode, languageCode, page, pageSize, input.SortMode, input.SortOrder, input.Filters, input.Search})
	if err != nil {
		return KeywordsPageResult{}, err
	}
	var cached KeywordsPageResult
	if s.readCache(ctx, key, &cached) {
		return cached, nil
	}
	request := map[string]any{
		"target": target.Hostname, "location_code": locationCode,
		"language_code": languageCode, "limit": pageSize, "offset": offset,
		"order_by": orderBy,
	}
	if len(filters) > 0 {
		request["filters"] = filters
	}
	response, err := s.provider.rankedKeywords(ctx, organizationID, request)
	if err != nil {
		return KeywordsPageResult{}, err
	}
	keywords := make([]KeywordRow, 0, len(response.Items))
	for _, item := range response.Items {
		if mapped, ok := mapKeyword(item); ok {
			keywords = append(keywords, mapped)
		}
	}
	result := KeywordsPageResult{
		Domain: target.Hostname, Page: page, PageSize: pageSize,
		TotalCount: response.TotalCount,
		HasMore:    hasMore(offset, len(response.Items), response.TotalCount, pageSize),
		Keywords:   keywords, FetchedAt: s.now().UTC().Format(time.RFC3339Nano),
	}
	s.writeCache(ctx, key, result)
	return result, nil
}

// PagesPage returns the organic pages reported for a target.
func (s *Service) PagesPage(ctx context.Context, organizationID string, input PagesPageInput) (PagesPageResult, error) {
	target, err := parseResearchTarget(input.Domain, input.Scope)
	if err != nil {
		return PagesPageResult{}, badRequest(err.Error())
	}
	page, pageSize, offset, err := normalizePaging(input.Page, input.PageSize)
	if err != nil {
		return PagesPageResult{}, err
	}
	orderBy, err := pageOrderBy(input.SortMode, input.SortOrder)
	if err != nil {
		return PagesPageResult{}, err
	}
	locationCode, languageCode := normalizeLocationCode(input.LocationCode), normalizeLanguageCode(input.LanguageCode)
	scopeFilter := buildRelevantPagesScopeFilter(target)
	filters, err := buildPageFilters(input.Filters, input.Search, scopeFilter)
	if err != nil {
		return PagesPageResult{}, badRequest(err.Error())
	}
	key, err := makeCacheKey("pages-page", organizationID, input.ProjectID, struct {
		Domain       string
		Scope        Scope
		Path         string
		LocationCode int
		LanguageCode string
		Page         int
		PageSize     int
		SortMode     string
		SortOrder    string
		Filters      keywordFilters
		Search       string
	}{target.Hostname, target.Scope, target.Path, locationCode, languageCode, page, pageSize, input.SortMode, input.SortOrder, input.Filters, input.Search})
	if err != nil {
		return PagesPageResult{}, err
	}
	var cached PagesPageResult
	if s.readCache(ctx, key, &cached) {
		return cached, nil
	}
	request := map[string]any{
		"target": target.Hostname, "location_code": locationCode,
		"language_code": languageCode, "limit": pageSize, "offset": offset,
		"order_by": orderBy,
	}
	if len(filters) > 0 {
		request["filters"] = filters
	}
	response, err := s.provider.relevantPages(ctx, organizationID, request)
	if err != nil {
		return PagesPageResult{}, err
	}
	pages := make([]PageResult, 0, len(response.Items))
	for _, item := range response.Items {
		mapped := mapPage(item)
		if mapped != nil {
			pages = append(pages, *mapped)
		}
	}
	result := PagesPageResult{
		Domain: target.Hostname, Page: page, PageSize: pageSize,
		TotalCount: response.TotalCount,
		HasMore:    hasMore(offset, len(response.Items), response.TotalCount, pageSize),
		Pages:      pages, FetchedAt: s.now().UTC().Format(time.RFC3339Nano),
	}
	s.writeCache(ctx, key, result)
	return result, nil
}

func mapKeyword(item rankedKeywordItem) (KeywordRow, bool) {
	var keyword *string
	var info *struct {
		SearchVolume      *float64 `json:"search_volume"`
		CPC               *float64 `json:"cpc"`
		KeywordDifficulty *float64 `json:"keyword_difficulty"`
	}
	var properties *struct {
		KeywordDifficulty *float64 `json:"keyword_difficulty"`
	}
	if item.KeywordData != nil {
		keyword, info, properties = item.KeywordData.Keyword, item.KeywordData.KeywordInfo, item.KeywordData.KeywordProperties
	}
	if keyword == nil {
		keyword = item.Keyword
	}
	if keyword == nil || *keyword == "" {
		return KeywordRow{}, false
	}
	var serp *struct {
		URL          *string  `json:"url"`
		RelativeURL  *string  `json:"relative_url"`
		RankAbsolute *float64 `json:"rank_absolute"`
		ETV          *float64 `json:"etv"`
	}
	var rank, traffic *float64
	var pageURL, relativeURL *string
	if element := item.RankedSERPElement; element != nil {
		serp = element.SERPItem
		rank, traffic = element.RankAbsolute, element.ETV
		pageURL, relativeURL = element.URL, element.RelativeURL
		if serp != nil {
			rank = firstFloat(serp.RankAbsolute, rank)
			traffic = firstFloat(serp.ETV, traffic)
			if serp.URL != nil {
				pageURL = serp.URL
			}
			if serp.RelativeURL != nil {
				relativeURL = serp.RelativeURL
			}
		}
	}
	if relativeURL == nil && pageURL != nil {
		relativeURL = toRelativePath(*pageURL)
	}
	var difficulty *float64
	if properties != nil {
		difficulty = properties.KeywordDifficulty
	}
	if difficulty == nil && info != nil {
		difficulty = info.KeywordDifficulty
	}
	var volume, cpc *float64
	if info != nil {
		volume, cpc = info.SearchVolume, info.CPC
	}
	return KeywordRow{
		KeywordSuggestion: KeywordSuggestion{
			Keyword: *keyword, Position: roundNullable(rank), SearchVolume: roundNullable(volume),
			Traffic: traffic, CPC: cpc, KeywordDifficulty: roundNullable(difficulty),
		},
		URL: pageURL, RelativeURL: relativeURL,
	}, true
}

func mapPage(item relevantPageItem) *PageResult {
	if item.PageAddress == nil || *item.PageAddress == "" {
		return nil
	}
	result := &PageResult{Page: *item.PageAddress, RelativePath: toRelativePath(*item.PageAddress)}
	if item.Metrics != nil && item.Metrics.Organic != nil {
		result.OrganicTraffic = roundNullable(item.Metrics.Organic.ETV)
		result.Keywords = roundNullable(item.Metrics.Organic.Count)
	}
	return result
}

// toRelativePath returns path and query of an absolute URL, or nil when the
// value is not an absolute URL.
func toRelativePath(raw string) *string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	relative := u.EscapedPath()
	if u.RawQuery != "" {
		relative += "?" + u.RawQuery
	}
	if relative == "" {
		relative = "/"
	}
	return &relative
}

func firstFloat(a, b *float64) *float64 {
	if a != nil {
		return a
	}
	return b
}

func normalizeLocationCode(value int) int {
	if value == 0 {
		return defaultLocationCode
	}
	return value
}
func normalizeLanguageCode(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultLanguageCode
	}
	return strings.TrimSpace(value)
}

func normalizePaging(page, pageSize int) (int, int, int, error) {
	if page == 0 {
		page = defaultPage
	}
	if page < 1 || page > maxPage {
		return 0, 0, 0, badRequest("page must be between 1 and 1000000.")
	}
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	if pageSize != 50 && pageSize != 100 && pageSize != 200 {
		return 0, 0, 0, badRequest("pageSize must be 50, 100 or 200.")
	}
	return page, pageSize, (page - 1) * pageSize, nil
}

func keywordOrderBy(mode, order string) ([]string, error) {
	fields := map[string]string{
		"rank":    "ranked_serp_element.serp_item.rank_absolute",
		"traffic": "ranked_serp_element.serp_item.etv",
		"volume":  "keyword_data.keyword_info.search_volume",
		"score":   "keyword_data.keyword_properties.keyword_difficulty",
		"cpc":     "keyword_data.keyword_info.cpc",
	}
	if mode == "" {
		mode = "traffic"
	}
	if order == "" {
		order = "desc"
	}
	field, ok := fields[mode]
	if !ok || (order != "asc" && order != "desc") {
		return nil, badRequest("sort mode or order is invalid.")
	}
	return []string{field + "," + order}, nil
}

func pageOrderBy(mode, order string) ([]string, error) {
	fields := map[string]string{"traffic": "metrics.organic.etv", "keywords": "metrics.organic.count"}
	if mode == "" {
		mode = "traffic"
	}
	if order == "" {
		order = "desc"
	}
	field, ok := fields[mode]
	if !ok || (order != "asc" && order != "desc") {
		return nil, badRequest("sort mode or order is invalid.")
	}
	return []string{field + "," + order}, nil
}

func hasMore(offset, fetched int, total *int, pageSize int) bool {
	if total != nil {
		return offset+fetched < *total
	}
	return fetched == pageSize
}

func (s *Service) readCache(ctx context.Context, key string, destination any) bool {
	if s.cache == nil {
		return false
	}
	value, err := s.cache.Get(ctx, key)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			s.logger.WarnContext(ctx, "read domain cache", "err", err)
		}
		return false
	}
	if err := json.Unmarshal(value, destination); err != nil {
		s.logger.WarnContext(ctx, "decode domain cache", "err", err)
		return false
	}
	return true
}

func (s *Service) writeCache(ctx context.Context, key string, value any) {
	if s.cache == nil {
		return
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		s.logger.ErrorContext(ctx, "encode domain cache", "err", err)
		return
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cacheWriteTimeout)
	defer cancel()
	if err := s.cache.Set(writeCtx, key, encoded, cacheTTL); err != nil {
		s.logger.WarnContext(ctx, "write domain cache", "err", err)
	}
}

func makeCacheKey(kind, organizationID, projectID string, value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode domain cache key: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "domain:" + kind + ":" + organizationID + ":" + projectID + ":" + hex.EncodeToString(sum[:]), nil
}
