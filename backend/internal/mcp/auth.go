// Package mcp serves Go-owned MCP tools and proxies unported tools to the legacy server.
package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// errResp is a non-JSON-RPC HTTP error (bad credential, rate limit).
type errResp struct {
	status     int
	body       any
	retryAfter int
}

// apiKeyFromRequest extracts a Seomarine API key from the x-api-key header or
// an Authorization: Bearer value. Only oseo_-prefixed credentials are treated
// as API keys; everything else falls through to the legacy proxy.
func apiKeyFromRequest(r *http.Request) string {
	if header := r.Header.Get("x-api-key"); strings.HasPrefix(header, apiKeyPrefix) {
		return header
	}
	bearer := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), "bearer "))
	if strings.HasPrefix(bearer, apiKeyPrefix) {
		return bearer
	}
	return ""
}

// hashAPIKey is better-auth's default key hasher: base64url(SHA-256(key)) with
// no padding. Matching it is what keeps existing keys working.
func hashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// keyLookup is the apikey row state the verifier needs.
type keyLookup struct {
	id           string
	referenceID  string
	key          string
	enabled      bool
	remaining    *int
	refillAmount *int
	expiresAt    *time.Time
}

// authenticateAPIKey verifies the key, enforces the per-user rate limit, and
// returns the caller identity. On failure it returns an errResp ready to write.
func (h *handler) authenticateAPIKey(ctx context.Context, r *http.Request, key string) (Auth, *errResp) {
	if h.deps.DB == nil {
		return Auth{}, internalAPIKeyError()
	}
	row, ok, err := lookupAPIKey(ctx, h.deps.DB, hashAPIKey(key))
	if err != nil {
		h.deps.Logger.ErrorContext(ctx, "mcp api key lookup failed", "err", err)
		return Auth{}, internalAPIKeyError()
	}
	if !ok || !row.enabled || row.expired(time.Now()) {
		return Auth{}, invalidAPIKeyResponse()
	}
	// Exhausted quota with no refill is a 429, matching better-auth.
	if row.exhausted() {
		return Auth{}, rateLimitedResponse("usage_exceeded", "API key usage limit reached", 0)
	}

	if err := useAPIKey(ctx, h.deps.DB, row); err != nil {
		h.deps.Logger.ErrorContext(ctx, "mcp api key usage failed", "err", err)
		return Auth{}, internalAPIKeyError()
	}

	userID := row.referenceID
	var email string
	var name string
	if err := h.deps.DB.QueryRow(ctx, `SELECT email, coalesce(name, '') FROM "user" WHERE id = $1`, userID).Scan(&email, &name); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			h.deps.Logger.ErrorContext(ctx, "mcp api key user lookup failed", "err", err)
			return Auth{}, internalAPIKeyError()
		}
		return Auth{}, invalidAPIKeyResponse()
	}

	if limited, retryAfter := h.rateLimited(ctx, userID); limited {
		return Auth{}, rateLimitedResponse("rate_limited", "Rate limit exceeded: 5000 requests per minute", retryAfter)
	}

	org, err := resolveExistingActiveOrganization(ctx, h.deps.DB, userID)
	if err != nil {
		h.deps.Logger.ErrorContext(ctx, "mcp api key org resolution failed", "err", err)
		return Auth{}, internalAPIKeyError()
	}
	if org == nil {
		return Auth{}, &errResp{status: http.StatusForbidden, body: map[string]string{
			"error":             "account_access_revoked",
			"error_description": "This API key is no longer associated with an organization",
		}}
	}

	session := Auth{
		UserID:         userID,
		UserEmail:      email,
		OrganizationID: org.organizationID,
		Role:           org.role,
		OrgScope:       "user",
		BaseURL:        h.publicOrigin(r),
		ClientID:       "api_key",
		ClientLabel:    resolveClientLabel(r.Header.Get("User-Agent"), ""),
		Scopes:         []string{"offline_access", "mcp"},
	}
	return session, nil
}

func (row keyLookup) expired(now time.Time) bool {
	return row.expiresAt != nil && now.After(*row.expiresAt)
}

func (row keyLookup) exhausted() bool {
	return row.remaining != nil && *row.remaining == 0 && row.refillAmount == nil
}

