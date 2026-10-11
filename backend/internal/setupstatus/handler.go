// Package setupstatus serves the unauthenticated self-host setup report.
package setupstatus

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Pinger is implemented by the Postgres pool used by the server.
type Pinger interface {
	Ping(context.Context) error
}

// Config contains only values needed to report whether features are configured.
// Secret values are never included in responses.
type Config struct {
	Version           string
	AuthMode          string
	TeamDomain        string
	PolicyAudience    string
	DataForSEOKey     string
	GoogleClientID    string
	GoogleClientSecret string
	BetterAuthSecret  string
	OpenRouterAPIKey  string
	ContextAPIKey     string
}

type check struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type response struct {
	Status   string           `json:"status"`
	Version  string           `json:"version,omitempty"`
	AuthMode string           `json:"authMode,omitempty"`
	Checks   map[string]check `json:"checks,omitempty"`
}

// NewHandler returns the setup report used by self-host operators. Hosted
// deployments intentionally reveal no configuration details.
func NewHandler(config Config, db Pinger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if config.AuthMode == "hosted" {
			write(w, http.StatusOK, response{Status: "ok"})
			return
		}

		checks := map[string]check{
			"auth":       authCheck(config),
			"dataforseo": dataForSEOCheck(config.DataForSEOKey),
			"gsc":        googleCheck(config),
			"ai":         optionalCheck(config.OpenRouterAPIKey, "OPENROUTER_API_KEY set", "OPENROUTER_API_KEY not set (optional) — SAM is disabled."),
			"rendering":  optionalCheck(config.ContextAPIKey, "CONTEXT_API_KEY set", "CONTEXT_API_KEY not set (optional) — JavaScript rendering is disabled."),
			"database":   databaseCheck(r.Context(), db),
		}
		status := "ok"
		for _, item := range checks {
			if item.Status == "error" {
				status = "issues"
				break
			}
		}
		write(w, http.StatusOK, response{
			Status: status, Version: config.Version, AuthMode: config.AuthMode, Checks: checks,
		})
	})
}

func authCheck(config Config) check {
	switch config.AuthMode {
	case "local_noauth":
		return check{Status: "ok", Detail: "local_noauth"}
	case "hosted":
		return check{Status: "ok", Detail: "hosted"}
	default:
		if config.TeamDomain == "" || config.PolicyAudience == "" {
			return check{Status: "error", Detail: "cloudflare_access requires TEAM_DOMAIN and POLICY_AUD."}
		}
		return check{Status: "ok", Detail: "cloudflare_access"}
	}
}

func dataForSEOCheck(key string) check {
	if strings.TrimSpace(key) == "" {
		return check{Status: "warn", Detail: "DATAFORSEO_API_KEY not set; SEO data features are unavailable."}
	}
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil || !strings.Contains(string(decoded), ":") {
		return check{Status: "warn", Detail: "DATAFORSEO_API_KEY is set but is not valid base64 login:password."}
	}
	return check{Status: "ok", Detail: "Set"}
}

func googleCheck(config Config) check {
	idSet := strings.TrimSpace(config.GoogleClientID) != ""
	secretSet := strings.TrimSpace(config.GoogleClientSecret) != ""
	if !idSet && !secretSet {
		return check{Status: "ok", Detail: "Google integrations are not configured (optional)."}
	}
	if !idSet || !secretSet {
		return check{Status: "warn", Detail: "Both GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET are required."}
	}
	if len(config.BetterAuthSecret) < 32 {
		return check{Status: "warn", Detail: "Google integrations require BETTER_AUTH_SECRET to be at least 32 characters."}
	}
	return check{Status: "ok", Detail: "Configured"}
}

func optionalCheck(value, configured, missing string) check {
	if strings.TrimSpace(value) == "" {
		return check{Status: "ok", Detail: missing}
	}
	return check{Status: "ok", Detail: configured}
}

func databaseCheck(ctx context.Context, db Pinger) check {
	if db == nil {
		return check{Status: "error", Detail: "Database check is unavailable."}
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := db.Ping(checkCtx); err != nil {
		return check{Status: "error", Detail: "Database query failed — check server logs."}
	}
	return check{Status: "ok"}
}

func write(w http.ResponseWriter, status int, value response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}
