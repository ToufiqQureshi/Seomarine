// Package config loads the server configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Config is the validated server configuration.
type Config struct {
	// Addr is the TCP address the HTTP server listens on, e.g. ":8080".
	Addr string
	// DatabaseURL is the Postgres connection string.
	DatabaseURL string
	// RedisURL is the Redis connection string, e.g. "redis://localhost:6379/0".
	RedisURL string
	// BetterAuthSecret is the legacy app's BETTER_AUTH_SECRET, which signs the
	// session cookie.
	BetterAuthSecret string
	// UpstreamAppURL is the legacy TypeScript app that receives every request
	// the Go server does not route itself.
	UpstreamAppURL *url.URL
	// PublicURL is the site's public origin, e.g. https://seomarine.com. The
	// landing and pricing pages build canonical and Open Graph URLs from it.
	PublicURL *url.URL
	// Razorpay is the payment configuration, nil when billing is off.
	Razorpay *Razorpay
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

	rzp, err := loadRazorpay(getenv)
	if err != nil {
		return Config{}, err
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
		Addr:             fmt.Sprintf(":%d", port),
		DatabaseURL:      databaseURL,
		RedisURL:         redisURL,
		BetterAuthSecret: secret,
		UpstreamAppURL:   upstream,
		PublicURL:        public,
		Razorpay:         rzp,
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
