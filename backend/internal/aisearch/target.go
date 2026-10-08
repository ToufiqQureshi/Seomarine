package aisearch

import (
	"errors"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// Scope narrows a domain lookup to a hostname, its subdomains, a path or one
// page.
type Scope string

// Research scopes.
const (
	ScopeExactURL   Scope = "exact_url"
	ScopeSubfolder  Scope = "subfolder"
	ScopeDomain     Scope = "domain"
	ScopeSubdomains Scope = "subdomains"
)

func (s Scope) valid() bool {
	switch s {
	case ScopeExactURL, ScopeSubfolder, ScopeDomain, ScopeSubdomains:
		return true
	}
	return false
}

// usesPath reports whether the scope narrows by URL path, which the provider
// cannot do, so page rows are filtered after the call.
func (s Scope) usesPath() bool { return s == ScopeExactURL || s == ScopeSubfolder }

// TargetType says whether a query is a domain or a brand keyword.
type TargetType string

// Target types.
const (
	TargetDomain  TargetType = "domain"
	TargetKeyword TargetType = "keyword"
)

// Target is what a free-text query resolved to.
type Target struct {
	Type  TargetType
	Value string
}

var (
	hostChars = regexp.MustCompile(`^[a-z\d.-]+$`)
	hasScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z\d+.-]*://`)
)

// detectTarget decides whether free text is a domain ("example.com") or a
// brand keyword ("Example Brand"): no whitespace, a dot, and a hostname that
// normalizes.
func detectTarget(raw string) Target {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" && !strings.ContainsFunc(trimmed, unicode.IsSpace) && strings.Contains(trimmed, ".") {
		if host, err := normalizeHost(trimmed); err == nil && strings.Contains(host, ".") {
			return Target{Type: TargetDomain, Value: host}
		}
	}
	return Target{Type: TargetKeyword, Value: trimmed}
}

// normalizeHost extracts a lowercase ASCII hostname without a leading www.
// from input that may be a full URL.
func normalizeHost(input string) (string, error) {
	u, err := parseLoose(input)
	if err != nil {
		return "", err
	}
	host, err := idna.Lookup.ToASCII(strings.ToLower(u.Hostname()))
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(host, "www."), nil
}

// parseLoose parses input as a URL, assuming https when it has no scheme.
func parseLoose(input string) (*url.URL, error) {
	input = strings.TrimSpace(input)
	if !hasScheme.MatchString(input) {
		input = "https://" + input
	}
	u, err := url.Parse(input)
	if err != nil {
		return nil, err
	}
	if u.Hostname() == "" {
		return nil, errors.New("missing host")
	}
	return u, nil
}

// validDomainHost reports whether host ends in a real public suffix, which
// rejects IP addresses and made-up TLDs like example.por before they reach a
// provider that bills the failed task. The list marks ICANN suffixes (com,
// co.uk) and private ones (github.io); a made-up TLD matches neither.
func validDomainHost(host string) bool {
	if !wellFormedHost(host) {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return false
	}
	suffix, icann := publicsuffix.PublicSuffix(host)
	return icann || strings.Contains(suffix, ".")
}

// wellFormedHost reports whether host is a DNS name: at most 253 characters in
// labels of 1 to 63 that neither start nor end with a hyphen. A trailing dot
// leaves an empty label and is refused too.
func wellFormedHost(host string) bool {
	if len(host) > 253 {
		return false
	}
	for label := range strings.SplitSeq(host, ".") {
		if len(label) < 1 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
	}
	return true
}

// ResearchTarget is a validated domain or URL with its scope.
type ResearchTarget struct {
	Scope Scope
	// Hostname is lowercase with a leading www. removed.
	Hostname string
	// Path is "" for the root, otherwise "/like/This" without a trailing slash.
	Path string
	// Display is the hostname, plus the path under a URL scope.
	Display string
}

// inputError is a message meant for the person who typed the input.
type inputError string

func (e inputError) Error() string { return string(e) }

const (
	errNeedsPath = inputError("Add a path to use Subfolder (e.g. example.com/blog)")
	errBadDomain = inputError("Enter a valid domain like example.com")
)

// parseResearchTarget validates input and picks its scope: requested when
// given, otherwise subdomains for a root domain and subfolder for a path.
func parseResearchTarget(input string, requested Scope) (ResearchTarget, error) {
	if strings.TrimSpace(input) == "" {
		return ResearchTarget{}, inputError("Enter a domain or URL")
	}
	u, err := parseLoose(input)
	if err != nil {
		return ResearchTarget{}, errBadDomain
	}
	if u.User != nil {
		return ResearchTarget{}, inputError("URLs with embedded credentials are not supported")
	}
	host, err := idna.Lookup.ToASCII(strings.ToLower(u.Hostname()))
	if err != nil {
		return ResearchTarget{}, errBadDomain
	}
	host = strings.TrimPrefix(host, "www.")
	// The charset check rejects hosts like my_site.com that parse but that the
	// provider bills and fails with an opaque "Invalid Field".
	if host == "" || !strings.Contains(host, ".") || !hostChars.MatchString(host) || !validDomainHost(host) {
		return ResearchTarget{}, errBadDomain
	}

	path := normalizePath(u.EscapedPath())
	if requested == ScopeSubfolder && path == "" {
		return ResearchTarget{}, errNeedsPath
	}
	scope := requested
	if scope == "" {
		scope = ScopeSubdomains
		if path != "" {
			scope = ScopeSubfolder
		}
	}
	display := host
	if scope.usesPath() {
		display += path
	}
	return ResearchTarget{Scope: scope, Hostname: host, Path: path, Display: display}, nil
}

func normalizePath(p string) string {
	return strings.TrimRight(p, "/")
}

// matches reports whether pageURL belongs to the target. Subfolder matching
// takes the path and its children but not look-alike siblings: /blog matches
// /blog/post, not /blogging.
func (t ResearchTarget) matches(pageURL string) bool {
	u, err := url.Parse(pageURL)
	if err != nil {
		return false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if t.Scope == ScopeSubdomains {
		if host != t.Hostname && !strings.HasSuffix(host, "."+t.Hostname) {
			return false
		}
	} else if host != t.Hostname {
		return false
	}
	path := normalizePath(u.EscapedPath())
	switch t.Scope {
	case ScopeExactURL:
		return path == t.Path
	case ScopeSubfolder:
		return path == t.Path || strings.HasPrefix(path, t.Path+"/")
	}
	return true
}

// safeHTTPURL returns value unchanged when it is an absolute http(s) URL
// without embedded credentials. URLs from LLM answers are untrusted: a crafted
// prompt can coax a model into emitting javascript: payloads that the UI would
// render as links.
func safeHTTPURL(value string) (string, bool) {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", false
	}
	return value, true
}

// safeHostname returns the host of an http(s) URL without a leading www.
func safeHostname(value string) *string {
	if _, ok := safeHTTPURL(value); !ok {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	return &host
}
