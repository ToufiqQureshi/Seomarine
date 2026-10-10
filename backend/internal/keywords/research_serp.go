package keywords

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
	"github.com/toufiqqureshi/seomarine/backend/internal/ranktracking"
)

// SerpResult is one organic result of a keyword's SERP.
type SerpResult struct {
	Rank                     int      `json:"rank"`
	Title                    string   `json:"title"`
	URL                      string   `json:"url"`
	Domain                   string   `json:"domain"`
	Description              string   `json:"description"`
	Etv                      *float64 `json:"etv"`
	EstimatedPaidTrafficCost *float64 `json:"estimatedPaidTrafficCost"`
	ReferringDomains         *float64 `json:"referringDomains"`
	Backlinks                *float64 `json:"backlinks"`
	IsNew                    bool     `json:"isNew"`
	RankChange               *float64 `json:"rankChange"`
}

// SerpAnalysis is a SERP snapshot. Depth is how deep it was crawled.
type SerpAnalysis struct {
	RequestedKeyword string       `json:"requestedKeyword"`
	Items            []SerpResult `json:"items"`
	Depth            int          `json:"depth"`
	Reason           string       `json:"reason,omitempty"`
}

// SerpAnalysisInput asks for the organic results of a keyword.
type SerpAnalysisInput struct {
	OrganizationID string
	ProjectID      string
	Keyword        string
	LocationCode   int
	LanguageCode   string
	LocationName   *string
	Depth          int // SerpShallowDepth or SerpDeepDepth
}

// SerpAnalysis returns the organic SERP of a keyword. Each 10 of depth is
// another crawled Google page and billed, so a cached snapshot at least as deep
// as the request answers it without a call. Depth lives in the cached value, not
// the key: the provider has no offset, so a deeper crawl re-fetches the head
// and replaces the entry instead of extending it.
func (s *ResearchService) SerpAnalysis(ctx context.Context, in SerpAnalysisInput) (SerpAnalysis, error) {
	keyword := NormalizeKeyword(in.Keyword)
	if keyword == "" {
		return SerpAnalysis{}, ValidationError("Enter a keyword.")
	}
	if in.LocationName != nil {
		if err := s.assertLocalLocation(ctx, in.OrganizationID, in.LocationCode, *in.LocationName); err != nil {
			return SerpAnalysis{}, err
		}
	}
	key, err := s.cacheKey("kw:serp:", map[string]any{
		"org": in.OrganizationID, "project": in.ProjectID, "keyword": keyword, "location": in.LocationCode,
		"language": in.LanguageCode, "locationName": in.LocationName,
	})
	if err != nil {
		return SerpAnalysis{}, err
	}
	var cached SerpAnalysis
	hit := s.cached(ctx, key, &cached)
	if hit && cached.Depth == 0 {
		cached.Depth = SerpShallowDepth // written before depth was tracked: assume the floor
	}
	if hit && cached.Depth >= in.Depth {
		return cached, nil
	}

	raw, err := s.Data.SerpLive(ctx, in.OrganizationID, SerpRequest{Keyword: keyword, LocationCode: in.LocationCode, LanguageCode: in.LanguageCode, LocationName: in.LocationName, Depth: in.Depth})
	if err != nil {
		return SerpAnalysis{}, err
	}
	result := SerpAnalysis{RequestedKeyword: keyword, Items: organicResults(raw), Depth: in.Depth}
	if len(result.Items) == 0 {
		result.Reason = "no_organic_results"
		// The provider reports "no results" transiently, and a deeper crawl
		// overwrites the same key. One empty re-crawl must not evict a good
		// snapshot, and being the deeper entry, answer every shallower request
		// with nothing for the rest of the TTL.
		if hit && len(cached.Items) > 0 {
			return result, nil
		}
	}
	s.store(ctx, key, result, serpCacheTTL)
	return result, nil
}

