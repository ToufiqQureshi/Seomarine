package keywords

import (
	"context"
	"errors"
	"os"
	"slices"
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

func openSavedPool(t *testing.T) *pgxpool.Pool {
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

type savedFixture struct {
	svc     *SavedService
	pool    *pgxpool.Pool
	project string
}

func newSavedFixture(t *testing.T) savedFixture {
	t.Helper()
	pool := openSavedPool(t)
	project, err := ids.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c := context.WithoutCancel(context.Background())
		for _, table := range []string{"go_saved_keywords", "go_saved_keyword_tags", "go_keyword_metrics"} {
			if _, err := pool.Exec(c, `DELETE FROM `+table+` WHERE project_id = $1`, project); err != nil {
				t.Errorf("clean %s: %v", table, err)
			}
		}
	})
	return savedFixture{svc: &SavedService{Store: SavedRepository{DB: pool}}, pool: pool, project: project}
}

func (f savedFixture) save(t *testing.T, mutate func(*SaveInput)) []string {
	t.Helper()
	in := SaveInput{ProjectID: f.project, LocationCode: 2840, LanguageCode: "en", Keywords: []string{"alpha"}}
	if mutate != nil {
		mutate(&in)
	}
	got, err := f.svc.Save(context.Background(), in)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	return got
}

func (f savedFixture) list(t *testing.T, mutate func(*ListQuery)) ListResult {
	t.Helper()
	q := ListQuery{ProjectID: f.project, Page: 1, PageSize: 50, Descending: true}
	if mutate != nil {
		mutate(&q)
	}
	got, err := f.svc.List(context.Background(), q)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	return got
}

func words(rows []SavedKeyword) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Keyword
	}
	return out
}

func intp(v int) *int           { return &v }
func floatp(v float64) *float64 { return &v }
func strp(v string) *string     { return &v }

