package analytics

import (
	"net/url"
	"strings"
)

// Channels a pageview is attributed to. channelInternal marks a
// pageview reached from the site itself (a click between pages); it is
// stored so pageview counts stay complete, but it is not an acquisition
// channel and the summary leaves it out.
const (
	channelAI       = "ai"
	channelSearch   = "search"
	channelSocial   = "social"
	channelDirect   = "direct"
	channelReferral = "referral"
	channelInternal = "internal"
)

// AI assistants a visit can come from.
const (
	aiChatGPT    = "chatgpt"
	aiPerplexity = "perplexity"
	aiGemini     = "gemini"
	aiClaude     = "claude"
	aiCopilot    = "copilot"
	aiOther      = "other_ai"
)

// Devices, from the user agent and the screen width.
const (
	deviceDesktop = "desktop"
	deviceMobile  = "mobile"
	deviceTablet  = "tablet"
)

// aiDomains maps assistant hosts to the source they count as. A host
// matches its entry or any subdomain of it (www.perplexity.ai).
var aiDomains = map[string]string{
	"chatgpt.com":             aiChatGPT,
	"chat.openai.com":         aiChatGPT,
	"perplexity.ai":           aiPerplexity,
	"gemini.google.com":       aiGemini,
	"bard.google.com":         aiGemini,
	"claude.ai":               aiClaude,
	"copilot.microsoft.com":   aiCopilot,
	"copilot.cloud.microsoft": aiCopilot,
	"chat.deepseek.com":       aiOther,
	"chat.mistral.ai":         aiOther,
	"meta.ai":                 aiOther,
	"grok.com":                aiOther,
	"poe.com":                 aiOther,
	"you.com":                 aiOther,
	"phind.com":               aiOther,
	"chat.qwen.ai":            aiOther,
}

// aiTags maps bare utm_source values that assistants and their users put in
// links ("utm_source=chatgpt") to the source they count as.
var aiTags = map[string]string{
	"chatgpt":    aiChatGPT,
	"openai":     aiChatGPT,
	"perplexity": aiPerplexity,
	"gemini":     aiGemini,
	"bard":       aiGemini,
	"claude":     aiClaude,
	"copilot":    aiCopilot,
}

// brandChannels maps a brand label to its channel. Search engines and some
// networks run one host per country (google.co.in, yandex.ru), so any host
// with the brand as a whole label matches: www.google.co.in, l.facebook.com.
// The same table classifies bare utm_source values such as "google".
var brandChannels = map[string]string{
	"google":     channelSearch,
	"bing":       channelSearch,
	"yahoo":      channelSearch,
	"yandex":     channelSearch,
	"baidu":      channelSearch,
	"duckduckgo": channelSearch,
	"ecosia":     channelSearch,
	"naver":      channelSearch,
	"seznam":     channelSearch,
	"facebook":   channelSocial,
	"instagram":  channelSocial,
	"linkedin":   channelSocial,
	"reddit":     channelSocial,
	"pinterest":  channelSocial,
	"youtube":    channelSocial,
	"tiktok":     channelSocial,
	"twitter":    channelSocial,
	"quora":      channelSocial,
	"whatsapp":   channelSocial,
	"telegram":   channelSocial,
}

// domainChannels lists hosts whose name is not a brand label. A host
// matches its entry or any subdomain of it.
var domainChannels = map[string]string{
	"search.brave.com":     channelSearch,
	"startpage.com":        channelSearch,
	"qwant.com":            channelSearch,
	"kagi.com":             channelSearch,
	"t.co":                 channelSocial,
	"x.com":                channelSocial,
	"fb.com":               channelSocial,
	"lnkd.in":              channelSocial,
	"threads.net":          channelSocial,
	"bsky.app":             channelSocial,
	"news.ycombinator.com": channelSocial,
	"t.me":                 channelSocial,
	"wa.me":                channelSocial,
}

// source is where a pageview came from.
type source struct {
	Channel      string
	AISource     string // set only for channelAI
	ReferrerHost string // the external referrer's host, empty for direct and internal
}

