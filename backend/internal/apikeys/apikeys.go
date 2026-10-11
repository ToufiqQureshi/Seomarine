// Package apikeys lets signed-in users create, list and revoke API keys for the
// MCP endpoint. It keeps Better Auth's api-key plugin contract and table, so the
// settings page and every key already issued keep working. The MCP endpoint
// (internal/mcp) is what verifies a key; this package only manages them.
package apikeys

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"time"
	"unicode/utf16"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
)

const (
	// Prefix marks a credential as an API key on /mcp; anything else is OAuth.
	Prefix = "oseo_"
	// startLength is how much of the key is kept for display ("oseo_" + 4).
	startLength = 9
	keyLength   = 64
	maxNameLen  = 32
	// maxKeysPerUser bounds a runaway script; nobody needs more.
	maxKeysPerUser = 100
	maxBody        = 4 << 10
	timeLayout     = "2006-01-02T15:04:05.000Z"

	// Better Auth's rate-limit defaults, stored on each row. The limiter itself
	// is off (the MCP endpoint rate limits per user); the values only keep rows
	// identical to ones the plugin wrote.
	defaultRateWindowMS = 86_400_000
	defaultRateMax      = 10
)

const keyAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// Service manages API keys.
type Service struct {
	DB     *pgxpool.Pool
	Auth   *auth.Service
	Logger *slog.Logger
	Now    func() time.Time
}

type apiError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *apiError) Error() string { return e.Code + ": " + e.Message }

var (
	errUnauthorized = &apiError{http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized"}
	errBadBody      = &apiError{http.StatusBadRequest, "BAD_REQUEST", "Invalid request body"}
	errNameLength   = &apiError{http.StatusBadRequest, "INVALID_NAME_LENGTH", "Invalid name length"}
	errNameRequired = &apiError{http.StatusBadRequest, "NAME_REQUIRED", "API Key name is required"}
	errKeyNotFound  = &apiError{http.StatusNotFound, "KEY_NOT_FOUND", "API Key not found"}
	errTooManyKeys  = &apiError{http.StatusBadRequest, "TOO_MANY_API_KEYS", "You have reached the maximum number of API keys"}
	errInternal     = &apiError{http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Internal server error"}
)

// Mount registers the key routes.
func Mount(mux *http.ServeMux, s *Service) {
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	handle := func(pattern string, h func(http.ResponseWriter, *http.Request, auth.User) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			user, err := s.Auth.Authenticate(r)
			if err != nil {
				if errors.Is(err, auth.ErrUnauthenticated) {
					writeError(w, errUnauthorized)
					return
				}
				s.Logger.ErrorContext(r.Context(), "authenticate api key request", "err", err)
				writeError(w, errInternal)
				return
			}
			err = h(w, r, user)
			var apiErr *apiError
			switch {
			case err == nil:
			case errors.As(err, &apiErr):
				writeError(w, apiErr)
			case r.Context().Err() != nil:
			default:
				s.Logger.ErrorContext(r.Context(), "api key request failed", "path", r.URL.Path, "err", err)
				writeError(w, errInternal)
			}
		})
	}
	handle("POST /api/auth/api-key/create", s.create)
	handle("GET /api/auth/api-key/list", s.list)
	handle("POST /api/auth/api-key/delete", s.delete)
}

