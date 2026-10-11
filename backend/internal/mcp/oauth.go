package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
)

const (
	oauthRegisterPath   = "/api/auth/oauth2/register"
	oauthAuthorizePath  = "/api/auth/oauth2/authorize"
	oauthTokenPath      = "/api/auth/oauth2/token"
	oauthConsentPath    = "/api/oauth/consent"
	oauthResourcePath   = "/.well-known/oauth-protected-resource"
	oauthAuthServerPath = "/.well-known/oauth-authorization-server"
	oauthAccessTTL      = 24 * time.Hour
	oauthRefreshTTL     = 30 * 24 * time.Hour
	oauthClientTTL      = 365 * 24 * time.Hour
	oauthCodeTTL        = 10 * time.Minute
	maxOAuthBody        = 1 << 20
)

var oauthScopes = []string{"offline_access", "mcp"}

type clientRegistration struct {
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	ClientName              string   `json:"client_name,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	Scope                   string   `json:"scope,omitempty"`
}

type authorizationRequest struct {
	clientID            string
	redirectURI         string
	state               string
	scopes              []string
	codeChallenge       string
	codeChallengeMethod string
}

func (h *handler) mountOAuth(mux *http.ServeMux) {
	mux.Handle("POST "+oauthRegisterPath, oauthCORS(http.HandlerFunc(h.registerOAuthClient)))
	mux.Handle("GET "+oauthAuthorizePath, oauthCORS(http.HandlerFunc(h.authorizeOAuth)))
	mux.Handle("POST "+oauthTokenPath, oauthCORS(http.HandlerFunc(h.exchangeOAuthToken)))
	mux.Handle("POST "+oauthConsentPath, oauthCORS(http.HandlerFunc(h.respondOAuthConsent)))
	mux.Handle("GET "+oauthResourcePath, oauthCORS(http.HandlerFunc(h.protectedResourceMetadata)))
	mux.Handle("GET "+oauthResourcePath+"/mcp", oauthCORS(http.HandlerFunc(h.protectedResourceMetadata)))
	mux.Handle("GET "+oauthAuthServerPath, oauthCORS(http.HandlerFunc(h.authorizationServerMetadata)))
}

func oauthCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *handler) protectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	base := h.publicBaseURL()
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"resource":              base + Route,
		"authorization_servers": []string{base},
		"scopes_supported":      []string{"mcp"},
		"resource_name":         "Seomarine MCP",
	})
}

func (h *handler) authorizationServerMetadata(w http.ResponseWriter, _ *http.Request) {
	base := h.publicBaseURL()
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"issuer":                                base,
		"authorization_endpoint":                base + oauthAuthorizePath,
		"token_endpoint":                        base + oauthTokenPath,
		"registration_endpoint":                 base + oauthRegisterPath,
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_post"},
		"scopes_supported":                      oauthScopes,
	})
}

func (h *handler) registerOAuthClient(w http.ResponseWriter, r *http.Request) {
	if h.deps.DB == nil {
		writeOAuthError(w, http.StatusServiceUnavailable, "server_error", "OAuth storage is unavailable.")
		return
	}
	body, ok := readOAuthBody(w, r)
	if !ok {
		return
	}
	var input clientRegistration
	dec := json.NewDecoder(strings.NewReader(string(body)))
	if err := dec.Decode(&input); err != nil || dec.Decode(new(any)) != io.EOF || len(input.RedirectURIs) == 0 || len(input.RedirectURIs) > 32 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "A valid redirect_uris list is required.")
		return
	}
	for _, redirect := range input.RedirectURIs {
		if !validOAuthRedirectURI(redirect) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "Each redirect URI must be an absolute HTTPS or loopback HTTP URL without a fragment.")
			return
		}
	}
	for _, grantType := range input.GrantTypes {
		if grantType != "authorization_code" && grantType != "refresh_token" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "Unsupported grant type.")
			return
		}
	}
	for _, responseType := range input.ResponseTypes {
		if responseType != "code" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "Unsupported response type.")
			return
		}
	}
	if input.Scope != "" {
		for _, scope := range strings.Fields(input.Scope) {
			if !containsScope(oauthScopes, scope) {
				writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "Unsupported scope.")
				return
			}
		}
	}
	if input.TokenEndpointAuthMethod == "" {
		input.TokenEndpointAuthMethod = "none"
		for _, redirect := range input.RedirectURIs {
			if redirect == "https://www.perplexity.ai/api/mcp/oauth/callback" {
				input.TokenEndpointAuthMethod = "client_secret_post"
				break
			}
		}
	}
	if input.TokenEndpointAuthMethod != "none" && input.TokenEndpointAuthMethod != "client_secret_post" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "Unsupported token endpoint authentication method.")
		return
	}
	clientID, err := randomOAuthValue(24)
	if err != nil {
		h.oauthInternalError(w, r, "create OAuth client id", err)
		return
	}
	clientSecret := ""
	if input.TokenEndpointAuthMethod == "client_secret_post" {
		clientSecret, err = randomOAuthValue(32)
		if err != nil {
			h.oauthInternalError(w, r, "create OAuth client secret", err)
			return
		}
	}
	internalID, err := randomOAuthValue(18)
	if err != nil {
		h.oauthInternalError(w, r, "create OAuth client record", err)
		return
	}
	now := time.Now().UTC()
	_, err = h.deps.DB.Exec(r.Context(), `
		INSERT INTO go_mcp_oauth_clients
			(id, client_id, client_secret, redirect_uris, token_endpoint_auth_method, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, internalID, clientID, hashOAuthValue(clientSecret), input.RedirectURIs, input.TokenEndpointAuthMethod, now, now.Add(oauthClientTTL))
	if err != nil {
		h.oauthInternalError(w, r, "save OAuth client", err)
		return
	}
	response := map[string]any{
		"client_id":                  clientID,
		"client_id_issued_at":        now.Unix(),
		"redirect_uris":              input.RedirectURIs,
		"token_endpoint_auth_method": input.TokenEndpointAuthMethod,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"scope":                      strings.Join(oauthScopes, " "),
	}
	if input.ClientName != "" {
		response["client_name"] = input.ClientName
	}
	if clientSecret != "" {
		response["client_secret"] = clientSecret
		response["client_secret_expires_at"] = 0
	}
	writeOAuthJSON(w, http.StatusCreated, response)
}

