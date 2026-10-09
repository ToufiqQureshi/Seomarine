package backlinks

import (
	"net/netip"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

var schemePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z\d+.-]*://`)
var hostPattern = regexp.MustCompile(`^[a-z\d.-]+$`)

func normalizeTarget(input string, requested string) (Target, error) {
	input = strings.TrimSpace(input)
	if input == "" || len(input) > 2048 {
		return Target{}, inputError("Enter a valid domain or page URL.")
	}
	if requested == "page" {
		requested = string(ScopeExactURL)
	}
	if requested != "" && requested != string(ScopeExactURL) && requested != string(ScopeSubfolder) && requested != string(ScopeDomain) && requested != string(ScopeSubdomains) {
		return Target{}, inputError("scope must be exact_url, subfolder, domain or subdomains.")
	}
	parsedInput := input
	if !schemePattern.MatchString(parsedInput) {
		parsedInput = "https://" + parsedInput
	}
	u, err := url.Parse(parsedInput)
	if err != nil || u.Hostname() == "" {
		return Target{}, inputError("Enter a valid domain like example.com")
	}
	if u.User != nil {
		return Target{}, inputError("URLs with embedded credentials are not supported")
	}
	host, err := idna.Lookup.ToASCII(strings.ToLower(u.Hostname()))
	if err != nil {
		return Target{}, inputError("Enter a valid domain like example.com")
	}
	host = strings.TrimPrefix(host, "www.")
	if !validHost(host) {
		return Target{}, inputError("Enter a valid domain like example.com")
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	scope := requested
	if scope == "" {
		scope = string(ScopeSubdomains)
		if path != "" {
			scope = string(ScopeSubfolder)
		}
	}
	if scope == string(ScopeSubfolder) && path == "" {
		return Target{}, inputError("Add a path to use Subfolder (e.g. example.com/blog)")
	}
	if scope == string(ScopeExactURL) {
		if strings.ContainsAny(input, "?#") || u.RawQuery != "" || u.Fragment != "" {
			return Target{}, inputError("Page URLs with query strings or fragments are not supported")
		}
		protocol := "https"
		if strings.HasPrefix(strings.ToLower(input), "http://") {
			protocol = "http"
		}
		apiTarget := protocol + "://" + host + path
		if path == "" {
			apiTarget += "/"
		}
		return Target{APITarget: apiTarget, Display: apiTarget, Scope: ScopeExactURL, IncludeSubdomains: true}, nil
	}
	display := host
	if scope == string(ScopeSubfolder) {
		display += path
	}
	targetPath := ""
	if scope == string(ScopeSubfolder) {
		targetPath = path
	}
	return Target{APITarget: host, Display: display, Scope: Scope(scope), Path: targetPath, IncludeSubdomains: scope == string(ScopeSubdomains)}, nil
}

func validHost(host string) bool {
	if host == "" || len(host) > 253 || !strings.Contains(host, ".") || !hostPattern.MatchString(host) {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return false
	}
	for label := range strings.SplitSeq(host, ".") {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
	}
	suffix, icann := publicsuffix.PublicSuffix(host)
	return icann || strings.Contains(suffix, ".")
}

type inputError string

func (e inputError) Error() string { return string(e) }
