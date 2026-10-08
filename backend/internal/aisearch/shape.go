package aisearch

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf16"
)

const (
	topQueriesPerPlatform   = 25
	topSourcesPerPlatform   = 10
	keywordsPerSource       = 50
	maxURLLength            = 2048
	maxTitleLength          = 300
	maxQuestionLength       = 500
	maxBrandEntityLength    = 200
	maxCitedSourcesPerQuery = 10
	maxBrandsPerQuery       = 20
	monthsOfTrend           = 12
)

// platformBundle is everything fetched for one platform. complete is false
// when a sub-call failed and fell back to empty data.
type platformBundle struct {
	aggregated aggregatedTotal
	topPages   []topPagesItem
	mentions   []mentionItem
	complete   bool
}

// platformOutcome is one platform's result; bundle is meaningful only when ok.
type platformOutcome struct {
	platform string
	ok       bool
	bundle   platformBundle
}

// crossOutcome is one platform's share-of-voice call.
type crossOutcome struct {
	platform string
	ok       bool
	items    []crossItem
}

type competitorGroup struct {
	label    string
	detected Target
}

// resolveCompetitorGroups detects each competitor's target type, drops
// duplicates and any that collide with the target. A duplicate group adds a
// redundant leaderboard row and wastes a paid comparison slot. Comparison is
// case-insensitive because the provider matches keywords that way, so "Nike"
// and "nike" would be two paid groups returning the same counts.
func resolveCompetitorGroups(targetValue string, competitors []string) []competitorGroup {
	seen := map[string]bool{strings.ToLower(targetValue): true}
	groups := []competitorGroup{}
	for _, competitor := range competitors {
		detected := detectTarget(competitor)
		key := strings.ToLower(detected.Value)
		if seen[key] {
			continue
		}
		seen[key] = true
		groups = append(groups, competitorGroup{label: detected.Value, detected: detected})
	}
	return groups
}

// computeShareOfVoice sums each requested brand's mentions across the
// platforms that answered. Every requested brand is seeded as a row, so a
// brand the provider returned nothing for shows as "no data" instead of
// vanishing from a leaderboard the user paid to compare it on. Echoed keys are
// matched back case-insensitively, and rows we did not ask for are ignored.
//
// A brand whose mentions are nil has no data and is left out of the
// denominator; a brand with 0 is known-zero and counts. It returns nil when
// there are no competitors or every call failed, so the page omits the section.
func computeShareOfVoice(outcomes []crossOutcome, targetKey string, competitorKeys []string) *ShareOfVoice {
	if len(competitorKeys) == 0 {
		return nil
	}
	var platforms []string
	var successful []crossOutcome
	for _, outcome := range outcomes {
		if outcome.ok {
			successful = append(successful, outcome)
			platforms = append(platforms, outcome.platform)
		}
	}
	if len(successful) == 0 {
		return nil
	}

	type row struct {
		label    string
		mentions *int
	}
	var rows []*row
	byKey := map[string]*row{}
	for _, key := range append([]string{targetKey}, competitorKeys...) {
		lower := strings.ToLower(key)
		if byKey[lower] != nil {
			continue
		}
		r := &row{label: key}
		byKey[lower] = r
		rows = append(rows, r)
	}

	for _, outcome := range successful {
		for _, item := range outcome.items {
			if item.Key == nil {
				continue
			}
			// The provider should echo only the keys we sent; extras must not
			// change the requested brands' shares.
			r := byKey[strings.ToLower(*item.Key)]
			if r == nil {
				continue
			}
			counts := make([]*int, len(item.Platform))
			for i, entry := range item.Platform {
				counts[i] = roundOrNil(entry.Mentions)
			}
			r.mentions = sumNullable(r.mentions, sumNullable(counts...))
		}
	}

	denominator := 0
	for _, r := range rows {
		if r.mentions != nil {
			denominator += *r.mentions
		}
	}
	targetLower := strings.ToLower(targetKey)
	entries := make([]ShareOfVoiceEntry, len(rows))
	for i, r := range rows {
		entries[i] = ShareOfVoiceEntry{
			Label:    r.label,
			IsTarget: strings.ToLower(r.label) == targetLower,
			Mentions: r.mentions,
		}
		if r.mentions != nil && denominator > 0 {
			share := float64(*r.mentions) / float64(denominator) * 100
			entries[i].SharePct = &share
		}
	}
	slices.SortStableFunc(entries, func(a, b ShareOfVoiceEntry) int {
		return cmp.Compare(valueOr(b.Mentions, -1), valueOr(a.Mentions, -1))
	})
	return &ShareOfVoice{Platforms: platforms, Entries: entries}
}

