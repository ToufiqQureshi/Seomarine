package aisearch

import "time"

// Platform names a source of AI answers as the provider spells it.
const (
	platformChatGPT = "chat_gpt"
	platformGoogle  = "google"
)

// brandPlatforms are the platforms a brand lookup queries, in display order.
var brandPlatforms = []string{platformChatGPT, platformGoogle}

// ChatGPT's mentions database only holds US/English data, so ChatGPT is always
// queried with this locale and left out of totals for any other.
const (
	chatGPTLocationCode = 2840
	chatGPTLanguageCode = "en"
)

// timestampLayout is JavaScript's Date#toISOString, which the web app parses.
const timestampLayout = "2006-01-02T15:04:05.000Z"

func formatTime(t time.Time) string { return t.UTC().Format(timestampLayout) }

// Provider payloads. Fields the provider may leave out or null are pointers;
// fields we do not read are ignored so new provider fields cannot break us.

type monthlySearch struct {
	Year         int      `json:"year"`
	Month        int      `json:"month"`
	SearchVolume *float64 `json:"search_volume"`
}

type mentionSource struct {
	URL   *string `json:"url"`
	Title *string `json:"title"`
}

type brandEntity struct {
	Title *string `json:"title"`
}

// mentionItem is one LLM answer that matched the target.
type mentionItem struct {
	Question        *string         `json:"question"`
	Sources         []mentionSource `json:"sources"`
	AISearchVolume  *float64        `json:"ai_search_volume"`
	MonthlySearches []monthlySearch `json:"monthly_searches"`
	FirstResponseAt *string         `json:"first_response_at"`
	LastResponseAt  *string         `json:"last_response_at"`
	BrandEntities   []brandEntity   `json:"brand_entities"`
}

type groupElement struct {
	Key            *string  `json:"key"`
	Mentions       *float64 `json:"mentions"`
	AISearchVolume *float64 `json:"ai_search_volume"`
}

type aggregatedTotal struct {
	Platform []groupElement `json:"platform"`
}

// topPagesItem is a page the platforms cite, keyed by its URL.
type topPagesItem struct {
	Key      *string        `json:"key"`
	Platform []groupElement `json:"platform"`
}

// crossItem is one compared brand, keyed by the aggregation key we sent.
type crossItem struct {
	Key      *string        `json:"key"`
	Platform []groupElement `json:"platform"`
}

type responseAnnotation struct {
	Title *string `json:"title"`
	URL   *string `json:"url"`
}

type responseSection struct {
	Text        *string              `json:"text"`
	Annotations []responseAnnotation `json:"annotations"`
}

type responseItem struct {
	Type     *string           `json:"type"`
	Sections []responseSection `json:"sections"`
}

// llmResponse is one model's answer to a prompt.
type llmResponse struct {
	ModelName     *string        `json:"model_name"`
	OutputTokens  *float64       `json:"output_tokens"`
	WebSearch     *bool          `json:"web_search"`
	Items         []responseItem `json:"items"`
	FanOutQueries []string       `json:"fan_out_queries"`
}

// Brand lookup API.

// BrandLookupInput is a validated brand lookup request. Scope is empty to
// derive it from the query.
type BrandLookupInput struct {
	Query        string
	Competitors  []string
	Scope        Scope
	LocationCode int
	LanguageCode string
}

// PlatformBreakdown is one platform's totals, or its failure.
type PlatformBreakdown struct {
	Platform       string `json:"platform"`
	Status         string `json:"status"`
	Mentions       *int   `json:"mentions"`
	AISearchVolume *int   `json:"aiSearchVolume"`
}

// ShareOfVoiceEntry is one brand's share of the compared brands' mentions.
type ShareOfVoiceEntry struct {
	Label    string   `json:"label"`
	IsTarget bool     `json:"isTarget"`
	Mentions *int     `json:"mentions"`
	SharePct *float64 `json:"sharePct"`
}

// ShareOfVoice compares the target with its competitors. Platforms lists the
// platforms whose call succeeded and that are summed into Entries.
type ShareOfVoice struct {
	Platforms []string            `json:"platforms"`
	Entries   []ShareOfVoiceEntry `json:"entries"`
}

// PromptExample is a prompt that cited a page, with its search volume.
type PromptExample struct {
	Question       string `json:"question"`
	AISearchVolume *int   `json:"aiSearchVolume"`
}

