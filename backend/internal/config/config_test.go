package config

import (
	"maps"
	"strings"
	"testing"
)

// secret is the shortest BETTER_AUTH_SECRET Load accepts.
var secret = strings.Repeat("s", 32)

func TestLoad(t *testing.T) {
	valid := map[string]string{
		"DATABASE_URL":       "postgres://localhost/seomarine",
		"REDIS_URL":          "redis://localhost:6379/0",
		"BETTER_AUTH_SECRET": secret,
		"UPSTREAM_APP_URL":   "http://legacy.internal:3000",
		"PUBLIC_URL":         "https://seomarine.com",
	}
	with := func(key, value string) map[string]string {
		env := maps.Clone(valid)
		env[key] = value
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
		{name: "accepts an https upstream", env: with("UPSTREAM_APP_URL", "https://app.example.com"), wantAddr: ":8080"},
		{name: "accepts trusted proxy CIDRs", env: with("TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 2001:db8::/32"), wantAddr: ":8080"},
		{name: "rejects malformed trusted proxy CIDR", env: with("TRUSTED_PROXY_CIDRS", "10.0.0.1"), wantErr: true},
		{name: "rejects trusting all clients", env: with("TRUSTED_PROXY_CIDRS", "0.0.0.0/0"), wantErr: true},
		{name: "requires DATABASE_URL", env: with("DATABASE_URL", ""), wantErr: true},
		{name: "requires REDIS_URL", env: with("REDIS_URL", ""), wantErr: true},
		{name: "requires BETTER_AUTH_SECRET", env: with("BETTER_AUTH_SECRET", ""), wantErr: true},
		{name: "rejects a 31-character secret", env: with("BETTER_AUTH_SECRET", secret[:31]), wantErr: true},
		{name: "requires UPSTREAM_APP_URL", env: with("UPSTREAM_APP_URL", ""), wantErr: true},
		{name: "rejects a relative upstream", env: with("UPSTREAM_APP_URL", "/app"), wantErr: true},
		{name: "rejects an upstream without a host", env: with("UPSTREAM_APP_URL", "http://"), wantErr: true},
		{name: "rejects a non-http upstream", env: with("UPSTREAM_APP_URL", "ftp://legacy.internal"), wantErr: true},
		{name: "rejects a malformed upstream", env: with("UPSTREAM_APP_URL", "http://[::1"), wantErr: true},
		{name: "rejects a non-numeric port", env: with("PORT", "http"), wantErr: true},
		{name: "rejects port zero", env: with("PORT", "0"), wantErr: true},
		{name: "rejects a port above 65535", env: with("PORT", "65536"), wantErr: true},
		{name: "rejects a negative port", env: with("PORT", "-1"), wantErr: true},
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
				cfg.BetterAuthSecret != secret || cfg.UpstreamAppURL.String() != tt.env["UPSTREAM_APP_URL"] {
				t.Errorf("Load() = %+v, does not carry the environment through", cfg)
			}
		})
	}
}

func TestLoadPublicURL(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":       "postgres://localhost/seomarine",
		"REDIS_URL":          "redis://localhost:6379/0",
		"BETTER_AUTH_SECRET": secret,
		"UPSTREAM_APP_URL":   "http://legacy.internal:3000",
	}
	tests := []struct {
		value, want string // want "" means an error
	}{
		{value: "https://seomarine.com", want: "https://seomarine.com"},
		{value: "https://seomarine.com/", want: "https://seomarine.com"},
		{value: "http://localhost:8080", want: "http://localhost:8080"},
		{value: ""},
		{value: "seomarine.com"},
		{value: "ftp://seomarine.com"},
		{value: "https://"},
		{value: "https://seomarine.com/app"},
		{value: "https://seomarine.com/?ref=x"},
		{value: "https://seomarine.com/#top"},
		{value: "https://user:pass@seomarine.com"},
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
		"DATABASE_URL":       "postgres://localhost/seomarine",
		"REDIS_URL":          "redis://localhost:6379/0",
		"BETTER_AUTH_SECRET": secret,
		"UPSTREAM_APP_URL":   "http://legacy.internal:3000",
		"PUBLIC_URL":         "https://seomarine.com",
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
	want := Razorpay{KeyID: "rzp_test_key", KeySecret: "key-secret", WebhookSecret: "webhook-secret", PlanIDPro: "plan_pro"}
	if err != nil || cfg.Razorpay == nil || *cfg.Razorpay != want {
		t.Fatalf("with every RAZORPAY_* Load() = %+v, %v; want %+v", cfg.Razorpay, err, want)
	}

	for key := range full {
		if _, ok := base[key]; ok {
			continue
		}
		partial := maps.Clone(full)
		delete(partial, key)
		if _, err := load(partial); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("without %s Load() error = %v, want one naming it", key, err)
		}
	}
}
