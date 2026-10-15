// Package linksuggest suggests internal links from the project's own pages
// for content-editor drafts. Sources are the project's Search Console data
// and, without it, the latest site audit's crawled pages. No paid provider
// is ever called.
package linksuggest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	// MaxSuggestions is the most suggestions one request returns.
	MaxSuggestions = 10
	// MaxDraftRunes bounds how much draft text is scanned, so a
	// 150,000-character draft (the editor's limit) stays bounded in time.
	MaxDraftRunes = 50_000
	// MaxSegments bounds the draft segments scanned, so a pathological draft
	// of tiny lines cannot stretch the matching pass.
	MaxSegments = 2_000
	// MaxSourcePages bounds how many pages one request may consider.
	MaxSourcePages = 200
	// cacheTTL keeps repeated saves on the same draft from re-querying GSC.
	cacheTTL = 10 * time.Minute
	// minAnchorRunes is the shortest anchor text usable in an editor.
	minAnchorRunes = 4
	// maxAnchorRunes keeps suggested anchors readable.
	maxAnchorRunes = 80
)

// ErrEmpty means no page matched the draft: an empty list goes to the user,
// not an error.
var ErrEmpty = errors.New("no link suggestions")

// ValidationError describes input the client must correct.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

// Input is what the editor sends: the draft it is writing and enough
// identity fields to exclude the article's own URL.
type Input struct {
	Content string `json:"content"`
	Topic   string `json:"targetTopic"`
	Title   string `json:"title"`
	Slug    string `json:"slug"`
}

// Suggestion is one internal link for the draft.
type Suggestion struct {
	// URL is the page to link to, normalized to https.
	URL string `json:"url"`
	// Title is the page's title as the source reported it.
	Title string `json:"title"`
	// Anchor is the exact phrase from the draft to turn into the link text.
	Anchor string `json:"anchor"`
	// Reason explains the match in a line the writer can judge.
	Reason string `json:"reason"`
	// Clicks and Impressions come from Search Console; an audit page carries
	// none.
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
}

// Source is one origin of the project's own pages.
type Source interface {
	// Pages returns the project's known pages, best traffic first. It is
	// project-scoped by contract: a caller may never see another
	// workspace's pages.
	Pages(ctx context.Context, organizationID, projectID string) ([]SourcePage, error)
}

// SourcePage is one page candidate with what the source knows about it.
type SourcePage struct {
	URL         string
	Title       string
	Clicks      float64
	Impressions float64
	// Queries are the search terms linked to this page, best first.
	Queries []string
}

// Service holds the link suggestion rules. Sources are tried in order; the
// first one that returns pages wins, so a workspace without Search Console
// still gets suggestions from its latest audit.
type Service struct {
	Sources []Source
	// Redis caches answers per draft for cacheTTL. Nil disables caching, and
	// a Redis failure never fails a request: the data is free to recompute.
	Redis  redisClient
	Logger *slog.Logger
	// Now overrides the clock in tests.
	Now func() time.Time
}

// redisClient is the subset of the Redis client the cache needs.
type redisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

// Suggestions returns up to MaxSuggestions internal links for the draft.
func (s *Service) Suggestions(ctx context.Context, organizationID, projectID string, input Input) ([]Suggestion, error) {
	if strings.TrimSpace(input.Content) == "" {
		return nil, &ValidationError{Message: "Send the draft text to scan for link anchors."}
	}
	if len([]rune(input.Topic)) > 200 || len([]rune(input.Title)) > MaxTitleRunes || len([]rune(input.Slug)) > MaxSlugRunes {
		return nil, &ValidationError{Message: "The title, topic or slug is too long."}
	}
	if s.Redis != nil {
		key, err := cacheKey(projectID, input)
		if err != nil {
			s.logWarn(ctx, "build link suggestion cache key", err)
			return s.compute(ctx, organizationID, projectID, input)
		}
		if cached, err := s.Redis.Get(ctx, key); err == nil && cached != "" {
			answer := decodeCache(cached)
			if answer != nil {
				return answer, nil
			}
		} else if err != nil && !errors.Is(err, errCacheMiss) {
			s.logWarn(ctx, "load cached link suggestions", err)
		}
		answer, err := s.compute(ctx, organizationID, projectID, input)
		if err != nil {
			return nil, err
		}
		if err := s.Redis.Set(ctx, key, encodeCache(answer), cacheTTL); err != nil {
			s.logWarn(ctx, "store cached link suggestions", err)
		}
		return answer, nil
	}
	return s.compute(ctx, organizationID, projectID, input)
}

func (s *Service) compute(ctx context.Context, organizationID, projectID string, input Input) ([]Suggestion, error) {
	for _, source := range s.Sources {
		pages, err := source.Pages(ctx, organizationID, projectID)
		if err != nil {
			s.logWarn(ctx, "load internal link source", err)
			continue
		}
		if len(pages) == 0 {
			continue
		}
		return match(pages, input), nil
	}
	return []Suggestion{}, nil
}

