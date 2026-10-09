package backlinks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

const (
	cacheTTL            = 6 * time.Hour
	cacheWriteTimeout   = 2 * time.Second
	maxFilterConditions = 8
)

type cache interface {
	Get(context.Context, string) ([]byte, error)
	Set(context.Context, string, []byte, time.Duration) error
}
type redisCache struct{ client *redis.Client }

func (c redisCache) Get(ctx context.Context, key string) ([]byte, error) {
	return c.client.Get(ctx, key).Bytes()
}
func (c redisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

// Service retrieves and caches backlinks reports for organizations.
type Service struct {
	provider provider
	cache    cache
	logger   *slog.Logger
	now      func() time.Time
}

// NewService constructs a backlinks service using the shared DataForSEO client.
func NewService(client *dataforseo.Client, rdb *redis.Client, logger *slog.Logger) *Service {
	return newService(client, redisCache{rdb}, logger, time.Now)
}
func newService(client *dataforseo.Client, c cache, logger *slog.Logger, now func() time.Time) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{provider: provider{client: client}, cache: c, logger: logger, now: now}
}

type pageInput struct {
	Page     int
	PageSize int
	Sort     string
	Mode     string
}
type lookupInput struct {
	Target string `json:"target"`
	Scope  string `json:"scope,omitempty"`
}
type rowsFilters struct {
	Include          string   `json:"include,omitempty"`
	Exclude          string   `json:"exclude,omitempty"`
	MinDomainRank    *float64 `json:"minDomainRank,omitempty"`
	MaxDomainRank    *float64 `json:"maxDomainRank,omitempty"`
	MinLinkAuthority *float64 `json:"minLinkAuthority,omitempty"`
	MaxLinkAuthority *float64 `json:"maxLinkAuthority,omitempty"`
	MinSpamScore     *float64 `json:"minSpamScore,omitempty"`
	MaxSpamScore     *float64 `json:"maxSpamScore,omitempty"`
	LinkType         string   `json:"linkType,omitempty"`
	HideLost         *bool    `json:"hideLost,omitempty"`
	HideBroken       *bool    `json:"hideBroken,omitempty"`
	DomainFrom       string   `json:"domainFrom,omitempty"`
}
type domainsFilters struct {
	Include      string   `json:"include,omitempty"`
	Exclude      string   `json:"exclude,omitempty"`
	MinBacklinks *float64 `json:"minBacklinks,omitempty"`
	MaxBacklinks *float64 `json:"maxBacklinks,omitempty"`
	MinRank      *float64 `json:"minRank,omitempty"`
	MaxRank      *float64 `json:"maxRank,omitempty"`
	MinSpamScore *float64 `json:"minSpamScore,omitempty"`
	MaxSpamScore *float64 `json:"maxSpamScore,omitempty"`
}
type pagesFilters struct {
	Include             string   `json:"include,omitempty"`
	Exclude             string   `json:"exclude,omitempty"`
	MinBacklinks        *float64 `json:"minBacklinks,omitempty"`
	MaxBacklinks        *float64 `json:"maxBacklinks,omitempty"`
	MinReferringDomains *float64 `json:"minReferringDomains,omitempty"`
	MaxReferringDomains *float64 `json:"maxReferringDomains,omitempty"`
	MinRank             *float64 `json:"minRank,omitempty"`
	MaxRank             *float64 `json:"maxRank,omitempty"`
}

