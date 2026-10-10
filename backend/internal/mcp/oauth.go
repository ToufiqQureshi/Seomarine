// Package mcp OAuth provider (RFC 8414 + RFC 7591 + OAuth 2.1 subset).
//
// This file implements the OAuth 2.1 authorization server for MCP the way the
// legacy TypeScript provider behaves: dynamic client registration, PKCE,
// authorization code grant with optional refresh, and short-lived access
// tokens. Storage is Postgres (the go_mcp_oauth_* tables from migration
// 00012); tokens and codes are hashed at rest, and every comparison of a
// secret is constant time.
//
// The readySet of endpoints mirrors the legacy app:
//
//	POST   /api/auth/oauth2/register    Dynamic client registration (RFC 7591)
//	GET    /api/auth/oauth2/authorize   Authorization endpoint (response_type=code, PKCE S256)
//	POST   /api/auth/oauth2/token       Token endpoint (authorization_code, refresh_token)
//	GET    /.well-known/oauth-authorization-server  Server metadata (RFC 8414)
//	POST   /api/oauth/consent           Browser consent form target
package mcp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	oauthRegisterPath  = "/api/auth/oauth2/register"
	oauthAuthorizePath = "/api/auth/oauth2/authorize"
	oauthTokenPath     = "/api/auth/oauth2/token"
	oauthConsentPath   = "/api/oauth/consent"
	oauthMetadataPath  = "/.well-known/oauth-authorization-server"

	oauthScope = "mcp"

	accessTokenTTL        = 24 * time.Hour
	refreshTokenTTL       = 30 * 24 * time.Hour
	clientRegistrationTTL = 365 * 24 * time.Hour
	authorizationCodeTTL  = 10 * time.Minute

	maxRegistrationBodyBytes = 1024 * 1024
)

var oauthScopesSupported = []string{"offline_access", oauthScope}

func MountOAuth(mux *http.ServeMux, d Deps) {
	h := oauthHandler{deps: d, log: d.Logger}
	if h.log == nil {
		h.log = slog.Default()
	}
	mux.HandleFunc(oauthRegisterPath, h.registerClient)
	mux.HandleFunc(oauthAuthorizePath, h.authorize)
	mux.HandleFunc(oauthTokenPath, h.token)
	mux.HandleFunc(oauthConsentPath, h.consent)
	mux.HandleFunc(oauthMetadataPath, h.metadata)
}

type oauthHandler struct {
	deps Deps
	log  *slog.Logger
}

func (h *oauthHandler) oauthError(w http.ResponseWriter, status int, code string, description string) {
	if status >= 500 {
		h.log.Error("oauth error", "status", status, "code", code, "description", description)
	} else if status == http.StatusUnauthorized {
		h.log.Debug("oauth error", "status", status, "code", code, "description", description)
	} else {
		h.log.Warn("oauth error", "status", status, "code", code, "description", description)
	}
	writeOAuthJSON(w, status, map[string]string{"error": code, "error_description": description})
}

func writeOAuthJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func tokenHash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return fmt.Sprintf("%x", sum)
}
