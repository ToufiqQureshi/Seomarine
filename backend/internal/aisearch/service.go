package aisearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

const (
	// Brand lookup data refreshes daily; the provider updates it monthly.
	brandLookupTTL = 24 * time.Hour
	// An LLM answer is stable enough for a week.
	promptResponseTTL = 7 * 24 * time.Hour

	// Prompt rows give cited pages their example prompts. The ranked source
	// rows come from top_pages, so the table is not limited to this sample.
	mentionsPerPlatform = 100

	// Reasoning models spend hidden chain-of-thought tokens from this budget,
	// so anything lower regularly returns a near-empty visible answer. It is
	// the provider's per-call maximum.
	promptMaxOutputTokens = 4096

	cacheWriteTimeout = 2 * time.Second
)

// Service runs brand lookups and prompt explorations for an organization's
// project. Results are cached in Redis per organization and project, because
// every uncached call is billed to the organization.
type Service struct {
	provider *provider
	redis    *redis.Client
	logger   *slog.Logger
	now      func() time.Time
}

// NewService returns a Service that calls DataForSEO through client.
func NewService(client *dataforseo.Client, rdb *redis.Client, logger *slog.Logger) *Service {
	return &Service{
		provider: &provider{client: client, models: newModelCatalog(client, logger)},
		redis:    rdb,
		logger:   logger,
		now:      time.Now,
	}
}

// BrandLookup measures how often AI platforms mention a brand or domain, what
// they cite, and how it compares with competitors.
//
// The provider calls are sequenced, not parallel, so a failing account cannot
// fan out several billed calls at once. A failure of one platform is reported
// in its row and does not discard the other; only an account-level failure
// (dataforseo.ErrBillingIssue) fails the lookup.
func (s *Service) BrandLookup(ctx context.Context, organizationID, projectID string, in BrandLookupInput) (BrandLookupResult, error) {
	detected := detectTarget(in.Query)
	research, err := resolveResearchTarget(in, detected)
	if err != nil {
		return BrandLookupResult{}, err
	}
	// The mentions API only scopes a domain by subdomain inclusion; URL scopes
	// are applied by filtering page rows when shaping.
	includeSubdomains := research == nil || research.Scope == ScopeSubdomains
	groups := resolveCompetitorGroups(detected.Value, in.Competitors)

	competitorValues := make([]string, len(groups))
	for i, g := range groups {
		competitorValues[i] = strings.ToLower(g.detected.Value)
	}
	slices.Sort(competitorValues)
	// Scope changes both the provider call and the page filtering. The path
	// only matters under URL scopes: keying it for a domain scope would re-buy
	// an identical lookup for example.com and example.com/blog.
	scope, path := "", ""
	if research != nil {
		scope = string(research.Scope)
		if research.Scope.usesPath() {
			path = research.Path
		}
	}
	key := cacheKey("brand-lookup", organizationID, projectID, string(detected.Type),
		// Lowercased to match the provider's matching, so equivalent casing and
		// order share one paid cache entry.
		strings.ToLower(detected.Value), strings.Join(competitorValues, "|"),
		strconv.Itoa(in.LocationCode), in.LanguageCode, scope, path)

	var cached BrandLookupResult
	if s.cacheGet(ctx, key, &cached) {
		cached.Query = in.Query
		cached.ResolvedTarget = detected.Value
		if research != nil {
			cached.ResolvedTarget = research.Display
		}
		return cached, nil
	}

	platforms := make([]platformOutcome, len(brandPlatforms))
	for i, platform := range brandPlatforms {
		bundle, err := s.fetchPlatform(ctx, organizationID, platform, detected, includeSubdomains, in)
		if blocking(ctx, err) {
			return BrandLookupResult{}, err
		}
		if err != nil {
			s.logger.ErrorContext(ctx, "brand lookup platform failed", "platform", platform, "err", err)
			platforms[i] = platformOutcome{platform: platform}
			continue
		}
		platforms[i] = platformOutcome{platform: platform, ok: true, bundle: bundle}
	}

	var cross []crossOutcome
	if len(groups) > 0 {
		cross, err = s.fetchCross(ctx, organizationID, detected, groups, includeSubdomains, in)
		if err != nil {
			return BrandLookupResult{}, err
		}
	}

	competitorKeys := make([]string, len(groups))
	for i, g := range groups {
		competitorKeys[i] = g.label
	}
	result := shapeResult(shapeArgs{
		query:          in.Query,
		detected:       detected,
		research:       research,
		platforms:      platforms,
		cross:          cross,
		competitorKeys: competitorKeys,
		locationCode:   in.LocationCode,
		languageCode:   in.LanguageCode,
		now:            s.now(),
	})

	// Cache only when every call succeeded: a platform that swallowed a failed
	// sub-call into empty data renders fine but must not be frozen for a day.
	complete := slices.IndexFunc(platforms, func(p platformOutcome) bool { return !p.ok || !p.bundle.complete }) < 0 &&
		slices.IndexFunc(cross, func(c crossOutcome) bool { return !c.ok }) < 0
	if complete && result.HasData {
		s.cacheSet(ctx, key, result, brandLookupTTL)
	}
	return result, nil
}

