package audit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// blockedHosts are internal hostnames that must never be crawled.
var blockedHosts = map[string]struct{}{
	"localhost":                {},
	"metadata.google.internal": {},
	"metadata":                 {},
	"169.254.169.254":          {},
	"100.100.100.200":          {},
}

// blockedHostSuffixes are internal DNS suffixes that must never be crawled.
var blockedHostSuffixes = []string{
	".localhost",
	".local",
	".localdomain",
	".internal",
	".home.arpa",
}

// Resolver resolves a hostname to its addresses. *net.Resolver satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// normalizeHost lowercases and trims a hostname, removing IPv6 brackets, a zone
// suffix and a trailing dot.
func normalizeHost(hostname string) string {
	host := strings.ToLower(strings.TrimSpace(hostname))
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if index := strings.IndexByte(host, '%'); index >= 0 {
		host = host[:index]
	}
	host = strings.TrimSuffix(host, ".")
	return host
}

// parseIPv4 parses a dotted-quad hostname, reporting ok=false when it is not
// exactly four decimal octets in range.
func parseIPv4(host string) (netip.Addr, bool) {
	normalized := normalizeHost(host)
	if strings.Contains(normalized, ":") {
		return netip.Addr{}, false
	}
	parts := strings.Split(normalized, ".")
	if len(parts) != 4 {
		return netip.Addr{}, false
	}
	for _, part := range parts {
		if part == "" {
			return netip.Addr{}, false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return netip.Addr{}, false
			}
		}
		if len(part) > 1 && part[0] == '0' {
			// WHATWG would parse these as octal; reject to stay predictable.
			return netip.Addr{}, false
		}
	}
	addr, err := netip.ParseAddr(normalized)
	if err != nil || !addr.Is4() {
		return netip.Addr{}, false
	}
	return addr, true
}

// isPrivateIPv4 reports whether a dotted-quad address is in a non-public range.
// It mirrors isPrivateIpv4, including the ranges netip's IsPrivate misses
// (0/8, 100.64/10, 198.18/15, 224/4, 169.254/16, 127/8).
func isPrivateIPv4(host string) bool {
	addr, ok := parseIPv4(host)
	if !ok {
		return false
	}
	octets := addr.As4()
	a, b := octets[0], octets[1]
	switch {
	case a == 10, a == 127, a == 0:
		return true
	case a == 169 && b == 254:
		return true
	case a == 172 && b >= 16 && b <= 31:
		return true
	case a == 192 && b == 168:
		return true
	case a == 100 && b >= 64 && b <= 127:
		return true
	case a == 198 && (b == 18 || b == 19):
		return true
	case a >= 224:
		return true
	}
	return false
}

// parseMappedIPv4FromIPv6 extracts the embedded IPv4 dotted-quad from an
// IPv4-mapped IPv6 address (::ffff:x.x.x.x or ::ffff:hex:hex). Returns ok=false
// for anything else.
func parseMappedIPv4FromIPv6(host string) (string, bool) {
	normalized := normalizeHost(host)
	if !strings.HasPrefix(normalized, "::ffff:") {
		return "", false
	}
	mapped := normalized[len("::ffff:"):]
	if _, ok := parseIPv4(mapped); ok {
		return mapped, true
	}
	segments := strings.Split(mapped, ":")
	nonEmpty := segments[:0]
	for _, segment := range segments {
		if segment != "" {
			nonEmpty = append(nonEmpty, segment)
		}
	}
	if len(nonEmpty) != 2 {
		return "", false
	}
	addr, err := netip.ParseAddr(normalized)
	if err != nil || !addr.Is4In6() {
		return "", false
	}
	ipv4 := addr.Unmap().As4()
	return fmt.Sprintf("%d.%d.%d.%d", ipv4[0], ipv4[1], ipv4[2], ipv4[3]), true
}

// isPrivateIPv6 reports whether an IPv6 literal is loopback, unspecified,
// unique-local, link-local or an IPv4-mapped private address.
func isPrivateIPv6(host string) bool {
	value := normalizeHost(host)
	if value == "::1" || value == "::" {
		return true
	}
	if strings.HasPrefix(value, "fc") || strings.HasPrefix(value, "fd") {
		return true
	}
	for _, prefix := range []string{"fe8", "fe9", "fea", "feb"} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	if mapped, ok := parseMappedIPv4FromIPv6(value); ok {
		return isPrivateIPv4(mapped)
	}
	return false
}