func (h *handler) authorizeOAuth(w http.ResponseWriter, r *http.Request) {
	_, err := h.parseAuthorizationRequest(r.Context(), r.URL.Query())
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if h.deps.Auth == nil {
		writeOAuthError(w, http.StatusServiceUnavailable, "server_error", "Hosted sign-in is not configured.")
		return
	}
	_, authErr := h.deps.Auth.Authenticate(r)
	if errors.Is(authErr, auth.ErrUnauthenticated) {
		redirect := requestTarget(r)
		signIn := h.publicURLPath("/sign-in")
		query := url.Values{"redirect": []string{redirect}}
		http.Redirect(w, r, signIn+"?"+query.Encode(), http.StatusFound)
		return
	}
	if authErr != nil {
		h.oauthInternalError(w, r, "authenticate OAuth user", authErr)
		return
	}
	consent := h.publicURLPath("/oauth-consent")
	values := url.Values{}
	for key, vals := range r.URL.Query() {
		if allowedOAuthAuthorizationParam(key) {
			for _, value := range vals {
				values.Add(key, value)
			}
		}
	}
	http.Redirect(w, r, consent+"?"+values.Encode(), http.StatusFound)
}

func (h *handler) respondOAuthConsent(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r.Header.Get("Origin"), h.publicBaseURL()) {
		writeOAuthError(w, http.StatusForbidden, "invalid_request", "Invalid request origin.")
		return
	}
	body, ok := readOAuthBody(w, r)
	if !ok {
		return
	}
	var input struct {
		Accept bool   `json:"accept"`
		Query  string `json:"query"`
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil || dec.Decode(new(any)) != io.EOF || len(input.Query) > 8192 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid consent response.")
		return
	}
	query, err := url.ParseQuery(strings.TrimPrefix(input.Query, "?"))
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid consent request.")
		return
	}
	authRequest, err := h.parseAuthorizationRequest(r.Context(), query)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !input.Accept {
		writeOAuthJSON(w, http.StatusOK, map[string]string{"redirectTo": oauthErrorRedirect(authRequest, "access_denied", "The user denied access", h.publicBaseURL())})
		return
	}
	if h.deps.Auth == nil || h.deps.DB == nil {
		writeOAuthError(w, http.StatusServiceUnavailable, "server_error", "OAuth authorization is not configured.")
		return
	}
	user, err := h.deps.Auth.Authenticate(r)
	if errors.Is(err, auth.ErrUnauthenticated) {
		writeOAuthError(w, http.StatusUnauthorized, "unauthorized_client", "Sign in is required.")
		return
	}
	if err != nil {
		h.oauthInternalError(w, r, "authenticate OAuth consent", err)
		return
	}
	org, err := resolveExistingActiveOrganization(r.Context(), h.deps.DB, user.ID)
	if err != nil {
		h.oauthInternalError(w, r, "resolve OAuth organization", err)
		return
	}
	if org == nil {
		writeOAuthError(w, http.StatusForbidden, "access_denied", "Your account has no active organization.")
		return
	}
	code, err := randomOAuthValue(32)
	if err != nil {
		h.oauthInternalError(w, r, "create OAuth authorization code", err)
		return
	}
	tag, err := h.deps.DB.Exec(r.Context(), `
		INSERT INTO go_mcp_oauth_authorization_codes
			(code, client_id, redirect_uri, scope, user_id, organization_id, pkce_challenge, pkce_method, expires_at)
		SELECT $1, id, $2, $3, $4, $5, $6, $7, $8
		FROM go_mcp_oauth_clients WHERE client_id = $9 AND expires_at > now()`,
		hashOAuthValue(code), authRequest.redirectURI, strings.Join(authRequest.scopes, " "), user.ID, org.organizationID,
		authRequest.codeChallenge, authRequest.codeChallengeMethod, time.Now().Add(oauthCodeTTL), authRequest.clientID)
	if err != nil {
		h.oauthInternalError(w, r, "save OAuth authorization code", err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client", "OAuth client is unknown or expired.")
		return
	}
	h.recordOAuthAuthorization(r.Context(), org.organizationID)
	redirectURL, err := url.Parse(oauthRedirectURL(authRequest, h.publicBaseURL()))
	if err != nil {
		h.oauthInternalError(w, r, "build OAuth redirect", err)
		return
	}
	params := redirectURL.Query()
	params.Set("code", code)
	redirectURL.RawQuery = params.Encode()
	writeOAuthJSON(w, http.StatusOK, map[string]string{"redirectTo": redirectURL.String()})
}

