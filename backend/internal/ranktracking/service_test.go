package ranktracking

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
)

type fakeChecker struct {
	err   error
	calls int
	orgs  []string
}

type fakeRankMetricProvider struct {
	keywords []string
	location int
	language string
	city     *string
	metrics  []KeywordMetric
	err      error
}

func (f *fakeRankMetricProvider) RankTrackingMetrics(_ context.Context, _ string, keywords []string, location int, language string, city *string) ([]KeywordMetric, error) {
	f.keywords, f.location, f.language, f.city = keywords, location, language, city
	return f.metrics, f.err
}

func (f *fakeChecker) Check(_ context.Context, org, _, _ string, _ int) error {
	f.calls++
	f.orgs = append(f.orgs, org)
	return f.err
}

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	ctx := context.Background()
	pool, err := pgdb.Open(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	schema, err := os.ReadFile("../database/testdata/legacy_schema.sql")
	if err != nil {
		t.Fatalf("read legacy schema: %v", err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// newTestService returns a service on a real database and a fresh project id
// whose rows are removed when the test ends.
func newTestService(t *testing.T, checker LocationChecker) (*Service, string) {
	t.Helper()
	pool := openPool(t)
	project, err := ids.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.WithoutCancel(context.Background()),
			`DELETE FROM go_rank_tracking_configs WHERE project_id = $1`, project); err != nil {
			t.Errorf("clean configs: %v", err)
		}
	})
	svc := NewService(Store{DB: pool}, Store{DB: pool}, checker)
	svc.Schedule = Scheduler{Now: func() time.Time { return testNow }, Rand: func(int) int { return 0 }}
	return svc, project
}

var usMarket = market.Pair{LocationCode: 2840, LanguageCode: "en"}

func create(t *testing.T, s *Service, project string, mutate func(*CreateInput)) Config {
	t.Helper()
	in := CreateInput{ProjectID: project, ProjectMarket: usMarket, Domain: "Example.com", SerpDepth: 10}
	if mutate != nil {
		mutate(&in)
	}
	cfg, err := s.CreateConfig(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateConfig() error = %v", err)
	}
	return cfg
}

func isValidation(err error) bool {
	_, ok := errors.AsType[ValidationError](err)
	return ok
}

func TestCreateConfigDefaultsAndValidation(t *testing.T) {
	s, project := newTestService(t, nil)
	ctx := context.Background()

	cfg := create(t, s, project, nil)
	if cfg.Domain != "example.com" || cfg.Devices != "both" || cfg.ScheduleInterval != Weekly ||
		cfg.LocationCode != 2840 || cfg.LanguageCode != "en" || !cfg.IsActive || cfg.NextCheckAt == nil {
		t.Errorf("defaults = %+v", cfg)
	}
	if !cfg.NextCheckAt.Equal(utc(2026, 10, 16, 4, 0)) {
		t.Errorf("NextCheckAt = %v, want the random-window default a week out", cfg.NextCheckAt)
	}

	manual := create(t, s, project, func(in *CreateInput) { in.Domain = "manual.example.com"; in.ScheduleInterval = Manual })
	if manual.NextCheckAt != nil {
		t.Errorf("manual NextCheckAt = %v, want nil", manual.NextCheckAt)
	}

	for name, mutate := range map[string]func(*CreateInput){
		"bad domain":                func(in *CreateInput) { in.Domain = "localhost" },
		"domain with port":          func(in *CreateInput) { in.Domain = "example.com:8080" },
		"depth not a multiple":      func(in *CreateInput) { in.SerpDepth = 15 },
		"depth too high":            func(in *CreateInput) { in.SerpDepth = 110 },
		"bad devices":               func(in *CreateInput) { in.Devices = "tablet" },
		"unknown language":          func(in *CreateInput) { in.LanguageCode = "xx" },
		"unknown location":          func(in *CreateInput) { in.LocationCode = 999999 },
		"weekly time without a day": func(in *CreateInput) { in.ScheduleTime = &ScheduleTime{Hour: 5} },
		"time on a manual schedule": func(in *CreateInput) { in.ScheduleInterval = Manual; in.ScheduleTime = &ScheduleTime{Hour: 5} },
		"unknown interval":          func(in *CreateInput) { in.ScheduleInterval = "hourly" },
	} {
		t.Run(name, func(t *testing.T) {
			in := CreateInput{ProjectID: project, ProjectMarket: usMarket, Domain: "case.example.com", SerpDepth: 10}
			mutate(&in)
			if _, err := s.CreateConfig(ctx, in); !isValidation(err) {
				t.Errorf("CreateConfig() error = %v, want ValidationError", err)
			}
		})
	}
}

