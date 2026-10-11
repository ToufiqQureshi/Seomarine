package keywords

import (
	"context"
	"errors"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

// MetricLookupInput fetches metrics for a bounded list of known keywords.
type MetricLookupInput struct {
	OrganizationID string
	Keywords       []string
	LocationCode   int
	LanguageCode   string
	Clickstream    bool
}

// MetricLookupRow is the provider-backed result for one keyword.
type MetricLookupRow struct {
	Keyword           string
	SearchVolume      *int
	KeywordDifficulty *int
	Intent            string
	CPC               *float64
	Competition       *float64
	CompetitionLevel  *string
	Monthly           []MonthlySearch
}

// MetricLookup uses Labs where available and Google Ads for Ads-only markets.
// Input is checked before the paid provider request.
func (s *ResearchService) MetricLookup(ctx context.Context, in MetricLookupInput) ([]MetricLookupRow, error) {
	if len(in.Keywords) < 1 || len(in.Keywords) > metricsBatchSize {
		return nil, ValidationError("Provide between 1 and 700 keywords.")
	}
	terms := make([]string, 0, len(in.Keywords))
	seen := make(map[string]bool, len(in.Keywords))
	for _, keyword := range in.Keywords {
		trimmed := strings.TrimSpace(keyword)
		if trimmed == "" || len([]rune(trimmed)) > 80 {
			return nil, ValidationError("Each keyword must contain 1 to 80 characters.")
		}
		normalized := NormalizeKeyword(trimmed)
		if !seen[normalized] {
			seen[normalized] = true
			terms = append(terms, trimmed)
		}
	}
	if s == nil || s.Data == nil {
		return nil, errors.New("keyword research provider is not configured")
	}

	rows := make([]MetricLookupRow, 0, len(terms))
	if market.KeywordDataProvider(in.LocationCode) == "google_ads" {
		items, err := s.Data.AdsVolume(ctx, in.OrganizationID, AdsVolumeRequest{
			Keywords: terms, LocationCode: in.LocationCode, LanguageCode: in.LanguageCode,
		})
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			keyword := NormalizeKeyword(item.Keyword)
			if keyword == "" {
				continue
			}
			var competition *float64
			if item.CompetitionIndex != nil {
				value := *item.CompetitionIndex / 100
				competition = &value
			}
			rows = append(rows, MetricLookupRow{
				Keyword: keyword, SearchVolume: item.SearchVolume, Intent: "unknown",
				CPC: item.CPC, Competition: competition, CompetitionLevel: item.Competition,
				Monthly: nonNilMonths(item.Monthly),
			})
		}
		return rows, nil
	}

	items, err := s.Data.LabsOverview(ctx, in.OrganizationID, terms, in.LocationCode, in.LanguageCode, in.Clickstream)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		keyword := NormalizeKeyword(item.Keyword)
		if keyword == "" {
			continue
		}
		volume := item.Info
		if item.Clickstream != nil && item.Clickstream.SearchVolume != nil && *item.Clickstream.SearchVolume != 0 {
			volume = *item.Clickstream
		}
		rows = append(rows, MetricLookupRow{
			Keyword: keyword, SearchVolume: volume.SearchVolume, KeywordDifficulty: item.Difficulty,
			Intent: normalizeIntent(item.MainIntent), CPC: item.Info.CPC, Competition: item.Info.Competition,
			CompetitionLevel: item.Info.CompetitionLevel, Monthly: nonNilMonths(volume.Monthly),
		})
	}
	return rows, nil
}
