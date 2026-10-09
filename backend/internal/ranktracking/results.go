package ranktracking

import (
	"context"
	"fmt"
	"time"
)

// Run is one check execution, manual or scheduled.
type Run struct {
	ID              string     `json:"id"`
	ConfigID        string     `json:"configId"`
	ProjectID       string     `json:"projectId"`
	Status          string     `json:"status"`
	KeywordsTotal   int        `json:"keywordsTotal"`
	KeywordsChecked int        `json:"keywordsChecked"`
	IsSubsetRun     bool       `json:"isSubsetRun"`
	ErrorMessage    *string    `json:"errorMessage"`
	StartedAt       time.Time  `json:"startedAt"`
	CompletedAt     *time.Time `json:"completedAt"`
}

// Snapshot is the position of one keyword on one device in one run.
// A nil Position means the keyword was checked and not found within the depth.
type Snapshot struct {
	ID                int64     `json:"id"`
	RunID             string    `json:"runId"`
	TrackingKeywordID string    `json:"trackingKeywordId"`
	Keyword           string    `json:"keyword"`
	Device            string    `json:"device"`
	Position          *int      `json:"position"`
	URL               *string   `json:"url"`
	SerpFeatures      []string  `json:"serpFeatures"`
	CheckedAt         time.Time `json:"checkedAt"`
}

// DeviceResult is the latest position on one device and the one it is compared with.
type DeviceResult struct {
	Position         *int     `json:"position"`
	PreviousPosition *int     `json:"previousPosition"`
	RankingURL       *string  `json:"rankingUrl"`
	SerpFeatures     []string `json:"serpFeatures"`
}

// ResultRow is one tracked keyword with its latest positions.
type ResultRow struct {
	TrackingKeywordID string       `json:"trackingKeywordId"`
	Keyword           string       `json:"keyword"`
	MatchCase         bool         `json:"matchCase"`
	SearchVolume      *int         `json:"searchVolume"`
	KeywordDifficulty *int         `json:"keywordDifficulty"`
	CPC               *float64     `json:"cpc"`
	Desktop           DeviceResult `json:"desktop"`
	Mobile            DeviceResult `json:"mobile"`
}

// RunSummary is the latest run as the results page shows it.
type RunSummary struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	// LastCheckedAt comes from the newest snapshot, whichever run wrote it, so
	// a newer failed run does not erase the date of the results on screen. A
	// run that saved no snapshots leaves it nil; read CompletedAt for that run.
	LastCheckedAt *time.Time `json:"lastCheckedAt"`
	CompletedAt   *time.Time `json:"completedAt"`
	ErrorMessage  *string    `json:"errorMessage"`
}

// LatestResults is the keyword table of a config.
type LatestResults struct {
	Rows []ResultRow `json:"rows"`
	Run  *RunSummary `json:"run"`
}

// HistoryPoint is one position in a keyword's history.
type HistoryPoint struct {
	Device    string    `json:"device"`
	CheckedAt time.Time `json:"checkedAt"`
	Position  *int      `json:"position"`
}

// TrendPoint counts keywords per position bucket in one run. Keywords past
// position 20, or not found, are Total minus the three buckets.
type TrendPoint struct {
	RunID     string    `json:"runId"`
	CheckedAt time.Time `json:"checkedAt"`
	Total     int       `json:"total"`
	Top3      int       `json:"top3"`
	Top4to10  int       `json:"top4to10"`
	Top11to20 int       `json:"top11to20"`
}

// MatrixPoint is one keyword position in one of the recent runs.
type MatrixPoint struct {
	RunID             string    `json:"runId"`
	CheckedAt         time.Time `json:"checkedAt"`
	TrackingKeywordID string    `json:"trackingKeywordId"`
	Position          *int      `json:"position"`
}

