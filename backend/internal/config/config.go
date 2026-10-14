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
	// StaticDir is the built React app (Vite output) the server serves for
	// every page it does not render itself. Default: dist.
	StaticDir string
	// PublicURL is the site's public origin, e.g. https://uplumo.com. The
	// landing and pricing pages build canonical and Open Graph URLs from it.
	PublicURL *url.URL
	// BetterAuthURL is the hosted app origin used for invitation links.
	BetterAuthURL *url.URL
	// Razorpay is the payment configuration, nil when billing is off.
	Razorpay *Razorpay
	// DataForSEOAPIKey is the base64 "login:password" of the DataForSEO
	// account behind AI search. Empty turns those endpoints off.
	DataForSEOAPIKey string
	// OpenRouterAPIKey is Uplumo's own model key for SAM.
	OpenRouterAPIKey string
	// StaffEmails are the people whose own workspaces are never limited by a
	// plan, lower case.
	StaffEmails []string
	// ProviderDailyLimitUSD stops every paid DataForSEO and AI request, for
	// all workspaces, once today's (UTC) provider spend reaches it.
	ProviderDailyLimitUSD float64
	// OpenRouterModel overrides the SAM chat model (OPENROUTER_MODEL).
	OpenRouterModel string
	// Loops configures hosted teammate invitation email.
	LoopsAPIKey               string
	LoopsInvitationTemplateID string
	// Loops templates for the sign-up verification and password reset emails.
	LoopsVerifyEmailTemplateID   string
	LoopsResetPasswordTemplateID string
	// TurnstileSecretKey turns on the sign-up captcha when set.
	TurnstileSecretKey string
	// OpenAIAppsChallenge is the public verification token served at
	// /.well-known/openai-apps-challenge; unset means the path is a 404.
	OpenAIAppsChallenge string
	// BypassEmailVerification skips email verification (development only).
	BypassEmailVerification bool
	// SentryDSN turns on error reporting to Sentry when set.
	SentryDSN string
	// Environment names where the server runs in error reports: production
	// on a live site, development anywhere else.
	Environment string
	// Release is the deployed commit, which Railway provides.
	Release string
	// TrustedProxyCIDRs are peers allowed to supply client IP headers.
	TrustedProxyCIDRs []netip.Prefix
	// SupabaseStorage configures the optional public media bucket used by
	// editor uploads. The service role key stays on the server.
	SupabaseStorage *SupabaseStorage
}

// SupabaseStorage contains the project API URL, server-only service key and
// public bucket used for editorial images.
type SupabaseStorage struct {
	URL                *url.URL
	ServiceRoleKey     string
	ContentImageBucket string
}

// Razorpay holds the Razorpay credentials and the plan that is sold.
type Razorpay struct {
	KeyID         string
	KeySecret     string
	WebhookSecret string
	// PlanIDs maps a plan ("basic", "pro", "agency") to the Razorpay plan
	// that bills it. A plan without an id cannot be bought.
	PlanIDs map[string]string
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

	staticDir := strings.TrimSpace(getenv("STATIC_DIR"))
	if staticDir == "" {
		staticDir = "dist"
	}

	public, err := url.Parse(getenv("PUBLIC_URL"))
	if err != nil || (public.Scheme != "http" && public.Scheme != "https") || public.Host == "" ||
		(public.Path != "" && public.Path != "/") || public.RawQuery != "" || public.Fragment != "" || public.User != nil {
		return Config{}, errors.New("PUBLIC_URL must be an http or https origin such as https://uplumo.com, without a path")
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

	var staffEmails []string
	for email := range strings.SplitSeq(getenv("STAFF_EMAILS"), ",") {
		if email = strings.ToLower(strings.TrimSpace(email)); email != "" {
			staffEmails = append(staffEmails, email)
		}
	}

	dailyLimit := 100.0
	if raw := getenv("PROVIDER_DAILY_LIMIT_USD"); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || !(parsed >= 1 && parsed <= 10000) {
			return Config{}, fmt.Errorf("PROVIDER_DAILY_LIMIT_USD must be a number from 1 to 10000, got %q", raw)
		}
		dailyLimit = parsed
	}

	dataForSEOKey := strings.TrimSpace(getenv("DATAFORSEO_API_KEY"))
	openRouterKey := strings.TrimSpace(getenv("OPENROUTER_API_KEY"))
	supabaseStorage, err := loadSupabaseStorage(getenv)
	if err != nil {
		return Config{}, err
	}
	bypassVerification := strings.TrimSpace(getenv("BYPASS_EMAIL_VERIFICATION")) == "true"
	environment := "development"
	if isLive(public) {
		environment = "production"
		if bypassVerification {
			return Config{}, errors.New("BYPASS_EMAIL_VERIFICATION is for local development and cannot be set on a live site")
		}
		// Without billing nothing limits usage, so every lookup would be
		// charged to our provider accounts with no plan behind it.
		if rzp == nil && (dataForSEOKey != "" || openRouterKey != "") {
			return Config{}, errors.New("a live site with DATAFORSEO_API_KEY or OPENROUTER_API_KEY also needs the RAZORPAY_* variables")
		}
	}

	return Config{
		StaffEmails:                  staffEmails,
		ProviderDailyLimitUSD:        dailyLimit,
		Addr:                         fmt.Sprintf(":%d", port),
		DatabaseURL:                  databaseURL,
		RedisURL:                     redisURL,
		BetterAuthSecret:             secret,
		GoogleClientID:               strings.TrimSpace(getenv("GOOGLE_CLIENT_ID")),
		GoogleClientSecret:           strings.TrimSpace(getenv("GOOGLE_CLIENT_SECRET")),
		GoogleTokenEncryptionKey:     strings.TrimSpace(getenv("GOOGLE_TOKEN_ENCRYPTION_KEY")),
		StaticDir:                    staticDir,
		PublicURL:                    public,
		BetterAuthURL:                betterAuthURL,
		Razorpay:                     rzp,
		DataForSEOAPIKey:             dataForSEOKey,
		OpenRouterAPIKey:             openRouterKey,
		OpenRouterModel:              strings.TrimSpace(getenv("OPENROUTER_MODEL")),
		LoopsAPIKey:                  strings.TrimSpace(getenv("LOOPS_API_KEY")),
		LoopsInvitationTemplateID:    strings.TrimSpace(getenv("LOOPS_TRANSACTIONAL_INVITATION_ID")),
		LoopsVerifyEmailTemplateID:   strings.TrimSpace(getenv("LOOPS_TRANSACTIONAL_VERIFY_EMAIL_ID")),
		LoopsResetPasswordTemplateID: strings.TrimSpace(getenv("LOOPS_TRANSACTIONAL_RESET_PASSWORD_ID")),
		TurnstileSecretKey:           strings.TrimSpace(getenv("TURNSTILE_SECRET_KEY")),
		OpenAIAppsChallenge:          strings.TrimSpace(getenv("OPENAI_APPS_CHALLENGE")),
		BypassEmailVerification:      bypassVerification,
		SentryDSN:                    strings.TrimSpace(getenv("SENTRY_DSN")),
		Environment:                  environment,
		Release:                      strings.TrimSpace(getenv("RAILWAY_GIT_COMMIT_SHA")),
		TrustedProxyCIDRs:            trusted,
		SupabaseStorage:              supabaseStorage,
	}, nil
}

