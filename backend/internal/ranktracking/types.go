package ranktracking

import (
	"errors"
	"time"
)

// Errors the service returns; the handler maps each to an HTTP status.
var (
	ErrNotFound            = errors.New("rank tracking config not found")
	ErrDuplicate           = errors.New("already tracked")
	ErrLimit               = errors.New("limit reached")
	ErrLocationUnavailable = errors.New("city check unavailable")
	ErrMetricsUnavailable  = errors.New("rank tracking metrics are not available")
)

// ValidationError is a message that is safe to show the caller.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

// Config says which domain to track in which market and how often.
type Config struct {
	ID               string     `json:"id"`
	ProjectID        string     `json:"projectId"`
	Domain           string     `json:"domain"`
	LocationCode     int        `json:"locationCode"`
	LanguageCode     string     `json:"languageCode"`
	LocationName     *string    `json:"locationName"`
	Devices          string     `json:"devices"`
	SerpDepth        int        `json:"serpDepth"`
	ScheduleInterval Interval   `json:"scheduleInterval"`
	IsActive         bool       `json:"isActive"`
	LastCheckedAt    *time.Time `json:"lastCheckedAt"`
	NextCheckAt      *time.Time `json:"nextCheckAt"`
	LastSkipReason   *string    `json:"lastSkipReason"`
	CreatedAt        time.Time  `json:"createdAt"`
}

// Keyword is one tracked search term of a config.
type Keyword struct {
	ID                string     `json:"id"`
	ConfigID          string     `json:"configId"`
	Keyword           string     `json:"keyword"`
	MatchCase         bool       `json:"matchCase"`
	SearchVolume      *int       `json:"searchVolume"`
	KeywordDifficulty *int       `json:"keywordDifficulty"`
	CPC               *float64   `json:"cpc"`
	MetricsFetchedAt  *time.Time `json:"metricsFetchedAt"`
	CreatedAt         time.Time  `json:"createdAt"`
}

// KeywordMetric is the provider's volume, difficulty and CPC for a term.
type KeywordMetric struct {
	Keyword           string
	SearchVolume      *int
	KeywordDifficulty *int
	CPC               *float64
}

// KeywordMetricUpdate associates refreshed values with one stored keyword.
type KeywordMetricUpdate struct {
	ID                string    `json:"id"`
	SearchVolume      *int      `json:"search_volume"`
	KeywordDifficulty *int      `json:"keyword_difficulty"`
	CPC               *float64  `json:"cpc"`
	FetchedAt         time.Time `json:"fetched_at"`
}

// NewKeyword is a keyword ready to insert.
type NewKeyword struct {
	ID        string
	Keyword   string
	MatchCase bool
}

// ConfigUpdate lists the columns to change. A nil pointer leaves a column
// alone; the Set flags exist for columns that may be set to NULL.
type ConfigUpdate struct {
	Domain           *string
	LocationCode     *int
	LanguageCode     *string
	Devices          *string
	SerpDepth        *int
	IsActive         *bool
	ScheduleInterval *Interval

	SetLocationName bool
	LocationName    *string
	SetNextCheckAt  bool
	NextCheckAt     *time.Time
	SetSkipReason   bool
	LastSkipReason  *string
}