// isIPLiteral reports whether a hostname is an IPv4 dotted-quad or any IPv6
// literal.
func isIPLiteral(host string) bool {
	normalized := normalizeHost(host)
	if _, ok := parseIPv4(normalized); ok {
		return true
	}
	return strings.Contains(normalized, ":")
}

// isBlockedHost reports whether a hostname is internal, or a private IP literal.
func isBlockedHost(hostname string) bool {
	host := normalizeHost(hostname)
	if host == "" {
		return true
	}
	if _, ok := blockedHosts[host]; ok {
		return true
	}
	for _, suffix := range blockedHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	if isIPLiteral(host) {
		return isPrivateIPv4(host) || isPrivateIPv6(host)
	}
	return false
}

// isPublicAddr reports whether a resolved address is safe to connect to.
func isPublicAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	if addr.Is4() {
		return !isPrivateIPv4(addr.String())
	}
	return !isPrivateIPv6(addr.String())
}

// DefaultResolver is the resolver used when none is supplied.
var DefaultResolver Resolver = net.DefaultResolver

// Guard enforces the SSRF policy on every outbound connection and on the
// audit's start URL.
type Guard struct {
	Resolver Resolver
	// Dialer dials a validated address. Defaults to a net.Dialer with a
	// 10 second connect timeout.
	Dialer *net.Dialer
	// Client, when set, is returned by NewClient. Tests inject a client;
	// production builds one from the guard's transport.
	Client *http.Client
}

// NewGuard returns a Guard using the system resolver.
func NewGuard() *Guard { return &Guard{} }

func (g *Guard) resolver() Resolver {
	if g == nil || g.Resolver == nil {
		return DefaultResolver
	}
	return g.Resolver
}

func (g *Guard) dialer() *net.Dialer {
	if g == nil || g.Dialer == nil {
		return &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	}
	return g.Dialer
}

// resolveAllowed resolves host and returns its addresses, failing when the host
// is blocked or any resolved address is private. An IP literal is validated
// directly without a DNS lookup.
func (g *Guard) resolveAllowed(ctx context.Context, host string) ([]netip.Addr, error) {
	if isBlockedHost(host) {
		return nil, fmt.Errorf("%w: %s", ErrCrawlTargetBlocked, host)
	}
	if literal, err := netip.ParseAddr(normalizeHost(host)); err == nil {
		if !isPublicAddr(literal) {
			return nil, fmt.Errorf("%w: %s", ErrCrawlTargetBlocked, host)
		}
		return []netip.Addr{literal}, nil
	}
	addrs, err := g.resolver().LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	allowed := make([]netip.Addr, 0, len(addrs))
	for _, addr := range addrs {
		if !isPublicAddr(addr) {
			return nil, fmt.Errorf("%w: %s resolves to %s", ErrCrawlTargetBlocked, host, addr)
		}
		allowed = append(allowed, addr)
	}
	return allowed, nil
}

// DialContext resolves and validates the target address before connecting, so
// DNS rebinding between validation and connection cannot bypass the guard. TLS
// SNI still uses the request hostname because the transport dials the address,
// not the URL.
func (g *Guard) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split address %q: %w", address, err)
	}
	addrs, err := g.resolveAllowed(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses for %s", host)
	}
	var lastErr error
	for _, addr := range addrs {
		conn, err := g.dialer().DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("dial %s: %w", host, lastErr)
}

// NewClient returns an HTTP client whose transport validates every connection,
// so a host that resolves to a private address is refused at dial time. The
// client never follows redirects on its own: every caller (crawler, discovery,
// start-URL probe) follows hops explicitly so each hop is revalidated against
// the crawl policy and crawler-access headers are re-matched per host.
func (g *Guard) NewClient(timeout time.Duration) *http.Client {
	if g != nil && g.Client != nil {
		return g.Client
	}
	transport := &http.Transport{
		DialContext:           g.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	}
	return &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// isCrawlableURL is the synchronous SSRF check for URLs discovered mid-crawl
// (links, redirect targets, sitemap entries). It blocks non-http(s) schemes,
// private/loopback IP literals and internal hostnames. DNS resolution is only
// performed for the start URL, because a per-link lookup would be prohibitively
// slow. Mirrors isCrawlableUrl.
func isCrawlableURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return !isBlockedHost(parsed.Hostname())
}

