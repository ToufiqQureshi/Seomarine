package contenteditor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/market"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
)

// Brief limits. The ceiling is the worst case at DataForSEO's list prices:
// a depth-30 live SERP ($0.002 per 10 results = $0.006) plus eight content
// parses ($0.00015 each = $0.0012). About 9.2 credits; the customer is
// charged what the provider reports, never more than this.
const (
	briefCacheTTL        = 30 * 24 * time.Hour
	briefDepth           = 30
	briefParses          = 8
	briefMinParsed       = 3
	briefCeiling         = 0.006 + briefParses*0.00015
	briefMaxQuestions    = 100
	briefMaxHeadings     = 100
	briefMaxHeadingRunes = 1_000
	briefMaxTextRunes    = 20_000
)

var (
	// ErrBriefUnavailable means no brief can be built: the provider is not
	// configured, or too few competitor pages could be read.
	ErrBriefUnavailable = errors.New("SERP brief is unavailable")
	briefOperator       = regexp.MustCompile(`(?i)(allinanchor|allintext|allintitle|allinurl|cache|define|definition|filetype|id|inanchor|info|intext|intitle|inurl|link|site):`)
)

// BriefInput selects one search query and market. OwnDomain is used only when
// shaping the response and is deliberately excluded from the shared cache key.
type BriefInput struct {
	Keyword      string `json:"keyword"`
	LocationCode int    `json:"locationCode"`
	LanguageCode string `json:"languageCode"`
	OwnDomain    string `json:"-"` // Set only from the authorized project's saved domain.
}

// ContentBrief contains provider evidence used by the editor and writer.
type ContentBrief struct {
	Keyword      string            `json:"keyword"`
	LocationCode int               `json:"locationCode"`
	LanguageCode string            `json:"languageCode"`
	FetchedAt    time.Time         `json:"fetchedAt"`
	Questions    []string          `json:"questions"`
	Competitors  []BriefCompetitor `json:"competitors"`
	Terms        []BriefTerm       `json:"terms"`
	Subtopics    []string          `json:"subtopics"`
}

// BriefTerm is a phrase most competitor pages use, with the per-page use
// range (25th to 75th percentile) among the pages that use it.
type BriefTerm struct {
	Term    string `json:"term"`
	Min     int    `json:"min"`
	Max     int    `json:"max"`
	Domains int    `json:"domains"`
}

// BriefCompetitor is an organic result and its parsed headings.
type BriefCompetitor struct {
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	Headings  []string `json:"headings"`
	WordCount int      `json:"wordCount"`
	// Text is the page's main content, cut to 20,000 characters. The editor
	// uses it for article length and the copied-text check.
	Text string `json:"text"`
}

// BriefProvider performs the bounded DataForSEO request sequence.
type BriefProvider interface {
	Fetch(context.Context, string, BriefInput) (ContentBrief, error)
}

// BriefCache serializes cache misses by key across all application instances.
// With refresh set it rebuilds even when an unexpired entry exists.
type BriefCache interface {
	GetOrBuild(ctx context.Context, key string, refresh bool, build func() (ContentBrief, error)) (ContentBrief, error)
}

// BriefService validates requests, uses a shared cache, and filters a site's
// own URLs only after reading the cross-workspace cached result.
type BriefService struct {
	Cache    BriefCache
	Provider BriefProvider
}