func TestNormalizeTagAndKeyword(t *testing.T) {
	tests := []struct {
		in, name, normalized string
		ok                   bool
	}{
		{"  Local   SEO ", "Local SEO", "local seo", true},
		{"Ünïcode\tTag", "Ünïcode Tag", "ünïcode tag", true},
		{"   ", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		name, normalized, ok := NormalizeTag(tt.in)
		if name != tt.name || normalized != tt.normalized || ok != tt.ok {
			t.Errorf("NormalizeTag(%q) = %q, %q, %v; want %q, %q, %v", tt.in, name, normalized, ok, tt.name, tt.normalized, tt.ok)
		}
	}
	if got := NormalizeTags([]string{"SEO", "seo", " Seo ", "Other", ""}); !slices.Equal(got, []string{"SEO", "Other"}) {
		t.Errorf("NormalizeTags() = %q, want the first spelling of each", got)
	}
	if got := NormalizeKeyword("  Best SEO Tool "); got != "best seo tool" {
		t.Errorf("NormalizeKeyword() = %q", got)
	}
}

func TestSaveNormalizesAndKeepsIDsStable(t *testing.T) {
	f := newSavedFixture(t)
	first := f.save(t, func(in *SaveInput) { in.Keywords = []string{" Beta ", "alpha", "BETA", "  ", "Gamma"} })
	if len(first) != 3 {
		t.Fatalf("ids = %v, want 3 (deduped, blanks dropped)", first)
	}
	again := f.save(t, func(in *SaveInput) { in.Keywords = []string{"gamma", "alpha", "delta"} })
	if again[0] != first[2] || again[1] != first[1] {
		t.Errorf("re-saved ids = %v, want the existing ids %v in the order asked", again, first)
	}
	got := f.list(t, func(q *ListQuery) { q.Sort, q.Descending = "keyword", false })
	if got.TotalCount != 4 || !slices.Equal(words(got.Rows), []string{"alpha", "beta", "delta", "gamma"}) {
		t.Errorf("saved = %v (%d)", words(got.Rows), got.TotalCount)
	}
	other := f.save(t, func(in *SaveInput) { in.LocationCode, in.LanguageCode = 2356, "hi" })
	if other[0] == first[1] {
		t.Error("the same keyword in another market must be a separate saved keyword")
	}
	if _, err := f.svc.Save(context.Background(), SaveInput{ProjectID: f.project, LocationCode: 2840, LanguageCode: "en", Keywords: []string{" ", ""}}); !isValidation(err) {
		t.Errorf("only blank keywords error = %v, want ValidationError", err)
	}
}

func isValidation(err error) bool {
	_, ok := errors.AsType[ValidationError](err)
	return ok
}

func TestSaveStoresMetricsOnlyForSavedKeywords(t *testing.T) {
	f := newSavedFixture(t)
	f.save(t, func(in *SaveInput) {
		in.Keywords = []string{"Alpha", "beta"}
		in.Metrics = []Metric{
			{Keyword: " ALPHA ", SearchVolume: intp(100), CPC: floatp(1.5), Competition: floatp(0.4), KeywordDifficulty: intp(30), Intent: strp("commercial"),
				MonthlySearches: []MonthlySearch{{Year: 2026, Month: 9, SearchVolume: 90}}},
			{Keyword: "alpha", SearchVolume: intp(120)}, // the later metric for a keyword wins
			{Keyword: "not saved", SearchVolume: intp(999)},
		}
	})
	got := f.list(t, func(q *ListQuery) { q.Sort, q.Descending = "keyword", false })
	alpha, beta := got.Rows[0], got.Rows[1]
	if pos := alpha.SearchVolume; pos == nil || *pos != 120 || alpha.CPC != nil || alpha.FetchedAt == nil {
		t.Errorf("alpha = %+v, want the later metric (volume 120, no cpc)", alpha)
	}
	if beta.SearchVolume != nil || beta.MonthlySearches == nil || beta.Tags == nil || beta.FetchedAt != nil {
		t.Errorf("beta = %+v, want no metrics and empty (non-nil) lists", beta)
	}
	var stray int
	_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM go_keyword_metrics WHERE project_id = $1 AND keyword = 'not saved'`, f.project).Scan(&stray)
	if stray != 0 {
		t.Error("stored a metric for a keyword that was not saved")
	}

	f.save(t, func(in *SaveInput) {
		in.Metrics = []Metric{{Keyword: "alpha", SearchVolume: intp(7), CPC: floatp(2), MonthlySearches: []MonthlySearch{{Year: 2026, Month: 1, SearchVolume: 5}}}}
	})
	again := f.list(t, func(q *ListQuery) { q.Search = "alpha" })
	if a := again.Rows[0]; *a.SearchVolume != 7 || *a.CPC != 2 || len(a.MonthlySearches) != 1 {
		t.Errorf("after re-save = %+v, want the metrics replaced", a)
	}

	// A value the database refuses must roll the whole save back.
	bad := Metric{Keyword: "gamma", Competition: floatp(2)}
	if _, err := f.svc.Save(context.Background(), SaveInput{ProjectID: f.project, LocationCode: 2840, LanguageCode: "en", Keywords: []string{"gamma"}, Metrics: []Metric{bad}}); err == nil {
		t.Fatal("an out-of-range competition was stored")
	}
	if f.list(t, func(q *ListQuery) { q.Search = "gamma" }).TotalCount != 0 {
		t.Error("a failed save left the keyword behind")
	}
}

func TestSaveTags(t *testing.T) {
	f := newSavedFixture(t)
	f.save(t, func(in *SaveInput) {
		in.Keywords = []string{"one", "two"}
		in.Tags = []string{"Brand", " brand ", "Blog"}
	})
	got := f.list(t, nil)
	if len(got.Tags) != 2 || got.Tags[0].NormalizedName != "blog" || got.Tags[0].KeywordCount != 2 || got.Tags[1].Name != "Brand" {
		t.Fatalf("tags = %+v, want blog and Brand on 2 keywords each", got.Tags)
	}
	for _, r := range got.Rows {
		if len(r.Tags) != 2 || r.Tags[0].NormalizedName != "blog" {
			t.Errorf("row tags = %+v, want both tags ordered by name", r.Tags)
		}
	}
	f.save(t, func(in *SaveInput) { in.Keywords = []string{"one"}; in.Tags = []string{"Offer"} })
	if r := f.list(t, func(q *ListQuery) { q.Search = "one" }).Rows[0]; len(r.Tags) != 3 {
		t.Errorf("append kept %d tags, want 3", len(r.Tags))
	}
	f.save(t, func(in *SaveInput) { in.Keywords = []string{"one"}; in.Tags = []string{"blog"}; in.ReplaceTags = true })
	one := f.list(t, func(q *ListQuery) { q.Search = "one" }).Rows[0]
	two := f.list(t, func(q *ListQuery) { q.Search = "two" }).Rows[0]
	if len(one.Tags) != 1 || one.Tags[0].Name != "Blog" || len(two.Tags) != 2 {
		t.Errorf("replace: one = %+v, two = %+v; want only the saved keywords replaced", one.Tags, two.Tags)
	}
	if _, err := f.svc.Save(context.Background(), SaveInput{ProjectID: f.project, LocationCode: 2840, LanguageCode: "en", Keywords: []string{"one"}, ReplaceTags: true}); !isValidation(err) {
		t.Errorf("replace without tags error = %v, want ValidationError", err)
	}
}

func TestListFiltersSortsAndPages(t *testing.T) {
	f := newSavedFixture(t)
	f.save(t, func(in *SaveInput) {
		in.Keywords = []string{"seo tool", "seo_tool free", "100% seo", "local seo", "no metrics"}
		in.Metrics = []Metric{
			{Keyword: "seo tool", SearchVolume: intp(1000), CPC: floatp(2.5), KeywordDifficulty: intp(40)},
			{Keyword: "seo_tool free", SearchVolume: intp(10), CPC: floatp(0.5), KeywordDifficulty: intp(10)},
			{Keyword: "100% seo", SearchVolume: intp(500), CPC: floatp(5), KeywordDifficulty: intp(80)},
			{Keyword: "local seo", SearchVolume: intp(500), CPC: floatp(1), KeywordDifficulty: intp(55)},
		}
	})
	check := func(name string, mutate func(*ListQuery), want ...string) {
		t.Helper()
		got := words(f.list(t, func(q *ListQuery) { q.Sort, q.Descending = "keyword", false; mutate(q) }).Rows)
		if !slices.Equal(got, want) {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	check("search", func(q *ListQuery) { q.Search = "TOOL" }, "seo tool", "seo_tool free")
	check("a percent sign is a literal", func(q *ListQuery) { q.Search = "100%" }, "100% seo")
	check("an underscore is a literal", func(q *ListQuery) { q.Search = "o_t" }, "seo_tool free")
	check("include and exclude", func(q *ListQuery) { q.IncludeTerms, q.ExcludeTerms = []string{"seo"}, []string{"local", "free"} }, "100% seo", "seo tool")
	check("volume range", func(q *ListQuery) { q.MinVolume, q.MaxVolume = intp(500), intp(900) }, "100% seo", "local seo")
	check("cpc range", func(q *ListQuery) { q.MinCPC, q.MaxCPC = floatp(1), floatp(2.5) }, "local seo", "seo tool")
	check("difficulty range", func(q *ListQuery) { q.MinDifficulty, q.MaxDifficulty = intp(10), intp(40) }, "seo tool", "seo_tool free")
	check("a range excludes keywords without metrics", func(q *ListQuery) { q.MinVolume = intp(0) }, "100% seo", "local seo", "seo tool", "seo_tool free")

	sorted := func(sort string, desc bool) []string {
		return words(f.list(t, func(q *ListQuery) { q.Sort, q.Descending = sort, desc }).Rows)
	}
	if got := sorted("searchVolume", true); got[0] != "seo tool" || got[len(got)-1] != "no metrics" {
		t.Errorf("volume desc = %q, want the unmeasured keyword last", got)
	}
	if got := sorted("searchVolume", false); got[0] != "seo_tool free" || got[len(got)-1] != "no metrics" {
		t.Errorf("volume asc = %q, want the unmeasured keyword last in both directions", got)
	}
	if got := sorted("cpc", true); got[0] != "100% seo" {
		t.Errorf("cpc desc = %q", got)
	}
	// Equal sort values fall back to the id, so paging never repeats or skips a row.
	seen := map[string]bool{}
	pageOf := func(page, size int) ListResult {
		return f.list(t, func(q *ListQuery) { q.Sort, q.Descending, q.Page, q.PageSize = "keyword", false, page, size })
	}
	if p := pageOf(1, 2); len(p.Rows) != 2 || p.TotalCount != 5 {
		t.Errorf("page 1 = %v of %d", words(p.Rows), p.TotalCount)
	}
	for page := 1; page <= 3; page++ {
		for _, w := range words(pageOf(page, 2).Rows) {
			if seen[w] {
				t.Errorf("%q appeared on two pages", w)
			}
			seen[w] = true
		}
	}
	if len(seen) != 5 {
		t.Errorf("paging saw %d keywords, want all 5", len(seen))
	}
	if p := pageOf(9, 2); len(p.Rows) != 0 || p.TotalCount != 5 {
		t.Errorf("past the end = %v of %d", words(p.Rows), p.TotalCount)
	}
	if _, err := f.svc.List(context.Background(), ListQuery{ProjectID: f.project, Sort: "bogus"}); !isValidation(err) {
		t.Errorf("unknown sort error = %v, want ValidationError", err)
	}
	all := f.list(t, func(q *ListQuery) { q.PageSize = 0 })
	if len(all.Rows) != 5 {
		t.Errorf("an unpaged list = %d rows, want all 5 for an export", len(all.Rows))
	}
}

func TestListFiltersByTagsAndStaysInTheProject(t *testing.T) {
	f := newSavedFixture(t)
	f.save(t, func(in *SaveInput) { in.Keywords = []string{"a", "b"}; in.Tags = []string{"Red"} })
	f.save(t, func(in *SaveInput) { in.Keywords = []string{"c"}; in.Tags = []string{"Blue"} })
	f.save(t, func(in *SaveInput) { in.Keywords = []string{"d"} })
	all := f.list(t, nil)
	var redID string
	for _, tag := range all.Tags {
		if tag.Name == "Red" {
			redID = tag.ID
		}
	}
	sortKw := func(q *ListQuery) { q.Sort, q.Descending = "keyword", false }
	got := func(m func(*ListQuery)) []string {
		return words(f.list(t, func(q *ListQuery) { sortKw(q); m(q) }).Rows)
	}
	if g := got(func(q *ListQuery) { q.TagIDs = []string{redID} }); !slices.Equal(g, []string{"a", "b"}) {
		t.Errorf("by id = %q", g)
	}
	if g := got(func(q *ListQuery) { q.TagNames = []string{" BLUE "} }); !slices.Equal(g, []string{"c"}) {
		t.Errorf("by name = %q", g)
	}
	if g := got(func(q *ListQuery) { q.TagNames, q.TagIDs = []string{"blue"}, []string{redID} }); !slices.Equal(g, []string{"a", "b", "c"}) {
		t.Errorf("by name and id = %q, want the union", g)
	}
	res := f.list(t, func(q *ListQuery) { q.TagNames = []string{"nonexistent"} })
	if len(res.Rows) != 0 || res.TotalCount != 0 || len(res.Tags) != 2 {
		t.Errorf("a tag name that matches nothing = %+v, want no rows but the project's tags", res)
	}
	if g := got(func(q *ListQuery) { q.TagNames, q.TagIDs = []string{"nonexistent"}, []string{redID} }); !slices.Equal(g, []string{"a", "b"}) {
		t.Errorf("unknown name plus a real id = %q, want the id's keywords", g)
	}

	other := newSavedFixture(t)
	if len(other.list(t, nil).Rows) != 0 || len(other.list(t, nil).Tags) != 0 {
		t.Error("another project sees this project's keywords or tags")
	}
}

func TestTagAssignments(t *testing.T) {
	f := newSavedFixture(t)
	keywordIDs := f.save(t, func(in *SaveInput) { in.Keywords = []string{"a", "b"} })
	stranger := newSavedFixture(t)
	strangerIDs := stranger.save(t, nil)
	ctx := context.Background()

	res, err := f.svc.UpdateAssignments(ctx, f.project, append(keywordIDs, strangerIDs...), []string{"Other", "New", "new"}, nil)
	if err == nil && len(res.AddedTags) == 2 && (res.AddedTags[0].NormalizedName != "new" || res.AddedTags[1].NormalizedName != "other") {
		t.Errorf("added tags = %+v, want them ordered by name", res.AddedTags)
	}
	if err != nil || res.TaggedCount != 2 || len(res.AddedTags) != 2 {
		t.Fatalf("assign = %+v, %v; want 2 own keywords tagged with 2 tags (the stranger's ignored)", res, err)
	}
	if r := stranger.list(t, nil).Rows[0]; len(r.Tags) != 0 {
		t.Errorf("another project's keyword was tagged: %+v", r.Tags)
	}
	again, _ := f.svc.UpdateAssignments(ctx, f.project, keywordIDs, []string{"NEW"}, nil)
	if again.AddedTags[0].ID != res.AddedTags[1].ID && again.AddedTags[0].ID != res.AddedTags[0].ID {
		t.Error("an existing tag was created twice")
	}

	var removeID string
	for _, tg := range res.AddedTags {
		if tg.NormalizedName == "other" {
			removeID = tg.ID
		}
	}
	rem, err := f.svc.UpdateAssignments(ctx, f.project, keywordIDs[:1], nil, []string{removeID, "no-such-tag"})
	if err != nil || rem.RemovedAssignments != 1 || !slices.Equal(rem.RemovedTagIDs, []string{removeID}) || rem.TaggedCount != 1 {
		t.Errorf("remove = %+v, %v", rem, err)
	}
	if _, err := f.svc.UpdateAssignments(ctx, f.project, keywordIDs, nil, nil); !isValidation(err) {
		t.Errorf("nothing to do error = %v, want ValidationError", err)
	}
	// Another project's tag id must not be removable here.
	strangerTag, _ := stranger.svc.UpdateAssignments(ctx, stranger.project, strangerIDs, []string{"theirs"}, nil)
	if rem, _ := f.svc.UpdateAssignments(ctx, f.project, keywordIDs, nil, []string{strangerTag.AddedTags[0].ID}); len(rem.RemovedTagIDs) != 0 {
		t.Error("removed another project's tag")
	}
}

func TestUpdateAndDeleteTag(t *testing.T) {
	f := newSavedFixture(t)
	keywordIDs := f.save(t, func(in *SaveInput) { in.Keywords = []string{"a"}; in.Tags = []string{"First", "Second"} })
	tags := f.list(t, nil).Tags
	first, second := tags[0], tags[1]
	ctx := context.Background()

	renamed, err := f.svc.UpdateTag(ctx, f.project, first.ID, strp("  Renamed   Tag "), nil)
	if err != nil || renamed == nil || renamed.Name != "Renamed Tag" || renamed.NormalizedName != "renamed tag" {
		t.Fatalf("rename = %+v, %v", renamed, err)
	}
	green := strp("emerald")
	if tag, err := f.svc.UpdateTag(ctx, f.project, first.ID, nil, &green); err != nil || tag.Color == nil || *tag.Color != "emerald" {
		t.Errorf("recolor = %+v, %v", tag, err)
	}
	var none *string
	if tag, err := f.svc.UpdateTag(ctx, f.project, first.ID, nil, &none); err != nil || tag.Color != nil {
		t.Errorf("clearing the color = %+v, %v", tag, err)
	}
	if _, err := f.svc.UpdateTag(ctx, f.project, second.ID, strp("RENAMED tag"), nil); !errors.Is(err, ErrTagExists) {
		t.Errorf("rename onto an existing name error = %v, want ErrTagExists", err)
	}
	purple := strp("purple")
	for name, call := range map[string]func() (*Tag, error){
		"unknown color": func() (*Tag, error) { return f.svc.UpdateTag(ctx, f.project, first.ID, nil, &purple) },
		"empty name":    func() (*Tag, error) { return f.svc.UpdateTag(ctx, f.project, first.ID, strp("   "), nil) },
		"long name": func() (*Tag, error) {
			return f.svc.UpdateTag(ctx, f.project, first.ID, strp(strings.Repeat("x", 65)), nil)
		},
		"nothing": func() (*Tag, error) { return f.svc.UpdateTag(ctx, f.project, first.ID, nil, nil) },
	} {
		if _, err := call(); !isValidation(err) {
			t.Errorf("%s error = %v, want ValidationError", name, err)
		}
	}
	if tag, err := f.svc.UpdateTag(ctx, f.project, "no-such-tag", strp("x"), nil); err != nil || tag != nil {
		t.Errorf("unknown tag = %+v, %v; want nil", tag, err)
	}
	other := newSavedFixture(t)
	if tag, _ := other.svc.UpdateTag(ctx, other.project, first.ID, strp("hijacked"), nil); tag != nil {
		t.Error("renamed another project's tag")
	}

	var inUse *TagInUseError
	if err := f.svc.DeleteTag(ctx, f.project, first.ID); !errors.As(err, &inUse) || inUse.AssignmentCount != 1 || !strings.Contains(err.Error(), "1 keyword.") {
		t.Fatalf("delete of a used tag error = %v, want TagInUseError(1)", err)
	}
	if _, err := f.svc.UpdateAssignments(ctx, f.project, keywordIDs, nil, []string{first.ID}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteTag(ctx, f.project, first.ID); err != nil {
		t.Errorf("delete of an unused tag: %v", err)
	}
	if err := f.svc.DeleteTag(ctx, f.project, first.ID); !errors.Is(err, ErrTagNotFound) {
		t.Errorf("second delete error = %v, want ErrTagNotFound", err)
	}
	if err := other.svc.DeleteTag(ctx, other.project, second.ID); !errors.Is(err, ErrTagNotFound) {
		t.Errorf("another project deleting this tag: %v", err)
	}
	if err := (&TagInUseError{AssignmentCount: 3}).Error(); !strings.Contains(err, "3 keywords.") {
		t.Errorf("plural message = %q", err)
	}
}

func TestRemoveSavedKeywords(t *testing.T) {
	f := newSavedFixture(t)
	idsA := f.save(t, func(in *SaveInput) {
		in.Keywords = []string{"a", "b"}
		in.Tags = []string{"T"}
		in.Metrics = []Metric{{Keyword: "a", SearchVolume: intp(5)}}
	})
	stranger := newSavedFixture(t)
	strangerIDs := stranger.save(t, nil)
	ctx := context.Background()
	n, err := f.svc.Remove(ctx, f.project, []string{idsA[0], strangerIDs[0], "no-such-id"})
	if err != nil || n != 1 {
		t.Fatalf("Remove() = %d, %v; want only this project's keyword", n, err)
	}
	if stranger.list(t, nil).TotalCount != 1 {
		t.Error("removed another project's keyword")
	}
	var assignments, metrics int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM go_saved_keyword_tag_assignments WHERE saved_keyword_id = $1`, idsA[0]).Scan(&assignments)
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM go_keyword_metrics WHERE project_id = $1`, f.project).Scan(&metrics)
	if assignments != 0 || metrics != 1 {
		t.Errorf("assignments left %d, metrics left %d; want the assignments gone and the metric cache kept", assignments, metrics)
	}
	tags := f.list(t, nil).Tags
	if len(tags) != 1 || tags[0].KeywordCount != 1 {
		t.Errorf("tags = %+v, want the tag kept with its other keyword", tags)
	}
}

func TestMalformedMonthlySearchesReadAsEmpty(t *testing.T) {
	f := newSavedFixture(t)
	f.save(t, func(in *SaveInput) { in.Metrics = []Metric{{Keyword: "alpha", SearchVolume: intp(1)}} })
	if _, err := f.pool.Exec(context.Background(), `UPDATE go_keyword_metrics SET monthly_searches = '{"not":"a list"}' WHERE project_id = $1`, f.project); err != nil {
		t.Fatal(err)
	}
	got := f.list(t, nil)
	if len(got.Rows) != 1 || got.Rows[0].MonthlySearches == nil || len(got.Rows[0].MonthlySearches) != 0 {
		t.Errorf("rows = %+v, want an empty monthly list instead of a failure", got.Rows)
	}
}

func TestSaveRaceKeepsOneRowPerKeyword(t *testing.T) {
	f := newSavedFixture(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := f.svc.Save(context.Background(), SaveInput{ProjectID: f.project, LocationCode: 2840, LanguageCode: "en", Keywords: []string{"same", "other"}, Tags: []string{"Shared"}}); err != nil {
				t.Errorf("Save() error = %v", err)
			}
		})
	}
	wg.Wait()
	got := f.list(t, nil)
	if got.TotalCount != 2 || len(got.Tags) != 1 || got.Tags[0].KeywordCount != 2 {
		t.Errorf("after 8 racing saves: %d keywords, tags %+v; want 2 keywords and one tag on both", got.TotalCount, got.Tags)
	}
}

func TestResolveMarket(t *testing.T) {
	project := market.Pair{LocationCode: 2356, LanguageCode: "hi"}
	if got, err := ResolveMarket(0, "", project); err != nil || got != project {
		t.Errorf("project default = %+v, %v", got, err)
	}
	if got, err := ResolveMarket(2840, "", project); err != nil || got.LocationCode != 2840 || got.LanguageCode != "en" {
		t.Errorf("another country = %+v, %v; want its own language", got, err)
	}
	for _, bad := range []struct {
		loc  int
		lang string
	}{{999999, ""}, {2840, "xx"}} {
		if _, err := ResolveMarket(bad.loc, bad.lang, project); !isValidation(err) {
			t.Errorf("ResolveMarket(%d, %q) error = %v, want ValidationError", bad.loc, bad.lang, err)
		}
	}
}

func TestDeleteTagWaitsForAKeywordBeingTagged(t *testing.T) {
	f := newSavedFixture(t)
	keywordIDs := f.save(t, func(in *SaveInput) { in.Keywords = []string{"a"}; in.Tags = []string{"Busy"} })
	ctx := context.Background()
	tag := f.list(t, nil).Tags[0]
	if _, err := f.svc.UpdateAssignments(ctx, f.project, keywordIDs, nil, []string{tag.ID}); err != nil {
		t.Fatal(err)
	}

	// Another request is labelling a keyword with the tag and has not committed.
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, `INSERT INTO go_saved_keyword_tag_assignments (saved_keyword_id, tag_id) VALUES ($1, $2)`, keywordIDs[0], tag.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- f.svc.DeleteTag(ctx, f.project, tag.ID) }()
	select {
	case err := <-result:
		t.Fatalf("DeleteTag() returned %v while a keyword was being tagged; it must wait and then see the assignment", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var inUse *TagInUseError
	if err := <-result; !errors.As(err, &inUse) || inUse.AssignmentCount != 1 {
		t.Errorf("DeleteTag() error = %v, want TagInUseError(1): the cascade must not silently drop the new label", err)
	}
}