// sumNullable adds the non-nil values; it is nil when there are none.
func sumNullable(values ...*int) *int {
	var total int
	var found bool
	for _, v := range values {
		if v != nil {
			total += *v
			found = true
		}
	}
	if !found {
		return nil
	}
	return &total
}

// roundOrNil rounds a provider number to a whole count.
func roundOrNil(v *float64) *int {
	if v == nil {
		return nil
	}
	n := int(math.Round(*v))
	return &n
}

func valueOr(v *int, fallback int) int {
	if v == nil {
		return fallback
	}
	return *v
}

// truncate cuts s to at most limit UTF-16 units, which is how the web app's
// schemas count length, without splitting a character.
func truncate(s string, limit int) string {
	n := 0
	for i, r := range s {
		n += utf16.RuneLen(r)
		if n > limit {
			return s[:i]
		}
	}
	return s
}

// utf16Len is the length JavaScript clients and the provider's limits count.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

func stringPtr(s string) *string { return &s }

func derefOr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}

// pageSource keeps only well-formed http(s) URLs of a sane length.
func pageSource(raw *string) (string, bool) {
	if raw == nil {
		return "", false
	}
	u, ok := safeHTTPURL(*raw)
	return u, ok && utf16Len(u) <= maxURLLength
}

func sourceKey(platform, pageURL string) string { return platform + "::" + pageURL }

// promptExamples collects, per cited page, the distinct prompts that cited it
// in the mentions sample, in first-seen order.
type promptExamples map[string][]PromptExample

func buildPromptExamples(bundles []successBundle) promptExamples {
	examples := promptExamples{}
	seen := map[string]map[string]bool{}
	for _, b := range bundles {
		for _, mention := range b.bundle.mentions {
			question := ""
			if mention.Question != nil {
				question = truncate(*mention.Question, maxQuestionLength)
			}
			if question == "" {
				continue
			}
			volume := roundOrNil(mention.AISearchVolume)
			for _, source := range mention.Sources {
				pageURL, ok := safeHTTPURL(derefOr(source.URL, ""))
				if !ok {
					continue
				}
				key := sourceKey(b.platform, pageURL)
				if seen[key] == nil {
					seen[key] = map[string]bool{}
				}
				if seen[key][question] {
					continue
				}
				seen[key][question] = true
				examples[key] = append(examples[key], PromptExample{Question: question, AISearchVolume: volume})
			}
		}
	}
	return examples
}

type successBundle struct {
	platform string
	bundle   platformBundle
}

