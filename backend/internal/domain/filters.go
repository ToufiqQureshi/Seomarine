package domain

import (
	"fmt"
	"math"
	"strings"
)

const maxFilterConditions = 8

type keywordFilters struct {
	Include    string   `json:"include,omitempty"`
	Exclude    string   `json:"exclude,omitempty"`
	MinTraffic *float64 `json:"minTraffic,omitempty"`
	MaxTraffic *float64 `json:"maxTraffic,omitempty"`
	MinVol     *float64 `json:"minVol,omitempty"`
	MaxVol     *float64 `json:"maxVol,omitempty"`
	MinCPC     *float64 `json:"minCpc,omitempty"`
	MaxCPC     *float64 `json:"maxCpc,omitempty"`
	MinKD      *float64 `json:"minKd,omitempty"`
	MaxKD      *float64 `json:"maxKd,omitempty"`
	MinRank    *float64 `json:"minRank,omitempty"`
	MaxRank    *float64 `json:"maxRank,omitempty"`
}

type scopeFilter struct {
	clauses []any
	count   int
}

// newScopeFilter counts the leaf conditions the provider will charge against
// its 8-condition budget: nested groups count through, separators do not.
func newScopeFilter(clauses []any) scopeFilter {
	return scopeFilter{clauses: clauses, count: countConditions(clauses)}
}

func countConditions(parts []any) int {
	total := 0
	for _, part := range parts {
		group, ok := part.([]any)
		if !ok {
			continue
		}
		if len(group) > 0 {
			if _, nested := group[0].([]any); nested {
				total += countConditions(group)
				continue
			}
		}
		total++
	}
	return total
}

func buildRankedKeywordsScopeFilter(target ResearchTarget) scopeFilter {
	field := "ranked_serp_element.serp_item.relative_url"
	path := target.Path
	if path == "" {
		path = "/"
	}
	hostPin := []any{"ranked_serp_element.serp_item.domain", "in", []string{target.Hostname, "www." + target.Hostname}}
	switch target.Scope {
	case ScopeSubdomains:
		return scopeFilter{}
	case ScopeDomain:
		return newScopeFilter([]any{hostPin})
	case ScopeSubfolder:
		return newScopeFilter([]any{hostPin, join([]any{
			[]any{field, "=", path},
			[]any{field, "like", escapeLikeTerm(path) + "/%"},
			[]any{field, "like", escapeLikeTerm(path) + "?%"},
		}, "or")})
	case ScopeExactURL:
		return newScopeFilter([]any{hostPin, join([]any{
			[]any{field, "in", []string{path, path + "/"}},
			[]any{field, "like", escapeLikeTerm(path) + "?%"},
			[]any{field, "like", escapeLikeTerm(path) + "/?%"},
		}, "or")})
	default:
		return scopeFilter{}
	}
}

func buildRelevantPagesScopeFilter(target ResearchTarget) scopeFilter {
	hosts := []string{escapeLikeTerm(target.Hostname), escapeLikeTerm("www." + target.Hostname)}
	path := target.Path
	if path == "" {
		path = "/"
	}
	path = escapeLikeTerm(path)
	switch target.Scope {
	case ScopeSubdomains:
		return scopeFilter{}
	case ScopeDomain:
		clauses := make([]any, 0, len(hosts))
		for _, host := range hosts {
			clauses = append(clauses, []any{"page_address", "like", "%://" + host + "/%"})
		}
		return newScopeFilter([]any{join(clauses, "or")})
	case ScopeSubfolder:
		return subfolderPageFilter(target.Hostname, target.Path)
	case ScopeExactURL:
		clauses := make([]any, 0, len(hosts)*2)
		for _, host := range hosts {
			clauses = append(clauses,
				[]any{"page_address", "like", "%://" + host + path},
				[]any{"page_address", "like", "%://" + host + path + "/"},
			)
		}
		return newScopeFilter([]any{join(clauses, "or")})
	default:
		return scopeFilter{}
	}
}

