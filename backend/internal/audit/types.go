package audit

import (
	"encoding/json"
	"strings"
)

// Config is the stored configuration of one audit.
type Config struct {
	MaxPages           int                `json:"maxPages"`
	LighthouseStrategy LighthouseStrategy `json:"lighthouseStrategy"`
	RenderJavaScript   bool               `json:"renderJavaScript"`
	// SitePlatform is set to "shopify" when the start URL's powered-by header
	// identifies a Shopify storefront.
	SitePlatform string `json:"sitePlatform,omitempty"`
	// CrawlerCredentialID is the crawler-access credential the crawl replayed.
	CrawlerCredentialID string `json:"crawlerCredentialId,omitempty"`
}

// MarshalAuditConfig encodes a config the same way the TypeScript writes it.
func MarshalAuditConfig(config Config) (string, error) {
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// ParseAuditConfig parses a stored config string. Read-side only: rows may hold
// retired Lighthouse strategies ("all", "manual") so those are mapped onto the
// closest surviving strategy, and anything unknown falls back to "auto" instead
// of making the whole config parse fail and the audit unviewable. Returns
// ok=false when the value is missing or the required fields are invalid —
// matching the Zod safeParse that the TypeScript used.
func ParseAuditConfig(raw string) (Config, bool) {
	if strings.TrimSpace(raw) == "" {
		return Config{}, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return Config{}, false
	}
	config := Config{LighthouseStrategy: LighthouseAuto}

	rawMaxPages, ok := fields["maxPages"]
	if !ok {
		return Config{}, false
	}
	if err := json.Unmarshal(rawMaxPages, &config.MaxPages); err != nil {
		return Config{}, false
	}
	if config.MaxPages < MinAuditPages || config.MaxPages > PaidMaxAuditPages {
		return Config{}, false
	}

	if rawStrategy, ok := fields["lighthouseStrategy"]; ok {
		var strategy string
		if err := json.Unmarshal(rawStrategy, &strategy); err != nil {
			config.LighthouseStrategy = LighthouseAuto
		} else {
			switch strategy {
			case string(LighthouseNone), "manual":
				config.LighthouseStrategy = LighthouseNone
			default:
				// "auto", "all" and anything unknown.
				config.LighthouseStrategy = LighthouseAuto
			}
		}
	}

	if rawRender, ok := fields["renderJavaScript"]; ok {
		if err := json.Unmarshal(rawRender, &config.RenderJavaScript); err != nil {
			return Config{}, false
		}
	}

	// Absent on every audit stored before crawler access shipped; a future
	// platform value must not make an old report unviewable.
	if rawPlatform, ok := fields["sitePlatform"]; ok {
		var platform string
		if err := json.Unmarshal(rawPlatform, &platform); err == nil && platform == "shopify" {
			config.SitePlatform = platform
		}
	}
	if rawCredential, ok := fields["crawlerCredentialId"]; ok {
		var credentialID string
		if err := json.Unmarshal(rawCredential, &credentialID); err == nil {
			config.CrawlerCredentialID = credentialID
		}
	}
	return config, true
}

// PageLink is one outgoing link edge, deduped by target URL within a page.
type PageLink struct {
	TargetURL  string `json:"targetUrl"`
	Anchor     string `json:"anchor"`
	IsInternal bool   `json:"isInternal"`
	IsNofollow bool   `json:"isNofollow"`
}

// ImageRef is one image found on a page.
type ImageRef struct {
	Src string `json:"src"`
	Alt string `json:"alt"`
}

// PageAnalysis is the SEO-relevant data extracted from one page's HTML.
type PageAnalysis struct {
	URL            string `json:"url"`
	StatusCode     int    `json:"statusCode"`
	RedirectURL    string `json:"redirectUrl"`
	ResponseTimeMs int64  `json:"responseTimeMs"`

	Title           string `json:"title"`
	MetaDescription string `json:"metaDescription"`
	Canonical       string `json:"canonical"`
	RobotsMeta      string `json:"robotsMeta"`
	OgTitle         string `json:"ogTitle"`
	OgDescription   string `json:"ogDescription"`
	OgImage         string `json:"ogImage"`

	H1s          []string `json:"h1s"`
	HeadingOrder []int    `json:"headingOrder"`

	WordCount int    `json:"wordCount"`
	BodyText  string `json:"bodyText"`
	// JavaScriptShell is a conservative signal of an app shell, not proof of
	// missing content.
	JavaScriptShell bool `json:"javascriptShell"`

	Images []ImageRef `json:"images"`

	Links []PageLink `json:"links"`

	HasStructuredData bool     `json:"hasStructuredData"`
	HreflangTags      []string `json:"hreflangTags"`
}

// LighthouseResult is a Lighthouse check for a single URL and strategy.
type LighthouseResult struct {
	URL                string   `json:"url"`
	PageID             string   `json:"pageId"`
	Strategy           string   `json:"strategy"`
	PerformanceScore   *float64 `json:"performanceScore"`
	AccessibilityScore *float64 `json:"accessibilityScore"`
	BestPracticesScore *float64 `json:"bestPracticesScore"`
	SeoScore           *float64 `json:"seoScore"`
	LcpMs              *float64 `json:"lcpMs"`
	CLS                *float64 `json:"cls"`
	InpMs              *float64 `json:"inpMs"`
	TtfbMs             *float64 `json:"ttfbMs"`
	ErrorMessage       *string  `json:"errorMessage,omitempty"`
	R2Key              *string  `json:"r2Key,omitempty"`
	PayloadSizeBytes   *int     `json:"payloadSizeBytes,omitempty"`
}

// CrawledPageResult is the full result of crawling one page. It is persisted to
// the app database inside the crawl chunk; link edges are not persisted here.
type CrawledPageResult struct {
	ID                 string         `json:"id"`
	URL                string         `json:"url"`
	StatusCode         int            `json:"statusCode"`
	FetchClass         PageFetchClass `json:"fetchClass"`
	RedirectURL        string         `json:"redirectUrl"`
	Title              string         `json:"title"`
	MetaDescription    string         `json:"metaDescription"`
	CanonicalURL       string         `json:"canonicalUrl"`
	RobotsMeta         string         `json:"robotsMeta"`
	XRobotsTag         string         `json:"xRobotsTag"`
	HeaderCanonicalURL string         `json:"headerCanonicalUrl"`
	OgTitle            string         `json:"ogTitle"`
	OgDescription      string         `json:"ogDescription"`
	OgImage            string         `json:"ogImage"`
	H1Count            int            `json:"h1Count"`
	H2Count            int            `json:"h2Count"`
	H3Count            int            `json:"h3Count"`
	H4Count            int            `json:"h4Count"`
	H5Count            int            `json:"h5Count"`
	H6Count            int            `json:"h6Count"`
	HeadingOrder       []int          `json:"headingOrder"`
	WordCount          int            `json:"wordCount"`
	ContentHash        string         `json:"contentHash"`
	// IsHTML is true when an HTML document was fetched and analyzed. It gates
	// the content checks in page reporters (an empty-shell HTML page must still
	// be checked; a PDF must not). Transient — not persisted.
	IsHTML bool `json:"-"`
	// JavaScriptShell is an app-shell signal for the per-page reporter.
	JavaScriptShell bool `json:"-"`
	// HTMLBytes is the HTML size read for this page, feeding the crawl window's
	// memory-pressure signal because response time is measured at headers.
	HTMLBytes int `json:"-"`
	// RateLimited is true when a 429 was retried for this URL, whatever the
	// retry returned. Not persisted; narrows the crawl window.
	RateLimited       bool       `json:"-"`
	ImagesTotal       int        `json:"imagesTotal"`
	ImagesMissingAlt  int        `json:"imagesMissingAlt"`
	Images            []ImageRef `json:"images"`
	Links             []PageLink `json:"links"`
	HasStructuredData bool       `json:"hasStructuredData"`
	HreflangTags      []string   `json:"hreflangTags"`
	IsIndexable       bool       `json:"isIndexable"`
	ResponseTimeMs    int64      `json:"responseTimeMs"`
	// CrawlDepth is nil when the page was not reached via links.
	CrawlDepth *int `json:"crawlDepth"`
	InSitemap  bool `json:"inSitemap"`
}
