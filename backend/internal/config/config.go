// Package config loads the server configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// Config is the validated server configuration.
type Config struct {
	// AuthMode matches the legacy runtime mode; only hosted deployments may expose public reports.
	AuthMode string
	// Addr is the TCP address the HTTP server listens on, e.g. ":8080".
	Addr string
	// DatabaseURL is the Postgres connection string.
	DatabaseURL string
	// RedisURL is the Redis connection string, e.g. "redis://localhost:6379/0".
	RedisURL string
	// BetterAuthSecret is the legacy app's BETTER_AUTH_SECRET, which signs the
	// session cookie.
	BetterAuthSecret string
	// GoogleClientID and GoogleClientSecret configure incremental GSC/GA4 consent.
	GoogleClientID     string
	GoogleClientSecret string
	// GoogleTokenEncryptionKey is an optional base64 AES-256 key. When empty,
	// the server derives a domain-separated key from BETTER_AUTH_SECRET.
	GoogleTokenEncryptionKey string
	// UpstreamAppURL is the legacy TypeScript app that receives every request
	// the Go server does not route itself.
	UpstreamAppURL *url.URL
	// PublicURL is the site's public origin, e.g. https://seomarine.com. The
	// landing and pricing pages build canonical and Open Graph URLs from it.
	PublicURL *url.URL
	// BetterAuthURL is the hosted app origin used for invitation links.
	BetterAuthURL *url.URL
	// Razorpay is the payment configuration, nil when billing is off.
	Razorpay *Razorpay
	// DataForSEOAPIKey is the base64 "login:password" of the DataForSEO
	// account behind AI search. Empty turns those endpoints off.
	DataForSEOAPIKey string
	// OpenRouterAPIKey enables the self-hosted SAM agent.
	OpenRouterAPIKey string
	// AutumnSecretKey enables hosted billing-usage event history.
	AutumnSecretKey string
	// Loops configures hosted teammate invitation email.
	LoopsAPIKey               string
	LoopsInvitationTemplateID string
	// TrustedProxyCIDRs are peers allowed to supply client IP headers.
	TrustedProxyCIDRs []netip.Prefix
}

// Razorpay holds the Razorpay credentials and the plan that is sold.
type Razorpay struct {
	KeyID         string
	KeySecret     string
	WebhookSecret string
	PlanIDPro     string
}

const (
	defaultPort = 8080
	// minSecretLength matches the legacy app's own check on BETTER_AUTH_SECRET.
	minSecretLength = 32
)

// Load builds a Config from getenv, which is os.Getenv in production and a
// map lookup in tests.
func Load(getenv func(string) string) (Config, error) {
	databaseURL := getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	redisURL := getenv("REDIS_URL")
	if redisURL == "" {
		return Config{}, errors.New("REDIS_URL is required")
	}

	secret := getenv("BETTER_AUTH_SECRET")
	if len(secret) < minSecretLength {
		return Config{}, fmt.Errorf("BETTER_AUTH_SECRET must be at least %d characters", minSecretLength)
	}
	authMode := strings.TrimSpace(getenv("AUTH_MODE"))
	if authMode == "" {
		authMode = "cloudflare_access"
	}
	if authMode != "cloudflare_access" && authMode != "local_noauth" && authMode != "hosted" {
		return Config{}, fmt.Errorf("AUTH_MODE must be cloudflare_access, local_noauth, or hosted")
	}

	upstream, err := url.Parse(getenv("UPSTREAM_APP_URL"))
	if err != nil || (upstream.Scheme != "http" && upstream.Scheme != "https") || upstream.Host == "" {
		return Config{}, errors.New("UPSTREAM_APP_URL must be an absolute http or https URL")
	}

	public, err := url.Parse(getenv("PUBLIC_URL"))
	if err != nil || (public.Scheme != "http" && public.Scheme != "https") || public.Host == "" ||
		(public.Path != "" && public.Path != "/") || public.RawQuery != "" || public.Fragment != "" || public.User != nil {
		return Config{}, errors.New("PUBLIC_URL must be an http or https origin such as https://seomarine.com, without a path")
	}
	public.Path = ""
	betterAuthURL := public
	if raw := strings.TrimSpace(getenv("BETTER_AUTH_URL")); raw != "" {
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return Config{}, errors.New("BETTER_AUTH_URL must be an absolute http or https origin")
		}
		parsed.Path = ""
		betterAuthURL = parsed
	}

	rzp, err := loadRazorpay(getenv)
	if err != nil {
		return Config{}, err
	}
	var trusted []netip.Prefix
	if raw := strings.TrimSpace(getenv("TRUSTED_PROXY_CIDRS")); raw != "" {
		for part := range strings.SplitSeq(raw, ",") {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
			if err != nil {
				return Config{}, fmt.Errorf("TRUSTED_PROXY_CIDRS contains invalid CIDR %q: %w", part, err)
			}
			if prefix.Bits() == 0 {
				return Config{}, errors.New("TRUSTED_PROXY_CIDRS cannot trust every address")
			}
			trusted = append(trusted, prefix.Masked())
		}
	}

	port := defaultPort
	if raw := getenv("PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			return Config{}, fmt.Errorf("PORT must be a number from 1 to 65535, got %q", raw)
		}
		port = parsed
	}

	return Config{
		AuthMode:                  authMode,
		Addr:                      fmt.Sprintf(":%d", port),
		DatabaseURL:               databaseURL,
		RedisURL:                  redisURL,
		BetterAuthSecret:          secret,
		GoogleClientID:            strings.TrimSpace(getenv("GOOGLE_CLIENT_ID")),
		GoogleClientSecret:        strings.TrimSpace(getenv("GOOGLE_CLIENT_SECRET")),
		GoogleTokenEncryptionKey:  strings.TrimSpace(getenv("GOOGLE_TOKEN_ENCRYPTION_KEY")),
		UpstreamAppURL:            upstream,
		PublicURL:                 public,
		BetterAuthURL:             betterAuthURL,
		Razorpay:                  rzp,
		DataForSEOAPIKey:          strings.TrimSpace(getenv("DATAFORSEO_API_KEY")),
		OpenRouterAPIKey:          strings.TrimSpace(getenv("OPENROUTER_API_KEY")),
		AutumnSecretKey:           strings.TrimSpace(getenv("AUTUMN_SECRET_KEY")),
		LoopsAPIKey:               strings.TrimSpace(getenv("LOOPS_API_KEY")),
		LoopsInvitationTemplateID: strings.TrimSpace(getenv("LOOPS_TRANSACTIONAL_INVITATION_ID")),
		TrustedProxyCIDRs:         trusted,
	}, nil
}

// loadRazorpay returns nil when no RAZORPAY_* variable is set, so the server
// runs without billing, and an error when only some are: a half-configured
// payment setup would fail at checkout or drop webhooks.
func loadRazorpay(getenv func(string) string) (*Razorpay, error) {
	names := []string{"RAZORPAY_KEY_ID", "RAZORPAY_KEY_SECRET", "RAZORPAY_WEBHOOK_SECRET", "RAZORPAY_PLAN_ID_PRO"}
	values := make([]string, len(names))
	var missing []string
	for i, name := range names {
		values[i] = getenv(name)
		if values[i] == "" {
			missing = append(missing, name)
		}
	}
	switch len(missing) {
	case len(names):
		return nil, nil
	case 0:
		return &Razorpay{KeyID: values[0], KeySecret: values[1], WebhookSecret: values[2], PlanIDPro: values[3]}, nil
	default:
		return nil, fmt.Errorf("set all RAZORPAY_* variables or none; missing %s", strings.Join(missing, ", "))
	}
}
