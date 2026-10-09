package google

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// OAuthService starts and completes Google data-grant consent flows.
type OAuthService struct {
	States       StateStore
	Tokens       *TokenService
	Verifier     *IDTokenVerifier
	PublicOrigin string
	AuthorizeURL string
}

type integration struct {
	providerID   string
	callbackPath string
	scopes       []string
}

func integrationFor(provider string) (integration, error) {
	switch provider {
	case "gsc":
		return integration{"google-search-console", "/api/gsc/oauth/callback", []string{"openid", "email", "profile", "https://www.googleapis.com/auth/webmasters.readonly"}}, nil
	case "ga4":
		return integration{"google-analytics", "/api/ga4/oauth/callback", []string{"openid", "email", "profile", "https://www.googleapis.com/auth/analytics.readonly"}}, nil
	default:
		return integration{}, errors.New("invalid google provider")
	}
}

func (s *OAuthService) authorizationURL() string {
	if s.AuthorizeURL != "" {
		return s.AuthorizeURL
	}
	return "https://accounts.google.com/o/oauth2/v2/auth"
}

// Start returns the Google authorization URL for one signed-in user.
func (s *OAuthService) Start(ctx context.Context, userID, provider, callbackURL string) (string, error) {
	if s == nil || s.Tokens == nil || s.Tokens.ClientID == "" || s.Tokens.ClientSecret == "" || userID == "" || callbackURL == "" {
		return "", errors.New("google oauth unavailable")
	}
	integration, err := integrationFor(provider)
	if err != nil {
		return "", err
	}
	state, err := s.States.Create(ctx, provider, userID, callbackURL, s.PublicOrigin)
	if err != nil {
		return "", err
	}
	verifier, err := CodeVerifier(state, provider, s.Tokens.ClientSecret)
	if err != nil {
		return "", err
	}
	endpoint, err := url.Parse(s.authorizationURL())
	if err != nil {
		return "", errors.New("invalid google authorization url")
	}
	values := endpoint.Query()
	values.Set("client_id", s.Tokens.ClientID)
	values.Set("redirect_uri", s.PublicOrigin+integration.callbackPath)
	values.Set("response_type", "code")
	values.Set("scope", strings.Join(integration.scopes, " "))
	values.Set("access_type", "offline")
	values.Set("prompt", "select_account consent")
	values.Set("state", state)
	values.Set("code_challenge", CodeChallenge(verifier))
	values.Set("code_challenge_method", "S256")
	endpoint.RawQuery = values.Encode()
	return endpoint.String(), nil
}

// Complete burns the state, exchanges the single-use code, verifies Google's
// identity token and upserts the encrypted grant. It returns the safe browser
// path and a short error code for the UI's existing Google link alert.
func (s *OAuthService) Complete(ctx context.Context, userID, provider, stateValue, code, googleError string) (string, string) {
	if s == nil || s.Tokens == nil || s.Verifier == nil {
		return "/auth-error?error=auth_config_missing", ""
	}
	integration, err := integrationFor(provider)
	if err != nil {
		return "/auth-error?error=state_mismatch", ""
	}
	state, err := s.States.Consume(ctx, provider, stateValue, userID)
	if err != nil {
		return "/auth-error?error=state_mismatch", ""
	}
	if googleError != "" {
		return state.CallbackPath, googleError
	}
	if code == "" || len(code) > 4096 {
		return state.CallbackPath, "missing_code"
	}
	verifier, err := CodeVerifier(stateValue, provider, s.Tokens.ClientSecret)
	if err != nil {
		return state.CallbackPath, "oauth_code_verification_failed"
	}
	tokens, err := s.Tokens.exchange(ctx, url.Values{
		"code":          {code},
		"client_id":     {s.Tokens.ClientID},
		"client_secret": {s.Tokens.ClientSecret},
		"redirect_uri":  {s.PublicOrigin + integration.callbackPath},
		"grant_type":    {"authorization_code"},
		"code_verifier": {verifier},
	})
	if err != nil {
		return state.CallbackPath, "oauth_code_verification_failed"
	}
	accountID, err := s.Verifier.AccountID(ctx, tokens.IDToken, s.Tokens.ClientID)
	if err != nil {
		return state.CallbackPath, "oauth_code_verification_failed"
	}
	if err := s.upsertGrant(ctx, userID, integration, accountID, tokens); err != nil {
		return state.CallbackPath, "connection_save_failed"
	}
	return state.CallbackPath, ""
}

func (s *OAuthService) upsertGrant(ctx context.Context, userID string, in integration, accountID string, tokens tokenResponse) error {
	access, err := s.Tokens.Cipher.Encrypt(tokens.AccessToken)
	if err != nil {
		return fmt.Errorf("encrypt google access token: %w", err)
	}
	var refresh *string
	if tokens.RefreshToken != "" {
		ciphertext, err := s.Tokens.Cipher.Encrypt(tokens.RefreshToken)
		if err != nil {
			return fmt.Errorf("encrypt google refresh token: %w", err)
		}
		refresh = &ciphertext
	}
	scope := strings.Join(in.scopes, ",")
	if tokens.Scope != "" {
		scope = strings.Join(strings.Fields(tokens.Scope), ",")
	}
	now := s.Tokens.now()
	_, err = s.Tokens.Pool.Exec(ctx, `INSERT INTO account
		(id, account_id, provider_id, user_id, access_token, refresh_token, access_token_expires_at, scope, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)
		ON CONFLICT (user_id, provider_id, account_id) WHERE provider_id IN ('google-search-console','google-analytics')
		DO UPDATE SET access_token=EXCLUDED.access_token,
			refresh_token=COALESCE(EXCLUDED.refresh_token, account.refresh_token),
			access_token_expires_at=EXCLUDED.access_token_expires_at,
			scope=EXCLUDED.scope, updated_at=EXCLUDED.updated_at`,
		rand.Text(), accountID, in.providerID, userID, access, refresh,
		now.Add(time.Duration(tokens.ExpiresIn)*time.Second), scope, now)
	if err != nil {
		return fmt.Errorf("save google data grant: %w", err)
	}
	return nil
}