func loadSupabaseStorage(getenv func(string) string) (*SupabaseStorage, error) {
	rawURL := strings.TrimSpace(getenv("SUPABASE_URL"))
	serviceRoleKey := strings.TrimSpace(getenv("SUPABASE_SERVICE_ROLE_KEY"))
	bucket := strings.TrimSpace(getenv("SUPABASE_CONTENT_IMAGE_BUCKET"))
	configured := rawURL != "" || serviceRoleKey != "" || bucket != ""
	if !configured {
		return nil, nil
	}
	if rawURL == "" || serviceRoleKey == "" || bucket == "" {
		return nil, errors.New("SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY, and SUPABASE_CONTENT_IMAGE_BUCKET must be set together")
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil || parsedURL.RawQuery != "" || parsedURL.Fragment != "" || (parsedURL.Path != "" && parsedURL.Path != "/") {
		return nil, errors.New("SUPABASE_URL must be an https project origin without credentials, path, query, or fragment")
	}
	if strings.Trim(bucket, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "" || strings.ContainsAny(serviceRoleKey, "\r\n") {
		return nil, errors.New("SUPABASE_CONTENT_IMAGE_BUCKET or SUPABASE_SERVICE_ROLE_KEY is invalid")
	}
	parsedURL.Path = ""
	return &SupabaseStorage{URL: parsedURL, ServiceRoleKey: serviceRoleKey, ContentImageBucket: bucket}, nil
}

// isLive reports whether the site is served to the public: over https, on a
// host that is not this machine.
func isLive(public *url.URL) bool {
	host := public.Hostname()
	return public.Scheme == "https" && host != "localhost" && !strings.HasSuffix(host, ".localhost") &&
		host != "127.0.0.1" && host != "::1"
}

// loadRazorpay returns nil when no RAZORPAY_* variable is set, so the server
// runs without billing, and an error when only some are: a half-configured
// payment setup would fail at checkout or drop webhooks.
func loadRazorpay(getenv func(string) string) (*Razorpay, error) {
	names := []string{"RAZORPAY_KEY_ID", "RAZORPAY_KEY_SECRET", "RAZORPAY_WEBHOOK_SECRET"}
	values := make([]string, len(names))
	var missing []string
	for i, name := range names {
		values[i] = getenv(name)
		if values[i] == "" {
			missing = append(missing, name)
		}
	}
	planIDs := map[string]string{}
	for plan, name := range map[string]string{
		"basic": "RAZORPAY_PLAN_ID_BASIC", "pro": "RAZORPAY_PLAN_ID_PRO", "agency": "RAZORPAY_PLAN_ID_AGENCY",
	} {
		if id := strings.TrimSpace(getenv(name)); id != "" {
			planIDs[plan] = id
		}
	}
	switch {
	case len(missing) == len(names) && len(planIDs) == 0:
		return nil, nil
	case len(missing) > 0:
		return nil, fmt.Errorf("set RAZORPAY_KEY_ID, RAZORPAY_KEY_SECRET, RAZORPAY_WEBHOOK_SECRET and a plan id, or none; missing %s", strings.Join(missing, ", "))
	case len(planIDs) == 0:
		return nil, errors.New("set at least one of RAZORPAY_PLAN_ID_BASIC, RAZORPAY_PLAN_ID_PRO, RAZORPAY_PLAN_ID_AGENCY: without a plan nothing can be bought")
	}
	return &Razorpay{KeyID: values[0], KeySecret: values[1], WebhookSecret: values[2], PlanIDs: planIDs}, nil
}