// deriveCitedSources ranks cited pages from the provider's top_pages data and
// attaches example prompts from the mentions sample when that exact page
// appears there. Page metrics stay authoritative; the examples are plainly
// sample-based.
//
// pageFilter narrows the rows to a URL-scoped target (the provider has no
// URL-level targeting). It runs before the per-platform cap so in-scope pages
// are not crowded out by out-of-scope ones.
func deriveCitedSources(bundles []successBundle, pageFilter *ResearchTarget) []TopPage {
	examples := buildPromptExamples(bundles)

	var platformOrder []string
	byPlatform := map[string][]TopPage{}
	for _, b := range bundles {
		for _, page := range b.bundle.topPages {
			pageURL, ok := pageSource(page.Key)
			if !ok || (pageFilter != nil && !pageFilter.matches(pageURL)) {
				continue
			}
			var group groupElement
			for _, entry := range page.Platform {
				if entry.Key != nil && *entry.Key == b.platform {
					group = entry
					break
				}
			}
			keywords := slices.Clone(examples[sourceKey(b.platform, pageURL)])
			slices.SortStableFunc(keywords, func(x, y PromptExample) int {
				return cmp.Compare(valueOr(y.AISearchVolume, 0), valueOr(x.AISearchVolume, 0))
			})
			if len(keywords) > keywordsPerSource {
				keywords = keywords[:keywordsPerSource]
			}
			if keywords == nil {
				keywords = []PromptExample{}
			}
			if _, ok := byPlatform[b.platform]; !ok {
				platformOrder = append(platformOrder, b.platform)
			}
			byPlatform[b.platform] = append(byPlatform[b.platform], TopPage{
				URL:            pageURL,
				Domain:         safeHostname(pageURL),
				Platform:       b.platform,
				Mentions:       roundOrNil(group.Mentions),
				CapturedVolume: roundOrNil(group.AISearchVolume),
				Keywords:       keywords,
			})
		}
	}

	// Keep the top sources per platform so a high-volume platform cannot crowd
	// out a sparse one entirely, then order the combined set by volume.
	capped := []TopPage{}
	for _, platform := range platformOrder {
		rows := byPlatform[platform]
		slices.SortStableFunc(rows, func(x, y TopPage) int {
			return cmp.Compare(valueOr(y.CapturedVolume, 0), valueOr(x.CapturedVolume, 0))
		})
		capped = append(capped, rows[:min(len(rows), topSourcesPerPlatform)]...)
	}
	slices.SortStableFunc(capped, func(x, y TopPage) int {
		return cmp.Or(
			cmp.Compare(valueOr(y.CapturedVolume, 0), valueOr(x.CapturedVolume, 0)),
			cmp.Compare(valueOr(y.Mentions, 0), valueOr(x.Mentions, 0)),
		)
	})
	return capped
}

type shapeArgs struct {
	query          string
	detected       Target
	research       *ResearchTarget
	platforms      []platformOutcome
	cross          []crossOutcome
	competitorKeys []string
	locationCode   int
	languageCode   string
	now            time.Time
}

// shapeResult turns the provider data into the page's result.
func shapeResult(args shapeArgs) BrandLookupResult {
	var pageFilter *ResearchTarget
	if args.research != nil && args.research.Scope.usesPath() {
		pageFilter = args.research
	}
	var successful []successBundle
	for _, outcome := range args.platforms {
		if outcome.ok {
			successful = append(successful, successBundle{platform: outcome.platform, bundle: outcome.bundle})
		}
	}

	language, _, _ := strings.Cut(strings.ToLower(strings.ReplaceAll(args.languageCode, "_", "-")), "-")
	chatGPTLocale := args.locationCode == chatGPTLocationCode && language == chatGPTLanguageCode

	perPlatform := make([]PlatformBreakdown, len(args.platforms))
	var mentions, volumes []*int
	for i, outcome := range args.platforms {
		row := PlatformBreakdown{Platform: outcome.platform, Status: "error"}
		if outcome.ok {
			row.Status = "success"
			for _, group := range outcome.bundle.aggregated.Platform {
				if group.Key != nil && *group.Key == outcome.platform {
					row.Mentions = roundOrNil(group.Mentions)
					row.AISearchVolume = roundOrNil(group.AISearchVolume)
					break
				}
			}
		}
		perPlatform[i] = row
		if chatGPTLocale || outcome.platform != platformChatGPT {
			mentions = append(mentions, row.Mentions)
			volumes = append(volumes, row.AISearchVolume)
		}
	}
	totalMentions := sumNullable(mentions...)

	topPages := deriveCitedSources(successful, pageFilter)
	topQueries := shapeTopQueries(successful, pageFilter)
	trend := successful
	cross := args.cross
	if !chatGPTLocale {
		trend = slices.DeleteFunc(slices.Clone(successful), func(b successBundle) bool { return b.platform == platformChatGPT })
		cross = slices.DeleteFunc(slices.Clone(args.cross), func(c crossOutcome) bool { return c.platform == platformChatGPT })
	}
	monthly := aggregateMonthlyVolume(trend)
	shareOfVoice := computeShareOfVoice(cross, args.detected.Value, args.competitorKeys)

	hasData := valueOr(totalMentions, 0) > 0 || len(topPages) > 0 || len(topQueries) > 0 || len(monthly) > 0 ||
		(shareOfVoice != nil && slices.ContainsFunc(shareOfVoice.Entries, func(e ShareOfVoiceEntry) bool { return e.Mentions != nil }))

	result := BrandLookupResult{
		Query:                    args.query,
		DetectedTargetType:       string(args.detected.Type),
		ResolvedTarget:           args.detected.Value,
		AggregatesAreDomainLevel: pageFilter != nil,
		FetchedAt:                formatTime(args.now),
		HasData:                  hasData,
		TotalMentions:            totalMentions,
		TotalAISearchVolume:      sumNullable(volumes...),
		PerPlatform:              perPlatform,
		ShareOfVoice:             shareOfVoice,
		TopPages:                 topPages,
		TopQueries:               topQueries,
		MonthlyVolume:            monthly,
	}
	if args.research != nil {
		result.ResolvedTarget = args.research.Display
		scope := args.research.Scope
		result.Scope = &scope
	}
	return result
}