func TestCreateConfigDuplicatesAndReactivation(t *testing.T) {
	s, project := newTestService(t, nil)
	ctx := context.Background()
	first := create(t, s, project, nil)

	if _, err := s.CreateConfig(ctx, CreateInput{ProjectID: project, ProjectMarket: usMarket, Domain: "https://www.example.com/page", SerpDepth: 10}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate error = %v, want ErrDuplicate", err)
	}
	other := create(t, s, project, func(in *CreateInput) { in.LocationCode = 2356; in.LanguageCode = "en" })
	if other.ID == first.ID {
		t.Fatal("same domain in another country must be its own config")
	}

	if _, err := s.AddKeywords(ctx, project, first.ID, []string{"seo tool"}, false); err != nil {
		t.Fatal(err)
	}
	reason := "plan_required"
	if err := s.Repo.UpdateConfig(ctx, project, first.ID, ConfigUpdate{SetSkipReason: true, LastSkipReason: &reason}); err != nil {
		t.Fatal(err)
	}
	archived := false
	if _, err := s.UpdateConfig(ctx, project, first.ID, UpdateInput{IsActive: &archived}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListConfigs(ctx, project)
	if err != nil || len(list) != 1 || list[0].ID != other.ID {
		t.Fatalf("active list after archive = %+v, %v; want only the other config", list, err)
	}

	back, err := s.CreateConfig(ctx, CreateInput{ProjectID: project, ProjectMarket: usMarket, Domain: "example.com",
		Devices: "mobile", SerpDepth: 50, ScheduleInterval: Daily})
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != first.ID || !back.IsActive || back.Devices != "mobile" || back.SerpDepth != 50 ||
		back.ScheduleInterval != Daily || back.LastSkipReason != nil {
		t.Errorf("reactivated = %+v, want the old row with new settings and no skip reason", back)
	}
	kws, err := s.ListKeywords(ctx, project, back.ID)
	if err != nil || len(kws) != 1 {
		t.Errorf("keywords after reactivation = %v, %v; want the old keyword kept", kws, err)
	}
}

func TestCreateConfigProjectLimit(t *testing.T) {
	s, project := newTestService(t, nil)
	pool := s.Repo.(Store).DB
	_, err := pool.Exec(context.Background(), `INSERT INTO go_rank_tracking_configs (id, project_id, domain, serp_depth, schedule_interval)
		SELECT 'limit-' || $1 || '-' || n, $1, 'd' || n || '.example.com', 10, 'manual' FROM generate_series(1, $2) n`, project, MaxConfigsPerProject)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateConfig(context.Background(), CreateInput{ProjectID: project, ProjectMarket: usMarket, Domain: "one-too-many.com", SerpDepth: 10})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("error = %v, want ErrLimit", err)
	}
}