// Overview returns summary and history reports for a target.
func (s *Service) Overview(ctx context.Context, org string, in lookupInput) (Overview, error) {
	target, err := normalizeTarget(in.Target, in.Scope)
	if err != nil {
		return Overview{}, err
	}
	key, err := cacheKey("overview", org, target)
	if err != nil {
		return Overview{}, err
	}
	var cached Overview
	if s.readCache(ctx, key, &cached) {
		return cached, nil
	}
	now := s.now().UTC()
	result := Overview{Target: target.APITarget, DisplayTarget: target.Display, Scope: target.Scope, Trends: []Trend{}, NewLostTrends: []NewLostTrend{}, FetchedAt: timestamp(now)}
	if target.Scope == ScopeSubfolder {
		filters := scopeFilters("url_to", target)
		// Sequential because provider-side usage/balance can be checked per task.
		_, allTotal, err := s.provider.rows(ctx, org, target, pageInput{Page: 1, PageSize: 1, Sort: "rank,desc", Mode: "as_is"}, filters, false)
		if err != nil {
			return Overview{}, err
		}
		_, domainTotal, err := s.provider.rows(ctx, org, target, pageInput{Page: 1, PageSize: 1, Sort: "rank,desc", Mode: "one_per_domain"}, filters, false)
		if err != nil {
			return Overview{}, err
		}
		result.Summary.Backlinks = floatInt(allTotal)
		result.Summary.ReferringDomains = floatInt(domainTotal)
		s.writeCache(ctx, key, result)
		return result, nil
	}
	var summary providerSummary
	var history []providerHistory
	var summaryErr, historyErr error
	var calls sync.WaitGroup
	calls.Go(func() { summary, summaryErr = s.provider.summary(ctx, org, target) })
	if target.Scope != ScopeExactURL {
		dateTo := now.Truncate(24 * time.Hour).Add(-24 * time.Hour)
		dateFrom := dateTo.AddDate(-1, 0, 0)
		calls.Go(func() {
			history, historyErr = s.provider.history(ctx, org, target.APITarget, dateFrom.Format("2006-01-02"), dateTo.Format("2006-01-02"))
		})
	}
	calls.Wait()
	if summaryErr != nil {
		return Overview{}, summaryErr
	}
	if historyErr != nil {
		return Overview{}, historyErr
	}
	result.Summary = mapSummary(summary)
	if target.Scope != ScopeExactURL {
		for _, item := range history {
			if item.Date == nil || len(*item.Date) < 10 {
				continue
			}
			date := (*item.Date)[:10]
			result.Trends = append(result.Trends, Trend{Date: date, Backlinks: item.Backlinks, ReferringDomains: item.ReferringDomains, Rank: item.Rank})
			result.NewLostTrends = append(result.NewLostTrends, NewLostTrend{Date: date, NewBacklinks: item.NewBacklinks, LostBacklinks: item.LostBacklinks, NewReferringDomains: first(item.NewReferringDomains, item.NewReferringDomainsLegacy), LostReferringDomains: first(item.LostReferringDomains, item.LostReferringDomainsLegacy)})
		}
	}
	s.writeCache(ctx, key, result)
	return result, nil
}

// Rows returns paginated backlink rows matching the request filters.
func (s *Service) Rows(ctx context.Context, org string, in lookupInput, page pageInput, filters rowsFilters, hideSpam bool) (Page[BacklinkRow], error) {
	target, err := normalizeTarget(in.Target, in.Scope)
	if err != nil {
		return Page[BacklinkRow]{}, err
	}
	key, err := cacheKey("rows", org, struct {
		Target   Target
		Page     pageInput
		Filters  rowsFilters
		HideSpam bool
	}{target, page, filters, hideSpam})
	if err != nil {
		return Page[BacklinkRow]{}, err
	}
	var cached Page[BacklinkRow]
	if s.readCache(ctx, key, &cached) {
		return cached, nil
	}
	expr := buildRowsFilters(filters)
	scope := scopeFilters("url_to", target)
	if countFilters(expr)+countFilters(scope)+boolInt(hideSpam)*2 > maxFilterConditions {
		return Page[BacklinkRow]{}, inputError("Too many filter conditions (maximum 8).")
	}
	expr = joinWithAnd(scope, expr)
	items, total, err := s.provider.rows(ctx, org, target, page, expr, hideSpam)
	if err != nil {
		return Page[BacklinkRow]{}, err
	}
	rows := make([]BacklinkRow, 0, len(items))
	for _, v := range items {
		rows = append(rows, mapBacklink(v))
	}
	out := pageResult(rows, total, page, s.now())
	s.writeCache(ctx, key, out)
	return out, nil
}

