package keywords

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

// Research limits and cache lifetimes, as in the legacy app.
const (
	MaxResearchSeeds  = 200
	minNonSeedForAuto = 5
	researchCacheTTL  = 24 * time.Hour
	serpCacheTTL      = 12 * time.Hour
	researchCacheVer  = 5 // v5: auto mode blends suggestions and ideas
	metricsBatchSize  = 700
	SerpShallowDepth  = 20
	SerpDeepDepth     = 100
	labsRelatedDepth  = 3
)

// ResultLimits and Modes research accepts.
var (
	ResultLimits = []int{150, 300, 500}
	Modes        = []string{"auto", "related", "suggestions", "ideas"}
)

// ResearchRow is one researched keyword.
type ResearchRow struct {
	Keyword           string          `json:"keyword"`
	SearchVolume      *int            `json:"searchVolume"`
	Trend             []MonthlySearch `json:"trend"`
	KeywordDifficulty *int            `json:"keywordDifficulty"`
	CPC               *float64        `json:"cpc"`
	Competition       *float64        `json:"competition"`
	Intent            string          `json:"intent"`
}

// ResearchResult is what research returns and caches. Source says where the
// rows came from: a Labs endpoint, google_ads for countries Labs does not
// serve, or blended for auto mode.
type ResearchResult struct {
	Rows         []ResearchRow `json:"rows"`
	Source       string        `json:"source"`
	UsedFallback bool          `json:"usedFallback"`
}

// ResearchInput asks for keyword ideas around the first keyword.
type ResearchInput struct {
	OrganizationID string
	ProjectID      string
	Keywords       []string
	LocationCode   int
	LanguageCode   string
	LocationName   *string // a city or region; volume, CPC and competition become local
	ResultLimit    int
	Mode           string
	Clickstream    bool
	GroupKeywords  bool

	dataLanguage string // the language sent to Labs, which serves only some per country
}

// Registry lists the canonical city and region names of a country.
type Registry interface {
	Locations(ctx context.Context, organizationID, country string) ([]SerpLocation, error)
}

// MetricStore keeps researched metrics for saved keywords.
type MetricStore interface {
	UpsertMetrics(ctx context.Context, projectID string, locationCode int, languageCode string, rows []Metric) error
}

// ResearchService runs keyword research and SERP analysis.
type ResearchService struct {
	Data     DataProvider
	Cache    locationCache
	Registry Registry
	Metrics  MetricStore
	Logger   *slog.Logger
	Now      func() time.Time
}

// NewResearchService builds a service that caches in Redis.
func NewResearchService(data DataProvider, rdb *redis.Client, registry Registry, metrics MetricStore, logger *slog.Logger) *ResearchService {
	return &ResearchService{Data: data, Cache: redisLocationCache{Client: rdb}, Registry: registry, Metrics: metrics, Logger: logger, Now: time.Now}
}

func normalizeIntent(raw *string) string {
	if raw == nil {
		return "unknown"
	}
	v := strings.ToLower(*raw)
	switch {
	case strings.Contains(v, "inform"):
		return "informational"
	case strings.Contains(v, "commerc"):
		return "commercial"
	case strings.Contains(v, "transact"):
		return "transactional"
	case strings.Contains(v, "navig"):
		return "navigational"
	}
	return "unknown"
}

// labsRows maps Labs items to rows, deduping by keyword. Clickstream-refined
// volume wins when the request asked for it and the item has one.
func labsRows(items []LabsItem) []ResearchRow {
	rows := make([]ResearchRow, 0, len(items))
	seen := map[string]bool{}
	for _, it := range items {
		k := NormalizeKeyword(it.Keyword)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		volume := it.Info
		if it.Clickstream != nil && it.Clickstream.SearchVolume != nil && *it.Clickstream.SearchVolume != 0 {
			volume = *it.Clickstream
		}
		rows = append(rows, ResearchRow{
			Keyword: k, SearchVolume: volume.SearchVolume, Trend: nonNilMonths(volume.Monthly), KeywordDifficulty: it.Difficulty,
			CPC: it.Info.CPC, Competition: it.Info.Competition, Intent: normalizeIntent(it.MainIntent),
		})
	}
	return rows
}

