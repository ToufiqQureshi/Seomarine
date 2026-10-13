package config

import (
	"maps"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// secret is the shortest BETTER_AUTH_SECRET Load accepts.
var secret = strings.Repeat("s", 32)

func TestLoad(t *testing.T) {
	valid := map[string]string{
		"DATABASE_URL":       "postgres://localhost/uplumo",
		"REDIS_URL":          "redis://localhost:6379/0",
		"BETTER_AUTH_SECRET": secret,
		"PUBLIC_URL":         "https://uplumo.com",
	}
	with := func(key, value string) map[string]string {
		env := maps.Clone(valid)
		env[key] = value
		return env
	}

	withAll := func(extra map[string]string) map[string]string {
		env := maps.Clone(valid)
		maps.Copy(env, extra)
		return env
	}

	tests := []struct {
		name     string
		env      map[string]string
		wantAddr string
		wantErr  bool
	}{
		{name: "defaults the port", env: valid, wantAddr: ":8080"},
		{name: "uses PORT", env: with("PORT", "3000"), wantAddr: ":3000"},
		{name: "accepts the highest port", env: with("PORT", "65535"), wantAddr: ":65535"},
		{name: "accepts trusted proxy CIDRs", env: with("TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 2001:db8::/32"), wantAddr: ":8080"},
		{name: "rejects malformed trusted proxy CIDR", env: with("TRUSTED_PROXY_CIDRS", "10.0.0.1"), wantErr: true},
		{name: "rejects trusting all clients", env: with("TRUSTED_PROXY_CIDRS", "0.0.0.0/0"), wantErr: true},
		{name: "requires DATABASE_URL", env: with("DATABASE_URL", ""), wantErr: true},
		{name: "requires REDIS_URL", env: with("REDIS_URL", ""), wantErr: true},
		{name: "requires BETTER_AUTH_SECRET", env: with("BETTER_AUTH_SECRET", ""), wantErr: true},
		{name: "rejects a 31-character secret", env: with("BETTER_AUTH_SECRET", secret[:31]), wantErr: true},
		{name: "rejects a non-numeric port", env: with("PORT", "http"), wantErr: true},
		{name: "rejects port zero", env: with("PORT", "0"), wantErr: true},
		{name: "rejects a port above 65535", env: with("PORT", "65536"), wantErr: true},
		{name: "rejects a negative port", env: with("PORT", "-1"), wantErr: true},
		{name: "rejects skipping email verification on a live site", env: with("BYPASS_EMAIL_VERIFICATION", "true"), wantErr: true},
		{name: "rejects a provider key without billing on a live site", env: with("DATAFORSEO_API_KEY", "key"), wantErr: true},
		{name: "rejects a model key without billing on a live site", env: with("OPENROUTER_API_KEY", "key"), wantErr: true},
		{name: "allows a provider key without billing locally", env: withAll(map[string]string{"PUBLIC_URL": "http://localhost:8080", "DATAFORSEO_API_KEY": "key", "BYPASS_EMAIL_VERIFICATION": "true"}), wantAddr: ":8080"},
		{name: "allows a provider key with billing on a live site", env: withAll(map[string]string{"DATAFORSEO_API_KEY": "key", "RAZORPAY_KEY_ID": "a", "RAZORPAY_KEY_SECRET": "b", "RAZORPAY_WEBHOOK_SECRET": "c", "RAZORPAY_PLAN_ID_PRO": "d"}), wantAddr: ":8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(func(key string) string { return tt.env[key] })
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() = %+v, want an error", cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Addr != tt.wantAddr {
				t.Errorf("Addr = %q, want %q", cfg.Addr, tt.wantAddr)
			}
			if cfg.DatabaseURL != tt.env["DATABASE_URL"] || cfg.RedisURL != tt.env["REDIS_URL"] ||
				cfg.BetterAuthSecret != secret {
				t.Errorf("Load() = %+v, does not carry the environment through", cfg)
			}
		})
	}
}

