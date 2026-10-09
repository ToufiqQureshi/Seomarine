package audit

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// normalizeURL normalizes a URL for crawl deduplication: resolve relative
// references against base, strip the fragment, sort query parameters and
// lowercase the hostname. Trailing slashes are preserved on purpose: a trailing
// slash is the canonical form on most CMS platforms, which 301-redirect the
// non-slash version to it, so stripping it here would rewrite the canonical URL
// into its own redirect source (a 508 loop). Returns ok=false when the input is
// not a valid http(s) URL. Mirrors normalizeUrl.
func normalizeURL(raw string, base string) (string, bool) {
	parsed, err := parseURL(raw, base)
	if err != nil {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	parsed.Fragment = ""
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	return serialize(parsed), true
}

// canonicalURLKey is a canonical key for URL equality checks that should
// survive the redirect patterns a site uses to reach its canonical form:
// trailing-slash redirects, www <-> non-www and http -> https. It forces https,
// drops a leading "www.", lowercases the host, sorts query params and strips
// the fragment. Trailing slashes are intentionally kept so two genuinely
// different paths never collapse together. Mirrors canonicalUrlKey.
func canonicalURLKey(raw string) string {
	parsed, err := parseURL(raw, "")
	if err != nil {
		return strings.ToLower(raw)
	}
	parsed.Scheme = "https"
	host := strings.ToLower(parsed.Host)
	parsed.Host = strings.TrimPrefix(host, "www.")
	parsed.Fragment = ""
	return serialize(parsed)
}

// effectivePort returns the explicit port or the scheme default.
func effectivePort(parsed *url.URL) string {
	if port := parsed.Port(); port != "" {
		return port
	}
	if parsed.Scheme == "https" {
		return "443"
	}
	return "80"
}

// equivalentHostnames reports whether two hostnames match, tolerating a leading
// "www." on either side.
func equivalentHostnames(a, b string) bool {
	hostA := strings.ToLower(a)
	hostB := strings.ToLower(b)
	if hostA == hostB {
		return true
	}
	return hostA == "www."+hostB || hostB == "www."+hostA
}

// isSameOrigin reports whether url belongs to the same crawl boundary as
// origin: the hostname must match (or differ only by a leading www.), the
// protocol and port must match, and an http -> https upgrade on the default
// ports is allowed. Mirrors isSameOrigin.
func isSameOrigin(raw, origin string) bool {
	parsedURL, err := parseURL(raw, "")
	if err != nil {
		return false
	}
	parsedOrigin, err := parseURL(origin, "")
	if err != nil {
		return false
	}
	if !equivalentHostnames(parsedURL.Hostname(), parsedOrigin.Hostname()) {
		return false
	}
	originProtocol := strings.ToLower(parsedOrigin.Scheme)
	urlProtocol := strings.ToLower(parsedURL.Scheme)
	originPort := effectivePort(parsedOrigin)
	urlPort := effectivePort(parsedURL)
	if originProtocol == urlProtocol {
		return originPort == urlPort
	}
	return originProtocol == "http" && urlProtocol == "https" && originPort == "80" && urlPort == "443"
}

var (
	numericSegment = regexp.MustCompile(`^\d+$`)
	uuidSegment    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	dateSegment    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// detectURLTemplate detects a URL template pattern by replacing path segments
// that look like dynamic values (IDs, slugs, dates) with ":param". Mirrors
// detectUrlTemplate.
func detectURLTemplate(pathname string) string {
	segments := strings.FieldsFunc(pathname, func(r rune) bool { return r == '/' })
	normalized := make([]string, 0, len(segments))
	for _, segment := range segments {
		switch {
		case numericSegment.MatchString(segment):
			normalized = append(normalized, ":id")
		case uuidSegment.MatchString(strings.ToLower(segment)):
			normalized = append(normalized, ":uuid")
		case dateSegment.MatchString(segment):
			normalized = append(normalized, ":date")
		case strings.Contains(segment, "-") && strings.Count(segment, "-")+1 > 2:
			// A slug contains hyphens and has more than two parts, so short
			// fixed routes like "my-account" are left alone.
			normalized = append(normalized, ":slug")
		default:
			normalized = append(normalized, segment)
		}
	}
	return "/" + strings.Join(normalized, "/")
}

// getOrigin extracts the origin (scheme + host + port) from an absolute URL.
// It returns "" when the URL cannot be parsed. Mirrors getOrigin.
func getOrigin(raw string) string {
	parsed, err := parseURL(raw, "")
	if err != nil {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// parseURL resolves raw against base the way the WHATWG URL constructor does
// for the http(s) subset the crawler cares about.
func parseURL(raw, base string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" && base == "" {
		return nil, errInvalidURL
	}
	if base != "" {
		baseURL, err := url.Parse(base)
		if err != nil {
			return nil, err
		}
		ref, err := url.Parse(raw)
		if err != nil {
			return nil, err
		}
		return baseURL.ResolveReference(ref), nil
	}
	return url.Parse(raw)
}

// serialize renders a URL the way the WHATWG URL serializer does for the
// scheme/host/path/query subset the crawler compares: an empty path becomes
// "/" and query parameters are sorted. The fragment is never emitted.
func serialize(parsed *url.URL) string {
	var builder strings.Builder
	builder.WriteString(strings.ToLower(parsed.Scheme))
	builder.WriteString("://")
	builder.WriteString(strings.ToLower(parsed.Host))
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	builder.WriteString(path)
	if parsed.RawQuery != "" {
		builder.WriteString("?")
		builder.WriteString(sortedQuery(parsed.RawQuery))
	}
	return builder.String()
}

// sortedQuery re-encodes a raw query string with its keys sorted, matching the
// WHATWG URLSearchParams.sort() serialization.
func sortedQuery(rawQuery string) string {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}
	if len(values) == 0 {
		return rawQuery
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for i, key := range keys {
		if i > 0 {
			builder.WriteString("&")
		}
		for j, value := range values[key] {
			if j > 0 {
				builder.WriteString("&")
			}
			builder.WriteString(url.QueryEscape(key))
			builder.WriteString("=")
			builder.WriteString(url.QueryEscape(value))
		}
	}
	return builder.String()
}