// Get returns a shared 30-day SERP brief, fetching only on a cache miss or
// when the writer asks to refresh it. Only the fetching workspace pays.
func (s BriefService) Get(ctx context.Context, organizationID string, input BriefInput, refresh bool) (ContentBrief, error) {
	if strings.TrimSpace(organizationID) == "" {
		return ContentBrief{}, &ValidationError{Message: "An organization is required."}
	}
	clean, key, ownDomain, err := validateBriefInput(input)
	if err != nil {
		return ContentBrief{}, err
	}
	if s.Cache == nil || s.Provider == nil {
		return ContentBrief{}, ErrBriefUnavailable
	}
	brief, err := s.Cache.GetOrBuild(ctx, key, refresh, func() (ContentBrief, error) {
		result, fetchErr := s.Provider.Fetch(ctx, organizationID, clean)
		if fetchErr != nil {
			return ContentBrief{}, fetchErr
		}
		if err := validateBriefResult(result); err != nil {
			return ContentBrief{}, err
		}
		result.Keyword = clean.Keyword
		result.LocationCode = clean.LocationCode
		result.LanguageCode = clean.LanguageCode
		result.FetchedAt = time.Now().UTC()
		return result, nil
	})
	if err != nil {
		return ContentBrief{}, err
	}
	// Clone first: DeleteFunc edits in place, and the cached brief is shared.
	brief.Competitors = slices.DeleteFunc(slices.Clone(brief.Competitors), func(item BriefCompetitor) bool {
		return ownDomain != "" && sameOrSubdomain(hostFromURL(item.URL), ownDomain)
	})
	return brief, nil
}

func validateBriefInput(input BriefInput) (BriefInput, string, string, error) {
	keyword := strings.Join(strings.Fields(strings.TrimSpace(input.Keyword)), " ")
	if keyword == "" || utf8.RuneCountInString(keyword) > MaxKeywordLength || strings.ContainsAny(keyword, "\r\n\x00") || briefOperator.MatchString(keyword) {
		return BriefInput{}, "", "", &ValidationError{Message: "Enter one keyword up to 120 characters without search operators."}
	}
	if _, ok := market.Lookup(input.LocationCode); !ok {
		return BriefInput{}, "", "", &ValidationError{Message: "Choose a supported search location."}
	}
	language := strings.ToLower(strings.TrimSpace(input.LanguageCode))
	if len(language) < 2 || len(language) > 8 || !market.IsSupportedLanguageCode(language) {
		return BriefInput{}, "", "", &ValidationError{Message: "Choose a supported search language."}
	}
	if !market.IsLanguageServedForLocation(input.LocationCode, language) {
		return BriefInput{}, "", "", &ValidationError{Message: "That language is not available for this search location."}
	}
	ownDomain, err := normalizeBriefDomain(input.OwnDomain)
	if err != nil {
		return BriefInput{}, "", "", &ValidationError{Message: "Enter a valid website domain."}
	}
	clean := BriefInput{Keyword: keyword, LocationCode: input.LocationCode, LanguageCode: language, OwnDomain: ownDomain}
	keyBytes, _ := json.Marshal([]any{strings.ToLower(keyword), input.LocationCode, language})
	digest := sha256.Sum256(keyBytes)
	return clean, hex.EncodeToString(digest[:]), ownDomain, nil
}

func normalizeBriefDomain(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("invalid domain")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if net.ParseIP(host) == nil && (strings.Contains(host, "..") || !strings.Contains(host, ".")) {
		return "", errors.New("invalid domain")
	}
	return strings.TrimPrefix(host, "www."), nil
}

func hostFromURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(parsed.Hostname()), "."), "www.")
}

func sameOrSubdomain(host, domain string) bool {
	return host != "" && (host == domain || strings.HasSuffix(host, "."+domain))
}

func validateBriefResult(result ContentBrief) error {
	if len(result.Questions) > briefMaxQuestions || len(result.Competitors) > briefParses || len(result.Terms) > briefMaxTerms || len(result.Subtopics) > briefMaxSubtopics {
		return errors.New("SERP brief result exceeded the supported limits")
	}
	for _, question := range result.Questions {
		if strings.TrimSpace(question) == "" || utf8.RuneCountInString(question) > 500 {
			return errors.New("SERP brief returned an invalid question")
		}
	}
	for _, competitor := range result.Competitors {
		if hostFromURL(competitor.URL) == "" || utf8.RuneCountInString(competitor.Title) > 1_000 || len(competitor.Headings) > briefMaxHeadings || utf8.RuneCountInString(competitor.Text) > briefMaxTextRunes || competitor.WordCount < 0 {
			return errors.New("SERP brief returned invalid competitor data")
		}
		for _, heading := range competitor.Headings {
			if utf8.RuneCountInString(heading) > briefMaxHeadingRunes {
				return errors.New("SERP brief returned an oversized competitor heading")
			}
		}
	}
	return nil
}

