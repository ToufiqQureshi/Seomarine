package audit

import (
	"regexp"
	"strings"
)

// challenge2xx matches bot-protection that answers 2xx with a challenge in
// place of the page: Cloudflare's interstitial and SiteGround's sgcaptcha
// redirect (a 202).
var challenge2xx = regexp.MustCompile(`(?i)<title>\s*(?:just a moment\.\.\.|attention required! \| cloudflare)\s*</title>|/\.well-known/sgcaptcha/`)

// challengeBodyMarkers are body fingerprints of a bot challenge.
var challengeBodyMarkers = []string{
	"just a moment...",
	"challenge-platform",
	"cf-browser-verification",
	"attention required! | cloudflare",
	"verifying you are human",
}

// classifyFetch maps an HTTP status, the cf-mitigated flag and a body snippet
// onto a PageFetchClass. Mirrors classifyFetch.
func classifyFetch(statusCode int, mitigated bool, bodySnippet string) PageFetchClass {
	if statusCode == 0 {
		return FetchError
	}
	// A final 429 means rate limiting, whether retries were exhausted or the
	// requested cooldown exceeded the crawl budget. Checked before
	// cf-mitigated: a Cloudflare rate-limiting rule sets that header too.
	if statusCode == 429 {
		return FetchRateLimited
	}
	if mitigated {
		return FetchBlocked
	}
	if statusCode == 401 || statusCode == 403 {
		return FetchBlocked
	}
	if statusCode >= 200 && statusCode < 300 && challenge2xx.MatchString(bodySnippet) {
		return FetchBlocked
	}
	if statusCode == 503 {
		snippet := strings.ToLower(bodySnippet)
		for _, marker := range challengeBodyMarkers {
			if strings.Contains(snippet, marker) {
				return FetchBlocked
			}
		}
	}
	return FetchOK
}
