package audit

import (
	"net/url"
	"strings"
)

// RobotsResult is the parsed form of a robots.txt document: an allow predicate
// and the sitemap URLs it declares.
type RobotsResult struct {
	// IsAllowed reports whether the crawler may fetch a URL.
	IsAllowed func(rawURL string) bool
	// SitemapURLs are the absolute sitemap URLs declared by Sitemap directives.
	SitemapURLs []string
}

// robotsRule is one Allow/Disallow line.
type robotsRule struct {
	pattern string
	allow   bool
}

// robotsGroup is a set of rules for one or more user-agent tokens.
type robotsGroup struct {
	agents []string
	rules  []robotsRule
}

// parseRobotsTxt parses a robots.txt body for an origin. A nil text means the
// document was missing or unreachable, so everything is allowed. Mirrors
// parseRobotsTxt (a hand-rolled RFC 9309 matcher replaces the robots-parser
// dependency; see the README's deliberate differences).
func parseRobotsTxt(origin string, text *string) RobotsResult {
	if text == nil {
		return RobotsResult{IsAllowed: func(string) bool { return true }}
	}
	groups, sitemaps := parseRobotsGroups(*text, origin)
	rules := selectRobotsRules(groups, auditUserAgent)
	return RobotsResult{
		IsAllowed:   func(rawURL string) bool { return robotsAllowed(rawURL, rules) },
		SitemapURLs: sitemaps,
	}
}

// parseRobotsGroups splits a robots.txt body into groups and collects the
// document's Sitemap directives.
func parseRobotsGroups(text string, origin string) ([]robotsGroup, []string) {
	groups := []robotsGroup{}
	sitemaps := []string{}
	var current *robotsGroup
	lastKey := ""
	for rawLine := range strings.Lines(text) {
		line := rawLine
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if current == nil || lastKey != "user-agent" {
				groups = append(groups, robotsGroup{})
				current = &groups[len(groups)-1]
			}
			current.agents = append(current.agents, strings.ToLower(value))
		case "allow", "disallow":
			if current == nil {
				continue
			}
			if key == "disallow" && value == "" {
				// "Disallow:" with an empty value allows everything; it is a
				// no-op rule rather than a match-all deny.
				lastKey = key
				continue
			}
			current.rules = append(current.rules, robotsRule{pattern: value, allow: key == "allow"})
		case "sitemap":
			if absolute, ok := robotsAbsolute(value, origin); ok {
				sitemaps = append(sitemaps, absolute)
			}
		}
		lastKey = key
	}
	return groups, sitemaps
}

// selectRobotsRules picks the rules that apply to userAgent: the most specific
// matching group wins, falling back to the "*" group.
func selectRobotsRules(groups []robotsGroup, userAgent string) []robotsRule {
	agent := strings.ToLower(userAgent)
	best := -1
	bestSpecificity := -1
	for i, group := range groups {
		for _, token := range group.agents {
			if token == "" {
				continue
			}
			if token == "*" {
				if bestSpecificity < 0 {
					best = i
					bestSpecificity = 0
				}
				continue
			}
			if strings.Contains(agent, token) && len(token) > bestSpecificity {
				best = i
				bestSpecificity = len(token)
			}
		}
	}
	if best < 0 {
		return nil
	}
	return groups[best].rules
}

// robotsAllowed reports whether rules allow fetching rawURL.
func robotsAllowed(rawURL string, rules []robotsRule) bool {
	if len(rules) == 0 {
		return true
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if parsed.RawQuery != "" {
		path += "?" + parsed.RawQuery
	}

	bestMatch := -1
	bestAllow := true
	for _, rule := range rules {
		matched, ok := robotsMatch(rule.pattern, path)
		if !ok {
			continue
		}
		switch {
		case matched > bestMatch:
			bestMatch = matched
			bestAllow = rule.allow
		case matched == bestMatch && rule.allow:
			// A tie goes to Allow.
			bestAllow = true
		}
	}
	if bestMatch < 0 {
		return true
	}
	return bestAllow
}

// robotsMatch reports how many characters of path a robots pattern matches,
// honoring '*' wildcards and a trailing '$'. ok=false means no match.
func robotsMatch(pattern, path string) (int, bool) {
	endAnchored := strings.HasSuffix(pattern, "$")
	if endAnchored {
		pattern = strings.TrimSuffix(pattern, "$")
	}
	segments := strings.Split(pattern, "*")
	position := 0
	for i, segment := range segments {
		if segment == "" {
			continue
		}
		if i == 0 {
			if !strings.HasPrefix(path[position:], segment) {
				return 0, false
			}
			position += len(segment)
			continue
		}
		index := strings.Index(path[position:], segment)
		if index < 0 {
			return 0, false
		}
		position += index + len(segment)
	}
	if endAnchored && position != len(path) {
		return 0, false
	}
	return position, true
}

// robotsAbsolute resolves a Sitemap directive value, tolerating relative values.
func robotsAbsolute(value string, origin string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", false
	}
	if parsed.IsAbs() {
		return parsed.String(), true
	}
	base, err := url.Parse(origin)
	if err != nil {
		return "", false
	}
	return base.ResolveReference(parsed).String(), true
}
