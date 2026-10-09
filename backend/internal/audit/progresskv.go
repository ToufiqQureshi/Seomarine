package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// progressKeyPrefix namespaces the live-progress keys.
	progressKeyPrefix = "audit-progress:"
	// progressTTL is how long the live feed lives; it is only needed while the
	// audit runs.
	progressTTL = 30 * time.Minute
	// progressMaxEntries caps the stored feed.
	progressMaxEntries = 300
)

// ProgressEntry is one crawled URL in the live crawl feed.
type ProgressEntry struct {
	URL        string `json:"url"`
	StatusCode int    `json:"statusCode"`
	Title      string `json:"title"`
	// CrawledAt is the Unix timestamp in milliseconds when the page was crawled.
	CrawledAt int64 `json:"crawledAt"`
}

// Progress stores the live crawl feed in Redis so the UI can poll it.
type Progress struct {
	client *redis.Client
	now    func() time.Time
}

// NewProgress returns a Redis-backed progress store. A nil client makes
// every method a no-op, so a deployment without Redis still serves audits.
func NewProgress(client *redis.Client) *Progress {
	return &Progress{client: client, now: time.Now}
}

func progressKey(auditID string) string { return progressKeyPrefix + auditID }

// parseProgressEntries decodes a stored feed, returning an empty slice for
// missing or malformed data.
func parseProgressEntries(raw string) []ProgressEntry {
	if raw == "" {
		return []ProgressEntry{}
	}
	var entries []ProgressEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return []ProgressEntry{}
	}
	return entries
}

// PushCrawledURLs appends entries in one Redis write. New entries are prepended
// and the list is capped, so the feed is newest-first. Mirrors pushCrawledUrls.
func (p *Progress) PushCrawledURLs(ctx context.Context, auditID string, entries []ProgressEntry) error {
	if p == nil || p.client == nil || len(entries) == 0 {
		return nil
	}
	key := progressKey(auditID)
	for attempt := 0; attempt < 3; attempt++ {
		err := p.client.Watch(ctx, func(tx *redis.Tx) error {
			existing, err := tx.Get(ctx, key).Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				return err
			}
			merged := append(append([]ProgressEntry{}, entries...), parseProgressEntries(existing)...)
			if len(merged) > progressMaxEntries {
				merged = merged[:progressMaxEntries]
			}
			encoded, err := json.Marshal(merged)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, key, encoded, progressTTL)
				return nil
			})
			return err
		}, key)
		if err == nil {
			return nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return fmt.Errorf("push crawl progress: %w", err)
		}
	}
	return fmt.Errorf("push crawl progress: too many concurrent writers for %s", auditID)
}

// CrawledURLs reads the live feed for a running audit, newest-first.
func (p *Progress) CrawledURLs(ctx context.Context, auditID string) ([]ProgressEntry, error) {
	if p == nil || p.client == nil {
		return []ProgressEntry{}, nil
	}
	raw, err := p.client.Get(ctx, progressKey(auditID)).Result()
	if errors.Is(err, redis.Nil) {
		return []ProgressEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read crawl progress: %w", err)
	}
	return parseProgressEntries(raw), nil
}

// Clear deletes the progress key after an audit finishes.
func (p *Progress) Clear(ctx context.Context, auditID string) error {
	if p == nil || p.client == nil {
		return nil
	}
	if err := p.client.Del(ctx, progressKey(auditID)).Err(); err != nil {
		return fmt.Errorf("clear crawl progress: %w", err)
	}
	return nil
}
