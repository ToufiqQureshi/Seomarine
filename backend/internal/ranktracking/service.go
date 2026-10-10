package ranktracking

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

// Repository is the storage the service needs; Store implements it.
type Repository interface {
	ListActiveConfigs(ctx context.Context, projectID string) ([]Config, error)
	GetConfig(ctx context.Context, projectID, id string) (Config, error)
	FindConfig(ctx context.Context, projectID, domain string, locationCode int, locationName *string) (Config, error)
	InsertConfig(ctx context.Context, c Config) error
	UpdateConfig(ctx context.Context, projectID, id string, u ConfigUpdate) error
	Keywords(ctx context.Context, configID string) ([]Keyword, error)
	AddKeywords(ctx context.Context, configID string, keywords []NewKeyword, maxKeywords int) ([]string, error)
	RemoveKeywords(ctx context.Context, configID string, ids []string) ([]string, error)
	UpdateKeywordMetrics(ctx context.Context, configID string, updates []KeywordMetricUpdate) (int, error)
}

// KeywordMetricProvider fetches already tracked terms from DataForSEO.
type KeywordMetricProvider interface {
	RankTrackingMetrics(ctx context.Context, organizationID string, keywords []string, locationCode int, languageCode string, locationName *string) ([]KeywordMetric, error)
}

// LocationChecker confirms the provider accepts a city name for a market.
// An unusable name would be billed as a failed task, so it is checked before
// a config is saved.
type LocationChecker interface {
	Check(ctx context.Context, organizationID, locationName, languageCode string, locationCode int) error
}

// NameRegistry lists the canonical city and region names of a country.
type NameRegistry interface {
	LocationNames(ctx context.Context, organizationID, country string) ([]string, error)
}

// RegistryChecker accepts a city name only if it is in the country's registry,
// the list the location picker searches. A name outside it would be billed as a
// failed task on every run. If the registry cannot be read the check fails
// closed, because an unverified name may be one the provider rejects.
type RegistryChecker struct{ Registry NameRegistry }

// Check implements LocationChecker.
func (c RegistryChecker) Check(ctx context.Context, organizationID, locationName, _ string, locationCode int) error {
	names, err := c.Registry.LocationNames(ctx, organizationID, market.ISOCode(locationCode))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrLocationUnavailable, err)
	}
	if slices.Contains(names, locationName) {
		return nil
	}
	return ValidationError(fmt.Sprintf("%q is not a Google location name. Search the locations and pass the returned name exactly.", locationName))
}

// Service holds the rank-tracking rules for configs and keywords.
type Service struct {
	Repo      Repository
	Results   ResultsRepository
	Locations LocationChecker // nil means city-level configs are refused
	Schedule  Scheduler
	Metrics   KeywordMetricProvider
	Plans     PaidPlans // nil allows metric refresh on self-hosted installs
}

// NewService builds a Service that schedules with the real clock.
func NewService(repo Repository, results ResultsRepository, locations LocationChecker) *Service {
	return &Service{Repo: repo, Results: results, Locations: locations, Schedule: Scheduler{
		Now:  time.Now,
		Rand: CryptoRand,
	}}
}

// RefreshKeywordMetrics re-fetches volume, difficulty and CPC for one config.
// Lowercased terms are sent once because DataForSEO is case-insensitive and
// echoes normalized terms; returned values are applied to every matching
// case-sensitive keyword row.
func (s *Service) RefreshKeywordMetrics(ctx context.Context, organizationID, projectID, configID string) (int, error) {
	if s.Metrics == nil {
		return 0, ErrMetricsUnavailable
	}
	config, err := s.Repo.GetConfig(ctx, projectID, configID)
	if err != nil {
		return 0, err
	}
	if s.Plans != nil {
		paid, err := s.Plans.HasPaidPlan(ctx, organizationID)
		if err != nil {
			return 0, fmt.Errorf("check rank tracking plan: %w", err)
		}
		if !paid {
			return 0, ErrPaymentRequired
		}
	}
	keywords, err := s.Repo.Keywords(ctx, config.ID)
	if err != nil {
		return 0, err
	}
	if len(keywords) == 0 {
		return 0, nil
	}
	unique, seen := make([]string, 0, len(keywords)), make(map[string]bool, len(keywords))
	for _, keyword := range keywords {
		lower := strings.ToLower(keyword.Keyword)
		if !seen[lower] {
			seen[lower] = true
			unique = append(unique, lower)
		}
	}
	metrics, err := s.Metrics.RankTrackingMetrics(ctx, organizationID, unique, config.LocationCode, config.LanguageCode, config.LocationName)
	if err != nil {
		return 0, fmt.Errorf("fetch rank tracking keyword metrics: %w", err)
	}
	byKeyword := make(map[string]KeywordMetric, len(metrics))
	for _, metric := range metrics {
		key := strings.ToLower(metric.Keyword)
		if key == "" || !seen[key] || invalidKeywordMetric(metric) {
			return 0, errors.New("DataForSEO returned invalid rank tracking metrics")
		}
		byKeyword[key] = metric
	}
	updates := make([]KeywordMetricUpdate, 0, len(keywords))
	fetchedAt := time.Now().UTC()
	for _, keyword := range keywords {
		metric, ok := byKeyword[strings.ToLower(keyword.Keyword)]
		if !ok {
			continue
		}
		updates = append(updates, KeywordMetricUpdate{
			ID: keyword.ID, SearchVolume: metric.SearchVolume, KeywordDifficulty: metric.KeywordDifficulty,
			CPC: metric.CPC, FetchedAt: fetchedAt,
		})
	}
	if len(updates) == 0 {
		return 0, nil
	}
	updated, err := s.Repo.UpdateKeywordMetrics(ctx, config.ID, updates)
	if err != nil {
		return 0, err
	}
	return updated, nil
}