// resolveResearchTarget scopes domain and URL queries; a brand keyword has no
// URL to narrow. A domain the parser rejects keeps the unscoped lookup and
// fails at the provider's own validation, but an explicit scope that does not
// fit the input (a subfolder without a path) is the caller's error.
func resolveResearchTarget(in BrandLookupInput, detected Target) (*ResearchTarget, error) {
	if detected.Type != TargetDomain {
		return nil, nil
	}
	target, err := parseResearchTarget(in.Query, in.Scope)
	if err != nil {
		if in.Scope != "" {
			return nil, err
		}
		return nil, nil
	}
	return &target, nil
}

// blocking reports whether err must fail the whole request: the caller went
// away, or the provider account itself is unusable.
func blocking(ctx context.Context, err error) bool {
	return err != nil && (ctx.Err() != nil || errors.Is(err, dataforseo.ErrBillingIssue))
}

// fetchPlatform gathers one platform's totals, cited pages and prompt sample.
// The three calls are independent, so one failing does not discard the others
// that were already paid for.
func (s *Service) fetchPlatform(ctx context.Context, organizationID, platform string, detected Target, includeSubdomains bool, in BrandLookupInput) (platformBundle, error) {
	q := platformQuery{
		organizationID: organizationID,
		target:         buildLLMTarget(detected, includeSubdomains),
		platform:       platform,
		locationCode:   in.LocationCode,
		languageCode:   in.LanguageCode,
	}
	if platform == platformChatGPT {
		q.locationCode, q.languageCode = chatGPTLocationCode, chatGPTLanguageCode
	}

	aggregated, aggErr := s.provider.aggregatedMetrics(ctx, q, 20)
	if blocking(ctx, aggErr) {
		return platformBundle{}, aggErr
	}
	topPages, pagesErr := s.provider.topPages(ctx, q, topSourcesPerPlatform)
	if blocking(ctx, pagesErr) {
		return platformBundle{}, pagesErr
	}
	mentions, mentionsErr := s.provider.mentionsSearch(ctx, q, mentionsPerPlatform)
	if blocking(ctx, mentionsErr) {
		return platformBundle{}, mentionsErr
	}
	if aggErr != nil && pagesErr != nil && mentionsErr != nil {
		return platformBundle{}, aggErr
	}
	failures := errors.Join(aggErr, pagesErr, mentionsErr)
	if failures != nil {
		s.logger.ErrorContext(ctx, "brand lookup sub-call failed", "platform", platform, "err", failures)
	}
	return platformBundle{aggregated: aggregated, topPages: topPages, mentions: mentions, complete: failures == nil}, nil
}

// fetchCross runs one cross-aggregated call per platform, each comparing the
// target with the competitors. Share of voice always compares domain with
// domain, because the provider has no URL-level targeting, so every group uses
// the same subdomain rule.
func (s *Service) fetchCross(ctx context.Context, organizationID string, detected Target, competitors []competitorGroup, includeSubdomains bool, in BrandLookupInput) ([]crossOutcome, error) {
	groups := []crossGroup{{key: detected.Value, target: buildLLMTarget(detected, includeSubdomains)}}
	for _, c := range competitors {
		groups = append(groups, crossGroup{key: c.label, target: buildLLMTarget(c.detected, includeSubdomains)})
	}

	outcomes := make([]crossOutcome, len(brandPlatforms))
	for i, platform := range brandPlatforms {
		location, lang := in.LocationCode, in.LanguageCode
		if platform == platformChatGPT {
			location, lang = chatGPTLocationCode, chatGPTLanguageCode
		}
		items, err := s.provider.crossAggregatedMetrics(ctx, organizationID, groups, platform, location, lang)
		if blocking(ctx, err) {
			return nil, err
		}
		if err != nil {
			s.logger.ErrorContext(ctx, "brand lookup share of voice failed", "platform", platform, "err", err)
			outcomes[i] = crossOutcome{platform: platform}
			continue
		}
		outcomes[i] = crossOutcome{platform: platform, ok: true, items: items}
	}
	return outcomes, nil
}

