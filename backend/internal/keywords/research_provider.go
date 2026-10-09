package keywords

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

// Info is the volume block of a keyword item.
type Info struct {
	SearchVolume     *int
	CPC              *float64
	Competition      *float64 // 0-1 paid-competition ratio
	CompetitionLevel *string
	Monthly          []MonthlySearch
}

// LabsItem is a keyword from DataForSEO Labs. Clickstream is set only when the
// request asked for clickstream-refined volumes, which double its cost.
type LabsItem struct {
	Keyword     string
	Info        Info
	Clickstream *Info
	Difficulty  *int
	MainIntent  *string
}

// AdsItem is a keyword from Google Ads: volume, CPC and a 0-100 competition
// index, but no difficulty or intent.
type AdsItem struct {
	Keyword          string
	SearchVolume     *int
	CPC              *float64
	Competition      *string
	CompetitionIndex *float64
	Monthly          []MonthlySearch
}

// LabsSource is one Labs keyword endpoint.
type LabsSource string

// The Labs keyword endpoints research can use.
const (
	SourceRelated     LabsSource = "related"
	SourceSuggestions LabsSource = "suggestions"
	SourceIdeas       LabsSource = "ideas"
)

// LabsRequest asks Labs for keywords around a seed.
type LabsRequest struct {
	Source         LabsSource
	Seed           string
	LocationCode   int
	LanguageCode   string
	Limit          int
	Clickstream    bool
	IgnoreSynonyms bool
}

// AdsVolumeRequest asks Google Ads for the volume of known keywords.
type AdsVolumeRequest struct {
	Keywords     []string
	LocationCode int
	LocationName *string // a city or region; wins over LocationCode
	LanguageCode string
}

// SerpRequest asks for a live organic SERP.
type SerpRequest struct {
	Keyword      string
	LocationCode int
	LanguageCode string
	LocationName *string
	Depth        int
}

// SerpItem is one SERP element; only organic ones are shown.
type SerpItem struct {
	Type                     string
	RankGroup                *int
	RankAbsolute             *int
	Title, URL, Domain       string
	Description              string
	Etv                      *float64
	EstimatedPaidTrafficCost *float64
	ReferringDomains         *float64
	Backlinks                *float64
}

// DataProvider is the keyword data the research service needs. Every call is
// billed to the organization.
type DataProvider interface {
	LabsKeywords(ctx context.Context, org string, req LabsRequest) ([]LabsItem, error)
	LabsOverview(ctx context.Context, org string, keywords []string, locationCode int, languageCode string, clickstream bool) ([]LabsItem, error)
	AdsIdeas(ctx context.Context, org, seed string, locationCode int, languageCode string, limit int) ([]AdsItem, error)
	AdsVolume(ctx context.Context, org string, req AdsVolumeRequest) ([]AdsItem, error)
	SerpLive(ctx context.Context, org string, req SerpRequest) ([]SerpItem, error)
}

// DataForSEOProvider implements DataProvider on the shared client.
type DataForSEOProvider struct{ Client *dataforseo.Client }

const (
	labsRelatedPath     = "/v3/dataforseo_labs/google/related_keywords/live"
	labsSuggestionsPath = "/v3/dataforseo_labs/google/keyword_suggestions/live"
	labsIdeasPath       = "/v3/dataforseo_labs/google/keyword_ideas/live"
	labsOverviewPath    = "/v3/dataforseo_labs/google/keyword_overview/live"
	adsIdeasPath        = "/v3/keywords_data/google_ads/keywords_for_keywords/live"
	adsVolumePath       = "/v3/keywords_data/google_ads/search_volume/live"
	serpLiveAdvanced    = "/v3/serp/google/organic/live/advanced"
)

type wireMonthly struct {
	Year         *int `json:"year"`
	Month        *int `json:"month"`
	SearchVolume *int `json:"search_volume"`
}