func subfolderPageFilter(hostname, path string) scopeFilter {
	hosts := []string{escapeLikeTerm(hostname), escapeLikeTerm("www." + hostname)}
	clauses := make([]any, 0, 4)
	for _, host := range hosts {
		clauses = append(clauses,
			[]any{"page_address", "like", "%://" + host + escapeLikeTerm(path)},
			[]any{"page_address", "like", "%://" + host + escapeLikeTerm(path) + "/%"},
		)
	}
	return newScopeFilter([]any{join(clauses, "or")})
}

func buildKeywordFilters(filters keywordFilters, search string, scope scopeFilter) ([]any, error) {
	conditions := make([]any, 0, 16)
	appendTerms(&conditions, "keyword_data.keyword", filters.Include, "ilike")
	appendTerms(&conditions, "keyword_data.keyword", filters.Exclude, "not_ilike")
	appendRange(&conditions, "keyword_data.keyword_info.search_volume", filters.MinVol, filters.MaxVol)
	appendRange(&conditions, "ranked_serp_element.serp_item.etv", filters.MinTraffic, filters.MaxTraffic)
	appendRange(&conditions, "keyword_data.keyword_info.cpc", filters.MinCPC, filters.MaxCPC)
	appendRange(&conditions, "keyword_data.keyword_properties.keyword_difficulty", filters.MinKD, filters.MaxKD)
	appendRange(&conditions, "ranked_serp_element.serp_item.rank_absolute", filters.MinRank, filters.MaxRank)
	search = strings.TrimSpace(search)
	searchGroup := []any(nil)
	if search != "" {
		escaped := "%" + escapeLikeTerm(search) + "%"
		searchGroup = []any{
			[]any{"keyword_data.keyword", "ilike", escaped}, "or",
			[]any{"ranked_serp_element.serp_item.url", "ilike", escaped},
		}
	}
	count := scope.count + len(conditions)
	if searchGroup != nil {
		count += 2
	}
	if count > maxFilterConditions {
		return nil, fmt.Errorf("Too many filter conditions (maximum %d).", maxFilterConditions)
	}
	return combine(scope.clauses, conditions, searchGroup), nil
}

func buildPageFilters(filters keywordFilters, search string, scope scopeFilter) ([]any, error) {
	conditions := make([]any, 0, 8)
	appendTerms(&conditions, "page_address", filters.Include, "ilike")
	appendTerms(&conditions, "page_address", filters.Exclude, "not_ilike")
	appendRange(&conditions, "metrics.organic.etv", filters.MinTraffic, filters.MaxTraffic)
	appendRange(&conditions, "metrics.organic.count", filters.MinVol, filters.MaxVol)
	if search = strings.TrimSpace(search); search != "" {
		conditions = append(conditions, []any{"page_address", "ilike", "%" + escapeLikeTerm(search) + "%"})
	}
	if scope.count+len(conditions) > maxFilterConditions {
		return nil, fmt.Errorf("Too many filter conditions (maximum %d).", maxFilterConditions)
	}
	return combine(scope.clauses, conditions, nil), nil
}

func appendTerms(out *[]any, field, raw, operator string) {
	for _, term := range parseTerms(raw) {
		*out = append(*out, []any{field, operator, "%" + escapeLikeTerm(term) + "%"})
	}
}

func appendRange(out *[]any, field string, minimum, maximum *float64) {
	if minimum != nil {
		*out = append(*out, []any{field, ">=", *minimum})
	}
	if maximum != nil {
		*out = append(*out, []any{field, "<=", *maximum})
	}
}

func parseTerms(raw string) []string {
	terms := strings.FieldsFunc(strings.ToLower(raw), func(r rune) bool { return r == ',' || r == '+' })
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		if term = strings.TrimSpace(term); term != "" {
			out = append(out, term)
		}
	}
	return out
}

func escapeLikeTerm(term string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(term)
}

func join(parts []any, separator string) []any {
	joined := make([]any, 0, max(0, len(parts)*2-1))
	for i, part := range parts {
		if i > 0 {
			joined = append(joined, separator)
		}
		joined = append(joined, part)
	}
	return joined
}

func combine(scope, conditions []any, search []any) []any {
	all := make([]any, 0, len(scope)+len(conditions)+len(search))
	all = append(all, scope...)
	all = append(all, conditions...)
	if search != nil {
		all = append(all, search)
	}
	return join(all, "and")
}

func roundNullable(value *float64) *float64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	rounded := math.Round(*value)
	return &rounded
}