// ExplorePrompt asks one prompt of one to four models and returns their
// answers side by side. A model that fails is reported in its own result, so
// one provider outage does not hide the other answers; only an account-level
// failure fails the request.
func (s *Service) ExplorePrompt(ctx context.Context, organizationID, projectID string, in PromptExplorerInput) (PromptExplorerResult, error) {
	results := make([]ModelResult, len(in.Models))
	errs := make([]error, len(in.Models))
	var wg sync.WaitGroup
	for i, model := range in.Models {
		wg.Go(func() {
			results[i], errs[i] = s.runModel(ctx, organizationID, projectID, model, in)
		})
	}
	wg.Wait()

	for _, err := range errs {
		if blocking(ctx, err) {
			return PromptExplorerResult{}, err
		}
	}
	for i, err := range errs {
		if err != nil {
			// The detail stays in the log: upstream error bodies can echo
			// request paths or fields that must not reach the browser.
			s.logger.ErrorContext(ctx, "prompt explorer model failed", "model", in.Models[i], "err", err)
			results[i] = upstreamErrorResult(in.Models[i])
		}
	}

	result := PromptExplorerResult{Prompt: in.Prompt, FetchedAt: formatTime(s.now()), Results: results}
	if in.HighlightBrand != "" {
		result.HighlightBrand = &in.HighlightBrand
	}
	return result, nil
}

func (s *Service) runModel(ctx context.Context, organizationID, projectID, model string, in PromptExplorerInput) (ModelResult, error) {
	country := ""
	if in.WebSearch {
		country = in.WebSearchCountryCode
	}
	if country != "" && !supportsWebSearchCountry(model, country) {
		return unsupportedCountryResult(model, country), nil
	}

	// Part of the cache key, so a model upgrade refetches instead of serving
	// answers from the previous model.
	modelName := s.provider.models.latest(ctx, organizationID, model)
	key := cacheKey("prompt-response", organizationID, projectID, model, modelName,
		// Only whitespace is collapsed. Case is kept: "Compare Go vs go" and
		// case-sensitive code snippets must not collide with lowercase twins.
		strings.Join(strings.Fields(in.Prompt), " "), strconv.FormatBool(in.WebSearch), country)

	var cached ModelSuccess
	if s.cacheGet(ctx, key, &cached) && cached.Status == "success" {
		// The brand is not in the key, so one cached answer serves any brand.
		return cached.withHighlightBrand(in.HighlightBrand), nil
	}

	request := responseRequest{
		organizationID: organizationID,
		model:          model,
		modelName:      modelName,
		prompt:         in.Prompt,
		webSearch:      in.WebSearch,
		countryCode:    country,
		maxTokens:      promptMaxOutputTokens,
	}
	response, err := s.provider.llmResponse(ctx, request)
	if err != nil {
		return nil, err
	}
	// web_search only permits searching, and some models cannot be forced to:
	// they often answer from memory with no citations. The browse decision is
	// random per call, so one paid retry noticeably raises the odds of a cited
	// answer. The retry is kept only if it searched; if it fails, the first
	// answer, already paid for, stands.
	if in.WebSearch && (response.WebSearch == nil || !*response.WebSearch) {
		retried, err := s.provider.llmResponse(ctx, request)
		switch {
		case blocking(ctx, err):
			return nil, err
		case err != nil:
			s.logger.WarnContext(ctx, "prompt explorer web search retry failed", "model", model, "err", err)
		case retried.WebSearch != nil && *retried.WebSearch:
			response = retried
		}
	}

	shaped := shapeSuccess(model, response)
	if country != "" {
		shaped.WebSearchCountryCode = &country
	}
	s.cacheSet(ctx, key, shaped, promptResponseTTL)
	return shaped.withHighlightBrand(in.HighlightBrand), nil
}

// cacheKey is ai-search:<kind>:<organization>:<digest>. The organization is in
// the clear so everything cached for it can be found by prefix and deleted,
// which privacy erasure needs: cached prompts can hold personal data. The
// digest covers every part, each length-prefixed so no input can shift a
// boundary and collide with another set of parts.
func cacheKey(kind, organizationID string, parts ...string) string {
	var b strings.Builder
	for _, part := range append([]string{organizationID}, parts...) {
		b.WriteString(strconv.Itoa(len(part)))
		b.WriteByte(':')
		b.WriteString(part)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return "ai-search:" + kind + ":" + organizationID + ":" + hex.EncodeToString(sum[:])
}

// cacheGet decodes the entry at key into v. The cache is an optimization, so
// a Redis failure or a corrupt entry reads as a miss.
func (s *Service) cacheGet(ctx context.Context, key string, v any) bool {
	raw, err := s.redis.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return false
	}
	if err != nil {
		s.logger.WarnContext(ctx, "read ai-search cache", "err", err)
		return false
	}
	if err := json.Unmarshal(raw, v); err != nil {
		s.logger.WarnContext(ctx, "decode ai-search cache entry", "err", err)
		return false
	}
	return true
}

// cacheSet stores v at key. It outlives a client that disconnected right after
// the paid call, and a failure only costs a repeat call later.
func (s *Service) cacheSet(ctx context.Context, key string, v any, ttl time.Duration) {
	raw, err := json.Marshal(v)
	if err != nil {
		s.logger.ErrorContext(ctx, "encode ai-search cache entry", "err", err)
		return
	}
	setCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cacheWriteTimeout)
	defer cancel()
	if err := s.redis.Set(setCtx, key, raw, ttl).Err(); err != nil {
		s.logger.WarnContext(ctx, "write ai-search cache", "err", err)
	}
}
