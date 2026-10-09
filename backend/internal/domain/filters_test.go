package domain

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func f64(v float64) *float64 { return &v }

func TestBuildKeywordFiltersTranslatesEachFilterKind(t *testing.T) {
	t.Parallel()
	got, err := buildKeywordFilters(keywordFilters{Include: "audit, checker", Exclude: "jobs+salary", MinVol: f64(100), MaxVol: f64(5000), MinCPC: f64(0.5)}, "", scopeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	want := []any{
		[]any{"keyword_data.keyword", "ilike", "%audit%"}, "and",
		[]any{"keyword_data.keyword", "ilike", "%checker%"}, "and",
		[]any{"keyword_data.keyword", "not_ilike", "%jobs%"}, "and",
		[]any{"keyword_data.keyword", "not_ilike", "%salary%"}, "and",
		[]any{"keyword_data.keyword_info.search_volume", ">=", 100.0}, "and",
		[]any{"keyword_data.keyword_info.search_volume", "<=", 5000.0}, "and",
		[]any{"keyword_data.keyword_info.cpc", ">=", 0.5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filters =\n%#v\nwant\n%#v", got, want)
	}
}

func TestBuildKeywordFiltersEscapesLikeWildcards(t *testing.T) {
	t.Parallel()
	got, err := buildKeywordFilters(keywordFilters{Include: `100%_a\b`}, "", scopeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"keyword_data.keyword", "ilike", `%100\%\_a\\b%`}
	if !reflect.DeepEqual(got[0], want) {
		t.Fatalf("clause = %#v, want %#v", got[0], want)
	}
}

func TestBuildKeywordFiltersAddsSearchGroupAfterStructuredFilters(t *testing.T) {
	t.Parallel()
	got, err := buildKeywordFilters(keywordFilters{MinVol: f64(100)}, "  audit ", scopeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	want := []any{
		[]any{"keyword_data.keyword_info.search_volume", ">=", 100.0}, "and",
		[]any{
			[]any{"keyword_data.keyword", "ilike", "%audit%"}, "or",
			[]any{"ranked_serp_element.serp_item.url", "ilike", "%audit%"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filters = %#v, want %#v", got, want)
	}
}

func TestFilterBudgetIsEightConditions(t *testing.T) {
	t.Parallel()
	eight := keywordFilters{Include: "a,b,c,d", Exclude: "e,f", MinVol: f64(1), MaxVol: f64(2)}
	nine := eight
	nine.MinTraffic = f64(3)
	tests := []struct {
		name    string
		filters keywordFilters
		search  string
		scope   scopeFilter
		wantErr bool
	}{
		{"exactly eight", eight, "", scopeFilter{}, false},
		{"nine", nine, "", scopeFilter{}, true},
		{"search group costs two", keywordFilters{Include: "a,b,c,d", Exclude: "e,f", MinVol: f64(1)}, "audit", scopeFilter{}, true},
		{"search group fits", keywordFilters{Include: "a,b,c,d", Exclude: "e,f"}, "audit", scopeFilter{}, false},
		{"scope consumes budget", keywordFilters{Include: "a,b,c,d,e"}, "", scopeFilter{count: 4}, true},
		{"scope plus four fits", keywordFilters{Include: "a,b,c,d"}, "", scopeFilter{count: 4}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := buildKeywordFilters(tc.filters, tc.search, tc.scope)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "Too many filter conditions") {
				t.Fatalf("error = %q", err)
			}
		})
	}
}

// The UI shrinks the filter budget by the scope slot count
// (RESEARCH_SCOPE_FILTER_SLOTS); the server must count the same way or it
// rejects requests the UI allowed.
func TestScopeFilterSlotsMatchTheClientBudget(t *testing.T) {
	t.Parallel()
	keywordSlots := map[Scope]int{ScopeExactURL: 4, ScopeSubfolder: 4, ScopeDomain: 1, ScopeSubdomains: 0}
	pageSlots := map[Scope]int{ScopeExactURL: 4, ScopeSubfolder: 4, ScopeDomain: 2, ScopeSubdomains: 0}
	for scope, want := range keywordSlots {
		target := ResearchTarget{Scope: scope, Hostname: "example.com", Path: "/blog"}
		if got := buildRankedKeywordsScopeFilter(target); got.count != want {
			t.Errorf("keywords %s: slots = %d, want %d", scope, got.count, want)
		}
	}
	for scope, want := range pageSlots {
		target := ResearchTarget{Scope: scope, Hostname: "example.com", Path: "/blog"}
		if got := buildRelevantPagesScopeFilter(target); got.count != want {
			t.Errorf("pages %s: slots = %d, want %d", scope, got.count, want)
		}
	}
}

func TestSubfolderScopeLeavesFourUserConditions(t *testing.T) {
	t.Parallel()
	scope := buildRankedKeywordsScopeFilter(ResearchTarget{Scope: ScopeSubfolder, Hostname: "example.com", Path: "/blog"})
	if _, err := buildKeywordFilters(keywordFilters{Include: "a,b,c,d"}, "", scope); err != nil {
		t.Fatalf("four user conditions with subfolder scope rejected: %v", err)
	}
	if _, err := buildKeywordFilters(keywordFilters{Include: "a,b,c,d,e"}, "", scope); err == nil {
		t.Fatal("five user conditions with subfolder scope accepted, want budget error")
	}
}

func TestScopeFiltersCoverHostAndPath(t *testing.T) {
	t.Parallel()
	target := ResearchTarget{Scope: ScopeSubfolder, Hostname: "example.com", Path: "/100%_off"}
	keywords := buildRankedKeywordsScopeFilter(target)
	wantHost := []any{"ranked_serp_element.serp_item.domain", "in", []string{"example.com", "www.example.com"}}
	if !reflect.DeepEqual(keywords.clauses[0], wantHost) {
		t.Fatalf("host pin = %#v", keywords.clauses[0])
	}
	group := keywords.clauses[1].([]any)
	if got := group[0].([]any); got[1] != "=" || got[2] != "/100%_off" {
		t.Fatalf("exact path clause = %#v (must stay unescaped for =)", got)
	}
	if got := group[2].([]any); got[2] != `/100\%\_off/%` {
		t.Fatalf("child path clause = %#v", got)
	}
	pages := buildRelevantPagesScopeFilter(target)
	if !strings.Contains(toJSON(t, pages.clauses), `%://www.example.com/100\\%\\_off/%`) {
		t.Fatalf("pages filter lacks the www child pattern: %s", toJSON(t, pages.clauses))
	}
	if got := buildRankedKeywordsScopeFilter(ResearchTarget{Scope: ScopeDomain, Hostname: "example.com"}); len(got.clauses) != 1 {
		t.Fatalf("domain scope clauses = %v", got.clauses)
	}
	if got := buildRankedKeywordsScopeFilter(ResearchTarget{Scope: ScopeSubdomains, Hostname: "example.com"}); len(got.clauses) != 0 {
		t.Fatalf("subdomains scope must not filter, got %v", got.clauses)
	}
	exact := buildRankedKeywordsScopeFilter(ResearchTarget{Scope: ScopeExactURL, Hostname: "example.com"})
	if !strings.Contains(toJSON(t, exact.clauses), `"in",["/","//"]`) {
		t.Fatalf("exact URL on the root must pin '/': %s", toJSON(t, exact.clauses))
	}
}

func TestBuildPageFilters(t *testing.T) {
	t.Parallel()
	got, err := buildPageFilters(keywordFilters{Include: "blog", Exclude: "tag", MinTraffic: f64(10), MaxVol: f64(3)}, "post", scopeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	want := []any{
		[]any{"page_address", "ilike", "%blog%"}, "and",
		[]any{"page_address", "not_ilike", "%tag%"}, "and",
		[]any{"metrics.organic.etv", ">=", 10.0}, "and",
		[]any{"metrics.organic.count", "<=", 3.0}, "and",
		[]any{"page_address", "ilike", "%post%"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("page filters = %#v, want %#v", got, want)
	}
	if _, err := buildPageFilters(keywordFilters{Include: "a,b,c,d,e"}, "x", scopeFilter{count: 4}); err == nil {
		t.Fatal("over-budget page filters accepted")
	}
}

func TestParseTermsAndRounding(t *testing.T) {
	t.Parallel()
	if got := parseTerms(" A, ,b+C ,,ÜBER "); !reflect.DeepEqual(got, []string{"a", "b", "c", "über"}) {
		t.Fatalf("terms = %#v", got)
	}
	if got := parseTerms(""); len(got) != 0 {
		t.Fatalf("terms = %#v", got)
	}
	if got := roundNullable(f64(2.5)); got == nil || *got != 3 {
		t.Fatalf("round 2.5 = %v", got)
	}
	if roundNullable(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	if roundNullable(f64(math.NaN())) != nil || roundNullable(f64(math.Inf(1))) != nil {
		t.Fatal("NaN and Inf must become nil")
	}
}
