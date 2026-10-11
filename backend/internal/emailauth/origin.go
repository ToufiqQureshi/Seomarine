package emailauth

import (
	"net/http"
	"net/url"
	"strings"
)

// origins decides which origins and redirect targets the auth routes trust.
type origins struct {
	trusted []string
}

func newOrigins(base *url.URL, extra ...string) origins {
	o := origins{trusted: []string{originOf(base)}}
	for _, e := range extra {
		if parsed, err := url.Parse(e); err == nil && parsed.Host != "" {
			o.trusted = append(o.trusted, originOf(parsed))
		}
	}
	return o
}

func originOf(u *url.URL) string { return u.Scheme + "://" + u.Host }

func (o origins) isTrusted(origin string) bool {
	for _, t := range o.trusted {
		if origin == t {
			return true
		}
	}
	return false
}

// callbackAllowed reports whether a post-auth redirect target is safe: a path on
// this site, or a URL on a trusted origin. Anything else would let a crafted
// verification or reset link bounce the user, token included, to another site.
func (o origins) callbackAllowed(target string) bool {
	if target == "" {
		return true
	}
	for _, r := range target {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	if strings.HasPrefix(target, "/") {
		// "//host" and "/\host" are protocol-relative URLs to browsers, and the
		// encoded forms decode to them.
		if len(target) > 1 {
			switch next := target[1]; {
			case next == '/' || next == '\\':
				return false
			case next == '%' && len(target) >= 4 && strings.EqualFold(target[2:4], "2f"), next == '%' && len(target) >= 4 && strings.EqualFold(target[2:4], "5c"):
				return false
			}
		}
		return true
	}
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return false
	}
	return o.isTrusted(originOf(parsed))
}

// requestAllowed is the CSRF guard for state-changing requests. A request that
// carries cookies must come from a trusted origin; one without cookies cannot
// ride a victim's session, so it passes.
func (o origins) requestAllowed(r *http.Request) bool {
	if r.Header.Get("Cookie") == "" {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		if referer := r.Header.Get("Referer"); referer != "" {
			if parsed, err := url.Parse(referer); err == nil && parsed.Host != "" {
				origin = originOf(parsed)
			}
		}
	}
	return origin != "" && o.isTrusted(origin)
}
