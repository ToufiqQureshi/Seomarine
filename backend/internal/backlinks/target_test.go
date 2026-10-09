package backlinks

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, input, scope, wantTarget, wantDisplay string
		wantScope                                   Scope
		wantSubdomains                              bool
		wantErr                                     bool
	}{
		{name: "default root includes subdomains", input: "WWW.Example.com", wantTarget: "example.com", wantDisplay: "example.com", wantScope: ScopeSubdomains, wantSubdomains: true},
		{name: "domain excludes subdomains", input: "www.example.co.uk/path", scope: "domain", wantTarget: "example.co.uk", wantDisplay: "example.co.uk", wantScope: ScopeDomain},
		{name: "subfolder", input: "example.com/blog/", wantTarget: "example.com", wantDisplay: "example.com/blog", wantScope: ScopeSubfolder},
		{name: "page alias preserves http", input: "http://example.com/a", scope: "page", wantTarget: "http://example.com/a", wantDisplay: "http://example.com/a", wantScope: ScopeExactURL, wantSubdomains: true},
		{name: "unicode hostname", input: "bücher.de", wantTarget: "xn--bcher-kva.de", wantDisplay: "xn--bcher-kva.de", wantScope: ScopeSubdomains, wantSubdomains: true},
		{name: "exact query refused", input: "example.com/a?x=1", scope: "exact_url", wantErr: true},
		{name: "root subfolder refused", input: "example.com", scope: "subfolder", wantErr: true},
		{name: "IP refused", input: "192.0.2.1", wantErr: true},
		{name: "invalid suffix refused", input: "example.invalidtld", wantErr: true},
		{name: "credentials refused", input: "user:pass@example.com", wantErr: true},
		{name: "invalid scope refused", input: "example.com", scope: "other", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeTarget(tt.input, tt.scope)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeTarget error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got.APITarget != tt.wantTarget || got.Display != tt.wantDisplay || got.Scope != tt.wantScope || got.IncludeSubdomains != tt.wantSubdomains {
				t.Fatalf("normalizeTarget() = %#v", got)
			}
		})
	}
}

func TestScopeFilters(t *testing.T) {
	t.Parallel()
	target, err := normalizeTarget("example.com/blog", "subfolder")
	if err != nil {
		t.Fatal(err)
	}
	filters := scopeFilters("url_to", target)
	if got := countFilters(filters); got != 4 {
		t.Fatalf("condition count = %d, want 4", got)
	}
	encoded, err := json.Marshal(filters)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"%://example.com/blog", "%://example.com/blog/%", "%://www.example.com/blog"} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("scope filter %s missing %q", encoded, want)
		}
	}
}

func TestFilterTranslationMatchesLegacySemantics(t *testing.T) {
	t.Parallel()
	f := buildRowsFilters(rowsFilters{Include: "Blog, न्यूज़", Exclude: "spam", MinDomainRank: floatPtr(30), LinkType: "dofollow", HideLost: new(true)})
	want := []any{[]any{[]any{"url_from", "ilike", "%blog%"}, "or", []any{"url_from", "ilike", "%न्यूज़%"}}, "and", []any{"url_from", "not_ilike", "%spam%"}, "and", []any{"domain_from_rank", ">=", float64(30)}, "and", []any{"dofollow", "=", true}, "and", []any{"is_lost", "=", false}}
	got, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(expected) {
		t.Fatalf("filters = %s, want %s", got, expected)
	}
	if got := countFilters(f); got != 6 {
		t.Fatalf("condition count = %d, want 6", got)
	}
	filtersGot := buildRowsFilters(rowsFilters{Include: "x%_\\y"})
	condition, ok := filtersGot[0].([]any)
	if !ok || condition[2] != `%x\%\_\\y%` {
		t.Fatalf("LIKE metacharacters not escaped: %#v", filtersGot)
	}
}