func organicResults(items []SerpItem) []SerpResult {
	out := []SerpResult{}
	for _, it := range items {
		if it.Type != "organic" {
			continue
		}
		rank := 0
		if it.RankGroup != nil {
			rank = *it.RankGroup
		} else if it.RankAbsolute != nil {
			rank = *it.RankAbsolute
		}
		out = append(out, SerpResult{Rank: rank, Title: it.Title, URL: it.URL, Domain: it.Domain, Description: it.Description,
			Etv: it.Etv, EstimatedPaidTrafficCost: it.EstimatedPaidTrafficCost, ReferringDomains: it.ReferringDomains, Backlinks: it.Backlinks})
	}
	return out
}

// metricRow is a keyword's refreshed metrics. Intent is raw: Labs main_intent,
// nil for Google Ads.
type metricRow struct {
	Keyword           string
	SearchVolume      *int
	CPC               *float64
	Competition       *float64
	KeywordDifficulty *int
	Intent            *string
	Monthly           []MonthlySearch
}

func nullMetricRow(keyword string) metricRow {
	return metricRow{Keyword: keyword, Monthly: []MonthlySearch{}}
}

func overviewRow(it LabsItem) metricRow {
	volume := it.Info
	// The clickstream block exists only when the caller opted in (it doubles the cost).
	if it.Clickstream != nil && it.Clickstream.SearchVolume != nil {
		volume = *it.Clickstream
	}
	return metricRow{Keyword: it.Keyword, SearchVolume: volume.SearchVolume, CPC: it.Info.CPC, Competition: it.Info.Competition,
		KeywordDifficulty: it.Difficulty, Intent: it.MainIntent, Monthly: nonNilMonths(volume.Monthly)}
}

func adsMetricRow(it AdsItem) metricRow {
	var competition *float64
	if it.CompetitionIndex != nil {
		c := *it.CompetitionIndex / 100
		competition = &c
	}
	return metricRow{Keyword: it.Keyword, SearchVolume: it.SearchVolume, CPC: it.CPC, Competition: competition, Monthly: nonNilMonths(it.Monthly)}
}

// metricsForList hydrates keywords with fresh metrics: Labs where it serves the
// country, Google Ads where it does not, batched under the per-call cap. For a
// city it merges both: local volume from Ads, national difficulty and intent
// from Labs. National volume is never shown for a local request; a keyword Ads
// does not return keeps difficulty and intent but gets no volume or CPC.
func (s *ResearchService) metricsForList(ctx context.Context, org string, keywords []string, locationCode int, languageCode string, locationName *string) ([]metricRow, error) {
	adsCountry := market.KeywordDataProvider(locationCode) == "google_ads"
	dataLanguage := languageCode
	if !adsCountry {
		dataLanguage = market.ResolveKeywordDataLanguage(locationCode, languageCode)
	}
	var rows []metricRow
	for batch := range slices.Chunk(keywords, metricsBatchSize) {
		switch {
		case adsCountry:
			items, err := s.Data.AdsVolume(ctx, org, AdsVolumeRequest{Keywords: batch, LocationCode: locationCode, LocationName: locationName, LanguageCode: languageCode})
			if err != nil {
				return nil, err
			}
			covered := map[string]bool{}
			for _, it := range items {
				covered[strings.ToLower(it.Keyword)] = true
				rows = append(rows, adsMetricRow(it))
			}
			// A local request must overwrite whatever scope the stored metrics had,
			// so keywords Ads collapsed away get explicit nulls. Keywords Ads
			// rejects can never have Ads metrics, so they get nulls on every request.
			for _, k := range batch {
				if !covered[strings.ToLower(k)] && (locationName != nil || !IsAdsKeyword(k)) {
					rows = append(rows, nullMetricRow(k))
				}
			}
		case locationName != nil:
			var adsItems []AdsItem
			var labsItems []LabsItem
			var adsErr, labsErr error
			var wg sync.WaitGroup
			wg.Go(func() {
				adsItems, adsErr = s.Data.AdsVolume(ctx, org, AdsVolumeRequest{Keywords: batch, LocationCode: locationCode, LocationName: locationName, LanguageCode: languageCode})
			})
			wg.Go(func() { labsItems, labsErr = s.Data.LabsOverview(ctx, org, batch, locationCode, dataLanguage, false) })
			wg.Wait()
			if adsErr != nil {
				return nil, adsErr
			}
			if labsErr != nil {
				return nil, labsErr
			}
			rows = append(rows, mergeLocalAndNational(batch, adsItems, labsItems)...)
		default:
			items, err := s.Data.LabsOverview(ctx, org, batch, locationCode, dataLanguage, false)
			if err != nil {
				return nil, err
			}
			for _, it := range items {
				rows = append(rows, overviewRow(it))
			}
		}
	}
	return rows, nil
}