// shapeTopQueries lists the prompts that mentioned the target, most searched
// first, capped per platform.
func shapeTopQueries(bundles []successBundle, pageFilter *ResearchTarget) []TopQuery {
	byVolume := func(x, y TopQuery) int {
		return cmp.Compare(valueOr(y.AISearchVolume, 0), valueOr(x.AISearchVolume, 0))
	}
	all := []TopQuery{}
	for _, b := range bundles {
		var queries []TopQuery
		for _, item := range b.bundle.mentions {
			if item.Question == nil || *item.Question == "" {
				continue
			}
			// Under a URL scope a prompt only counts when its answer cited a
			// page in scope: a brand named in the answer text is domain-level
			// evidence that must not be attributed to the page.
			if pageFilter != nil && !slices.ContainsFunc(item.Sources, func(s mentionSource) bool {
				pageURL, ok := safeHTTPURL(derefOr(s.URL, ""))
				return ok && pageFilter.matches(pageURL)
			}) {
				continue
			}
			queries = append(queries, TopQuery{
				Question:        truncate(*item.Question, maxQuestionLength),
				Platform:        b.platform,
				AISearchVolume:  roundOrNil(item.AISearchVolume),
				FirstSeenAt:     item.FirstResponseAt,
				LastSeenAt:      item.LastResponseAt,
				CitedSources:    shapeQuerySources(item),
				BrandsMentioned: shapeBrands(item),
			})
		}
		slices.SortStableFunc(queries, byVolume)
		all = append(all, queries[:min(len(queries), topQueriesPerPlatform)]...)
	}
	slices.SortStableFunc(all, byVolume)
	return all
}

func shapeQuerySources(item mentionItem) []CitedSource {
	sources := []CitedSource{}
	for _, source := range item.Sources {
		pageURL, ok := pageSource(source.URL)
		if !ok {
			continue
		}
		cited := CitedSource{URL: pageURL, Domain: safeHostname(pageURL)}
		if source.Title != nil {
			cited.Title = stringPtr(truncate(*source.Title, maxTitleLength))
		}
		sources = append(sources, cited)
	}
	return sources[:min(len(sources), maxCitedSourcesPerQuery)]
}

func shapeBrands(item mentionItem) []string {
	brands := []string{}
	for _, entity := range item.BrandEntities {
		if title := derefOr(entity.Title, ""); title != "" {
			brands = append(brands, truncate(title, maxBrandEntityLength))
		}
	}
	return brands[:min(len(brands), maxBrandsPerQuery)]
}

// aggregateMonthlyVolume sums each month's volume over every platform and
// returns the latest twelve months, oldest first.
func aggregateMonthlyVolume(bundles []successBundle) []MonthlyVolume {
	type month struct{ year, month int }
	totals := map[month]float64{}
	for _, b := range bundles {
		for _, item := range b.bundle.mentions {
			for _, m := range item.MonthlySearches {
				if m.SearchVolume != nil {
					totals[month{m.Year, m.Month}] += *m.SearchVolume
				}
			}
		}
	}
	volumes := make([]MonthlyVolume, 0, len(totals))
	for key, total := range totals {
		volumes = append(volumes, MonthlyVolume{Year: key.year, Month: key.month, Volume: int(math.Round(total))})
	}
	slices.SortFunc(volumes, func(x, y MonthlyVolume) int {
		return cmp.Or(cmp.Compare(x.Year, y.Year), cmp.Compare(x.Month, y.Month))
	})
	return volumes[max(0, len(volumes)-monthsOfTrend):]
}