func invalidKeywordMetric(metric KeywordMetric) bool {
	if metric.SearchVolume != nil && *metric.SearchVolume < 0 {
		return true
	}
	if metric.KeywordDifficulty != nil && (*metric.KeywordDifficulty < 0 || *metric.KeywordDifficulty > 100) {
		return true
	}
	return metric.CPC != nil && (math.IsNaN(*metric.CPC) || math.IsInf(*metric.CPC, 0) || *metric.CPC < 0)
}

// CreateInput is a request to track a domain.
type CreateInput struct {
	OrganizationID   string // billed for the city check
	ProjectID        string
	ProjectMarket    market.Pair
	Domain           string
	LocationCode     int    // 0 uses the project's
	LanguageCode     string // "" uses the market's
	LocationName     *string
	Devices          string // "" means both
	SerpDepth        int
	ScheduleInterval Interval // "" means weekly
	ScheduleTime     *ScheduleTime
}

// UpdateInput changes a config. Nil fields stay as they are.
type UpdateInput struct {
	OrganizationID   string // billed for the city check
	Domain           *string
	LocationCode     *int
	LanguageCode     *string
	SetLocationName  bool
	LocationName     *string
	Devices          *string
	SerpDepth        *int
	ScheduleInterval *Interval
	ScheduleTime     *ScheduleTime
	IsActive         *bool
}

// ListConfigs returns the project's active configs.
func (s *Service) ListConfigs(ctx context.Context, projectID string) ([]Config, error) {
	return s.Repo.ListActiveConfigs(ctx, projectID)
}

// CreateConfig starts tracking a domain. Re-adding an archived domain brings
// the old row back with its keywords and history, using the new settings.
func (s *Service) CreateConfig(ctx context.Context, in CreateInput) (Config, error) {
	domain, err := ParseDomain(in.Domain)
	if err != nil {
		return Config{}, ValidationError("Enter a valid domain like example.com")
	}
	devices := orDefault(in.Devices, "both")
	if !validDevices(devices) || !validDepth(in.SerpDepth) {
		return Config{}, ValidationError("Devices or SERP depth is not valid")
	}
	pair := market.Resolve(market.Pair{LocationCode: in.LocationCode, LanguageCode: in.LanguageCode}, in.ProjectMarket)
	if _, ok := market.Lookup(pair.LocationCode); !ok || !market.IsSupportedLanguageCode(pair.LanguageCode) {
		return Config{}, ValidationError("Location or language is not supported")
	}
	interval := Interval(orDefault(string(in.ScheduleInterval), string(Weekly)))
	next, err := s.Schedule.ResolveNext(interval, in.ScheduleTime)
	if err != nil {
		return Config{}, ValidationError(capitalize(err.Error()))
	}
	// Before the duplicate and limit checks, so an unusable city is the error
	// the caller sees.
	if err := s.checkLocation(ctx, in.OrganizationID, in.LocationName, pair); err != nil {
		return Config{}, err
	}

	existing, err := s.Repo.FindConfig(ctx, in.ProjectID, domain, pair.LocationCode, in.LocationName)
	found := err == nil
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Config{}, err
	}
	if found && existing.IsActive {
		return Config{}, fmt.Errorf("%w: this domain and location combination is already being tracked", ErrDuplicate)
	}
	// Counted for reactivations too, or archiving and re-adding would walk a
	// project past the cap.
	active, err := s.Repo.ListActiveConfigs(ctx, in.ProjectID)
	if err != nil {
		return Config{}, err
	}
	if len(active) >= MaxConfigsPerProject {
		return Config{}, fmt.Errorf("%w: maximum %d tracked domains per project", ErrLimit, MaxConfigsPerProject)
	}

	if found {
		active, noReason := true, (*string)(nil)
		err := s.Repo.UpdateConfig(ctx, in.ProjectID, existing.ID, ConfigUpdate{
			IsActive: &active, LanguageCode: &pair.LanguageCode, Devices: &devices, SerpDepth: &in.SerpDepth,
			ScheduleInterval: &interval, SetNextCheckAt: true, NextCheckAt: next,
			// An old skip reason would show a stale warning on the re-added domain.
			SetSkipReason: true, LastSkipReason: noReason,
		})
		if err != nil {
			return Config{}, err
		}
		return s.Repo.GetConfig(ctx, in.ProjectID, existing.ID)
	}
	id, err := ids.New()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		ID: id, ProjectID: in.ProjectID, Domain: domain, LocationCode: pair.LocationCode,
		LanguageCode: pair.LanguageCode, LocationName: in.LocationName, Devices: devices,
		SerpDepth: in.SerpDepth, ScheduleInterval: interval, IsActive: true, NextCheckAt: next,
	}
	if err := s.Repo.InsertConfig(ctx, cfg); err != nil {
		return Config{}, err
	}
	return s.Repo.GetConfig(ctx, in.ProjectID, id)
}