// TopPage is a page an AI platform cites.
type TopPage struct {
	URL            string          `json:"url"`
	Domain         *string         `json:"domain"`
	Platform       string          `json:"platform"`
	Mentions       *int            `json:"mentions"`
	CapturedVolume *int            `json:"capturedVolume"`
	Keywords       []PromptExample `json:"keywords"`
}

// CitedSource is a page cited in the answer to a prompt.
type CitedSource struct {
	URL    string  `json:"url"`
	Domain *string `json:"domain"`
	Title  *string `json:"title"`
}

// TopQuery is a prompt whose answer mentioned the target.
type TopQuery struct {
	Question        string        `json:"question"`
	Platform        string        `json:"platform"`
	AISearchVolume  *int          `json:"aiSearchVolume"`
	FirstSeenAt     *string       `json:"firstSeenAt"`
	LastSeenAt      *string       `json:"lastSeenAt"`
	CitedSources    []CitedSource `json:"citedSources"`
	BrandsMentioned []string      `json:"brandsMentioned"`
}

// MonthlyVolume is the summed AI search volume of one month.
type MonthlyVolume struct {
	Year   int `json:"year"`
	Month  int `json:"month"`
	Volume int `json:"volume"`
}

// BrandLookupResult is everything the brand lookup page renders.
type BrandLookupResult struct {
	Query              string `json:"query"`
	DetectedTargetType string `json:"detectedTargetType"`
	// ResolvedTarget is the hostname, plus the path under a URL scope.
	ResolvedTarget string `json:"resolvedTarget"`
	// Scope is null for a keyword lookup.
	Scope *Scope `json:"scope"`
	// AggregatesAreDomainLevel is true under a URL scope: the provider cannot
	// target a URL, so totals, platform counts, monthly volume and share of
	// voice stay domain-wide and the page must say so.
	AggregatesAreDomainLevel bool                `json:"aggregatesAreDomainLevel"`
	FetchedAt                string              `json:"fetchedAt"`
	HasData                  bool                `json:"hasData"`
	TotalMentions            *int                `json:"totalMentions"`
	TotalAISearchVolume      *int                `json:"totalAiSearchVolume"`
	PerPlatform              []PlatformBreakdown `json:"perPlatform"`
	ShareOfVoice             *ShareOfVoice       `json:"shareOfVoice"`
	TopPages                 []TopPage           `json:"topPages"`
	TopQueries               []TopQuery          `json:"topQueries"`
	MonthlyVolume            []MonthlyVolume     `json:"monthlyVolume"`
}

// Prompt explorer API.

// PromptExplorerInput is a validated prompt explorer request. The models are
// distinct, HighlightBrand is trimmed or empty, and WebSearchCountryCode is a
// known country code or empty.
type PromptExplorerInput struct {
	Prompt               string
	Models               []string
	HighlightBrand       string
	WebSearch            bool
	WebSearchCountryCode string
}

// Citation is a page an answer cites.
type Citation struct {
	URL          string  `json:"url"`
	Domain       *string `json:"domain"`
	Title        *string `json:"title"`
	MatchedBrand bool    `json:"matchedBrand"`
}

// ModelResult is one model's outcome in a prompt run: a ModelSuccess or a
// ModelError, told apart by the JSON field status.
type ModelResult interface{ modelResult() }

// ModelSuccess is a model's answer.
type ModelSuccess struct {
	Status               string     `json:"status"`
	Model                string     `json:"model"`
	ModelName            *string    `json:"modelName"`
	Text                 string     `json:"text"`
	Citations            []Citation `json:"citations"`
	FanOutQueries        []string   `json:"fanOutQueries"`
	BrandMentioned       *bool      `json:"brandMentioned"`
	OutputTokens         *int       `json:"outputTokens"`
	WebSearch            bool       `json:"webSearch"`
	WebSearchCountryCode *string    `json:"webSearchCountryCode"`
}

// ModelError says why a model produced no answer.
type ModelError struct {
	Status    string `json:"status"`
	Model     string `json:"model"`
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
}

func (ModelSuccess) modelResult() {}
func (ModelError) modelResult()   {}

// PromptExplorerResult is the side-by-side answers of one prompt.
type PromptExplorerResult struct {
	Prompt         string        `json:"prompt"`
	HighlightBrand *string       `json:"highlightBrand"`
	FetchedAt      string        `json:"fetchedAt"`
	Results        []ModelResult `json:"results"`
}