func (h *handler) parseAuthorizationRequest(ctx context.Context, values url.Values) (authorizationRequest, error) {
	request := authorizationRequest{
		clientID: values.Get("client_id"), redirectURI: values.Get("redirect_uri"), state: values.Get("state"),
		codeChallenge: values.Get("code_challenge"), codeChallengeMethod: values.Get("code_challenge_method"),
	}
	if values.Get("response_type") != "code" || request.clientID == "" || request.redirectURI == "" {
		return request, errors.New("response_type=code, client_id, and redirect_uri are required")
	}
	if values.Get("resource") != "" && values.Get("resource") != h.publicBaseURL()+Route {
		return request, errors.New("resource does not match the Seomarine MCP endpoint")
	}
	if h.deps.DB == nil {
		return request, errors.New("OAuth storage is unavailable")
	}
	var redirectURIs []string
	err := h.deps.DB.QueryRow(ctx, `SELECT redirect_uris FROM go_mcp_oauth_clients WHERE client_id=$1 AND expires_at > now()`, request.clientID).Scan(&redirectURIs)
	if errors.Is(err, pgx.ErrNoRows) {
		return request, errors.New("unknown or expired client")
	}
	if err != nil {
		return request, fmt.Errorf("load OAuth client: %w", err)
	}
	registered := false
	for _, uri := range redirectURIs {
		if uri == request.redirectURI {
			registered = true
			break
		}
	}
	if !registered {
		return request, errors.New("redirect_uri is not registered for this client")
	}
	if request.codeChallenge == "" || request.codeChallengeMethod != "S256" {
		return request, errors.New("PKCE with code_challenge_method=S256 is required")
	}
	requested := strings.Fields(values.Get("scope"))
	if len(requested) == 0 {
		requested = append([]string(nil), oauthScopes...)
	}
	seen := map[string]bool{}
	for _, scope := range requested {
		if scope != "mcp" && scope != "offline_access" {
			return request, errors.New("unsupported scope")
		}
		if !seen[scope] {
			request.scopes = append(request.scopes, scope)
			seen[scope] = true
		}
	}
	if !seen["mcp"] {
		return request, errors.New("the mcp scope is required")
	}
	return request, nil
}

