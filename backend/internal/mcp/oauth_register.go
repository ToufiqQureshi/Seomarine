package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
)

const perplexityCallback = "https://www.perplexity.ai/api/mcp/oauth/callback"

type clientRegistrationRequest struct {
	RedirectURIs      []string `json:"redirect_uris"`
	TokenEndpointAuth string   `json:"token_endpoint_auth_method"`
	ClientName        string   `json:"client_name"`
	GrantTypes        []string `json:"grant_types"`
	ResponseTypes     []string `json:"response_types"`
	Scope             string   `json:"scope"`
}

func (h *oauthHandler) registerClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOAuthJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "invalid_request", "error_description": "Method not allowed."})
		return
	}
	if r.ContentLength > maxRegistrationBodyBytes {
		h.oauthError(w, http.StatusRequestEntityTooLarge, "invalid_request", "Registration body too large.")
		return
	}
	var req clientRegistrationRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(maxRegistrationBodyBytes)))
	if err := decoder.Decode(&req); err != nil {
		h.oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "Invalid JSON body.")
		return
	}
	if len(req.RedirectURIs) == 0 {
		h.oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "redirect_uris is required.")
		return
	}

	authMethod := normalizeAuthMethod(req.TokenEndpointAuth, req.RedirectURIs)

	secret, err := randomToken(32)
	if err != nil {
		h.oauthError(w, http.StatusInternalServerError, "server_error", "Could not generate a client secret.")
		return
	}
	clientID, err := ids.New()
	if err != nil {
		h.oauthError(w, http.StatusInternalServerError, "server_error", "Could not generate a client id.")
		return
	}

	recordID, err := ids.New()
	if err != nil {
		h.oauthError(w, http.StatusInternalServerError, "server_error", "Could not generate a client record.")
		return
	}
	now := time.Now().UTC()
	redirects := append([]string(nil), req.RedirectURIs...)
	if _, err := h.deps.DB.Exec(r.Context(), `
		INSERT INTO go_mcp_oauth_clients (id, client_id, client_secret, redirect_uris, token_endpoint_auth_method, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6 + make_interval(secs => $7))`,
		recordID, clientID, tokenHash(secret), redirects, authMethod, now, int64((clientRegistrationTTL / time.Second)),
	); err != nil {
		h.log.ErrorContext(r.Context(), "store oauth client failed", "err", err)
		h.oauthError(w, http.StatusInternalServerError, "server_error", "Could not store the client.")
		return
	}

	issuer := h.oauthIssuer(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"client_id":                  clientID,
		"client_secret":              secret,
		"client_id_issued_at":        now.Unix(),
		"client_secret_expires_at":   0,
		"redirect_uris":              req.RedirectURIs,
		"token_endpoint_auth_method": authMethod,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"scope":                      strings.Join(oauthScopesSupported, " "),
		"client_name":                req.ClientName,
		"registration_client_uri":    issuer + oauthRegisterPath + "/" + clientID,
	})
}

func normalizeAuthMethod(method string, redirectURIs []string) string {
	if method != "" {
		return method
	}
	for _, uri := range redirectURIs {
		if uri == perplexityCallback {
			return "client_secret_post"
		}
	}
	return "none"
}
