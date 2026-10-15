package contenteditor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresBriefCache stores one shared response per normalized query and
// serializes a cache miss with a session-level advisory lock. The connection
// stays pinned through provider work so independent instances cannot both pay.
type PostgresBriefCache struct{ DB *pgxpool.Pool }

// GetOrBuild returns an unexpired value or runs build once for all callers
// holding the same key. Failed builds are never persisted.
func (c PostgresBriefCache) GetOrBuild(ctx context.Context, key string, refresh bool, build func() (ContentBrief, error)) (brief ContentBrief, retErr error) {
	if c.DB == nil || len(key) != 64 || build == nil {
		return ContentBrief{}, errors.New("content brief cache is unavailable")
	}
	conn, err := c.DB.Acquire(ctx)
	if err != nil {
		return ContentBrief{}, fmt.Errorf("acquire shared content brief cache connection: %w", err)
	}
	locked := false
	defer func() {
		if locked {
			unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			var unlocked bool
			unlockErr := conn.QueryRow(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended('uplumo-content-brief:' || $1, 0))`, key).Scan(&unlocked)
			if unlockErr == nil && !unlocked {
				unlockErr = errors.New("PostgreSQL content brief advisory lock was not held")
			}
			if unlockErr != nil {
				conn.Hijack() // Never return a session with a possibly-held lock to the pool.
				retErr = errors.Join(retErr, fmt.Errorf("release content brief cache lock: %w", unlockErr))
			} else {
				conn.Release()
			}
		} else {
			conn.Release()
		}
	}()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended('uplumo-content-brief:' || $1, 0))`, key); err != nil {
		return ContentBrief{}, fmt.Errorf("lock shared content brief cache key: %w", err)
	}
	locked = true
	var cached []byte
	err = pgx.ErrNoRows
	if !refresh {
		err = conn.QueryRow(ctx, `SELECT brief FROM go_content_brief_cache WHERE cache_key=$1 AND expires_at > now()`, key).Scan(&cached)
	}
	if err == nil {
		if decodeErr := json.Unmarshal(cached, &brief); decodeErr == nil && validateBriefResult(brief) == nil {
			return brief, nil
		}
		// Invalid persisted bytes cannot be served as a valid brief. Remove them
		// under the lock and rebuild; a failed rebuild leaves no cache entry.
		if _, deleteErr := conn.Exec(ctx, `DELETE FROM go_content_brief_cache WHERE cache_key=$1`, key); deleteErr != nil {
			return ContentBrief{}, fmt.Errorf("remove invalid content brief cache entry: %w", deleteErr)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ContentBrief{}, fmt.Errorf("read shared content brief cache: %w", err)
	}
	brief, err = build()
	if err != nil {
		return ContentBrief{}, err
	}
	encoded, err := json.Marshal(brief)
	if err != nil {
		return ContentBrief{}, fmt.Errorf("encode content brief cache entry: %w", err)
	}
	_, err = conn.Exec(ctx, `INSERT INTO go_content_brief_cache (cache_key, brief, fetched_at, expires_at)
		VALUES ($1, $2::jsonb, $3::timestamptz, $3::timestamptz + interval '30 days')
		ON CONFLICT (cache_key) DO UPDATE SET brief=EXCLUDED.brief, fetched_at=EXCLUDED.fetched_at, expires_at=EXCLUDED.expires_at`,
		key, encoded, brief.FetchedAt)
	if err != nil {
		return ContentBrief{}, fmt.Errorf("write shared content brief cache: %w", err)
	}
	return brief, nil
}
