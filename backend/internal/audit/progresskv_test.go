package audit

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// progressRedis opens the test Redis, or skips when TEST_REDIS_URL is unset.
func progressRedis(ctx context.Context, t *testing.T) *redis.Client {
	t.Helper()
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_REDIS_URL must be set in CI")
		}
		t.Skip("TEST_REDIS_URL not set; skipping Redis integration test")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("parse redis url: %v", err)
	}
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping redis: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestAuditProgressFeed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := progressRedis(ctx, t)
	progress := NewAuditProgress(client)
	auditID := "audit-progress-" + suffix(t)
	t.Cleanup(func() { _ = progress.Clear(context.Background(), auditID) })

	if entries, err := progress.CrawledURLs(ctx, auditID); err != nil || len(entries) != 0 {
		t.Fatalf("empty feed = %v err = %v", entries, err)
	}

	if err := progress.PushCrawledURLs(ctx, auditID, []ProgressEntry{{URL: "https://example.com/a", StatusCode: 200, Title: "A", CrawledAt: 1}}); err != nil {
		t.Fatalf("first push: %v", err)
	}
	// New entries are prepended, so the feed is newest-first.
	if err := progress.PushCrawledURLs(ctx, auditID, []ProgressEntry{{URL: "https://example.com/b", StatusCode: 200, Title: "B", CrawledAt: 2}}); err != nil {
		t.Fatalf("second push: %v", err)
	}
	entries, err := progress.CrawledURLs(ctx, auditID)
	if err != nil {
		t.Fatalf("read feed: %v", err)
	}
	if len(entries) != 2 || entries[0].URL != "https://example.com/b" {
		t.Fatalf("feed = %+v", entries)
	}

	if err := progress.Clear(ctx, auditID); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if entries, err := progress.CrawledURLs(ctx, auditID); err != nil || len(entries) != 0 {
		t.Fatalf("feed after clear = %v err = %v", entries, err)
	}
}

func TestAuditProgressCaps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := progressRedis(ctx, t)
	progress := NewAuditProgress(client)
	auditID := "audit-progress-cap-" + suffix(t)
	t.Cleanup(func() { _ = progress.Clear(context.Background(), auditID) })

	entries := make([]ProgressEntry, progressMaxEntries+50)
	for i := range entries {
		entries[i] = ProgressEntry{URL: "https://example.com/" + strconv.Itoa(i), StatusCode: 200, CrawledAt: int64(i)}
	}
	if err := progress.PushCrawledURLs(ctx, auditID, entries); err != nil {
		t.Fatalf("push: %v", err)
	}
	stored, err := progress.CrawledURLs(ctx, auditID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(stored) != progressMaxEntries {
		t.Fatalf("feed = %d, want %d", len(stored), progressMaxEntries)
	}
}

func TestAuditProgressNilClientIsSafe(t *testing.T) {
	progress := NewAuditProgress(nil)
	if err := progress.PushCrawledURLs(context.Background(), "audit", []ProgressEntry{{URL: "x"}}); err != nil {
		t.Fatalf("push: %v", err)
	}
	if entries, err := progress.CrawledURLs(context.Background(), "audit"); err != nil || len(entries) != 0 {
		t.Fatalf("read = %v err = %v", entries, err)
	}
	if err := progress.Clear(context.Background(), "audit"); err != nil {
		t.Fatalf("clear: %v", err)
	}
}
