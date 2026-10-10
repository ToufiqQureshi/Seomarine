package mcp

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const defaultClientLabel = "API key"

const maxClientLabelChars = 60

var knownClients = []struct {
	pattern *regexp.Regexp
	label   string
}{
	{regexp.MustCompile(`(?i)^claude-code/`), "Claude Code"},
	{regexp.MustCompile(`(?i)^codex-mcp-client/`), "Codex"},
}

var labelUnsafe = regexp.MustCompile(`[^A-Za-z0-9._+\- ]`)

// resolveClientLabel ports the legacy display-only client label. It is a hint,
// never an identity; nothing authorizes, filters or bills on it.
func resolveClientLabel(userAgent, clientTitle string) string {
	trimmed := strings.TrimSpace(userAgent)
	for _, known := range knownClients {
		if known.pattern.MatchString(trimmed) {
			return known.label
		}
	}
	if title := sanitizeLabel(clientTitle); title != "" {
		return title
	}
	productToken := ""
	if fields := strings.FieldsFunc(trimmed, func(r rune) bool { return r == ' ' || r == '/' || r == '\t' || r == '\n' }); len(fields) > 0 {
		productToken = sanitizeLabel(fields[0])
	}
	if productToken != "" {
		return productToken
	}
	return defaultClientLabel
}

func sanitizeLabel(value string) string {
	cleaned := labelUnsafe.ReplaceAllString(value, "")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if len(cleaned) > maxClientLabelChars {
		cleaned = cleaned[:maxClientLabelChars]
	}
	return strings.TrimSpace(cleaned)
}

// publicOrigin returns the origin dashboard links are built from. It prefers
// the configured PUBLIC_URL (stable across hosted and self-hosted) and falls
// back to the request's forwarded host.
func (h *handler) publicOrigin(r *http.Request) string {
	if h.deps.PublicURL != nil && h.deps.PublicURL.Host != "" {
		return h.deps.PublicURL.String()
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if i := strings.IndexByte(proto, ','); i >= 0 {
		proto = strings.TrimSpace(proto[:i])
	}
	host := r.Header.Get("X-Forwarded-Host")
	if i := strings.IndexByte(host, ','); i >= 0 {
		host = strings.TrimSpace(host[:i])
	}
	if host == "" {
		host = r.Host
	}
	if proto != "http" && proto != "https" {
		proto = "https"
	}
	return proto + "://" + host
}

// buildDashboardURL joins a dashboard path onto the base origin.
func buildDashboardURL(baseURL, path string, params map[string]string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + path
	}
	ref, err := url.Parse(path)
	if err != nil {
		return baseURL + path
	}
	resolved := base.ResolveReference(ref)
	if len(params) > 0 {
		query := resolved.Query()
		for key, value := range params {
			query.Set(key, value)
		}
		resolved.RawQuery = query.Encode()
	}
	return resolved.String()
}