// adsRows maps Google Ads items to rows. Ads has no difficulty or intent, and
// reports competition as a 0-100 index where the app stores a 0-1 ratio.
func adsRows(items []AdsItem) []ResearchRow {
	rows := make([]ResearchRow, 0, len(items))
	seen := map[string]bool{}
	for _, it := range items {
		k := NormalizeKeyword(it.Keyword)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		var competition *float64
		if it.CompetitionIndex != nil {
			c := *it.CompetitionIndex / 100
			competition = &c
		}
		rows = append(rows, ResearchRow{Keyword: k, SearchVolume: it.SearchVolume, Trend: nonNilMonths(it.Monthly), CPC: it.CPC,
			Competition: competition, Intent: "unknown"})
	}
	return rows
}

// interleave alternates two sources into one list, dropping duplicates, so a
// volume-sorted source cannot crowd the other out before the limit.
func interleave(first, second []ResearchRow, limit int) []ResearchRow {
	rows := []ResearchRow{}
	seen := map[string]bool{}
	for i := 0; i < max(len(first), len(second)) && len(rows) < limit; i++ {
		for _, source := range [][]ResearchRow{first, second} {
			if i >= len(source) || seen[source[i].Keyword] || len(rows) >= limit {
				continue
			}
			seen[source[i].Keyword] = true
			rows = append(rows, source[i])
		}
	}
	return rows
}

func nonSeedCount(rows []ResearchRow, seed string) int {
	n := 0
	for _, r := range rows {
		if r.Keyword != seed {
			n++
		}
	}
	return n
}

func (s *ResearchService) cacheKey(prefix string, parts any) (string, error) {
	raw, err := json.Marshal(parts)
	if err != nil {
		return "", fmt.Errorf("encode cache key: %w", err)
	}
	sum := sha256.Sum256(raw)
	return prefix + hex.EncodeToString(sum[:]), nil
}

func (s *ResearchService) cached(ctx context.Context, key string, dst any) bool {
	raw, err := s.Cache.Get(ctx, key)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			s.Logger.WarnContext(ctx, "read keyword cache", "err", err)
		}
		return false
	}
	return json.Unmarshal(raw, dst) == nil
}

