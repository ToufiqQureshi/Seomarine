package kv

import (
	"context"
	"os"
	"testing"
	"time"
)

// testRedisURL returns the Redis URL for integration tests. CI always sets
// it, so a missing value there fails instead of silently skipping.
func testRedisURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url != "" {
		return url
	}
	if os.Getenv("CI") != "" {
		t.Fatal("TEST_REDIS_URL must be set in CI")
	}
	t.Skip("TEST_REDIS_URL not set; skipping Redis integration test")
	return ""
}

func TestOpenConnectsToRedis(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Open(ctx, testRedisURL(t))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	key := "kv-test:" + t.Name()
	if err := client.Set(ctx, key, "ü", time.Minute).Err(); err != nil {
		t.Fatalf("SET: %v", err)
	}
	got, err := client.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if got != "ü" {
		t.Fatalf("GET = %q, want %q", got, "ü")
	}
}

func TestOpenFails(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		// Port 1 on localhost refuses connections, so the startup ping must fail.
		{name: "unreachable server", url: "redis://127.0.0.1:1/0"},
		{name: "wrong scheme", url: "postgres://127.0.0.1:6379/0"},
		{name: "non-numeric database", url: "redis://127.0.0.1:6379/abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			client, err := Open(ctx, tt.url)
			if err == nil {
				t.Errorf("Open(%q) succeeded", tt.url)
				if err := client.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			}
		})
	}
}
