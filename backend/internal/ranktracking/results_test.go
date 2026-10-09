package ranktracking

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fixture struct {
	s       *Service
	project string
	cfg     Config
	kw      map[string]string // keyword -> id
}

func day(m time.Month, d, h int) time.Time { return utc(2026, m, d, h, 0) }

// newFixture tracks k1, k2 and k3 and seeds four runs:
//
//	A  full, completed, Sep 1     k1 desktop 15, k2 desktop 30, k1 mobile 20, removed keywords at 10 and 11
//	B  full, completed, Oct 5     k1 desktop 8,  k2 desktop 3,  k1 mobile 12, k3 desktop 40, a removed keyword
//	C  subset, completed, Oct 8   k1 desktop 5, k3 desktop 35
//	E  subset, completed, Oct 2 10:00 (exactly the 7-day mark)  k2 desktop 25
//	D  full, failed, Oct 9 09:00  k1 desktop 1 (must never be read)
func newFixture(t *testing.T) fixture {
	t.Helper()
	s, project := newTestService(t, nil)
	ctx := context.Background()
	cfg := create(t, s, project, nil)
	if _, err := s.AddKeywords(ctx, project, cfg.ID, []string{"k1", "k2", "k3"}, false); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListKeywords(ctx, project, cfg.ID)
	kw := map[string]string{}
	for _, k := range list {
		kw[k.Keyword] = k.ID
	}
	pool := s.Repo.(Store).DB
	run := func(id, status string, subset bool, started time.Time, completed *time.Time, errMsg *string) {
		t.Helper()
		_, err := pool.Exec(ctx, `INSERT INTO go_rank_check_runs (id, config_id, project_id, status, is_subset_run, started_at, completed_at, error_message)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id+"-"+cfg.ID, cfg.ID, project, status, subset, started, completed, errMsg)
		if err != nil {
			t.Fatal(err)
		}
	}
	snap := func(runID, keyword, device string, pos int, at time.Time) {
		t.Helper()
		id, ok := kw[keyword]
		if !ok {
			id = "removed-" + keyword
		}
		_, err := pool.Exec(ctx, `INSERT INTO go_rank_snapshots (run_id, tracking_keyword_id, keyword, device, position, url, serp_features, checked_at)
			VALUES ($1, $2, $3, $4, $5, 'https://example.com/'||$3, '["video"]', $6)`, runID+"-"+cfg.ID, id, keyword, device, pos, at)
		if err != nil {
			t.Fatal(err)
		}
	}
	done := func(at time.Time) *time.Time { return &at }
	msg := "provider down"

	run("A", "completed", false, day(9, 1, 8), done(day(9, 1, 9)), nil)
	snap("A", "k1", "desktop", 15, day(9, 1, 8))
	snap("A", "k2", "desktop", 30, day(9, 1, 8))
	snap("A", "k1", "mobile", 20, day(9, 1, 8))
	snap("A", "gone2", "desktop", 10, day(9, 1, 8))
	snap("A", "gone3", "desktop", 11, day(9, 1, 8))
	run("B", "completed", false, day(10, 5, 8), done(day(10, 5, 9)), nil)
	snap("B", "k1", "desktop", 8, day(10, 5, 8))
	snap("B", "k2", "desktop", 3, day(10, 5, 8))
	snap("B", "k1", "mobile", 12, day(10, 5, 8))
	snap("B", "k3", "desktop", 40, day(10, 5, 8))
	snap("B", "gone", "desktop", 2, day(10, 5, 8))
	run("C", "completed", true, day(10, 8, 0), done(day(10, 8, 1)), nil)
	snap("C", "k1", "desktop", 5, day(10, 8, 0))
	snap("C", "k3", "desktop", 35, day(10, 8, 0))
	run("E", "completed", true, day(10, 2, 10), done(day(10, 2, 10)), nil)
	snap("E", "k2", "desktop", 25, day(10, 2, 10))
	run("D", "failed", false, day(10, 9, 9), done(day(10, 9, 9)), &msg)
	snap("D", "k1", "desktop", 1, day(10, 9, 9))
	return fixture{s: s, project: project, cfg: cfg, kw: kw}
}

func pos(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

func TestLatestResults(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	got, err := f.s.LatestResults(ctx, f.project, f.cfg.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 3 {
		t.Fatalf("rows = %d, want the 3 tracked keywords (history of a removed keyword stays out)", len(got.Rows))
	}
	byKeyword := map[string]ResultRow{}
	for _, r := range got.Rows {
		byKeyword[r.Keyword] = r
	}
	k1, k2, k3 := byKeyword["k1"], byKeyword["k2"], byKeyword["k3"]
	// Newest completed snapshot wins, subset runs included, failed runs ignored.
	if pos(k1.Desktop.Position) != 5 || pos(k1.Desktop.PreviousPosition) != 15 {
		t.Errorf("k1 desktop = %d (was %d), want 5 (was 15)", pos(k1.Desktop.Position), pos(k1.Desktop.PreviousPosition))
	}
	if pos(k1.Mobile.Position) != 12 || pos(k1.Mobile.PreviousPosition) != 20 {
		t.Errorf("k1 mobile = %d (was %d), want 12 (was 20)", pos(k1.Mobile.Position), pos(k1.Mobile.PreviousPosition))
	}
	// A snapshot taken exactly at the cut-off counts as the comparison.
	if pos(k2.Desktop.Position) != 3 || pos(k2.Desktop.PreviousPosition) != 25 {
		t.Errorf("k2 desktop = %d (was %d), want 3 (was 25)", pos(k2.Desktop.Position), pos(k2.Desktop.PreviousPosition))
	}
	if k2.Mobile.Position != nil || k2.Mobile.PreviousPosition != nil || k2.Mobile.SerpFeatures == nil {
		t.Errorf("k2 mobile = %+v, want an empty result with an empty (non-nil) feature list", k2.Mobile)
	}
	// k3 has nothing older than the 7-day mark, so its first snapshot is the baseline.
	if pos(k3.Desktop.Position) != 35 || pos(k3.Desktop.PreviousPosition) != 40 {
		t.Errorf("k3 desktop = %d (was %d), want 35 with the first snapshot (40) as baseline", pos(k3.Desktop.Position), pos(k3.Desktop.PreviousPosition))
	}
	if k1.Desktop.RankingURL == nil || *k1.Desktop.RankingURL != "https://example.com/k1" || len(k1.Desktop.SerpFeatures) != 1 {
		t.Errorf("k1 desktop url/features = %v / %v", k1.Desktop.RankingURL, k1.Desktop.SerpFeatures)
	}
	// The newest run failed, but freshness comes from the newest snapshot of any completed run.
	if got.Run == nil || got.Run.Status != "failed" || got.Run.ErrorMessage == nil || *got.Run.ErrorMessage != "provider down" {
		t.Fatalf("run = %+v, want the failed run", got.Run)
	}
	if got.Run.LastCheckedAt == nil || !got.Run.LastCheckedAt.Equal(day(10, 8, 0)) {
		t.Errorf("LastCheckedAt = %v, want Oct 8 (the newest completed snapshot), not the failed run's", got.Run.LastCheckedAt)
	}

	// A one-day window compares with the newest snapshot at or before yesterday.
	day1, err := f.s.LatestResults(ctx, f.project, f.cfg.ID, "1d")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range day1.Rows {
		if r.Keyword == "k1" && pos(r.Desktop.PreviousPosition) != 5 {
			t.Errorf("1d k1 desktop previous = %d, want 5", pos(r.Desktop.PreviousPosition))
		}
		if r.Keyword == "k2" && pos(r.Desktop.PreviousPosition) != 3 {
			t.Errorf("1d k2 desktop previous = %d, want 3", pos(r.Desktop.PreviousPosition))
		}
	}
}

func TestLatestResultsWithoutRuns(t *testing.T) {
	s, project := newTestService(t, nil)
	cfg := create(t, s, project, nil)
	got, err := s.LatestResults(context.Background(), project, cfg.ID, "30d")
	if err != nil || got.Run != nil || got.Rows == nil || len(got.Rows) != 0 {
		t.Fatalf("results = %+v, %v; want no run and an empty (non-nil) row list", got, err)
	}
}

func TestHistoryTrendAndMatrix(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	history, err := f.s.KeywordHistory(ctx, f.project, f.cfg.ID, f.kw["k1"], 365)
	if err != nil || len(history) != 5 {
		t.Fatalf("history = %d points, %v; want 5 (failed run excluded)", len(history), err)
	}
	for i := 1; i < len(history); i++ {
		if history[i].CheckedAt.Before(history[i-1].CheckedAt) {
			t.Errorf("history not oldest first: %v", history)
		}
	}
	if recent, _ := f.s.KeywordHistory(ctx, f.project, f.cfg.ID, f.kw["k1"], 10); len(recent) != 3 {
		t.Errorf("10-day history = %d points, want 3 (Oct 5 desktop+mobile, Oct 8)", len(recent))
	}

	trend, err := f.s.ConfigTrend(ctx, f.project, f.cfg.ID, "desktop", 365)
	if err != nil || len(trend) != 2 {
		t.Fatalf("trend = %+v, %v; want runs A and B only (subset and failed excluded)", trend, err)
	}
	if a := trend[0]; a.Total != 4 || a.Top3 != 0 || a.Top4to10 != 1 || a.Top11to20 != 2 {
		t.Errorf("run A buckets = %+v", a)
	}
	if b := trend[1]; b.Total != 4 || b.Top3 != 2 || b.Top4to10 != 1 || b.Top11to20 != 0 {
		t.Errorf("run B buckets = %+v, want 4 snapshots, the removed keyword included", b)
	}
	if onlyB, _ := f.s.ConfigTrend(ctx, f.project, f.cfg.ID, "desktop", 10); len(onlyB) != 1 {
		t.Errorf("10-day trend = %d runs, want 1", len(onlyB))
	}

	if m, err := f.s.PositionMatrix(ctx, f.project, f.cfg.ID, "desktop", 1); err != nil || len(m) != 4 {
		t.Errorf("1-run matrix = %d points, %v; want run B's 4", len(m), err)
	}
	if m, _ := f.s.PositionMatrix(ctx, f.project, f.cfg.ID, "desktop", 2); len(m) != 8 {
		t.Errorf("2-run matrix = %d points, want 8 (A's 4 + B's 4)", len(m))
	}
	if m, _ := f.s.PositionMatrix(ctx, f.project, f.cfg.ID, "mobile", 12); len(m) != 2 {
		t.Errorf("mobile matrix = %d points, want 2", len(m))
	}
}

func TestResultsValidationAndScope(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	checks := map[string]error{
		"bad period":     second(f.s.LatestResults(ctx, f.project, f.cfg.ID, "2d")),
		"zero days":      second(f.s.KeywordHistory(ctx, f.project, f.cfg.ID, "k", 0)),
		"too many days":  second(f.s.ConfigTrend(ctx, f.project, f.cfg.ID, "desktop", MaxHistoryDays+1)),
		"bad device":     second(f.s.ConfigTrend(ctx, f.project, f.cfg.ID, "tablet", 30)),
		"matrix device":  second(f.s.PositionMatrix(ctx, f.project, f.cfg.ID, "", 5)),
		"matrix limit":   second(f.s.PositionMatrix(ctx, f.project, f.cfg.ID, "desktop", MaxMatrixRuns+1)),
		"matrix limit 0": second(f.s.PositionMatrix(ctx, f.project, f.cfg.ID, "desktop", 0)),
	}
	for name, err := range checks {
		if !isValidation(err) {
			t.Errorf("%s: error = %v, want ValidationError", name, err)
		}
	}
	other := "another-project"
	notFound := map[string]error{
		"results": second(f.s.LatestResults(ctx, other, f.cfg.ID, "7d")),
		"run":     second(f.s.LatestRun(ctx, other, f.cfg.ID)),
		"history": second(f.s.KeywordHistory(ctx, other, f.cfg.ID, f.kw["k1"], 30)),
		"trend":   second(f.s.ConfigTrend(ctx, other, f.cfg.ID, "desktop", 30)),
		"matrix":  second(f.s.PositionMatrix(ctx, other, f.cfg.ID, "desktop", 5)),
	}
	for name, err := range notFound {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s from another project: error = %v, want ErrNotFound", name, err)
		}
	}
	run, err := f.s.LatestRun(ctx, f.project, f.cfg.ID)
	if err != nil || run == nil || run.Status != "failed" {
		t.Errorf("LatestRun() = %+v, %v; want the failed run D", run, err)
	}
	active, err := f.s.Results.ActiveRun(ctx, f.cfg.ID)
	if err != nil || active != nil {
		t.Errorf("ActiveRun() = %+v, %v; want none", active, err)
	}
}

func second[T any](_ T, err error) error { return err }

func TestOneActiveRunPerConfig(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pool := f.s.Repo.(Store).DB
	insert := func(id string) error {
		_, err := pool.Exec(ctx, `INSERT INTO go_rank_check_runs (id, config_id, project_id, status) VALUES ($1, $2, $3, 'pending')`, id, f.cfg.ID, f.project)
		return err
	}
	if err := insert("active-1-" + f.cfg.ID); err != nil {
		t.Fatal(err)
	}
	if err := insert("active-2-" + f.cfg.ID); err == nil {
		t.Fatal("a second pending run was accepted, want the unique index to refuse it")
	}
	active, err := f.s.Results.ActiveRun(ctx, f.cfg.ID)
	if err != nil || active == nil || active.Status != "pending" {
		t.Errorf("ActiveRun() = %+v, %v; want the pending run", active, err)
	}
}
