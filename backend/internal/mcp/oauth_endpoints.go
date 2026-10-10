package mcp

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
)

func (h *oauthHandler) authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOAuthJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "invalid_request"})
		return
	}
	writeOAuthJSON(w, http.StatusFound, map[string]string{"error": "consent_required"})
}

func (h *oauthHandler) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOAuthJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "invalid_request"})
		return
	}
	writeOAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
}

func (h *oauthHandler) consent(w http.ResponseWriter, r *http.Request) {
	writeOAuthJSON(w, http.StatusNotImplemented, map[string]string{"error": "not_implemented"})
}

func (h *oauthHandler) metadata(w http.ResponseWriter, r *http.Request) {
	issuer := h.oauthIssuer(r)
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + oauthAuthorizePath,
		"token_endpoint":                        issuer + oauthTokenPath,
		"registration_endpoint":                 issuer + oauthRegisterPath,
		"scopes_supported":                      oauthScopesSupported,
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_post"},
		"code_challenge_methods_supported":      []string{"S256"},
	})
}

func (h *oauthHandler) oauthIssuer(r *http.Request) string {
	if h.deps.PublicURL != nil && h.deps.PublicURL.Host != "" {
		return h.deps.PublicURL.String()
	}
	scheme := "https"
	if r.TLS == nil {
		if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
			scheme = fwd
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + r.Host
}

var _ = sha256.New
var _ = base64.StdEncoding
var _ = url.URL{}