// startURLRedirectHops bounds how many redirects the start-URL probe follows.
const startURLRedirectHops = 5

// startURLProbeTimeout bounds a single probe hop.
const startURLProbeTimeout = 10 * time.Second

// StartURLProbe is the result of following the start URL's redirects.
type StartURLProbe struct {
	// URL is the validated final URL the audit anchors to.
	URL string
	// PoweredBy is the final response's powered-by header, which is how a
	// Shopify storefront identifies itself.
	PoweredBy string
}

// validator normalizes and validates an audit's start URL. A test may inject a
// client; production builds one from the guard.
type validator struct {
	guard  *Guard
	client *http.Client
}

func (v validator) normalize(ctx context.Context, input string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", ErrStartURLInvalid
	}
	lower := strings.ToLower(raw)
	hasScheme := strings.Contains(lower, "://")
	if hasScheme && !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return "", ErrStartURLInvalid
	}
	if !hasScheme {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", ErrStartURLInvalid
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrStartURLInvalid
	}
	if isBlockedHost(parsed.Hostname()) {
		return "", ErrCrawlTargetBlocked
	}
	if _, err := v.guard.resolveAllowed(ctx, parsed.Hostname()); err != nil {
		return "", err
	}
	parsed.Fragment = ""
	return whatwgString(parsed), nil
}

// whatwgString renders a URL the way the WHATWG URL serializer does for the
// http(s) subset the crawler uses: scheme and host lowercased, an empty path
// becomes "/", and the query is preserved exactly as given.
func whatwgString(parsed *url.URL) string {
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
		builder.WriteString(parsed.RawQuery)
	}
	return builder.String()
}

// resolveStartURLRedirects follows the start URL's redirects so the audit
// anchors to the site's real origin: auditing a domain that 301s elsewhere
// (…net -> …com, apex -> www) would otherwise dead-end after one page at the
// same-origin crawl boundary. Every hop re-runs the full start-URL validation,
// so a redirect cannot smuggle the audit somewhere the user could not have
// pointed it directly. Probe failures fall back to the last validated URL.
func (v validator) resolveRedirects(ctx context.Context, startURL string) (StartURLProbe, error) {
	client := v.client
	if client == nil {
		client = v.guard.NewClient(0)
	}
	current := startURL
	for hop := 0; hop < startURLRedirectHops; hop++ {
		hopCtx, cancel := context.WithTimeout(ctx, startURLProbeTimeout)
		request, err := http.NewRequestWithContext(hopCtx, http.MethodHead, current, nil)
		if err != nil {
			cancel()
			return StartURLProbe{URL: current}, nil
		}
		request.Header.Set("User-Agent", auditUserAgent)
		response, err := client.Do(request)
		if err != nil {
			cancel()
			return StartURLProbe{URL: current}, nil
		}
		poweredBy := response.Header.Get("Powered-By")
		status := response.StatusCode
		location := response.Header.Get("Location")
		_ = response.Body.Close()
		cancel()
		if status < 300 || status >= 400 || location == "" {
			return StartURLProbe{URL: current, PoweredBy: poweredBy}, nil
		}
		next, err := url.Parse(location)
		if err != nil {
			return StartURLProbe{URL: current}, nil
		}
		base, err := url.Parse(current)
		if err != nil {
			return StartURLProbe{URL: current}, nil
		}
		resolved, err := v.normalize(ctx, base.ResolveReference(next).String())
		if err != nil {
			return StartURLProbe{URL: current}, err
		}
		current = resolved
	}
	return StartURLProbe{URL: current}, nil
}

// NormalizeAndValidateStartURL trims, defaults the scheme and validates an
// audit's start URL against the SSRF policy, resolving its hostname.
func (g *Guard) NormalizeAndValidateStartURL(ctx context.Context, input string) (string, error) {
	return validator{guard: g}.normalize(ctx, input)
}

// ResolveStartURLRedirects follows the start URL's redirects, returning the
// anchored URL and the final response's powered-by header.
func (g *Guard) ResolveStartURLRedirects(ctx context.Context, startURL string) (StartURLProbe, error) {
	return validator{guard: g}.resolveRedirects(ctx, startURL)
}

// auditUserAgent is the crawler's identifying user agent. Site owners allowlist
// it in their WAF/bot-protection settings.
const auditUserAgent = "Seomarine-Audit/1.0"