func TestLoadPublicURL(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":       "postgres://localhost/uplumo",
		"REDIS_URL":          "redis://localhost:6379/0",
		"BETTER_AUTH_SECRET": secret,
	}
	tests := []struct {
		value, want string // want "" means an error
	}{
		{value: "https://uplumo.com", want: "https://uplumo.com"},
		{value: "https://uplumo.com/", want: "https://uplumo.com"},
		{value: "http://localhost:8080", want: "http://localhost:8080"},
		{value: ""},
		{value: "uplumo.com"},
		{value: "ftp://uplumo.com"},
		{value: "https://"},
		{value: "https://uplumo.com/app"},
		{value: "https://uplumo.com/?ref=x"},
		{value: "https://uplumo.com/#top"},
		{value: (&url.URL{Scheme: "https", User: url.UserPassword("user", "pass"), Host: "uplumo.com"}).String()},
		{value: "https://[::1"},
	}
	for _, tt := range tests {
		env["PUBLIC_URL"] = tt.value
		cfg, err := Load(func(key string) string { return env[key] })
		switch {
		case tt.want == "" && err == nil:
			t.Errorf("PUBLIC_URL %q: Load() = %v, want an error", tt.value, cfg.PublicURL)
		case tt.want != "" && err != nil:
			t.Errorf("PUBLIC_URL %q: Load() error = %v", tt.value, err)
		case tt.want != "" && cfg.PublicURL.String() != tt.want:
			t.Errorf("PUBLIC_URL %q: PublicURL = %q, want %q", tt.value, cfg.PublicURL, tt.want)
		}
	}
}

func TestLoadRazorpay(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL":       "postgres://localhost/uplumo",
		"REDIS_URL":          "redis://localhost:6379/0",
		"BETTER_AUTH_SECRET": secret,
		"PUBLIC_URL":         "https://uplumo.com",
	}
	full := maps.Clone(base)
	maps.Copy(full, map[string]string{
		"RAZORPAY_KEY_ID":         "rzp_test_key",
		"RAZORPAY_KEY_SECRET":     "key-secret",
		"RAZORPAY_WEBHOOK_SECRET": "webhook-secret",
		"RAZORPAY_PLAN_ID_PRO":    "plan_pro",
	})
	load := func(env map[string]string) (Config, error) {
		return Load(func(key string) string { return env[key] })
	}

	cfg, err := load(base)
	if err != nil || cfg.Razorpay != nil {
		t.Fatalf("without RAZORPAY_* Load() = %+v, %v; want billing off", cfg.Razorpay, err)
	}

	cfg, err = load(full)
	want := Razorpay{KeyID: "rzp_test_key", KeySecret: "key-secret", WebhookSecret: "webhook-secret", PlanIDs: map[string]string{"pro": "plan_pro"}}
	if err != nil || cfg.Razorpay == nil || !reflect.DeepEqual(*cfg.Razorpay, want) {
		t.Fatalf("with every RAZORPAY_* Load() = %+v, %v; want %+v", cfg.Razorpay, err, want)
	}

	for _, key := range []string{"RAZORPAY_KEY_ID", "RAZORPAY_KEY_SECRET", "RAZORPAY_WEBHOOK_SECRET"} {
		if _, ok := base[key]; ok {
			continue
		}
		partial := maps.Clone(full)
		delete(partial, key)
		if _, err := load(partial); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("without %s Load() error = %v, want one naming it", key, err)
		}
	}

	plans := maps.Clone(full)
	maps.Copy(plans, map[string]string{
		"RAZORPAY_PLAN_ID_BASIC":  "plan_basic",
		"RAZORPAY_PLAN_ID_PRO":    " plan_pro_tier ",
		"RAZORPAY_PLAN_ID_AGENCY": "plan_agency",
	})
	cfg, err = load(plans)
	if err != nil || cfg.Razorpay.PlanIDs["basic"] != "plan_basic" || cfg.Razorpay.PlanIDs["pro"] != "plan_pro_tier" || cfg.Razorpay.PlanIDs["agency"] != "plan_agency" {
		t.Fatalf("tier Razorpay config = %+v, %v", cfg.Razorpay, err)
	}
}

func TestLoadDataForSEOKey(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":       "postgres://localhost/uplumo",
		"REDIS_URL":          "redis://localhost:6379/0",
		"BETTER_AUTH_SECRET": secret,
		"PUBLIC_URL":         "http://localhost:8080",
	}
	load := func() Config {
		t.Helper()
		cfg, err := Load(func(key string) string { return env[key] })
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		return cfg
	}

	if got := load().DataForSEOAPIKey; got != "" {
		t.Errorf("without DATAFORSEO_API_KEY the key = %q, want empty (AI search off)", got)
	}
	env["DATAFORSEO_API_KEY"] = "  placeholder-key \n"
	if got := load().DataForSEOAPIKey; got != "placeholder-key" {
		t.Errorf("key = %q, want it trimmed", got)
	}
}

