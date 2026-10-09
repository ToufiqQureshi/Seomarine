package audit

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ShopifySignatureAgent is the constant Shopify expects, quotes included.
const ShopifySignatureAgent = `"https://shopify.com"`

// maxSignatureValueLength bounds a stored signature value.
const maxSignatureValueLength = 4096

// CrawlerAccess is a bot-protection credential bound to one host. The binding is
// the security boundary: these values must never be sent to another site,
// including across a redirect hop.
type CrawlerAccess struct {
	Host string
	// Headers are replayed on requests to Host.
	Headers map[string]string
	// ExpiresAt is checked on every request: a long crawl can outlive the
	// signature.
	ExpiresAt *time.Time
}

// ShopifyCrawlerHeaders builds the three static headers Shopify expects.
func ShopifyCrawlerHeaders(signatureInput, signature string) map[string]string {
	return map[string]string{
		"Signature-Input": signatureInput,
		"Signature":       signature,
		"Signature-Agent": ShopifySignatureAgent,
	}
}

var crawlerHostPattern = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9-]+)+$`)

// NormalizeCrawlerHost normalizes user input into a bare lowercase hostname.
// Accepts a full URL (people paste one) as well as a bare host. ok=false means
// not a usable hostname. Mirrors normalizeCrawlerHost.
func NormalizeCrawlerHost(input string) (string, bool) {
	raw := strings.ToLower(strings.TrimSpace(input))
	if raw == "" {
		return "", false
	}
	candidate := raw
	if !strings.Contains(raw, "://") {
		candidate = "https://" + raw
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", false
	}
	host := strings.TrimSuffix(parsed.Hostname(), ".")
	if !crawlerHostPattern.MatchString(host) {
		return "", false
	}
	return host, true
}

// CrawlerHeadersFor returns the headers to send with a request to rawURL, or nil
// when it is not the host the signature was created for over https before its
// expiry. The host match is exact: Shopify scopes a signature to one domain. A
// redirect to plain http must not replay the signature in cleartext.
func CrawlerHeadersFor(rawURL string, access *CrawlerAccess) map[string]string {
	if access == nil || IsCrawlerAccessExpired(access.ExpiresAt, time.Now()) {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	if parsed.Scheme != "https" || strings.ToLower(parsed.Hostname()) != access.Host {
		return nil
	}
	return access.Headers
}

var signatureExpiryPattern = regexp.MustCompile(`expires=(\d+)`)

// ParseSignatureExpiry extracts the RFC 9421 `expires=<unix seconds>` parameter
// from a Shopify Signature-Input value.
func ParseSignatureExpiry(signatureInput string) *time.Time {
	match := signatureExpiryPattern.FindStringSubmatch(signatureInput)
	if match == nil {
		return nil
	}
	seconds, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return nil
	}
	expiresAt := time.Unix(seconds, 0).UTC()
	return &expiresAt
}

// IsCrawlerAccessExpired reports whether a credential has expired at now.
func IsCrawlerAccessExpired(expiresAt *time.Time, now time.Time) bool {
	return expiresAt != nil && !expiresAt.After(now)
}
