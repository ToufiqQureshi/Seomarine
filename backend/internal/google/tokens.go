package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const tokenRequestTimeout = 8 * time.Second

// ErrGrantUnavailable means a Google data grant is missing or must be reconnected.
var ErrGrantUnavailable = errors.New("google grant unavailable")

// TokenService loads, refreshes and stores a user's Google data grants.
type TokenService struct {
	Pool         *pgxpool.Pool
	Cipher       *TokenCipher
	LegacyCipher *LegacyTokenCipher
	Client       *http.Client
	TokenURL     string
	ClientID     string
	ClientSecret string
	Now          func() time.Time
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
}

func (s *TokenService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *TokenService) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: tokenRequestTimeout}
}

func (s *TokenService) tokenURL() string {
	if s.TokenURL != "" {
		return s.TokenURL
	}
	return "https://oauth2.googleapis.com/token"
}

func (s *TokenService) exchange(ctx context.Context, values url.Values) (out tokenResponse, err error) {
	if s.ClientID == "" || s.ClientSecret == "" {
		return tokenResponse{}, ErrGrantUnavailable
	}
	requestCtx, cancel := context.WithTimeout(ctx, tokenRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, s.tokenURL(), strings.NewReader(values.Encode()))
	if err != nil {
		return tokenResponse{}, errors.New("google token request unavailable")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client().Do(req)
	if err != nil {
		return tokenResponse{}, errors.New("google token request unavailable")
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close google token response: %w", closeErr))
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return tokenResponse{}, ErrGrantUnavailable
	}
	var token tokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&token); err != nil || token.AccessToken == "" || token.ExpiresIn < 0 || token.ExpiresIn > 86400 {
		return tokenResponse{}, errors.New("invalid google token response")
	}
	if token.ExpiresIn == 0 {
		token.ExpiresIn = 3600
	}
	return token, nil
}

func (s *TokenService) decrypt(value string) (string, error) {
	if strings.Contains(value, ":") && !strings.HasPrefix(value, "$ba$") {
		if s.Cipher == nil {
			return "", ErrGrantUnavailable
		}
		return s.Cipher.Decrypt(value)
	}
	if s.LegacyCipher == nil {
		return "", ErrGrantUnavailable
	}
	return s.LegacyCipher.Decrypt(value)
}

// AccessToken returns a fresh token, serializing refresh through a Postgres row
// lock so concurrent requests across processes refresh the grant only once.
// rejectedToken forces refresh after a provider 401, unless another request has
// already rotated the token while this caller waited for the row lock.
func (s *TokenService) AccessToken(ctx context.Context, userID, provider, accountID, rejectedToken string) (string, error) {
	if s == nil || s.Pool == nil || s.Cipher == nil || userID == "" {
		return "", ErrGrantUnavailable
	}
	_, providerID, _, err := accountScope(provider)
	if err != nil {
		return "", err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin google grant read: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	query := `SELECT id, access_token, refresh_token, access_token_expires_at FROM account WHERE user_id=$1 AND provider_id=$2`
	args := []any{userID, providerID}
	if accountID != "" {
		query += ` AND account_id=$3`
		args = append(args, accountID)
	}
	query += ` ORDER BY id LIMIT 1 FOR UPDATE`
	var id string
	var access, refresh *string
	var expiry *time.Time
	if err := tx.QueryRow(ctx, query, args...).Scan(&id, &access, &refresh, &expiry); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrGrantUnavailable
		}
		return "", fmt.Errorf("read google data grant: %w", err)
	}
	if access != nil && expiry != nil && expiry.After(s.now().Add(5*time.Second)) {
		plain, err := s.decrypt(*access)
		if err != nil {
			return "", ErrGrantUnavailable
		}
		if rejectedToken == "" || plain != rejectedToken {
			if err := tx.Commit(ctx); err != nil {
				return "", fmt.Errorf("commit google grant read: %w", err)
			}
			return plain, nil
		}
	}
	if refresh == nil || *refresh == "" {
		return "", ErrGrantUnavailable
	}
	plainRefresh, err := s.decrypt(*refresh)
	if err != nil {
		return "", ErrGrantUnavailable
	}
	token, err := s.exchange(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {plainRefresh},
		"client_id":     {s.ClientID},
		"client_secret": {s.ClientSecret},
	})
	if err != nil {
		return "", err
	}
	newAccess, err := s.Cipher.Encrypt(token.AccessToken)
	if err != nil {
		return "", fmt.Errorf("encrypt google access token: %w", err)
	}
	newRefresh := *refresh
	if token.RefreshToken != "" {
		newRefresh, err = s.Cipher.Encrypt(token.RefreshToken)
		if err != nil {
			return "", fmt.Errorf("encrypt google refresh token: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE account SET access_token=$1, refresh_token=$2, access_token_expires_at=$3, updated_at=$4 WHERE id=$5 AND user_id=$6`, newAccess, newRefresh, s.now().Add(time.Duration(token.ExpiresIn)*time.Second), s.now(), id, userID); err != nil {
		return "", fmt.Errorf("save refreshed google grant: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit refreshed google grant: %w", err)
	}
	return token.AccessToken, nil
}