func TestCityLocationNeedsAcceptedName(t *testing.T) {
	city := "Mumbai"
	t.Run("no checker refuses", func(t *testing.T) {
		s, project := newTestService(t, nil)
		_, err := s.CreateConfig(context.Background(), CreateInput{ProjectID: project, ProjectMarket: usMarket, Domain: "a.example.com", SerpDepth: 10, LocationName: &city})
		if !errors.Is(err, ErrLocationUnavailable) {
			t.Fatalf("error = %v, want ErrLocationUnavailable", err)
		}
	})
	t.Run("rejected name is not stored", func(t *testing.T) {
		checker := &fakeChecker{err: ValidationError("City not accepted")}
		s, project := newTestService(t, checker)
		_, err := s.CreateConfig(context.Background(), CreateInput{ProjectID: project, ProjectMarket: usMarket, Domain: "a.example.com", SerpDepth: 10, LocationName: &city})
		if !isValidation(err) {
			t.Fatalf("error = %v, want the checker's ValidationError", err)
		}
		if list, _ := s.ListConfigs(context.Background(), project); len(list) != 0 {
			t.Errorf("stored %d configs after a rejected city", len(list))
		}
	})
	t.Run("accepted name is stored and rechecked when the market changes", func(t *testing.T) {
		checker := &fakeChecker{}
		s, project := newTestService(t, checker)
		cfg := create(t, s, project, func(in *CreateInput) { in.LocationName = &city })
		if cfg.LocationName == nil || *cfg.LocationName != city || checker.calls != 1 {
			t.Fatalf("config = %+v, checks = %d", cfg, checker.calls)
		}
		hindi := "hi"
		if _, err := s.UpdateConfig(context.Background(), project, cfg.ID, UpdateInput{LanguageCode: &hindi}); err != nil {
			t.Fatal(err)
		}
		if checker.calls != 2 {
			t.Errorf("checks after a language change = %d, want 2", checker.calls)
		}
		devices := "mobile"
		if _, err := s.UpdateConfig(context.Background(), project, cfg.ID, UpdateInput{Devices: &devices}); err != nil || checker.calls != 2 {
			t.Errorf("a devices change must not recheck the city (err %v, checks %d)", err, checker.calls)
		}
	})
	t.Run("empty name is invalid", func(t *testing.T) {
		s, project := newTestService(t, &fakeChecker{})
		empty := ""
		_, err := s.CreateConfig(context.Background(), CreateInput{ProjectID: project, ProjectMarket: usMarket, Domain: "a.example.com", SerpDepth: 10, LocationName: &empty})
		if !isValidation(err) {
			t.Fatalf("error = %v, want ValidationError", err)
		}
	})
}

func TestUpdateConfigSchedule(t *testing.T) {
	s, project := newTestService(t, nil)
	ctx := context.Background()
	cfg := create(t, s, project, nil)
	anchor := *cfg.NextCheckAt

	devices := "desktop"
	got, err := s.UpdateConfig(ctx, project, cfg.ID, UpdateInput{Devices: &devices, ScheduleInterval: &cfg.ScheduleInterval})
	if err != nil {
		t.Fatal(err)
	}
	if got.Devices != "desktop" || !got.NextCheckAt.Equal(anchor) {
		t.Errorf("resending the same schedule moved the anchor: %v -> %v", anchor, got.NextCheckAt)
	}

	pick := &ScheduleTime{Weekday: ptr(1), Hour: 9}
	got, err = s.UpdateConfig(ctx, project, cfg.ID, UpdateInput{ScheduleTime: pick})
	if err != nil || !got.NextCheckAt.Equal(utc(2026, 10, 12, 9, 0)) {
		t.Errorf("chosen time -> %v, %v; want Monday 09:00 UTC", got.NextCheckAt, err)
	}

	daily := Daily
	got, err = s.UpdateConfig(ctx, project, cfg.ID, UpdateInput{ScheduleInterval: &daily})
	if err != nil || got.ScheduleInterval != Daily || got.NextCheckAt == nil || !got.NextCheckAt.Equal(utc(2026, 10, 10, 4, 0)) {
		t.Errorf("interval change -> %+v, %v; want a fresh daily anchor", got, err)
	}

	manual := Manual
	got, err = s.UpdateConfig(ctx, project, cfg.ID, UpdateInput{ScheduleInterval: &manual})
	if err != nil || got.NextCheckAt != nil {
		t.Errorf("manual -> %+v, %v; want no anchor", got, err)
	}
	if _, err := s.UpdateConfig(ctx, project, cfg.ID, UpdateInput{ScheduleTime: pick}); !isValidation(err) {
		t.Errorf("time on a manual schedule error = %v, want ValidationError", err)
	}
	weekly := Weekly
	got, err = s.UpdateConfig(ctx, project, cfg.ID, UpdateInput{ScheduleInterval: &weekly})
	if err != nil || got.NextCheckAt == nil {
		t.Errorf("manual to weekly -> %+v, %v; want an anchor", got, err)
	}
}