// UpdateConfig edits a config. The run-time anchor moves only when the
// schedule really changes, so saving other settings keeps the run time.
func (s *Service) UpdateConfig(ctx context.Context, projectID, configID string, in UpdateInput) (Config, error) {
	cfg, err := s.Repo.GetConfig(ctx, projectID, configID)
	if err != nil {
		return Config{}, err
	}
	var u ConfigUpdate
	if in.Devices != nil {
		if !validDevices(*in.Devices) {
			return Config{}, ValidationError("Devices is not valid")
		}
		u.Devices = in.Devices
	}
	if in.SerpDepth != nil {
		if !validDepth(*in.SerpDepth) {
			return Config{}, ValidationError("SERP depth must be 10 to 100 in steps of 10")
		}
		u.SerpDepth = in.SerpDepth
	}
	if in.Domain != nil {
		d, err := ParseDomain(*in.Domain)
		if err != nil {
			return Config{}, ValidationError("Enter a valid domain like example.com")
		}
		u.Domain = &d
	}
	if in.LocationCode != nil {
		if _, ok := market.Lookup(*in.LocationCode); !ok {
			return Config{}, ValidationError("Location is not supported")
		}
		u.LocationCode = in.LocationCode
	}
	if in.LanguageCode != nil {
		if !market.IsSupportedLanguageCode(*in.LanguageCode) {
			return Config{}, ValidationError("Language is not supported")
		}
		u.LanguageCode = in.LanguageCode
	}
	if in.SetLocationName {
		u.SetLocationName, u.LocationName = true, in.LocationName
	}
	// A city name only works with its market, so re-check the resulting
	// (name, language, country) whenever any of the three changes.
	if in.SetLocationName || in.LocationCode != nil || in.LanguageCode != nil {
		name := cfg.LocationName
		if in.SetLocationName {
			name = in.LocationName
		}
		pair := market.Pair{LocationCode: cfg.LocationCode, LanguageCode: cfg.LanguageCode}
		if in.LocationCode != nil {
			pair.LocationCode = *in.LocationCode
		}
		if in.LanguageCode != nil {
			pair.LanguageCode = *in.LanguageCode
		}
		if err := s.checkLocation(ctx, in.OrganizationID, name, pair); err != nil {
			return Config{}, err
		}
	}
	if in.IsActive != nil {
		u.IsActive = in.IsActive
	}

	interval := cfg.ScheduleInterval
	if in.ScheduleInterval != nil {
		interval = *in.ScheduleInterval
	}
	if in.ScheduleTime != nil || interval != cfg.ScheduleInterval || (interval != Manual && cfg.NextCheckAt == nil) {
		next, err := s.Schedule.ResolveNext(interval, in.ScheduleTime)
		if err != nil {
			return Config{}, ValidationError(capitalize(err.Error()))
		}
		u.ScheduleInterval, u.SetNextCheckAt, u.NextCheckAt = &interval, true, next
	}
	if err := s.Repo.UpdateConfig(ctx, projectID, configID, u); err != nil {
		return Config{}, err
	}
	return s.Repo.GetConfig(ctx, projectID, configID)
}

// AddResult reports which keywords were stored.
type AddResult struct {
	Added    int      `json:"added"`
	AddedIDs []string `json:"addedIds"`
	TooLong  []string `json:"tooLong"`
}