// ResultsRepository is the read side of runs and snapshots; Store implements it.
type ResultsRepository interface {
	LatestRun(ctx context.Context, configID string) (*Run, error)
	ActiveRun(ctx context.Context, configID string) (*Run, error)
	Snapshots(ctx context.Context, q SnapshotQuery) ([]Snapshot, error)
	KeywordHistory(ctx context.Context, configID, keywordID string, since time.Time) ([]HistoryPoint, error)
	Trend(ctx context.Context, configID, device string, since time.Time) ([]TrendPoint, error)
	Matrix(ctx context.Context, configID, device string, runLimit int) ([]MatrixPoint, error)
}

// SnapshotQuery picks one snapshot per keyword and device from completed runs.
type SnapshotQuery struct {
	ConfigID   string
	Earliest   bool       // false picks the newest, true the oldest
	AtOrBefore *time.Time // only snapshots checked at or before this time
	KeywordIDs []string   // nil means every keyword
}

var comparePeriodDays = map[string]int{"1d": 1, "7d": 7, "30d": 30, "90d": 90}

// Limits on history requests, as in the legacy app.
const (
	DefaultHistoryDays = 365
	MaxHistoryDays     = 730
	DefaultMatrixRuns  = 12
	MaxMatrixRuns      = 26
)

func (s *Service) requireConfig(ctx context.Context, projectID, configID string) error {
	_, err := s.Repo.GetConfig(ctx, projectID, configID)
	return err
}

// LatestResults builds the keyword table with each keyword's latest position
// and the position it is compared with. The comparison is the newest snapshot
// at or before now minus the period; a keyword with none that old is compared
// with its very first snapshot, so a young config still shows movement.
func (s *Service) LatestResults(ctx context.Context, projectID, configID, period string) (LatestResults, error) {
	if period == "" {
		period = "7d"
	}
	days, ok := comparePeriodDays[period]
	if !ok {
		return LatestResults{}, ValidationError("Compare period must be 1d, 7d, 30d or 90d.")
	}
	if err := s.requireConfig(ctx, projectID, configID); err != nil {
		return LatestResults{}, err
	}
	keywords, err := s.Repo.Keywords(ctx, configID)
	if err != nil {
		return LatestResults{}, err
	}
	current, err := s.Results.Snapshots(ctx, SnapshotQuery{ConfigID: configID})
	if err != nil {
		return LatestResults{}, err
	}
	target := s.Schedule.Now().AddDate(0, 0, -days)
	comparison, err := s.Results.Snapshots(ctx, SnapshotQuery{ConfigID: configID, AtOrBefore: &target})
	if err != nil {
		return LatestResults{}, err
	}

	previous := make(map[string]*int, len(comparison))
	for _, snap := range comparison {
		previous[snap.TrackingKeywordID+":"+snap.Device] = snap.Position
	}
	var missing []string
	seen := map[string]bool{}
	for _, snap := range current {
		if _, has := previous[snap.TrackingKeywordID+":"+snap.Device]; !has && !seen[snap.TrackingKeywordID] {
			seen[snap.TrackingKeywordID] = true
			missing = append(missing, snap.TrackingKeywordID)
		}
	}
	if len(missing) > 0 {
		earliest, err := s.Results.Snapshots(ctx, SnapshotQuery{ConfigID: configID, Earliest: true, KeywordIDs: missing})
		if err != nil {
			return LatestResults{}, err
		}
		for _, snap := range earliest {
			key := snap.TrackingKeywordID + ":" + snap.Device
			if _, has := previous[key]; !has {
				previous[key] = snap.Position
			}
		}
	}

	rows := make([]ResultRow, len(keywords))
	index := make(map[string]int, len(keywords))
	for i, k := range keywords {
		index[k.ID] = i
		rows[i] = ResultRow{
			TrackingKeywordID: k.ID, Keyword: k.Keyword, MatchCase: k.MatchCase, SearchVolume: k.SearchVolume,
			KeywordDifficulty: k.KeywordDifficulty, CPC: k.CPC,
			Desktop: DeviceResult{PreviousPosition: previous[k.ID+":desktop"], SerpFeatures: []string{}},
			Mobile:  DeviceResult{PreviousPosition: previous[k.ID+":mobile"], SerpFeatures: []string{}},
		}
	}
	var newest *time.Time
	for _, snap := range current {
		i, tracked := index[snap.TrackingKeywordID]
		if !tracked {
			continue // history of a removed keyword stays out of the table
		}
		result := DeviceResult{
			Position: snap.Position, PreviousPosition: previous[snap.TrackingKeywordID+":"+snap.Device],
			RankingURL: snap.URL, SerpFeatures: snap.SerpFeatures,
		}
		if result.SerpFeatures == nil {
			result.SerpFeatures = []string{}
		}
		if snap.Device == "mobile" {
			rows[i].Mobile = result
		} else {
			rows[i].Desktop = result
		}
		if newest == nil || snap.CheckedAt.After(*newest) {
			t := snap.CheckedAt
			newest = &t
		}
	}

	out := LatestResults{Rows: rows}
	run, err := s.Results.LatestRun(ctx, configID)
	if err != nil {
		return LatestResults{}, err
	}
	if run != nil {
		out.Run = &RunSummary{ID: run.ID, Status: run.Status, LastCheckedAt: newest, CompletedAt: run.CompletedAt, ErrorMessage: run.ErrorMessage}
	}
	return out, nil
}