func TestUpdateConfigIsScopedToProject(t *testing.T) {
	s, project := newTestService(t, nil)
	cfg := create(t, s, project, nil)
	devices := "mobile"
	if _, err := s.UpdateConfig(context.Background(), "another-project", cfg.ID, UpdateInput{Devices: &devices}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update from another project error = %v, want ErrNotFound", err)
	}
	if got, _ := s.Repo.GetConfig(context.Background(), project, cfg.ID); got.Devices != "both" {
		t.Errorf("another project changed devices to %q", got.Devices)
	}
}

func TestUpdateConfigDomainCollision(t *testing.T) {
	s, project := newTestService(t, nil)
	create(t, s, project, nil)
	second := create(t, s, project, func(in *CreateInput) { in.Domain = "second.example.com" })
	clash := "example.com"
	if _, err := s.UpdateConfig(context.Background(), project, second.ID, UpdateInput{Domain: &clash}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("error = %v, want ErrDuplicate", err)
	}
}

func TestKeywords(t *testing.T) {
	s, project := newTestService(t, nil)
	ctx := context.Background()
	cfg := create(t, s, project, nil)
	other := create(t, s, project, func(in *CreateInput) { in.Domain = "other.example.com" })

	res, err := s.AddKeywords(ctx, project, cfg.ID, []string{" SEO Tool ", "seo tool", "", "Rank Tracker"}, false)
	if err != nil || res.Added != 2 {
		t.Fatalf("AddKeywords() = %+v, %v; want 2 added", res, err)
	}
	res, err = s.AddKeywords(ctx, project, cfg.ID, []string{"seo tool"}, false)
	if err != nil || res.Added != 0 || res.AddedIDs == nil || res.TooLong == nil {
		t.Errorf("re-adding = %+v, %v; want 0 added with empty (non-nil) lists", res, err)
	}
	res, err = s.AddKeywords(ctx, project, cfg.ID, []string{"Nodex", "nodex"}, true)
	if err != nil || res.Added != 2 {
		t.Errorf("match-case twins = %+v, %v; want both stored", res, err)
	}
	res, err = s.AddKeywords(ctx, project, cfg.ID, []string{strings.Repeat("a", MaxKeywordUnits+1)}, false)
	if err != nil || res.Added != 0 || len(res.TooLong) != 1 {
		t.Errorf("long keyword = %+v, %v; want it reported, not stored", res, err)
	}
	if _, err := s.AddKeywords(ctx, "another-project", cfg.ID, []string{"x"}, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("add from another project error = %v, want ErrNotFound", err)
	}

	mine, _ := s.ListKeywords(ctx, project, cfg.ID)
	theirs, err := s.AddKeywords(ctx, project, other.ID, []string{"theirs"}, false)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := s.RemoveKeywords(ctx, project, cfg.ID, []string{mine[0].ID, mine[0].ID, theirs.AddedIDs[0], "no-such-id"})
	if err != nil || len(removed) != 1 || removed[0] != mine[0].ID {
		t.Errorf("RemoveKeywords() = %v, %v; want only this config's keyword, once", removed, err)
	}
	if kws, _ := s.ListKeywords(ctx, project, other.ID); len(kws) != 1 {
		t.Errorf("another config lost keywords: %d left", len(kws))
	}
}