// RankTrackingMetrics refreshes only the metrics displayed by rank tracking.
// It delegates to the same market-aware, capped provider path as keyword
// research, including local-volume and country-served-language rules.
func (s *ResearchService) RankTrackingMetrics(ctx context.Context, organizationID string, keywords []string, locationCode int, languageCode string, locationName *string) ([]ranktracking.KeywordMetric, error) {
	rows, err := s.metricsForList(ctx, organizationID, keywords, locationCode, languageCode, locationName)
	if err != nil {
		return nil, err
	}
	metrics := make([]ranktracking.KeywordMetric, 0, len(rows))
	for _, row := range rows {
		metrics = append(metrics, ranktracking.KeywordMetric{
			Keyword: row.Keyword, SearchVolume: row.SearchVolume,
			KeywordDifficulty: row.KeywordDifficulty, CPC: row.CPC,
		})
	}
	return metrics, nil
}

func mergeLocalAndNational(keywords []string, ads []AdsItem, labs []LabsItem) []metricRow {
	labsBy := map[string]LabsItem{}
	for _, it := range labs {
		labsBy[strings.ToLower(it.Keyword)] = it
	}
	var rows []metricRow
	covered := map[string]bool{}
	for _, it := range ads {
		covered[strings.ToLower(it.Keyword)] = true
		row := adsMetricRow(it)
		if l, ok := labsBy[strings.ToLower(it.Keyword)]; ok {
			row.KeywordDifficulty, row.Intent = l.Difficulty, l.MainIntent
		}
		rows = append(rows, row)
	}
	// Every keyword gets a row, so a refresh clears stale stored metrics even
	// when neither source returned it.
	for _, k := range keywords {
		if covered[strings.ToLower(k)] {
			continue
		}
		row := nullMetricRow(k)
		if l, ok := labsBy[strings.ToLower(k)]; ok {
			row.KeywordDifficulty, row.Intent = l.Difficulty, l.MainIntent
		}
		rows = append(rows, row)
	}
	return rows
}

// RefreshSavedMetrics re-fetches metrics for every saved keyword of the
// project, one provider call group per market, and returns how many keywords
// the provider answered for.
func (s *ResearchService) RefreshSavedMetrics(ctx context.Context, org, projectID string, saved SavedLister) (int, error) {
	list, err := saved.List(ctx, ListQuery{ProjectID: projectID, Sort: "createdAt", Descending: true})
	if err != nil {
		return 0, err
	}
	type marketKey struct {
		location int
		language string
	}
	groups := map[marketKey][]string{}
	var order []marketKey
	for _, k := range list.Rows {
		key := marketKey{k.LocationCode, k.LanguageCode}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], k.Keyword)
	}
	updated := 0
	for _, key := range order {
		rows, err := s.metricsForList(ctx, org, groups[key], key.location, key.language, nil)
		if err != nil {
			return updated, err
		}
		byKeyword := map[string]metricRow{}
		for _, r := range rows {
			byKeyword[strings.ToLower(r.Keyword)] = r
		}
		var metrics []Metric
		for _, k := range groups[key] {
			r, ok := byKeyword[strings.ToLower(k)]
			if !ok {
				continue
			}
			intent := normalizeIntent(r.Intent)
			metrics = append(metrics, Metric{Keyword: k, SearchVolume: r.SearchVolume, CPC: r.CPC, Competition: r.Competition,
				KeywordDifficulty: r.KeywordDifficulty, Intent: &intent, MonthlySearches: r.Monthly})
		}
		if err := s.Metrics.UpsertMetrics(ctx, projectID, key.location, key.language, metrics); err != nil {
			return updated, fmt.Errorf("store refreshed metrics: %w", err)
		}
		updated += len(byKeyword)
	}
	return updated, nil
}

// SavedLister lists a project's saved keywords.
type SavedLister interface {
	List(ctx context.Context, q ListQuery) (ListResult, error)
}