// LatestRun returns the newest run of a config, or nil when it never ran.
func (s *Service) LatestRun(ctx context.Context, projectID, configID string) (*Run, error) {
	if err := s.requireConfig(ctx, projectID, configID); err != nil {
		return nil, err
	}
	return s.Results.LatestRun(ctx, configID)
}

// KeywordHistory returns a keyword's positions from completed runs, oldest first.
func (s *Service) KeywordHistory(ctx context.Context, projectID, configID, keywordID string, sinceDays int) ([]HistoryPoint, error) {
	if err := validDays(sinceDays); err != nil {
		return nil, err
	}
	if err := s.requireConfig(ctx, projectID, configID); err != nil {
		return nil, err
	}
	points, err := s.Results.KeywordHistory(ctx, configID, keywordID, s.Schedule.Now().AddDate(0, 0, -sinceDays))
	if points == nil {
		points = []HistoryPoint{}
	}
	return points, err
}

// ConfigTrend returns the position distribution per full run on one device.
// Subset runs are left out because they cover only some keywords.
func (s *Service) ConfigTrend(ctx context.Context, projectID, configID, device string, sinceDays int) ([]TrendPoint, error) {
	if err := validDays(sinceDays); err != nil {
		return nil, err
	}
	if device != "desktop" && device != "mobile" {
		return nil, ValidationError("Device must be desktop or mobile.")
	}
	if err := s.requireConfig(ctx, projectID, configID); err != nil {
		return nil, err
	}
	points, err := s.Results.Trend(ctx, configID, device, s.Schedule.Now().AddDate(0, 0, -sinceDays))
	if points == nil {
		points = []TrendPoint{}
	}
	return points, err
}

// PositionMatrix returns positions of the last runLimit full runs on one device.
func (s *Service) PositionMatrix(ctx context.Context, projectID, configID, device string, runLimit int) ([]MatrixPoint, error) {
	if runLimit < 1 || runLimit > MaxMatrixRuns {
		return nil, ValidationError(fmt.Sprintf("Run limit must be 1 to %d.", MaxMatrixRuns))
	}
	if device != "desktop" && device != "mobile" {
		return nil, ValidationError("Device must be desktop or mobile.")
	}
	if err := s.requireConfig(ctx, projectID, configID); err != nil {
		return nil, err
	}
	points, err := s.Results.Matrix(ctx, configID, device, runLimit)
	if points == nil {
		points = []MatrixPoint{}
	}
	return points, err
}

func validDays(days int) error {
	if days < 1 || days > MaxHistoryDays {
		return ValidationError(fmt.Sprintf("Days must be 1 to %d.", MaxHistoryDays))
	}
	return nil
}