// lookupAPIKey finds an enabled key by its hash. The hash is compared in
// constant time as defense in depth against a poisoned index.
func lookupAPIKey(ctx context.Context, db *pgxpool.Pool, hash string) (keyLookup, bool, error) {
	var row keyLookup
	err := db.QueryRow(ctx, `
		SELECT id, reference_id, key, coalesce(enabled, true), remaining, refill_amount, expires_at
		FROM apikey WHERE key = $1 LIMIT 1`, hash,
	).Scan(&row.id, &row.referenceID, &row.key, &row.enabled, &row.remaining, &row.refillAmount, &row.expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return keyLookup{}, false, nil
	}
	if err != nil {
		return keyLookup{}, false, fmt.Errorf("lookup api key: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(row.key), []byte(hash)) != 1 {
		return keyLookup{}, false, nil
	}
	return row, true, nil
}

// useAPIKey consumes one request: request_count increments and remaining
// decrements while it is set, guarded so it can never go below zero.
func useAPIKey(ctx context.Context, db *pgxpool.Pool, row keyLookup) error {
	_, err := db.Exec(ctx, `
		UPDATE apikey
		SET request_count = coalesce(request_count, 0) + 1,
		    last_request = now(),
		    remaining = CASE WHEN remaining IS NULL THEN NULL ELSE greatest(remaining - 1, 0) END,
		    updated_at = now()
		WHERE id = $1 AND (remaining IS NULL OR remaining > 0)`, row.id)
	if err != nil {
		return fmt.Errorf("consume api key usage: %w", err)
	}
	return nil
}

// rateLimited applies a fixed one-minute window in Redis, keyed by user id.
func (h *handler) rateLimited(ctx context.Context, userID string) (bool, int) {
	if h.deps.Redis == nil {
		return false, 0
	}
	window := time.Now().Unix() / 60
	key := fmt.Sprintf("mcp:ratelimit:%s:%d", userID, window)
	pipe := h.deps.Redis.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 2*time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		// A Redis outage must not take MCP down; log and allow.
		h.deps.Logger.WarnContext(ctx, "mcp rate limit check failed", "err", err)
		return false, 0
	}
	if incr.Val() > requestLimitPerMinute {
		return true, 60
	}
	return false, 0
}

type activeOrganization struct {
	organizationID string
	role           string
}

// resolveExistingActiveOrganization mirrors the legacy helper of the same name:
// the user's last-active org if still a member, else their most recently joined
// org, else nil. It never mints a default organization.
func resolveExistingActiveOrganization(ctx context.Context, db *pgxpool.Pool, userID string) (*activeOrganization, error) {
	var lastActive *string
	if err := db.QueryRow(ctx, `SELECT last_active_organization_id FROM "user" WHERE id = $1`, userID).Scan(&lastActive); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("read last active organization: %w", err)
	}
	if lastActive != nil && *lastActive != "" {
		role, ok, err := getMembership(ctx, db, userID, *lastActive)
		if err != nil {
			return nil, err
		}
		if ok {
			return &activeOrganization{organizationID: *lastActive, role: role}, nil
		}
	}
	var orgID, role string
	err := db.QueryRow(ctx, `
		SELECT organization_id, role FROM member
		WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`, userID).Scan(&orgID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find newest membership: %w", err)
	}
	return &activeOrganization{organizationID: orgID, role: role}, nil
}

// getMembership returns the user's role in organizationID, and false when they
// are not a member.
func getMembership(ctx context.Context, db *pgxpool.Pool, userID, organizationID string) (string, bool, error) {
	var role string
	err := db.QueryRow(ctx, `SELECT role FROM member WHERE user_id = $1 AND organization_id = $2 LIMIT 1`, userID, organizationID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get membership: %w", err)
	}
	return role, true, nil
}

type membership struct {
	organizationID   string
	organizationName string
	role             string
}

// listMembershipsForUser returns every org the user belongs to, oldest first.
func listMembershipsForUser(ctx context.Context, db *pgxpool.Pool, userID string) ([]membership, error) {
	rows, err := db.Query(ctx, `
		SELECT m.organization_id, o.name, m.role
		FROM member m JOIN organization o ON o.id = m.organization_id
		WHERE m.user_id = $1 ORDER BY m.created_at ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}
	defer rows.Close()
	var out []membership
	for rows.Next() {
		var m membership
		if err := rows.Scan(&m.organizationID, &m.organizationName, &m.role); err != nil {
			return nil, fmt.Errorf("scan membership: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func invalidAPIKeyResponse() *errResp {
	return &errResp{status: http.StatusUnauthorized, body: map[string]string{
		"error":             "invalid_api_key",
		"error_description": "The provided API key is invalid, expired, or disabled",
	}}
}

func internalAPIKeyError() *errResp {
	return &errResp{status: http.StatusInternalServerError, body: map[string]string{
		"error":             "internal_error",
		"error_description": "API key authentication failed",
	}}
}

func rateLimitedResponse(code, description string, retryAfter int) *errResp {
	resp := &errResp{status: http.StatusTooManyRequests, body: map[string]string{
		"error":             code,
		"error_description": description,
	}}
	if retryAfter > 0 {
		resp.retryAfter = retryAfter
	}
	return resp
}