func (h *handler) exchangeOAuthToken(w http.ResponseWriter, r *http.Request) {
	if h.deps.DB == nil {
		writeOAuthError(w, http.StatusServiceUnavailable, "server_error", "OAuth storage is unavailable.")
		return
	}
	mediaType, _, mediaTypeErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaTypeErr != nil || !strings.EqualFold(mediaType, "application/x-www-form-urlencoded") {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Token requests must use form encoding.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxOAuthBody)
	if err := r.ParseForm(); err != nil || int64(len(r.Form.Encode())) > maxOAuthBody {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid token request.")
		return
	}
	clientID, secret := r.Form.Get("client_id"), r.Form.Get("client_secret")
	var client struct {
		ID         string
		AuthMethod string
		SecretHash string
	}
	err := h.deps.DB.QueryRow(r.Context(), `SELECT id, token_endpoint_auth_method, client_secret FROM go_mcp_oauth_clients WHERE client_id=$1 AND expires_at > now()`, clientID).Scan(&client.ID, &client.AuthMethod, &client.SecretHash)
	if errors.Is(err, pgx.ErrNoRows) {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "Client authentication failed.")
		return
	}
	if err != nil {
		h.oauthInternalError(w, r, "load OAuth client for token", err)
		return
	}
	if client.AuthMethod == "client_secret_post" && (secret == "" || !constantTimeHashEqual(client.SecretHash, secret)) || client.AuthMethod == "none" && secret != "" {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "Client authentication failed.")
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		h.exchangeAuthorizationCode(w, r, client.ID, clientID)
	case "refresh_token":
		h.refreshOAuthToken(w, r, client.ID, clientID)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "This grant type is not supported.")
	}
}

func (h *handler) exchangeAuthorizationCode(w http.ResponseWriter, r *http.Request, clientInternalID, clientID string) {
	code, verifier, redirectURI := r.Form.Get("code"), r.Form.Get("code_verifier"), r.Form.Get("redirect_uri")
	if code == "" || verifier == "" || !validPKCEVerifier(verifier) || redirectURI == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Authorization code, redirect_uri, and valid code_verifier are required.")
		return
	}
	tx, err := h.deps.DB.Begin(r.Context())
	if err != nil {
		h.oauthInternalError(w, r, "begin OAuth code exchange", err)
		return
	}
	defer tx.Rollback(r.Context())
	var grant struct {
		RedirectURI, Scope, UserID, OrganizationID string
		Challenge, Method                          *string
	}
	err = tx.QueryRow(r.Context(), `SELECT redirect_uri, scope, user_id, organization_id, pkce_challenge, pkce_method FROM go_mcp_oauth_authorization_codes WHERE code=$1 AND client_id=$2 AND expires_at > now() FOR UPDATE`, hashOAuthValue(code), clientInternalID).Scan(&grant.RedirectURI, &grant.Scope, &grant.UserID, &grant.OrganizationID, &grant.Challenge, &grant.Method)
	if err != nil {
		h.oauthGrantFailure(w, r, "read OAuth authorization code", err)
		return
	}
	if grant.RedirectURI != redirectURI || grant.Challenge == nil || grant.Method == nil || *grant.Method != "S256" || subtle.ConstantTimeCompare([]byte(*grant.Challenge), []byte(base64.RawURLEncoding.EncodeToString(sha256Bytes(verifier)))) != 1 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Authorization code validation failed.")
		return
	}
	if !containsScope(strings.Fields(grant.Scope), "mcp") {
		writeOAuthError(w, http.StatusBadRequest, "invalid_scope", "The mcp scope is required.")
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM go_mcp_oauth_authorization_codes WHERE code=$1`, hashOAuthValue(code)); err != nil {
		h.oauthInternalError(w, r, "consume OAuth authorization code", err)
		return
	}
	response, err := issueOAuthTokens(r.Context(), tx, clientInternalID, clientID, grant.UserID, grant.OrganizationID, grant.Scope, time.Now().UTC())
	if err != nil {
		h.oauthInternalError(w, r, "issue OAuth tokens", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		h.oauthInternalError(w, r, "commit OAuth code exchange", err)
		return
	}
	writeOAuthJSON(w, http.StatusOK, response)
}

func (h *handler) refreshOAuthToken(w http.ResponseWriter, r *http.Request, clientInternalID, clientID string) {
	refresh := r.Form.Get("refresh_token")
	if refresh == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "refresh_token is required.")
		return
	}
	tx, err := h.deps.DB.Begin(r.Context())
	if err != nil {
		h.oauthInternalError(w, r, "begin OAuth refresh", err)
		return
	}
	defer tx.Rollback(r.Context())
	var grant struct{ UserID, OrganizationID, Scope string }
	err = tx.QueryRow(r.Context(), `SELECT user_id, organization_id, scope FROM go_mcp_oauth_refresh_tokens WHERE token=$1 AND client_id=$2 AND expires_at > now() FOR UPDATE`, hashOAuthValue(refresh), clientInternalID).Scan(&grant.UserID, &grant.OrganizationID, &grant.Scope)
	if err != nil {
		h.oauthGrantFailure(w, r, "read OAuth refresh token", err)
		return
	}
	requested := strings.Fields(r.Form.Get("scope"))
	scopes := strings.Fields(grant.Scope)
	if len(requested) != 0 {
		for _, scope := range requested {
			if !containsScope(scopes, scope) {
				writeOAuthError(w, http.StatusBadRequest, "invalid_scope", "Requested scope exceeds the original grant.")
				return
			}
		}
		scopes = requested
	}
	if !containsScope(scopes, "mcp") {
		writeOAuthError(w, http.StatusBadRequest, "invalid_scope", "The mcp scope is required.")
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM go_mcp_oauth_refresh_tokens WHERE token=$1`, hashOAuthValue(refresh)); err != nil {
		h.oauthInternalError(w, r, "rotate OAuth refresh token", err)
		return
	}
	response, err := issueOAuthTokens(r.Context(), tx, clientInternalID, clientID, grant.UserID, grant.OrganizationID, strings.Join(scopes, " "), time.Now().UTC())
	if err != nil {
		h.oauthInternalError(w, r, "issue refreshed OAuth tokens", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		h.oauthInternalError(w, r, "commit OAuth refresh", err)
		return
	}
	writeOAuthJSON(w, http.StatusOK, response)
}