// Domains returns paginated referring domains matching the request filters.
func (s *Service) Domains(ctx context.Context, org string, in lookupInput, page pageInput, filters domainsFilters) (Page[ReferringDomainRow], error) {
	target, err := normalizeTarget(in.Target, in.Scope)
	if err != nil {
		return Page[ReferringDomainRow]{}, err
	}
	key, err := cacheKey("domains", org, struct {
		Target  Target
		Page    pageInput
		Filters domainsFilters
	}{target, page, filters})
	if err != nil {
		return Page[ReferringDomainRow]{}, err
	}
	var cached Page[ReferringDomainRow]
	if s.readCache(ctx, key, &cached) {
		return cached, nil
	}
	if target.Scope == ScopeSubfolder {
		return Page[ReferringDomainRow]{}, inputError("Referring domains can't be broken down for a subfolder — use the Backlinks tab, or switch to Domain or Subdomains scope.")
	}
	expr := buildDomainsFilters(filters)
	if countFilters(expr) > maxFilterConditions {
		return Page[ReferringDomainRow]{}, inputError("Too many filter conditions (maximum 8).")
	}
	items, total, err := s.provider.domains(ctx, org, target, page, expr, false)
	if err != nil {
		return Page[ReferringDomainRow]{}, err
	}
	rows := make([]ReferringDomainRow, 0, len(items))
	for _, v := range items {
		rows = append(rows, ReferringDomainRow(v))
	}
	out := pageResult(rows, total, page, s.now())
	s.writeCache(ctx, key, out)
	return out, nil
}

// Pages returns top pages and their backlink metrics for a target.
func (s *Service) Pages(ctx context.Context, org string, in lookupInput, page pageInput, filters pagesFilters) (Page[TopPageRow], error) {
	target, err := normalizeTarget(in.Target, in.Scope)
	if err != nil {
		return Page[TopPageRow]{}, err
	}
	key, err := cacheKey("pages", org, struct {
		Target  Target
		Page    pageInput
		Filters pagesFilters
	}{target, page, filters})
	if err != nil {
		return Page[TopPageRow]{}, err
	}
	var cached Page[TopPageRow]
	if s.readCache(ctx, key, &cached) {
		return cached, nil
	}
	expr := buildPagesFilters(filters)
	scope := scopeFilters("url", target)
	if countFilters(expr)+countFilters(scope) > maxFilterConditions {
		return Page[TopPageRow]{}, inputError("Too many filter conditions (maximum 8).")
	}
	expr = joinWithAnd(scope, expr)
	items, total, err := s.provider.pages(ctx, org, target, page, expr)
	if err != nil {
		return Page[TopPageRow]{}, err
	}
	rows := make([]TopPageRow, 0, len(items))
	for _, v := range items {
		pageURL := v.Page
		if pageURL == nil {
			pageURL = v.URL
		}
		rows = append(rows, TopPageRow{Page: pageURL, Backlinks: v.Backlinks, ReferringDomains: v.ReferringDomains, Rank: v.Rank, BrokenBacklinks: v.BrokenBacklinks})
	}
	out := pageResult(rows, total, page, s.now())
	s.writeCache(ctx, key, out)
	return out, nil
}