// DataForSEOBriefProvider fetches one depth-30 SERP and parses up to eight
// organic competitors under one customer-credit hold.
//
// Live content parsing takes exactly one task per call (DataForSEO docs), so
// each page is its own request. A page that cannot be parsed is skipped; the
// brief needs at least briefMinParsed pages to say anything useful.
type DataForSEOBriefProvider struct {
	Client interface {
		DoBudgeted(context.Context, string, spend.Feature, float64, func(dataforseo.DoBudgetedRequest) error) error
	}
}

// Fetch implements BriefProvider.
func (p DataForSEOBriefProvider) Fetch(ctx context.Context, organizationID string, input BriefInput) (ContentBrief, error) {
	if p.Client == nil {
		return ContentBrief{}, ErrBriefUnavailable
	}
	var brief ContentBrief
	brief.Keyword, brief.LocationCode, brief.LanguageCode = input.Keyword, input.LocationCode, input.LanguageCode
	err := p.Client.DoBudgeted(ctx, organizationID, spend.KeywordResearch, briefCeiling, func(request dataforseo.DoBudgetedRequest) error {
		serpBody, err := json.Marshal([]map[string]any{{"keyword": input.Keyword, "location_code": input.LocationCode, "language_code": input.LanguageCode, "depth": briefDepth}})
		if err != nil {
			return fmt.Errorf("encode SERP brief request: %w", err)
		}
		serpPayload, err := request(http.MethodPost, "/v3/serp/google/organic/live/advanced", serpBody, false)
		if err != nil {
			return err
		}
		serpResult, err := firstTaskResult(serpPayload)
		if err != nil {
			return err
		}
		brief.Questions = extractBriefQuestions(serpResult)
		urls := make([]string, 0, briefParses)
		for _, item := range extractOrganicItems(serpResult) {
			link, _ := item["url"].(string)
			if hostFromURL(link) == "" || slices.Contains(urls, link) {
				continue
			}
			urls = append(urls, link)
			if len(urls) == briefParses {
				break
			}
		}
		for _, link := range urls {
			if err := ctx.Err(); err != nil {
				return err
			}
			body, err := json.Marshal([]map[string]any{{"url": link, "enable_javascript": false}})
			if err != nil {
				return fmt.Errorf("encode competitor parsing request: %w", err)
			}
			payload, err := request(http.MethodPost, "/v3/on_page/content_parsing/live", body, false)
			if err != nil {
				continue // One unreadable page must not lose the whole brief.
			}
			competitor, ok := parseCompetitorPage(payload, link)
			if ok {
				brief.Competitors = append(brief.Competitors, competitor)
			}
		}
		if len(brief.Competitors) < briefMinParsed {
			return fmt.Errorf("%w: only %d of %d competitor pages could be read", ErrBriefUnavailable, len(brief.Competitors), len(urls))
		}
		brief.Terms = briefTerms(brief.Competitors)
		brief.Subtopics = briefSubtopics(brief.Competitors)
		return nil
	})
	if recording, ok := errors.AsType[*dataforseo.CostRecordingError](err); ok && errors.Is(err, recording) && len(brief.Competitors) >= briefMinParsed {
		// The work succeeded and only recording its cost failed; billing
		// reconciliation reports it. The paid-for brief is still returned.
		err = nil
	}
	if err != nil {
		return ContentBrief{}, fmt.Errorf("fetch SERP brief: %w", err)
	}
	return brief, nil
}