// AddKeywords stores new keywords, skipping blanks and keywords already
// tracked, and stopping at the per-config cap.
func (s *Service) AddKeywords(ctx context.Context, projectID, configID string, raw []string, matchCase bool) (AddResult, error) {
	if _, err := s.Repo.GetConfig(ctx, projectID, configID); err != nil {
		return AddResult{}, err
	}
	existing, err := s.Repo.Keywords(ctx, configID)
	if err != nil {
		return AddResult{}, err
	}
	if len(existing) >= MaxKeywordsPerConfig {
		return AddResult{}, fmt.Errorf("%w: maximum %d keywords per domain, currently tracking %d", ErrLimit, MaxKeywordsPerConfig, len(existing))
	}
	stored := make([]string, len(existing))
	for i, k := range existing {
		stored[i] = k.Keyword
	}
	fresh, tooLong := PrepareKeywords(raw, stored, matchCase)
	rows := make([]NewKeyword, len(fresh))
	for i, kw := range fresh {
		id, err := ids.New()
		if err != nil {
			return AddResult{}, err
		}
		rows[i] = NewKeyword{ID: id, Keyword: kw, MatchCase: matchCase}
	}
	result := AddResult{AddedIDs: []string{}, TooLong: tooLong}
	if tooLong == nil {
		result.TooLong = []string{}
	}
	if len(rows) == 0 {
		return result, nil
	}
	added, err := s.Repo.AddKeywords(ctx, configID, rows, MaxKeywordsPerConfig)
	if err != nil {
		return AddResult{}, err
	}
	result.Added, result.AddedIDs = len(added), added
	return result, nil
}

// RemoveKeywords deletes keywords from a config and reports the ids removed.
func (s *Service) RemoveKeywords(ctx context.Context, projectID, configID string, ids []string) ([]string, error) {
	if _, err := s.Repo.GetConfig(ctx, projectID, configID); err != nil {
		return nil, err
	}
	unique := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; !dup {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	removed, err := s.Repo.RemoveKeywords(ctx, configID, unique)
	if err != nil {
		return nil, err
	}
	if removed == nil {
		removed = []string{}
	}
	return removed, nil
}

// ListKeywords returns a config's keywords.
func (s *Service) ListKeywords(ctx context.Context, projectID, configID string) ([]Keyword, error) {
	if _, err := s.Repo.GetConfig(ctx, projectID, configID); err != nil {
		return nil, err
	}
	return s.Repo.Keywords(ctx, configID)
}

// CostEstimate prices a manual check, plus the recurring cost if scheduled.
type CostEstimate struct {
	Estimate
	KeywordCount           int                `json:"keywordCount"`
	DevicesCount           int                `json:"devicesCount"`
	TotalChecks            int                `json:"totalChecks"`
	Method                 Method             `json:"method"`
	ExistingKeywordCount   int                `json:"existingKeywordCount"`
	AdditionalKeywordCount int                `json:"additionalKeywordCount"`
	ScheduledEstimate      *ScheduledEstimate `json:"scheduledEstimate,omitempty"`
}

// EstimateCost prices checking the config's keywords, plus any extra ones the
// caller is thinking of adding.
func (s *Service) EstimateCost(ctx context.Context, projectID, configID string, additional []string) (CostEstimate, error) {
	cfg, err := s.Repo.GetConfig(ctx, projectID, configID)
	if err != nil {
		return CostEstimate{}, err
	}
	existing, err := s.Repo.Keywords(ctx, configID)
	if err != nil {
		return CostEstimate{}, err
	}
	keywords := make([]string, 0, len(existing)+len(additional))
	for _, k := range existing {
		keywords = append(keywords, k.Keyword)
	}
	keywords = append(keywords, additional...)
	keywords = keywords[:min(len(keywords), max(len(existing), MaxKeywordsPerConfig))]

	devices := DevicesCount(cfg.Devices)
	out := CostEstimate{
		Estimate:               EstimateCheck(keywords, cfg.Devices, cfg.SerpDepth, Live),
		KeywordCount:           len(keywords),
		DevicesCount:           devices,
		TotalChecks:            len(keywords) * devices,
		Method:                 Live,
		ExistingKeywordCount:   len(existing),
		AdditionalKeywordCount: len(keywords) - len(existing),
	}
	if cfg.ScheduleInterval != Manual {
		sched := EstimateScheduled(keywords, cfg.Devices, cfg.SerpDepth, cfg.ScheduleInterval)
		out.ScheduledEstimate = &sched
	}
	return out, nil
}

func (s *Service) checkLocation(ctx context.Context, organizationID string, name *string, pair market.Pair) error {
	if name == nil {
		return nil
	}
	if *name == "" || len(*name) > 200 {
		return ValidationError("Location name must be 1 to 200 characters")
	}
	if s.Locations == nil {
		return ErrLocationUnavailable
	}
	if err := s.Locations.Check(ctx, organizationID, *name, pair.LanguageCode, pair.LocationCode); err != nil {
		return fmt.Errorf("check location %q: %w", *name, err)
	}
	return nil
}

func validDevices(d string) bool { return d == "both" || d == "desktop" || d == "mobile" }

func validDepth(d int) bool { return d >= 10 && d <= 100 && d%10 == 0 }

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