// match is the whole suggestion engine, pure and deterministic so tests can
// pin it down.
func match(pages []SourcePage, input Input) []Suggestion {
	segments := draftSegments(input.Content)
	if len(segments) == 0 {
		return []Suggestion{}
	}
	slug := normalizeSlug(input.Slug)
	topicWords := uniqueWords(input.Topic)
	candidates := make([]Suggestion, 0, len(pages))
	seen := map[string]bool{}
	for _, page := range pages {
		pageURL := strings.TrimSpace(page.URL)
		if pageURL == "" || ownURL(pageURL, slug) || seen[pageURL] {
			continue
		}
		seen[pageURL] = true
		pageWords := uniqueWords(page.Title)
		// A page whose title carries none of the draft's wording is not worth
		// linking from this draft, whatever its traffic.
		if len(pageWords) == 0 {
			continue
		}
		best := bestSegment(segments, pageWords)
		if best.anchor == "" {
			continue
		}
		reason := "This page's title matches this paragraph."
		if query := sharedQuery(best.queryWords, page.Queries, input.Topic); query != "" {
			reason = fmt.Sprintf("People find this page with \u201c%s\u201d.", query)
		} else if len(topicWords) > 0 && overlap(wordSet(topicWords), pageWords) > 0 {
			reason = "This page sits in the same target topic."
		}
		candidates = append(candidates, Suggestion{
			URL: pageURL, Title: strings.TrimSpace(page.Title), Anchor: best.anchor, Reason: reason,
			Clicks: page.Clicks, Impressions: page.Impressions,
		})
	}
	// Rank by the traffic the source reports: clicks first, then
	// impressions, then URL for a stable order.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && lessRank(candidates[j], candidates[j-1]); j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
	if len(candidates) > MaxSuggestions {
		candidates = candidates[:MaxSuggestions]
	}
	return candidates
}

// logWarn records a degraded-but-survivable condition: every free source
// failure above falls back instead of breaking the request.
func (s *Service) logWarn(ctx context.Context, message string, err error) {
	if s.Logger == nil {
		return
	}
	s.Logger.WarnContext(ctx, message, "err", err)
}

func lessRank(a, b Suggestion) bool {
	if a.Clicks != b.Clicks {
		return a.Clicks > b.Clicks
	}
	if a.Impressions != b.Impressions {
		return a.Impressions > b.Impressions
	}
	return a.URL < b.URL
}

// ownURL refuses the article's own address: linking a draft to itself is a
// classic SEO mistake.
func ownURL(pageURL, slug string) bool {
	if slug == "" {
		return false
	}
	trimmed := strings.TrimRight(strings.ToLower(strings.TrimSpace(pageURL)), "/")
	return strings.HasSuffix(trimmed, "/"+slug)
}

// draftSegments splits the draft into the phrases an anchor may come from:
// heading text and paragraph lines. Content is bounded at MaxDraftRunes
// runes so a full editor limit stays fast.
func draftSegments(content string) []string {
	runes := []rune(content)
	if len(runes) > MaxDraftRunes {
		runes = runes[:MaxDraftRunes]
	}
	lines := strings.Split(string(runes), "\n")
	segments := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimLeft(line, "#>*-| "))
		if line == "" || len([]rune(line)) < minAnchorRunes {
			continue
		}
		segments = append(segments, truncateRunes(line, maxAnchorRunes))
		if len(segments) >= MaxSegments {
			break
		}
	}
	return segments
}

// bestSegment finds one draft phrase that shares wording with the page
// title.
func bestSegment(segments []string, pageWords []string) (matched segmentMatch) {
	pagesSet := wordSet(pageWords)
	for _, segment := range segments {
		segmentWords := uniqueWords(segment)
		shared := overlap(pagesSet, segmentWords)
		if shared < 2 {
			continue
		}
		matched.anchor = segment
		matched.queryWords = segmentWords
		return matched
	}
	return matched
}

type segmentMatch struct {
	anchor     string
	queryWords []string
}

// sharedQuery returns the first query that shares wording with the matched
// segment (or the target topic), for the "why it matched" line.
func sharedQuery(segmentWords []string, queries []string, topic string) string {
	wordsWithTopic := append(segmentWords, uniqueWords(topic)...)
	for _, query := range queries {
		queryWords := uniqueWords(query)
		if len(wordsWithTopic) == 0 || len(queryWords) == 0 {
			continue
		}
		if overlap(wordSet(wordsWithTopic), queryWords) > 0 {
			return query
		}
	}
	return ""
}

func overlap(set map[string]struct{}, words []string) int {
	shared := 0
	seen := map[string]struct{}{}
	for _, word := range words {
		if _, ok := set[word]; !ok {
			continue
		}
		if _, dup := seen[word]; dup {
			continue
		}
		seen[word] = struct{}{}
		shared++
	}
	return shared
}

func wordSet(words []string) map[string]struct{} {
	set := make(map[string]struct{}, len(words))
	for _, word := range words {
		set[word] = struct{}{}
	}
	return set
}

// MaxTitleRunes and MaxSlugRunes bound the input identity fields.
const (
	MaxTitleRunes = 400
	MaxSlugRunes  = 300
)

// normalizeSlug trims a URL slug to the form the editor produces.
func normalizeSlug(slug string) string {
	value := strings.ToLower(strings.TrimSpace(strings.Trim(slug, "/-")))
	if len([]rune(value)) > MaxSlugRunes {
		value = string([]rune(value)[:MaxSlugRunes])
	}
	return value
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	cut := string(runes[:limit])
	// End at a word boundary when one exists, or the anchor looks broken.
	if space := strings.LastIndex(cut, " "); space > minAnchorRunes {
		cut = cut[:space]
	}
	return strings.TrimRight(cut, ",.;:\u2014-")
}