func monthly(in []wireMonthly) []MonthlySearch {
	out := make([]MonthlySearch, len(in))
	for i, m := range in {
		out[i] = MonthlySearch{Year: derefInt(m.Year), Month: derefInt(m.Month), SearchVolume: derefInt(m.SearchVolume)}
	}
	return out
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

type wireInfo struct {
	SearchVolume     *int          `json:"search_volume"`
	CPC              *float64      `json:"cpc"`
	Competition      *float64      `json:"competition"`
	CompetitionLevel *string       `json:"competition_level"`
	Monthly          []wireMonthly `json:"monthly_searches"`
}

func (w *wireInfo) info() Info {
	if w == nil {
		return Info{Monthly: []MonthlySearch{}}
	}
	return Info{SearchVolume: w.SearchVolume, CPC: w.CPC, Competition: w.Competition, CompetitionLevel: w.CompetitionLevel, Monthly: monthly(w.Monthly)}
}

type wireLabsItem struct {
	Keyword           *string   `json:"keyword"`
	KeywordInfo       *wireInfo `json:"keyword_info"`
	ClickstreamInfo   *wireInfo `json:"keyword_info_normalized_with_clickstream"`
	KeywordProperties *struct {
		KeywordDifficulty *int `json:"keyword_difficulty"`
	} `json:"keyword_properties"`
	SearchIntentInfo *struct {
		MainIntent *string `json:"main_intent"`
	} `json:"search_intent_info"`
	// Related keywords wrap the payload one level deeper.
	KeywordData *wireLabsItem `json:"keyword_data"`
}

func (w wireLabsItem) item() (LabsItem, bool) {
	if w.Keyword == nil || *w.Keyword == "" {
		return LabsItem{}, false
	}
	out := LabsItem{Keyword: *w.Keyword, Info: w.KeywordInfo.info()}
	if w.ClickstreamInfo != nil {
		c := w.ClickstreamInfo.info()
		out.Clickstream = &c
	}
	if w.KeywordProperties != nil {
		out.Difficulty = w.KeywordProperties.KeywordDifficulty
	}
	if w.SearchIntentInfo != nil {
		out.MainIntent = w.SearchIntentInfo.MainIntent
	}
	return out, true
}

func (p DataForSEOProvider) post(ctx context.Context, org, path string, body any) ([]json.RawMessage, error) {
	raw, err := json.Marshal([]any{body})
	if err != nil {
		return nil, fmt.Errorf("encode %s request: %w", path, err)
	}
	// Not retry-safe: a repeated billed call would be charged twice.
	response, err := p.Client.Do(ctx, org, http.MethodPost, path, raw, false)
	if err != nil {
		return nil, fmt.Errorf("DataForSEO %s: %w", path, err)
	}
	results, err := dataforseo.Results(response)
	if err != nil {
		return nil, err
	}
	return results, nil
}

// labsItems reads the items of a Labs result.
func labsItems(results []json.RawMessage) ([]wireLabsItem, error) {
	if len(results) == 0 {
		return nil, nil
	}
	var result struct {
		Items []wireLabsItem `json:"items"`
	}
	if err := json.Unmarshal(results[0], &result); err != nil {
		return nil, fmt.Errorf("decode Labs result: %w", err)
	}
	return result.Items, nil
}

// LabsKeywords fetches related keywords, suggestions or ideas around a seed.
func (p DataForSEOProvider) LabsKeywords(ctx context.Context, org string, req LabsRequest) ([]LabsItem, error) {
	var path string
	body := map[string]any{
		"location_code": req.LocationCode, "language_code": req.LanguageCode, "limit": req.Limit,
		// Clickstream-refined volumes double the request cost, so they are opt-in.
		"include_clickstream_data": req.Clickstream, "include_serp_info": false, "ignore_synonyms": req.IgnoreSynonyms,
	}
	switch req.Source {
	case SourceRelated:
		path = labsRelatedPath
		body["keyword"], body["depth"] = req.Seed, 3
	case SourceSuggestions:
		path = labsSuggestionsPath
		body["keyword"], body["include_seed_keyword"], body["exact_match"] = req.Seed, true, false
	case SourceIdeas:
		path = labsIdeasPath
		body["keywords"], body["closely_variants"] = []string{req.Seed}, false
	default:
		return nil, fmt.Errorf("unknown Labs source %q", req.Source)
	}
	results, err := p.post(ctx, org, path, body)
	if err != nil {
		return nil, err
	}
	wire, err := labsItems(results)
	if err != nil {
		return nil, err
	}
	var out []LabsItem
	for _, w := range wire {
		if req.Source == SourceRelated {
			if w.KeywordData == nil {
				continue
			}
			w = *w.KeywordData
		}
		if item, ok := w.item(); ok {
			out = append(out, item)
		}
	}
	return out, nil
}

// LabsOverview fetches the metrics of known keywords.
func (p DataForSEOProvider) LabsOverview(ctx context.Context, org string, keywords []string, locationCode int, languageCode string, clickstream bool) ([]LabsItem, error) {
	results, err := p.post(ctx, org, labsOverviewPath, map[string]any{
		"keywords": keywords, "location_code": locationCode, "language_code": languageCode, "include_clickstream_data": clickstream,
	})
	if err != nil {
		return nil, err
	}
	wire, err := labsItems(results)
	if err != nil {
		return nil, err
	}
	var out []LabsItem
	for _, w := range wire {
		if item, ok := w.item(); ok {
			out = append(out, item)
		}
	}
	return out, nil
}

type wireAdsItem struct {
	Keyword          *string       `json:"keyword"`
	SearchVolume     *int          `json:"search_volume"`
	CPC              *float64      `json:"cpc"`
	Competition      *string       `json:"competition"`
	CompetitionIndex *float64      `json:"competition_index"`
	Monthly          []wireMonthly `json:"monthly_searches"`
}

func adsItems(results []json.RawMessage) ([]AdsItem, error) {
	var out []AdsItem
	for _, raw := range results {
		var w wireAdsItem
		if err := json.Unmarshal(raw, &w); err != nil {
			return nil, fmt.Errorf("decode Google Ads item: %w", err)
		}
		if w.Keyword == nil || *w.Keyword == "" {
			continue
		}
		out = append(out, AdsItem{Keyword: *w.Keyword, SearchVolume: w.SearchVolume, CPC: w.CPC, Competition: w.Competition,
			CompetitionIndex: w.CompetitionIndex, Monthly: monthly(w.Monthly)})
	}
	return out, nil
}

// AdsIdeas fetches Google Ads keyword ideas, for countries Labs does not cover.
// The endpoint has no limit parameter and can return thousands of ideas for
// one flat fee, so the result is cut to what the caller asked for.
func (p DataForSEOProvider) AdsIdeas(ctx context.Context, org, seed string, locationCode int, languageCode string, limit int) ([]AdsItem, error) {
	results, err := p.post(ctx, org, adsIdeasPath, map[string]any{
		"keywords": []string{seed}, "location_code": locationCode, "language_code": languageCode, "sort_by": "search_volume",
	})
	if err != nil {
		return nil, err
	}
	items, err := adsItems(results)
	if err != nil {
		return nil, err
	}
	return items[:min(len(items), limit)], nil
}

// adsInvalidChars are symbols, or an emoji, that make Google Ads fail a whole
// search_volume task when a single keyword has one.
var adsInvalidChars = regexp.MustCompile("[!@%,*(){}<>|^~;=?`]|[" +
	"\u00a9\u00ae\u203c\u2049\u2122\u2139\u2194-\u2199\u21a9\u21aa\u231a\u231b\u2328\u23cf\u23e9-\u23f3\u23f8-\u23fa" +
	"\u24c2\u25aa\u25ab\u25b6\u25c0\u25fb-\u25fe\u2600-\u27bf\u2934\u2935\u2b05-\u2b07\u2b1b\u2b1c\u2b50\u2b55" +
	"\u3030\u303d\u3297\u3299\U0001F000-\U0001FAFF]")

// IsAdsKeyword reports whether Google Ads search_volume accepts the keyword.
func IsAdsKeyword(keyword string) bool {
	return len(keyword) <= 80 && len(strings.Fields(keyword)) <= 10 && !adsInvalidChars.MatchString(keyword)
}

// AdsVolume fetches volume for the keywords Google Ads accepts. One rejected
// keyword fails the whole task, so those are left out and get no Ads metrics.
func (p DataForSEOProvider) AdsVolume(ctx context.Context, org string, req AdsVolumeRequest) ([]AdsItem, error) {
	var accepted []string
	for _, k := range req.Keywords {
		if IsAdsKeyword(k) {
			accepted = append(accepted, k)
		}
	}
	if len(accepted) == 0 {
		return nil, nil
	}
	body := map[string]any{"keywords": accepted, "language_code": req.LanguageCode}
	if req.LocationName != nil && *req.LocationName != "" {
		body["location_name"] = *req.LocationName
	} else {
		body["location_code"] = req.LocationCode
	}
	results, err := p.post(ctx, org, adsVolumePath, body)
	if err != nil {
		return nil, err
	}
	return adsItems(results)
}

// SerpLive fetches a live organic SERP snapshot.
func (p DataForSEOProvider) SerpLive(ctx context.Context, org string, req SerpRequest) ([]SerpItem, error) {
	if req.Depth < 10 || req.Depth > 100 || req.Depth%10 != 0 {
		return nil, fmt.Errorf("SERP depth %d is not a provider depth", req.Depth)
	}
	body := map[string]any{"keyword": req.Keyword, "language_code": req.LanguageCode, "device": "desktop", "os": "windows", "depth": req.Depth}
	if req.LocationName != nil && *req.LocationName != "" {
		body["location_name"] = *req.LocationName
	} else {
		body["location_code"] = req.LocationCode
	}
	results, err := p.post(ctx, org, serpLiveAdvanced, body)
	// A valid empty SERP comes back as a billed task error.
	if taskErr, ok := errors.AsType[*dataforseo.TaskError](err); ok && taskErr.StatusCode == 40501 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	var result struct {
		Items []struct {
			Type          string   `json:"type"`
			RankGroup     *int     `json:"rank_group"`
			RankAbsolute  *int     `json:"rank_absolute"`
			Domain        *string  `json:"domain"`
			Title         *string  `json:"title"`
			URL           *string  `json:"url"`
			Description   *string  `json:"description"`
			Etv           *float64 `json:"etv"`
			PaidCost      *float64 `json:"estimated_paid_traffic_cost"`
			BacklinksInfo *struct {
				ReferringDomains *float64 `json:"referring_domains"`
				Backlinks        *float64 `json:"backlinks"`
			} `json:"backlinks_info"`
		} `json:"items"`
	}
	if err := json.Unmarshal(results[0], &result); err != nil {
		return nil, fmt.Errorf("decode SERP result: %w", err)
	}
	out := make([]SerpItem, 0, len(result.Items))
	for _, w := range result.Items {
		item := SerpItem{Type: w.Type, RankGroup: w.RankGroup, RankAbsolute: w.RankAbsolute, Etv: w.Etv, EstimatedPaidTrafficCost: w.PaidCost}
		item.Domain, item.Title, item.URL, item.Description = str(w.Domain), str(w.Title), str(w.URL), str(w.Description)
		if w.BacklinksInfo != nil {
			item.ReferringDomains, item.Backlinks = w.BacklinksInfo.ReferringDomains, w.BacklinksInfo.Backlinks
		}
		out = append(out, item)
	}
	return out, nil
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