func (s *ResearchService) store(ctx context.Context, key string, value any, ttl time.Duration) {
	raw, err := json.Marshal(value)
	if err != nil {
		s.Logger.ErrorContext(ctx, "encode keyword cache", "err", err)
		return
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := s.Cache.Set(writeCtx, key, raw, ttl); err != nil {
		s.Logger.WarnContext(ctx, "write keyword cache", "err", err)
	}
}

// Research returns keyword ideas around the first keyword.
func (s *ResearchService) Research(ctx context.Context, in ResearchInput) (ResearchResult, error) {
	var seeds []string
	seen := map[string]bool{}
	for _, k := range in.Keywords {
		if n := NormalizeKeyword(k); n != "" && !seen[n] {
			seen[n] = true
			seeds = append(seeds, n)
		}
	}
	if len(seeds) == 0 {
		return ResearchResult{}, ValidationError("Add at least one keyword.")
	}
	// Labs source modes, clickstream and synonym filtering do not exist for
	// Google Ads countries; normalizing them lets equivalent requests share one
	// cache entry. Local volume replaces the clickstream volume, so a local
	// request never pays for clickstream. Labs only serves some languages per
	// country, and a task in another one would be billed and fail.
	ads := market.KeywordDataProvider(in.LocationCode) == "google_ads"
	switch {
	case ads:
		in.Mode, in.Clickstream, in.GroupKeywords = "auto", false, false
	default:
		if in.LocationName != nil {
			in.Clickstream = false
		}
	}
	in.dataLanguage = in.LanguageCode
	if !ads {
		in.dataLanguage = market.ResolveKeywordDataLanguage(in.LocationCode, in.LanguageCode)
	}
	if in.Mode == "" {
		in.Mode = "auto"
	}
	key, err := s.cacheKey("kw:research:", map[string]any{
		"v": researchCacheVer, "org": in.OrganizationID, "project": in.ProjectID, "keywords": seeds, "location": in.LocationCode,
		"language": in.LanguageCode, "limit": in.ResultLimit, "mode": in.Mode, "depth": labsRelatedDepth,
		"clickstream": in.Clickstream, "group": in.GroupKeywords, "locationName": in.LocationName,
	})
	if err != nil {
		return ResearchResult{}, err
	}
	var hit ResearchResult
	if s.cached(ctx, key, &hit) && len(hit.Rows) > 0 && validSource(hit.Source) {
		return hit, nil
	}

	var result ResearchResult
	switch {
	case in.LocationName != nil:
		result, err = s.researchLocal(ctx, in)
	case ads:
		var items []AdsItem
		items, err = s.Data.AdsIdeas(ctx, in.OrganizationID, seeds[0], in.LocationCode, in.dataLanguage, in.ResultLimit)
		result = ResearchResult{Rows: adsRows(items), Source: "google_ads"}
	case in.Mode == "auto":
		result, err = s.autoRows(ctx, in, seeds[0])
	default:
		var rows []ResearchRow
		rows, err = s.labsRows(ctx, in, LabsSource(in.Mode), seeds[0], in.ResultLimit)
		result = ResearchResult{Rows: rows, Source: in.Mode}
	}
	if err != nil {
		return ResearchResult{}, err
	}
	if result.Rows == nil {
		result.Rows = []ResearchRow{}
	}
	s.store(ctx, key, result, researchCacheTTL)
	// Metrics are stored per country, so local rows are not persisted here.
	if in.LocationName == nil {
		s.persist(ctx, in, result.Rows)
	}
	return result, nil
}

func validSource(source string) bool {
	switch source {
	case "related", "suggestions", "ideas", "google_ads", "blended":
		return true
	}
	return false
}

func (s *ResearchService) labsRows(ctx context.Context, in ResearchInput, source LabsSource, seed string, limit int) ([]ResearchRow, error) {
	items, err := s.Data.LabsKeywords(ctx, in.OrganizationID, LabsRequest{
		Source: source, Seed: seed, LocationCode: in.LocationCode, LanguageCode: in.dataLanguage, Limit: limit,
		Clickstream: in.Clickstream, IgnoreSynonyms: !in.GroupKeywords,
	})
	if err != nil {
		return nil, err
	}
	return labsRows(items), nil
}

// autoRows blends phrase-match suggestions with category-level ideas, half the
// limit each. Neither works alone: suggestions miss sibling head terms and ideas
// drift toward generic category terms. Related keywords are noisy for broad
// seeds, so they are only a fallback when the blend comes back thin.
func (s *ResearchService) autoRows(ctx context.Context, in ResearchInput, seed string) (ResearchResult, error) {
	half := in.ResultLimit / 2 // the limit is 150, 300 or 500: always even
	var suggestions, ideas []ResearchRow
	var suggestionsErr, ideasErr error
	// Both legs run to completion: each call is billed, and abandoning one that
	// is in flight would lose track of its spend.
	var wg sync.WaitGroup
	wg.Go(func() { suggestions, suggestionsErr = s.labsRows(ctx, in, SourceSuggestions, seed, half) })
	wg.Go(func() { ideas, ideasErr = s.labsRows(ctx, in, SourceIdeas, seed, half) })
	wg.Wait()
	if suggestionsErr != nil {
		if ideasErr != nil {
			s.Logger.WarnContext(ctx, "keyword ideas leg failed", "err", ideasErr)
		}
		return ResearchResult{}, suggestionsErr
	}
	if ideasErr != nil {
		return ResearchResult{}, ideasErr
	}
	blended := interleave(suggestions, ideas, in.ResultLimit)
	if nonSeedCount(blended, seed) >= minNonSeedForAuto {
		return ResearchResult{Rows: blended, Source: "blended"}, nil
	}
	related, err := s.labsRows(ctx, in, SourceRelated, seed, in.ResultLimit)
	if err != nil {
		return ResearchResult{}, err
	}
	return ResearchResult{Rows: interleave(blended, related, in.ResultLimit), Source: "blended", UsedFallback: true}, nil
}

// researchLocal takes ideas, difficulty and intent from the national research
// (cached and persisted as usual), then one Google Ads call replaces volume,
// CPC and competition with numbers for the city, county or region.
func (s *ResearchService) researchLocal(ctx context.Context, in ResearchInput) (ResearchResult, error) {
	if err := s.assertLocalLocation(ctx, in.OrganizationID, in.LocationCode, *in.LocationName); err != nil {
		return ResearchResult{}, err
	}
	national := in
	national.LocationName = nil
	result, err := s.Research(ctx, national)
	if err != nil {
		return ResearchResult{}, err
	}
	// A save from local results sends no metrics, so the saved keyword relies on
	// the stored national ones. Store them also when the result came from cache.
	s.persist(ctx, national, result.Rows)

	keywords := make([]string, len(result.Rows))
	for i, r := range result.Rows {
		keywords[i] = r.Keyword
	}
	items, err := s.Data.AdsVolume(ctx, in.OrganizationID, AdsVolumeRequest{Keywords: keywords, LocationCode: in.LocationCode, LocationName: in.LocationName, LanguageCode: in.LanguageCode})
	if err != nil {
		return ResearchResult{}, err
	}
	local := map[string]ResearchRow{}
	for _, r := range adsRows(items) {
		local[r.Keyword] = r
	}
	// A keyword Google Ads does not return (collapsed close variants, or text
	// Ads rejects) gets empty metrics, never the national numbers.
	rows := make([]ResearchRow, len(result.Rows))
	for i, r := range result.Rows {
		l := local[r.Keyword]
		r.SearchVolume, r.Trend, r.CPC, r.Competition = l.SearchVolume, nonNilMonths(l.Trend), l.CPC, l.Competition
		rows[i] = r
	}
	result.Rows = rows
	return result, nil
}

// assertLocalLocation refuses a name that is not a city, county or region of
// the country before any paid call runs. The registry is the list the location
// picker searches, so it also keeps postal codes out.
func (s *ResearchService) assertLocalLocation(ctx context.Context, org string, locationCode int, name string) error {
	locations, err := s.Registry.Locations(ctx, org, market.ISOCode(locationCode))
	if err != nil {
		return fmt.Errorf("load locations: %w", err)
	}
	for _, l := range locations {
		if l.LocationName == name {
			return nil
		}
	}
	country := "this country"
	if l, ok := market.Lookup(locationCode); ok {
		country = l.Label
	}
	return UnknownLocationError{Message: fmt.Sprintf("%q is not a city, county, or region we can find in %s.", market.FormatLocationLabel(name, 0), country)}
}

// UnknownLocationError says a city or region name is not in the registry.
type UnknownLocationError struct{ Message string }

func (e UnknownLocationError) Error() string { return e.Message }

func (s *ResearchService) persist(ctx context.Context, in ResearchInput, rows []ResearchRow) {
	if s.Metrics == nil || len(rows) == 0 {
		return
	}
	metrics := make([]Metric, len(rows))
	for i, r := range rows {
		intent := r.Intent
		metrics[i] = Metric{Keyword: r.Keyword, SearchVolume: r.SearchVolume, CPC: r.CPC, Competition: r.Competition,
			KeywordDifficulty: r.KeywordDifficulty, Intent: &intent, MonthlySearches: r.Trend}
	}
	// Failing to store metrics must not fail the research the user paid for.
	if err := s.Metrics.UpsertMetrics(ctx, in.ProjectID, in.LocationCode, in.LanguageCode, metrics); err != nil {
		s.Logger.ErrorContext(ctx, "persist keyword metrics", "err", err)
	}
}