func writeError(w http.ResponseWriter, e *apiError) { writeJSON(w, e.Status, e) }

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// HashKey is Better Auth's default key hasher: base64url(SHA-256(key)), no
// padding. Only the hash is stored; the plaintext is shown once at creation.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// newKey returns "oseo_" followed by 64 random letters.
func newKey() (string, error) {
	out := make([]byte, keyLength)
	limit := big.NewInt(int64(len(keyAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("generate api key: %w", err)
		}
		out[i] = keyAlphabet[n.Int64()]
	}
	return Prefix + string(out), nil
}

type keyBody struct {
	ID               string          `json:"id"`
	ConfigID         string          `json:"configId"`
	Name             *string         `json:"name"`
	Start            *string         `json:"start"`
	Prefix           *string         `json:"prefix"`
	ReferenceID      string          `json:"referenceId"`
	RefillInterval   *int            `json:"refillInterval"`
	RefillAmount     *int            `json:"refillAmount"`
	LastRefillAt     *string         `json:"lastRefillAt"`
	Enabled          bool            `json:"enabled"`
	RateLimitEnabled bool            `json:"rateLimitEnabled"`
	RateLimitTimeWin *int            `json:"rateLimitTimeWindow"`
	RateLimitMax     *int            `json:"rateLimitMax"`
	RequestCount     int             `json:"requestCount"`
	Remaining        *int            `json:"remaining"`
	LastRequest      *string         `json:"lastRequest"`
	ExpiresAt        *string         `json:"expiresAt"`
	CreatedAt        string          `json:"createdAt"`
	UpdatedAt        string          `json:"updatedAt"`
	Permissions      json.RawMessage `json:"permissions"`
	Metadata         json.RawMessage `json:"metadata"`
	Key              string          `json:"key,omitempty"`
}

const columns = `id, config_id, name, start, prefix, reference_id, refill_interval, refill_amount, last_refill_at,
	coalesce(enabled, true), coalesce(rate_limit_enabled, false), rate_limit_time_window, rate_limit_max,
	coalesce(request_count, 0), remaining, last_request, expires_at, created_at, updated_at, permissions, metadata`

func iso(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(timeLayout)
	return &s
}

func jsonOrNull(text *string) json.RawMessage {
	if text == nil || !json.Valid([]byte(*text)) {
		return json.RawMessage("null")
	}
	return json.RawMessage(*text)
}

func scanKey(row pgx.Row) (keyBody, error) {
	var (
		k                                  keyBody
		lastRefill, lastRequest, expiresAt *time.Time
		created, updated                   time.Time
		permissions, metadata              *string
	)
	err := row.Scan(&k.ID, &k.ConfigID, &k.Name, &k.Start, &k.Prefix, &k.ReferenceID, &k.RefillInterval, &k.RefillAmount, &lastRefill,
		&k.Enabled, &k.RateLimitEnabled, &k.RateLimitTimeWin, &k.RateLimitMax, &k.RequestCount, &k.Remaining, &lastRequest, &expiresAt,
		&created, &updated, &permissions, &metadata)
	if err != nil {
		return keyBody{}, err
	}
	k.LastRefillAt, k.LastRequest, k.ExpiresAt = iso(lastRefill), iso(lastRequest), iso(expiresAt)
	k.CreatedAt, k.UpdatedAt = created.UTC().Format(timeLayout), updated.UTC().Format(timeLayout)
	k.Permissions, k.Metadata = jsonOrNull(permissions), jsonOrNull(metadata)
	return k, nil
}

func (s *Service) create(w http.ResponseWriter, r *http.Request, user auth.User) error {
	var in struct {
		Name      *string `json:"name"`
		ExpiresIn *int64  `json:"expiresIn"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody || json.Unmarshal(body, &in) != nil {
		return errBadBody
	}
	name := ""
	if in.Name != nil {
		name = *in.Name
	}
	if name == "" {
		return errNameRequired
	}
	if n := len(utf16.Encode([]rune(name))); n > maxNameLen {
		return errNameLength
	}

	var count int
	if err := s.DB.QueryRow(r.Context(), `SELECT count(*) FROM apikey WHERE reference_id = $1 AND config_id = 'default'`, user.ID).Scan(&count); err != nil {
		return fmt.Errorf("count api keys: %w", err)
	}
	if count >= maxKeysPerUser {
		return errTooManyKeys
	}

	key, err := newKey()
	if err != nil {
		return err
	}
	id, err := ids.New()
	if err != nil {
		return err
	}
	now := s.Now()
	var expiresAt *time.Time
	if in.ExpiresIn != nil && *in.ExpiresIn > 0 {
		t := now.Add(time.Duration(*in.ExpiresIn) * time.Second)
		expiresAt = &t
	}
	row := s.DB.QueryRow(r.Context(), `INSERT INTO apikey
		(id, config_id, name, start, prefix, key, reference_id, enabled, rate_limit_enabled, rate_limit_time_window, rate_limit_max, request_count, expires_at, created_at, updated_at)
		VALUES ($1, 'default', $2, $3, $4, $5, $6, true, false, $7, $8, 0, $9, $10, $10)
		RETURNING `+columns, id, name, key[:startLength], Prefix, HashKey(key), user.ID, defaultRateWindowMS, defaultRateMax, expiresAt, now)
	created, err := scanKey(row)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	created.Key = key
	writeJSON(w, http.StatusOK, created)
	return nil
}

func (s *Service) list(w http.ResponseWriter, r *http.Request, user auth.User) error {
	rows, err := s.DB.Query(r.Context(), `SELECT `+columns+` FROM apikey WHERE reference_id = $1 AND config_id = 'default' ORDER BY created_at DESC`, user.ID)
	if err != nil {
		return fmt.Errorf("list api keys: %w", err)
	}
	keys, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (keyBody, error) { return scanKey(row) })
	if err != nil {
		return fmt.Errorf("read api keys: %w", err)
	}
	if keys == nil {
		keys = []keyBody{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"apiKeys": keys, "total": len(keys), "limit": nil, "offset": nil})
	return nil
}

func (s *Service) delete(w http.ResponseWriter, r *http.Request, user auth.User) error {
	var in struct {
		KeyID string `json:"keyId"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody || json.Unmarshal(body, &in) != nil || in.KeyID == "" {
		return errBadBody
	}
	// Scoped to the caller: another user's key id is "not found", never "forbidden".
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM apikey WHERE id = $1 AND reference_id = $2`, in.KeyID, user.ID)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errKeyNotFound
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	return nil
}