func issueOAuthTokens(ctx context.Context, tx pgx.Tx, clientInternalID, clientID, userID, organizationID, scope string, now time.Time) (map[string]any, error) {
	access, err := randomOAuthValue(32)
	if err != nil {
		return nil, err
	}
	refresh, err := randomOAuthValue(32)
	if err != nil {
		return nil, err
	}
	// Hosted OAuth scopes follow the user across organizations. Keep the
	// consent-time organization as a fallback hint; each MCP request resolves
	// the user's current membership and role before dispatch.
	role := "member"
	err = tx.QueryRow(ctx, `SELECT role FROM member WHERE user_id=$1 AND organization_id=$2`, userID, organizationID).Scan(&role)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("read OAuth grant role hint: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO go_mcp_oauth_access_tokens (token, client_id, scope, user_id, organization_id, role, created_at, expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, hashOAuthValue(access), clientInternalID, scope, userID, organizationID, role, now, now.Add(oauthAccessTTL))
	if err != nil {
		return nil, fmt.Errorf("save OAuth access token: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO go_mcp_oauth_refresh_tokens (token, access_token, client_id, scope, user_id, organization_id, role, created_at, expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, hashOAuthValue(refresh), hashOAuthValue(access), clientInternalID, scope, userID, organizationID, role, now, now.Add(oauthRefreshTTL))
	if err != nil {
		return nil, fmt.Errorf("save OAuth refresh token: %w", err)
	}
	return map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": int(oauthAccessTTL.Seconds()), "refresh_token": refresh, "scope": scope}, nil
}

func (h *handler) authenticateOAuthToken(ctx context.Context, r *http.Request, token string) (Auth, bool, error) {
	if h.deps.DB == nil {
		return Auth{}, false, errors.New("OAuth storage unavailable")
	}
	var userID, organizationID, clientID, scope, email string
	err := h.deps.DB.QueryRow(ctx, `
		SELECT t.user_id, t.organization_id, c.client_id, t.scope, u.email
		FROM go_mcp_oauth_access_tokens t
		JOIN go_mcp_oauth_clients c ON c.id=t.client_id
		JOIN "user" u ON u.id=t.user_id
		WHERE t.token=$1 AND t.expires_at > now() AND c.expires_at > now()`, hashOAuthValue(token)).Scan(&userID, &organizationID, &clientID, &scope, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return Auth{}, false, nil
	}
	if err != nil {
		return Auth{}, false, err
	}
	scopes := strings.Fields(scope)
	if !containsScope(scopes, "mcp") {
		return Auth{}, false, nil
	}
	org, err := resolveExistingActiveOrganization(ctx, h.deps.DB, userID)
	if err != nil {
		return Auth{}, false, err
	}
	if org == nil {
		return Auth{}, false, nil
	}
	return Auth{UserID: userID, UserEmail: email, OrganizationID: org.organizationID, Role: org.role, OrgScope: "user", BaseURL: h.publicOrigin(r), ClientID: clientID, ClientLabel: resolveClientLabel(r.Header.Get("User-Agent"), ""), Scopes: scopes}, true, nil
}

func bearerTokenFromRequest(r *http.Request) string {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func (h *handler) oauthChallenge(w http.ResponseWriter, r *http.Request, code string) {
	metadata := h.publicBaseURL() + oauthResourcePath + "/mcp"
	challenge := `Bearer resource_metadata="` + metadata + `"`
	if code != "" {
		challenge += `, error="` + code + `"`
	}
	w.Header().Set("WWW-Authenticate", challenge)
	h.writeJSON(w, http.StatusUnauthorized, errorResponse(json.RawMessage("null"), codeInvalidRequest, "A valid Seomarine MCP access token is required."))
}

func (h *handler) oauthInternalError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	h.deps.Logger.ErrorContext(r.Context(), operation, "err", err)
	writeOAuthError(w, http.StatusInternalServerError, "server_error", "The authorization service is temporarily unavailable.")
}

func (h *handler) recordOAuthAuthorization(ctx context.Context, organizationID string) {
	stamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err := h.deps.DB.Exec(ctx, `INSERT INTO organization_activation_state (organization_id, first_mcp_authorized_at, updated_at)
		VALUES ($1, $2, $2)
		ON CONFLICT (organization_id) DO UPDATE SET
		first_mcp_authorized_at = COALESCE(organization_activation_state.first_mcp_authorized_at, EXCLUDED.first_mcp_authorized_at),
		updated_at = EXCLUDED.updated_at`, organizationID, stamp)
	if err != nil {
		h.deps.Logger.WarnContext(ctx, "record MCP OAuth activation", "err", err)
	}
}

func (h *handler) oauthGrantFailure(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "The authorization grant is invalid or expired.")
		return
	}
	h.oauthInternalError(w, r, operation, err)
}

func (h *handler) publicBaseURL() string {
	if h.deps.PublicURL == nil {
		return ""
	}
	u := *h.deps.PublicURL
	u.Path, u.RawPath, u.RawQuery, u.Fragment = "", "", "", ""
	return strings.TrimRight(u.String(), "/")
}

func (h *handler) publicURLPath(path string) string { return h.publicBaseURL() + path }

func requestTarget(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return r.URL.Path
	}
	return r.URL.Path + "?" + r.URL.RawQuery
}

func validOAuthRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func allowedOAuthAuthorizationParam(key string) bool {
	switch key {
	case "response_type", "client_id", "redirect_uri", "scope", "state", "code_challenge", "code_challenge_method", "resource":
		return true
	}
	return false
}

func oauthRedirectURL(req authorizationRequest, issuer string) string {
	u, _ := url.Parse(req.redirectURI)
	q := u.Query()
	if req.state != "" {
		q.Set("state", req.state)
	}
	q.Set("iss", issuer)
	u.RawQuery = q.Encode()
	return u.String()
}

func oauthErrorRedirect(req authorizationRequest, code, description, issuer string) string {
	u, _ := url.Parse(oauthRedirectURL(req, issuer))
	q := u.Query()
	q.Set("error", code)
	q.Set("error_description", description)
	u.RawQuery = q.Encode()
	return u.String()
}

func readOAuthBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxOAuthBody))
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Request body is invalid or too large.")
		return nil, false
	}
	return body, true
}

func writeOAuthJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}

func writeOAuthError(w http.ResponseWriter, status int, code, description string) {
	writeOAuthJSON(w, status, map[string]string{"error": code, "error_description": description})
}

func randomOAuthValue(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate OAuth secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func hashOAuthValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func sha256Bytes(value string) []byte { sum := sha256.Sum256([]byte(value)); return sum[:] }

func constantTimeHashEqual(expectedHash, rawValue string) bool {
	return subtle.ConstantTimeCompare([]byte(expectedHash), []byte(hashOAuthValue(rawValue))) == 1
}

func containsScope(scopes []string, target string) bool {
	for _, scope := range scopes {
		if scope == target {
			return true
		}
	}
	return false
}

func validPKCEVerifier(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, ch := range value {
		if !(ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '.' || ch == '_' || ch == '~') {
			return false
		}
	}
	return true
}

func sameOrigin(origin, expected string) bool {
	if origin == "" {
		return false
	}
	a, errA := url.Parse(origin)
	b, errB := url.Parse(expected)
	return errA == nil && errB == nil && a.Scheme == b.Scheme && strings.EqualFold(a.Host, b.Host) && a.User == nil && a.Path == "" && a.RawQuery == "" && a.Fragment == ""
}