func (s *Service) readCache(ctx context.Context, key string, dst any) bool {
	if s.cache == nil {
		return false
	}
	raw, err := s.cache.Get(ctx, key)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			s.logger.WarnContext(ctx, "read backlinks cache", "err", err)
		}
		return false
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		s.logger.WarnContext(ctx, "decode backlinks cache", "err", err)
		return false
	}
	return true
}
func (s *Service) writeCache(ctx context.Context, key string, value any) {
	if s.cache == nil {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		s.logger.ErrorContext(ctx, "encode backlinks cache", "err", err)
		return
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cacheWriteTimeout)
	defer cancel()
	if err := s.cache.Set(writeCtx, key, raw, cacheTTL); err != nil {
		s.logger.WarnContext(ctx, "write backlinks cache", "err", err)
	}
}
func cacheKey(kind, org string, value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode backlinks cache key: %w", err)
	}
	sum := sha256.Sum256(raw)
	return "backlinks:" + kind + ":" + org + ":" + hex.EncodeToString(sum[:]), nil
}
func pageResult[T any](rows []T, total *int, page pageInput, now time.Time) Page[T] {
	hasMore := len(rows) == page.PageSize
	if total != nil {
		hasMore = (page.Page-1)*page.PageSize+len(rows) < *total
	}
	return Page[T]{Rows: rows, TotalCount: total, HasMore: hasMore, Page: page.Page, PageSize: page.PageSize, FetchedAt: timestamp(now)}
}
func floatInt(v *int) *float64 {
	if v == nil {
		return nil
	}
	n := float64(*v)
	return &n
}
func first(a, b *float64) *float64 {
	if a != nil {
		return a
	}
	return b
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func mapSummary(v providerSummary) Summary {
	return Summary{Rank: v.Rank, Backlinks: v.Backlinks, ReferringPages: v.ReferringPages, ReferringDomains: v.ReferringDomains, BrokenBacklinks: v.BrokenBacklinks, BrokenPages: v.BrokenPages, BacklinksSpamScore: v.BacklinksSpamScore, TargetSpamScore: v.Info.TargetSpamScore, NewBacklinks: v.NewBacklinks, LostBacklinks: v.LostBacklinks, NewReferringDomains: first(v.NewReferringDomains, v.NewReferringDomainsLegacy), LostReferringDomains: first(v.LostReferringDomains, v.LostReferringDomainsLegacy)}
}
func mapBacklink(v providerBacklink) BacklinkRow {
	spam := first(v.SpamScore, v.SpamScoreLegacy)
	attrs := v.RelAttributes
	if attrs == nil {
		attrs = v.Attributes
	}
	if attrs == nil {
		attrs = []string{}
	}
	lost := false
	if v.IsLost != nil {
		lost = *v.IsLost
	} else if v.LostDate != nil {
		lost = *v.LostDate != ""
	}
	broken := false
	if v.IsBroken != nil {
		broken = *v.IsBroken
	}
	last := v.LostDate
	if last == nil {
		last = v.LastVisited
	}
	return BacklinkRow{DomainFrom: v.DomainFrom, URLFrom: v.URLFrom, URLTo: v.URLTo, Anchor: v.Anchor, ItemType: v.ItemType, IsDofollow: v.Dofollow, RelAttributes: attrs, Rank: v.Rank, DomainFromRank: v.DomainFromRank, PageFromRank: v.PageFromRank, SpamScore: spam, FirstSeen: v.FirstSeen, LastSeen: last, IsLost: lost, IsBroken: broken, LinksCount: v.LinksCount}
}

func buildRowsFilters(f rowsFilters) []any {
	out := []any{}
	appendTerms(&out, "url_from", f.Exclude, "not_ilike")
	appendRange(&out, "domain_from_rank", f.MinDomainRank, f.MaxDomainRank)
	appendRange(&out, "rank", f.MinLinkAuthority, f.MaxLinkAuthority)
	appendRange(&out, "backlink_spam_score", f.MinSpamScore, f.MaxSpamScore)
	if f.LinkType != "" {
		out = append(out, []any{"dofollow", "=", f.LinkType == "dofollow"})
	}
	if f.HideLost != nil && *f.HideLost {
		out = append(out, []any{"is_lost", "=", false})
	}
	if f.HideBroken != nil && *f.HideBroken {
		out = append(out, []any{"is_broken", "=", false})
	}
	if f.DomainFrom != "" {
		out = append(out, []any{"domain_from", "=", f.DomainFrom})
	}
	return prependInclude(out, "url_from", f.Include)
}
func buildDomainsFilters(f domainsFilters) []any {
	out := []any{}
	appendTerms(&out, "domain", f.Exclude, "not_ilike")
	appendRange(&out, "backlinks", f.MinBacklinks, f.MaxBacklinks)
	appendRange(&out, "rank", f.MinRank, f.MaxRank)
	appendRange(&out, "backlinks_spam_score", f.MinSpamScore, f.MaxSpamScore)
	return prependInclude(out, "domain", f.Include)
}
func buildPagesFilters(f pagesFilters) []any {
	out := []any{}
	appendTerms(&out, "url", f.Exclude, "not_ilike")
	appendRange(&out, "backlinks", f.MinBacklinks, f.MaxBacklinks)
	appendRange(&out, "referring_domains", f.MinReferringDomains, f.MaxReferringDomains)
	appendRange(&out, "rank", f.MinRank, f.MaxRank)
	return prependInclude(out, "url", f.Include)
}
func appendRange(out *[]any, field string, minimum, maximum *float64) {
	if minimum != nil {
		*out = append(*out, []any{field, ">=", *minimum})
	}
	if maximum != nil {
		*out = append(*out, []any{field, "<=", *maximum})
	}
}
func prependInclude(out []any, field, value string) []any {
	terms := filterTerms(value)
	conditions := make([]any, 0, len(terms))
	for _, term := range terms {
		conditions = append(conditions, []any{field, "ilike", "%" + escapeLike(term) + "%"})
	}
	clauses := make([]any, 0, len(out)+1)
	if len(conditions) == 1 {
		clauses = append(clauses, conditions[0])
	} else if len(conditions) > 1 {
		group := []any{}
		for i, c := range conditions {
			if i > 0 {
				group = append(group, "or")
			}
			group = append(group, c)
		}
		clauses = append(clauses, group)
	}
	clauses = append(clauses, out...)
	if len(clauses) == 0 {
		return nil
	}
	joined := make([]any, 0, len(clauses)*2-1)
	for i, clause := range clauses {
		if i > 0 {
			joined = append(joined, "and")
		}
		joined = append(joined, clause)
	}
	return joined
}
func appendTerms(out *[]any, field, value, operator string) {
	for _, term := range filterTerms(value) {
		*out = append(*out, []any{field, operator, "%" + escapeLike(term) + "%"})
	}
}
func filterTerms(value string) []string {
	terms := []string{}
	for _, term := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return r == ',' || r == '+' }) {
		if term = strings.TrimSpace(term); term != "" {
			terms = append(terms, term)
		}
	}
	return terms
}
func escapeLike(value string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(value)
}
func countFilters(expr []any) int {
	n := 0
	for _, item := range expr {
		switch x := item.(type) {
		case []any:
			if len(x) > 0 {
				if _, ok := x[0].([]any); ok {
					n += countFilters(x)
				} else {
					n++
				}
			}
		}
	}
	return n
}
func scopeFilters(field string, target Target) []any {
	if target.Scope != ScopeSubfolder {
		return nil
	}
	hosts := []string{target.APITarget, "www." + target.APITarget}
	group := []any{}
	for _, host := range hosts {
		for _, suffix := range []string{target.Path, target.Path + "/%"} {
			if len(group) > 0 {
				group = append(group, "or")
			}
			group = append(group, []any{field, "like", "%://" + escapeLike(host) + suffix})
		}
	}
	return []any{group}
}
func joinWithAnd(a, b []any) []any {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	out := append([]any{}, a...)
	out = append(out, "and")
	return append(out, b...)
}