func TestLoadErrorReporting(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":       "postgres://localhost/uplumo",
		"REDIS_URL":          "redis://localhost:6379/0",
		"BETTER_AUTH_SECRET": secret,
		"PUBLIC_URL":         "http://localhost:8080",
	}
	load := func() Config {
		t.Helper()
		cfg, err := Load(func(key string) string { return env[key] })
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		return cfg
	}

	cfg := load()
	if cfg.SentryDSN != "" || cfg.Release != "" {
		t.Errorf("unset: DSN %q, release %q; want both empty (reporting off)", cfg.SentryDSN, cfg.Release)
	}
	if cfg.Environment != "development" {
		t.Errorf("environment on localhost = %q, want development", cfg.Environment)
	}

	env["SENTRY_DSN"] = "  https://public@example.invalid/1 \n"
	env["RAILWAY_GIT_COMMIT_SHA"] = " abc123 "
	env["PUBLIC_URL"] = "https://uplumo.com"
	cfg = load()
	if cfg.SentryDSN != "https://public@example.invalid/1" || cfg.Release != "abc123" {
		t.Errorf("DSN %q, release %q; want both trimmed", cfg.SentryDSN, cfg.Release)
	}
	if cfg.Environment != "production" {
		t.Errorf("environment on a live https site = %q, want production", cfg.Environment)
	}
}

func TestLoadRazorpayNeedsCredentialsAndAPlanTogether(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL":       "postgres://localhost/uplumo",
		"REDIS_URL":          "redis://localhost:6379/0",
		"BETTER_AUTH_SECRET": secret,
		"PUBLIC_URL":         "http://localhost:8080",
	}
	with := func(extra map[string]string) error {
		env := maps.Clone(base)
		maps.Copy(env, extra)
		_, err := Load(func(key string) string { return env[key] })
		return err
	}
	credentials := map[string]string{"RAZORPAY_KEY_ID": "id", "RAZORPAY_KEY_SECRET": "secret", "RAZORPAY_WEBHOOK_SECRET": "hook"}

	if err := with(credentials); err == nil || !strings.Contains(err.Error(), "RAZORPAY_PLAN_ID_") {
		t.Errorf("credentials without a plan id: error = %v, want one naming the plan variables", err)
	}
	if err := with(map[string]string{"RAZORPAY_PLAN_ID_AGENCY": "plan_agency"}); err == nil || !strings.Contains(err.Error(), "RAZORPAY_KEY_ID") {
		t.Errorf("a plan id without credentials: error = %v, want one naming the missing credentials", err)
	}
	one := maps.Clone(credentials)
	one["RAZORPAY_PLAN_ID_BASIC"] = "plan_basic"
	if err := with(one); err != nil {
		t.Errorf("credentials with one plan id: error = %v, want billing on", err)
	}
}

func TestLoadStaffEmails(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":       "postgres://localhost/uplumo",
		"REDIS_URL":          "redis://localhost:6379/0",
		"BETTER_AUTH_SECRET": secret,
		"PUBLIC_URL":         "http://localhost:8080",
	}
	load := func() []string {
		t.Helper()
		cfg, err := Load(func(key string) string { return env[key] })
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		return cfg.StaffEmails
	}
	if got := load(); len(got) != 0 {
		t.Errorf("without STAFF_EMAILS = %v, want nobody", got)
	}
	env["STAFF_EMAILS"] = " Owner@Uplumo.com ,, second@uplumo.com	"
	if got := load(); !slices.Equal(got, []string{"owner@uplumo.com", "second@uplumo.com"}) {
		t.Errorf("STAFF_EMAILS = %v, want both addresses trimmed and lower case", got)
	}
}

func TestLoadProviderDailyLimit(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":       "postgres://localhost/uplumo",
		"REDIS_URL":          "redis://localhost:6379/0",
		"BETTER_AUTH_SECRET": secret,
		"PUBLIC_URL":         "http://localhost:8080",
	}
	load := func() (Config, error) { return Load(func(key string) string { return env[key] }) }
	cfg, err := load()
	if err != nil || cfg.ProviderDailyLimitUSD != 100 {
		t.Fatalf("default limit = %v, %v; want 100", cfg.ProviderDailyLimitUSD, err)
	}
	env["PROVIDER_DAILY_LIMIT_USD"] = "60.5"
	if cfg, err := load(); err != nil || cfg.ProviderDailyLimitUSD != 60.5 {
		t.Fatalf("limit = %v, %v; want 60.5", cfg.ProviderDailyLimitUSD, err)
	}
	// Zero would turn the safety limit off; below a dollar stops the product.
	for _, bad := range []string{"0", "0.5", "-5", "10001", "NaN", "Inf", "ten"} {
		env["PROVIDER_DAILY_LIMIT_USD"] = bad
		if _, err := load(); err == nil || !strings.Contains(err.Error(), "PROVIDER_DAILY_LIMIT_USD") {
			t.Errorf("PROVIDER_DAILY_LIMIT_USD=%q error = %v", bad, err)
		}
	}
}