// classify attributes a pageview of page, reached from referrer, to a
// channel. domain is the project's domain, empty when the project has none.
//
// The heuristic, in order:
//  1. A referrer on the page's own host, or on the project's domain or a
//     subdomain of it, is internal navigation. utm_source is ignored then,
//     because SPAs keep the landing page's query string as users click on.
//  2. A referrer on a known AI assistant host is that assistant.
//  3. A utm_source naming an assistant ("chatgpt.com", which ChatGPT appends
//     to every link it cites, or "perplexity") is that assistant. Many
//     assistants strip the referrer, so this recovers AI visits that would
//     otherwise read as direct.
//  4. A referrer on a search engine or social network is that channel.
//  5. Any other parseable referrer host is a referral.
//  6. With no usable referrer, a utm_source naming a search engine or social
//     network is that channel (in-app browsers often send no referrer).
//  7. Everything else is direct.
//
// Hosts compare case-insensitively without a leading "www.".
func classify(page *url.URL, referrer, domain string) source {
	pageHost := normalizeHost(page.Hostname())
	refHost := ""
	if ref, err := url.Parse(strings.TrimSpace(referrer)); err == nil && (ref.Scheme == "http" || ref.Scheme == "https") {
		refHost = normalizeHost(ref.Hostname())
	}
	if refHost != "" && (refHost == pageHost || (domain != "" && withinDomain(refHost, domain))) {
		return source{Channel: channelInternal}
	}

	utm := normalizeHost(page.Query().Get("utm_source"))
	if ai := aiSourceOf(refHost); ai != "" {
		return source{Channel: channelAI, AISource: ai, ReferrerHost: refHost}
	}
	if ai := aiSourceOf(utm); ai != "" {
		return source{Channel: channelAI, AISource: ai, ReferrerHost: refHost}
	}
	if refHost != "" {
		if ch := channelOf(refHost); ch != "" {
			return source{Channel: ch, ReferrerHost: refHost}
		}
		return source{Channel: channelReferral, ReferrerHost: refHost}
	}
	if ch := channelOf(utm); ch != "" {
		return source{Channel: ch}
	}
	return source{Channel: channelDirect}
}

// aiSourceOf returns the assistant host or utm tag names, or "".
func aiSourceOf(host string) string {
	if host == "" {
		return ""
	}
	if ai, ok := aiTags[host]; ok {
		return ai
	}
	for d, ai := range aiDomains {
		if withinDomain(host, d) {
			return ai
		}
	}
	return ""
}

// channelOf returns the search or social channel of host, or "".
func channelOf(host string) string {
	if host == "" {
		return ""
	}
	for label := range strings.SplitSeq(host, ".") {
		if ch, ok := brandChannels[label]; ok {
			return ch
		}
	}
	for d, ch := range domainChannels {
		if withinDomain(host, d) {
			return ch
		}
	}
	return ""
}

// withinDomain reports whether host is domain or a subdomain of it.
func withinDomain(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// normalizeHost lowercases host and drops a trailing dot and a leading
// "www.", the form legacy projects store their domain in.
func normalizeHost(host string) string {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	return strings.TrimPrefix(host, "www.")
}

// Screen widths below these are phones and tablets.
const (
	maxMobileWidth = 767
	maxTabletWidth = 1024
)

// deviceOf classifies the visitor's device. The user agent decides when it
// says: "Mobi" is the cross-browser mobile marker, and iPads and Android
// devices without "Mobi" are tablets. iPadOS sends a desktop Safari user
// agent, so a touch-sized screen width decides otherwise.
func deviceOf(userAgent string, screenWidth int) string {
	switch {
	case strings.Contains(userAgent, "Mobi"):
		return deviceMobile
	case strings.Contains(userAgent, "iPad"), strings.Contains(userAgent, "Tablet"), strings.Contains(userAgent, "Android"):
		return deviceTablet
	case screenWidth > 0 && screenWidth <= maxMobileWidth:
		return deviceMobile
	case screenWidth > maxMobileWidth && screenWidth < maxTabletWidth:
		return deviceTablet
	default:
		return deviceDesktop
	}
}

// isBot reports whether userAgent belongs to a crawler or headless browser.
// Crawlers such as Googlebot run JavaScript and would fire the tracker.
func isBot(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	for _, marker := range []string{"bot", "crawler", "spider", "headless", "lighthouse", "slurp"} {
		if strings.Contains(ua, marker) {
			return true
		}
	}
	return userAgent == ""
}
