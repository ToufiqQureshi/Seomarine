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

const (
	briefCacheTTL = 30 * 24 * time.Hour
	briefDepth    = 30
	briefParses   = 8
	briefCeiling  = 0.007
)

var (
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
}

// BriefCompetitor is an organic result and its parsed headings.
type BriefCompetitor struct {
	URL      string   `json:"url"`
	Title    string   `json:"title"`
	Headings []string `json:"headings"`
}

// BriefProvider performs the bounded DataForSEO request sequence.
type BriefProvider interface {
	Fetch(context.Context, string, BriefInput) (ContentBrief, error)
}

// BriefCache serializes cache misses by key across all application instances.
type BriefCache interface {
	GetOrBuild(context.Context, string, func() (ContentBrief, error)) (ContentBrief, error)
}

// BriefService validates requests, uses a shared cache, and filters a site's
// own URLs only after reading the cross-workspace cached result.
type BriefService struct {
	Cache    BriefCache
	Provider BriefProvider
}

// Get returns a shared 30-day SERP brief, fetching only on a cache miss.
func (s BriefService) Get(ctx context.Context, organizationID string, input BriefInput) (ContentBrief, error) {
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
	brief, err := s.Cache.GetOrBuild(ctx, key, func() (ContentBrief, error) {
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
	brief.Competitors = slices.DeleteFunc(brief.Competitors, func(item BriefCompetitor) bool {
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
	if len(result.Questions) > 100 || len(result.Competitors) > briefParses {
		return errors.New("SERP brief result exceeded the supported limits")
	}
	for _, question := range result.Questions {
		if strings.TrimSpace(question) == "" || utf8.RuneCountInString(question) > 500 {
			return errors.New("SERP brief returned an invalid question")
		}
	}
	for _, competitor := range result.Competitors {
		if hostFromURL(competitor.URL) == "" || utf8.RuneCountInString(competitor.Title) > 1_000 || len(competitor.Headings) > 100 {
			return errors.New("SERP brief returned invalid competitor data")
		}
		for _, heading := range competitor.Headings {
			if utf8.RuneCountInString(heading) > 1_000 {
				return errors.New("SERP brief returned an oversized competitor heading")
			}
		}
	}
	return nil
}

// DataForSEOBriefProvider fetches one depth-30 SERP and parses at most eight
// organic competitors under a single $0.007 customer-credit hold.
type DataForSEOBriefProvider struct {
	Client interface {
		DoBudgeted(context.Context, string, spend.Feature, float64, func(dataforseo.DoBudgetedRequest) error) error
	}
}

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
		urls := make([]string, 0, briefParses)
		brief.Questions = extractBriefQuestions(serpResult)
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
		if len(urls) == 0 {
			return errors.New("DataForSEO returned no parsable organic competitors")
		}
		parseTasks := make([]map[string]string, 0, len(urls))
		for _, link := range urls {
			parseTasks = append(parseTasks, map[string]string{"url": link})
		}
		parseBody, err := json.Marshal(parseTasks)
		if err != nil {
			return fmt.Errorf("encode competitor parsing request: %w", err)
		}
		parsePayload, err := request(http.MethodPost, "/v3/on_page/content_parsing/live", parseBody, false)
		if err != nil {
			return err
		}
		parsed, err := parseCompetitorTasks(parsePayload, urls)
		if err != nil {
			return err
		}
		brief.Competitors = parsed
		return nil
	})
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
	var root any
	if json.Unmarshal(value, &root) != nil {
		return nil
	}
	items := []map[string]any{}
	var walk func(any)
	walk = func(node any) {
		switch current := node.(type) {
		case map[string]any:
			if current["type"] == "organic" {
				items = append(items, current)
			}
			for _, child := range current {
				walk(child)
			}
		case []any:
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(root)
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
	if value != "" && utf8.RuneCountInString(value) <= 500 && !seen[key] && len(questions) < 100 {
		seen[key] = true
		questions = append(questions, value)
	}
	return questions
}

func parseCompetitorTasks(payload []byte, urls []string) ([]BriefCompetitor, error) {
	var envelope struct {
		StatusCode int `json:"status_code"`
		Tasks      []struct {
			StatusCode int               `json:"status_code"`
			Result     []json.RawMessage `json:"result"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.StatusCode != 20000 || len(envelope.Tasks) != len(urls) {
		return nil, errors.New("DataForSEO returned invalid competitor parsing data")
	}
	competitors := make([]BriefCompetitor, 0, len(urls))
	for i, task := range envelope.Tasks {
		if task.StatusCode != 20000 || len(task.Result) == 0 {
			return nil, errors.New("DataForSEO could not parse a competitor page")
		}
		var result map[string]any
		if err := json.Unmarshal(task.Result[0], &result); err != nil {
			return nil, errors.New("DataForSEO returned malformed competitor content")
		}
		competitor := BriefCompetitor{URL: urls[i], Title: firstString(result, "title", "meta_title")}
		competitor.Headings = collectHeadings(result)
		competitors = append(competitors, competitor)
	}
	return competitors, nil
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func collectHeadings(root map[string]any) []string {
	result := []string{}
	seen := map[string]bool{}
	var walk func(any, string)
	walk = func(node any, parent string) {
		switch current := node.(type) {
		case map[string]any:
			if kind, ok := current["type"].(string); ok {
				kind = strings.ToLower(kind)
				if kind == "h1" || kind == "h2" || kind == "h3" || kind == "heading" {
					result = collectHeadingValues(current, result, seen)
				}
			}
			for key, child := range current {
				lower := strings.ToLower(key)
				if lower == "h1" || lower == "h2" || lower == "h3" || lower == "headings" {
					result = collectHeadingValues(child, result, seen)
				} else if lower != "page_text" && lower != "content" && lower != "text" {
					walk(child, lower)
				}
			}
		case []any:
			for _, child := range current {
				walk(child, parent)
			}
		}
	}
	walk(root, "")
	if len(result) > 100 {
		result = result[:100]
	}
	return result
}

func collectHeadingValues(value any, output []string, seen map[string]bool) []string {
	switch current := value.(type) {
	case string:
		text := strings.TrimSpace(current)
		key := strings.ToLower(text)
		if text != "" && utf8.RuneCountInString(text) <= 1_000 && !seen[key] {
			seen[key] = true
			output = append(output, text)
		}
	case []any:
		for _, child := range current {
			output = collectHeadingValues(child, output, seen)
		}
	case map[string]any:
		for _, key := range []string{"text", "content", "title", "value"} {
			if text, ok := current[key].(string); ok {
				return collectHeadingValues(text, output, seen)
			}
		}
	}
	return output
}