func firstTaskResult(payload []byte) (json.RawMessage, error) {
	var envelope struct {
		StatusCode int `json:"status_code"`
		Tasks      []struct {
			StatusCode int               `json:"status_code"`
			Result     []json.RawMessage `json:"result"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.StatusCode != 20000 || len(envelope.Tasks) == 0 || envelope.Tasks[0].StatusCode != 20000 || len(envelope.Tasks[0].Result) == 0 {
		return nil, errors.New("DataForSEO returned an invalid SERP brief response")
	}
	return envelope.Tasks[0].Result[0], nil
}

func extractOrganicItems(value json.RawMessage) []map[string]any {
	var result struct {
		Items []map[string]any `json:"items"`
	}
	if json.Unmarshal(value, &result) != nil {
		return nil
	}
	items := []map[string]any{}
	for _, item := range result.Items {
		if item["type"] == "organic" {
			items = append(items, item)
		}
	}
	return items
}

func extractBriefQuestions(value json.RawMessage) []string {
	var root any
	if json.Unmarshal(value, &root) != nil {
		return nil
	}
	questions := []string{}
	seen := map[string]bool{}
	var walk func(any, bool)
	walk = func(node any, inPAA bool) {
		switch current := node.(type) {
		case map[string]any:
			if current["type"] == "people_also_ask" {
				inPAA = true
			}
			if inPAA {
				if question, ok := current["question"].(string); ok {
					questions = addBriefQuestion(questions, seen, question)
				}
				if question, ok := current["title"].(string); ok && !strings.EqualFold(strings.TrimSpace(question), "people also ask") {
					questions = addBriefQuestion(questions, seen, question)
				}
			}
			for _, child := range current {
				walk(child, inPAA)
			}
		case []any:
			for _, child := range current {
				walk(child, inPAA)
			}
		}
	}
	walk(root, false)
	return questions
}

func addBriefQuestion(questions []string, seen map[string]bool, value string) []string {
	value = strings.TrimSpace(value)
	key := strings.ToLower(value)
	if value != "" && utf8.RuneCountInString(value) <= 500 && !seen[key] && len(questions) < briefMaxQuestions {
		seen[key] = true
		questions = append(questions, value)
	}
	return questions
}

// parseCompetitorPage reads one content-parsing response:
// tasks[0].result[0].items[0].page_content.{main_topic,secondary_topic}[],
// each with h_title and primary_content[].text. Header and footer are left out.
func parseCompetitorPage(payload []byte, link string) (BriefCompetitor, bool) {
	var envelope struct {
		StatusCode int `json:"status_code"`
		Tasks      []struct {
			StatusCode int `json:"status_code"`
			Result     []struct {
				Items []struct {
					PageContent struct {
						MainTopic      []parsedTopic `json:"main_topic"`
						SecondaryTopic []parsedTopic `json:"secondary_topic"`
					} `json:"page_content"`
				} `json:"items"`
			} `json:"result"`
		} `json:"tasks"`
	}
	if json.Unmarshal(payload, &envelope) != nil || envelope.StatusCode != 20000 || len(envelope.Tasks) != 1 ||
		envelope.Tasks[0].StatusCode != 20000 || len(envelope.Tasks[0].Result) == 0 || len(envelope.Tasks[0].Result[0].Items) == 0 {
		return BriefCompetitor{}, false
	}
	content := envelope.Tasks[0].Result[0].Items[0].PageContent
	competitor := BriefCompetitor{URL: link}
	seen := map[string]bool{}
	var text strings.Builder
	for _, topic := range append(content.MainTopic, content.SecondaryTopic...) {
		heading := strings.TrimSpace(topic.HTitle)
		if competitor.Title == "" {
			competitor.Title = strings.TrimSpace(firstNonEmpty(topic.MainTitle, heading))
		}
		key := strings.ToLower(heading)
		if heading != "" && utf8.RuneCountInString(heading) <= briefMaxHeadingRunes && !seen[key] && len(competitor.Headings) < briefMaxHeadings {
			seen[key] = true
			competitor.Headings = append(competitor.Headings, heading)
		}
		for _, block := range topic.PrimaryContent {
			if value := strings.TrimSpace(block.Text); value != "" {
				text.WriteString(value)
				text.WriteString("\n")
			}
		}
	}
	competitor.WordCount = len(briefWords(text.String()))
	competitor.Text = truncateRunes(text.String(), briefMaxTextRunes)
	return competitor, competitor.WordCount > 0
}

type parsedTopic struct {
	HTitle         string `json:"h_title"`
	MainTitle      string `json:"main_title"`
	PrimaryContent []struct {
		Text string `json:"text"`
	} `json:"primary_content"`
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
