// Package google provides secure OAuth state, PKCE and token encryption for Google grants.
package google

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const stateTTL = 10 * time.Minute

// ErrInvalidState indicates an unknown, expired, replayed or mismatched OAuth state.
var ErrInvalidState = errors.New("invalid oauth state")

// State holds the authenticated owner and safe return path of one consent flow.
type State struct {
	UserID       string
	CallbackPath string
}

// StateStore persists one-time state hashes in Postgres.
type StateStore struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

// Create generates and persists a state bound to the provider and user.
func (s StateStore) Create(ctx context.Context, provider, userID, callbackURL, publicOrigin string) (string, error) {
	if s.Pool == nil || !validProvider(provider) || userID == "" {
		return "", errors.New("invalid oauth state parameters")
	}
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	now := s.now()
	if _, err := s.Pool.Exec(ctx, `DELETE FROM go_google_oauth_states WHERE expires_at < $1`, now); err != nil {
		return "", fmt.Errorf("sweep oauth states: %w", err)
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO go_google_oauth_states (state_hash, provider, user_id, callback_path, expires_at) VALUES ($1,$2,$3,$4,$5)`, hashState(state), provider, userID, SafeCallbackPath(callbackURL, publicOrigin), now.Add(stateTTL))
	if err != nil {
		return "", fmt.Errorf("save oauth state: %w", err)
	}
	return state, nil
}

// Consume atomically burns a state, including when the wrong user presents it.
func (s StateStore) Consume(ctx context.Context, provider, state, userID string) (State, error) {
	if s.Pool == nil || !validProvider(provider) || state == "" || userID == "" {
		return State{}, ErrInvalidState
	}
	var result State
	err := s.Pool.QueryRow(ctx, `DELETE FROM go_google_oauth_states WHERE state_hash=$1 AND provider=$2 AND expires_at>$3 RETURNING user_id, callback_path`, hashState(state), provider, s.now()).Scan(&result.UserID, &result.CallbackPath)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && result.UserID != userID {
		return State{}, ErrInvalidState
	}
	if err != nil {
		return State{}, fmt.Errorf("consume oauth state: %w", err)
	}
	return result, nil
}

func (s StateStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func validProvider(provider string) bool { return provider == "gsc" || provider == "ga4" }

func hashState(state string) []byte {
	h := sha256.Sum256([]byte(state))
	return h[:]
}

// SafeCallbackPath returns a same-origin browser path, or / for unsafe URLs.
func SafeCallbackPath(callbackURL, publicOrigin string) string {
	origin, err := url.Parse(publicOrigin)
	if err != nil || origin.Scheme == "" || origin.Host == "" || origin.User != nil {
		return "/"
	}
	callback, err := url.Parse(callbackURL)
	if err != nil {
		return "/"
	}
	resolved := origin.ResolveReference(callback)
	if !strings.EqualFold(resolved.Scheme, origin.Scheme) || !strings.EqualFold(resolved.Host, origin.Host) || resolved.User != nil {
		return "/"
	}
	path := resolved.EscapedPath()
	if path == "" {
		path = "/"
	}
	if strings.HasPrefix(path, "//") || strings.HasPrefix(path, "/\\") {
		return "/"
	}
	if resolved.RawQuery != "" {
		path += "?" + resolved.RawQuery
	}
	if resolved.Fragment != "" {
		path += "#" + resolved.EscapedFragment()
	}
	return path
}

// CodeVerifier derives a provider-bound PKCE verifier from the state and secret.
func CodeVerifier(state, provider, clientSecret string) (string, error) {
	if state == "" || !validProvider(provider) || clientSecret == "" {
		return "", errors.New("invalid pkce parameters")
	}
	mac := hmac.New(sha256.New, []byte("seomarine:"+provider+":pkce:"+clientSecret))
	_, _ = mac.Write([]byte(state))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// CodeChallenge computes the S256 challenge for a PKCE verifier.
func CodeChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}