func TestRefreshKeywordMetricsDeduplicatesCaseAndGatesProviderCalls(t *testing.T) {
	s, project := newTestService(t, nil)
	ctx := context.Background()
	cfg := create(t, s, project, nil)
	if _, err := s.AddKeywords(ctx, project, cfg.ID, []string{"Nodex", "nodex"}, true); err != nil {
		t.Fatal(err)
	}
	volume, difficulty, cpc := 90, 12, 0.5
	provider := &fakeRankMetricProvider{metrics: []KeywordMetric{{Keyword: "nodex", SearchVolume: &volume, KeywordDifficulty: &difficulty, CPC: &cpc}}}
	s.Metrics, s.Plans = provider, fakePlans{paid: true}
	updated, err := s.RefreshKeywordMetrics(ctx, "org-test", project, cfg.ID)
	if err != nil || updated != 2 {
		t.Fatalf("RefreshKeywordMetrics() = %d, %v; want 2", updated, err)
	}
	if len(provider.keywords) != 1 || provider.keywords[0] != "nodex" || provider.location != cfg.LocationCode || provider.language != cfg.LanguageCode || provider.city != nil {
		t.Fatalf("provider request = %+v; want normalized terms and config market", provider)
	}
	got, err := s.ListKeywords(ctx, project, cfg.ID)
	if err != nil || len(got) != 2 {
		t.Fatalf("ListKeywords() = %d, %v", len(got), err)
	}
	for _, keyword := range got {
		if keyword.SearchVolume == nil || *keyword.SearchVolume != volume || keyword.KeywordDifficulty == nil || *keyword.KeywordDifficulty != difficulty || keyword.CPC == nil || *keyword.CPC != cpc || keyword.MetricsFetchedAt == nil {
			t.Errorf("updated keyword metrics = %+v", keyword)
		}
	}
	s.Plans = fakePlans{paid: false}
	if _, err := s.RefreshKeywordMetrics(ctx, "org-test", project, cfg.ID); !errors.Is(err, ErrPaymentRequired) {
		t.Fatalf("unpaid RefreshKeywordMetrics() error = %v; want ErrPaymentRequired", err)
	}
	if len(provider.keywords) != 1 {
		t.Errorf("provider called before paid-plan gate: %d calls", len(provider.keywords))
	}
}

func TestAddKeywordsStopsAtTheCap(t *testing.T) {
	s, project := newTestService(t, nil)
	ctx := context.Background()
	cfg := create(t, s, project, nil)

	batch := func(prefix string, n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = prefix + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + string(rune('a'+i/676))
		}
		return out
	}
	// Two requests race with 1,200 keywords for 1,000 slots: the table must
	// fill to the cap and never pass it.
	var wg sync.WaitGroup
	for _, prefix := range []string{"p-", "q-"} {
		wg.Go(func() {
			if _, err := s.AddKeywords(ctx, project, cfg.ID, batch(prefix, 600), false); err != nil && !errors.Is(err, ErrLimit) {
				t.Errorf("AddKeywords() error = %v", err)
			}
		})
	}
	wg.Wait()
	kws, _ := s.ListKeywords(ctx, project, cfg.ID)
	if len(kws) != MaxKeywordsPerConfig {
		t.Fatalf("stored %d keywords from 1,200 racing adds, want exactly the cap %d", len(kws), MaxKeywordsPerConfig)
	}
	if _, err := s.AddKeywords(ctx, project, cfg.ID, []string{"one more"}, false); !errors.Is(err, ErrLimit) {
		t.Errorf("error past the cap = %v, want ErrLimit", err)
	}
}

func TestEstimateCost(t *testing.T) {
	s, project := newTestService(t, nil)
	ctx := context.Background()
	cfg := create(t, s, project, func(in *CreateInput) { in.ScheduleInterval = Manual })
	if _, err := s.AddKeywords(ctx, project, cfg.ID, []string{"one", "site:example.com"}, false); err != nil {
		t.Fatal(err)
	}
	got, err := s.EstimateCost(ctx, project, cfg.ID, []string{"three"})
	if err != nil {
		t.Fatal(err)
	}
	want := EstimateCheck([]string{"one", "site:example.com", "three"}, "both", 10, Live)
	if got.CostCredits != want.CostCredits || got.KeywordCount != 3 || got.TotalChecks != 6 ||
		got.ExistingKeywordCount != 2 || got.AdditionalKeywordCount != 1 || got.ScheduledEstimate != nil {
		t.Errorf("estimate = %+v, want %+v credits over 3 keywords, no schedule", got, want)
	}
	if _, err := s.EstimateCost(ctx, "another-project", cfg.ID, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("estimate from another project error = %v, want ErrNotFound", err)
	}
	weekly := Weekly
	if _, err := s.UpdateConfig(ctx, project, cfg.ID, UpdateInput{ScheduleInterval: &weekly}); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.EstimateCost(ctx, project, cfg.ID, nil); got.ScheduledEstimate == nil || got.ScheduledEstimate.ChecksPerMonth != 4 {
		t.Errorf("scheduled estimate = %+v, want 4 checks a month", got.ScheduledEstimate)
	}
}
