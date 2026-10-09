package audit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestClassifyFetch(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		mitigated bool
		snippet   string
		want      PageFetchClass
	}{
		{"zero is error", 0, false, "", FetchError},
		{"429 is rate limited", 429, false, "", FetchRateLimited},
		{"cf mitigated wins over 429", 429, true, "", FetchRateLimited},
		{"mitigated is blocked", 200, true, "", FetchBlocked},
		{"401 blocked", 401, false, "", FetchBlocked},
		{"403 blocked", 403, false, "", FetchBlocked},
		{"cloudflare interstitial", 200, false, "<title>Just a moment...</title>", FetchBlocked},
		{"sgcaptcha redirect", 202, false, "/.well-known/sgcaptcha/", FetchBlocked},
		{"503 challenge body", 503, false, "Verifying you are human", FetchBlocked},
		{"503 plain", 503, false, "service unavailable", FetchOK},
		{"200 ok", 200, false, "<html>hi</html>", FetchOK},
		{"404 is ok class", 404, false, "", FetchOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyFetch(tc.status, tc.mitigated, tc.snippet); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAdjustCrawlWindow(t *testing.T) {
	ok := func(ms int64, bytes int) CrawledPageResult {
		return CrawledPageResult{FetchClass: FetchOK, ResponseTimeMs: ms, HTMLBytes: bytes}
	}
	troubled := func() CrawledPageResult {
		return CrawledPageResult{FetchClass: FetchBlocked, ResponseTimeMs: 100, HTMLBytes: 10_000}
	}
	t.Run("empty batch keeps size", func(t *testing.T) {
		if got := AdjustCrawlWindow(4, nil, CrawlWindow); got != 4 {
			t.Fatalf("got %d", got)
		}
	})
	t.Run("trouble shrinks", func(t *testing.T) {
		batch := []CrawledPageResult{troubled(), troubled(), ok(100, 10_000)}
		got := AdjustCrawlWindow(4, batch, CrawlWindow)
		if got != 2 {
			t.Fatalf("got %d, want 2", got)
		}
	})
	t.Run("rate limited shrinks", func(t *testing.T) {
		batch := []CrawledPageResult{{FetchClass: FetchOK, RateLimited: true, ResponseTimeMs: 100, HTMLBytes: 10_000}}
		if got := AdjustCrawlWindow(4, batch, CrawlWindow); got != 2 {
			t.Fatalf("got %d, want 2", got)
		}
	})
	t.Run("slow shrinks", func(t *testing.T) {
		batch := []CrawledPageResult{ok(11_000, 10_000)}
		if got := AdjustCrawlWindow(2, batch, CrawlWindow); got != 1 {
			t.Fatalf("got %d, want 1", got)
		}
	})
	t.Run("grows on a clean full-size fast batch", func(t *testing.T) {
		limits := CrawlWindowLimits{Initial: 2, Min: 1, Max: 10, BudgetBytes: 8 * 1024 * 1024}
		batch := make([]CrawledPageResult, growthMinSample)
		for i := range batch {
			batch[i] = ok(200, 1024)
		}
		if got := AdjustCrawlWindow(2, batch, limits); got != 7 {
			t.Fatalf("got %d, want 7", got)
		}
	})
	t.Run("byte budget caps growth", func(t *testing.T) {
		limits := CrawlWindowLimits{Initial: 2, Min: 1, Max: 100, BudgetBytes: 8 * 1024 * 1024}
		batch := make([]CrawledPageResult, growthMinSample)
		for i := range batch {
			batch[i] = ok(200, 2*1024*1024)
		}
		// avg 2 MiB, budget 8 MiB -> bound 4.
		if got := AdjustCrawlWindow(2, batch, limits); got != 4 {
			t.Fatalf("got %d, want 4", got)
		}
	})
	t.Run("small clean batch does not grow", func(t *testing.T) {
		batch := []CrawledPageResult{ok(100, 1024), ok(100, 1024)}
		if got := AdjustCrawlWindow(2, batch, CrawlWindow); got != 2 {
			t.Fatalf("got %d, want 2", got)
		}
	})
	t.Run("clamp", func(t *testing.T) {
		if got := ClampCrawlWindow(0, CrawlWindow); got != 1 {
			t.Fatalf("got %d, want 1", got)
		}
		if got := ClampCrawlWindow(99, CrawlWindow); got != 2 {
			t.Fatalf("got %d, want 2", got)
		}
	})
}

// fakeClock is a manually advanced clock for throttle tests.
type fakeClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

func TestCrawlThrottleRefusesAfterDeadline(t *testing.T) {
	clock := &fakeClock{at: time.Unix(1_000_000, 0)}
	throttle := NewCrawlThrottle(clock.at.Add(-time.Second), nil, nil)
	throttle.SetClock(clock.Now)
	ready, err := throttle.Ready(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("expected not ready past the deadline")
	}
}

func TestCrawlThrottleReservesSlot(t *testing.T) {
	clock := &fakeClock{at: time.Unix(1_000_000, 0)}
	throttle := NewCrawlThrottle(clock.at.Add(time.Hour), nil, nil)
	throttle.SetClock(clock.Now)
	ready, err := throttle.Ready(context.Background())
	if err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if got := throttle.State().NextRequestAtMs; got != clock.at.Add(time.Second).UnixMilli() {
		t.Fatalf("NextRequestAtMs = %d, want %d", got, clock.at.Add(time.Second).UnixMilli())
	}
}

func TestCrawlThrottleBackoffAccumulatesCooldown(t *testing.T) {
	clock := &fakeClock{at: time.Unix(1_000_000, 0)}
	var saved []CrawlThrottleState
	throttle := NewCrawlThrottle(clock.at.Add(time.Hour), nil, func(_ context.Context, state CrawlThrottleState) error {
		saved = append(saved, state)
		return nil
	})
	throttle.SetClock(clock.Now)

	allowed, err := throttle.Backoff(context.Background(), 0, "")
	if err != nil || !allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	state := throttle.State()
	if state.ConsecutiveRateLimits != 1 {
		t.Fatalf("consecutive = %d", state.ConsecutiveRateLimits)
	}
	if state.IntervalMs != 2_000 {
		t.Fatalf("interval = %d, want 2000", state.IntervalMs)
	}
	if state.PausedUntilMs != clock.at.Add(30*time.Second).UnixMilli() {
		t.Fatalf("pausedUntil = %d", state.PausedUntilMs)
	}
	if state.CooldownMs != 30_000 {
		t.Fatalf("cooldown = %d, want 30000", state.CooldownMs)
	}
	if len(saved) != 1 {
		t.Fatalf("expected one checkpoint save, got %d", len(saved))
	}
}

func TestCrawlThrottleBackoffUsesRetryAfterHeader(t *testing.T) {
	clock := &fakeClock{at: time.Unix(1_000_000, 0)}
	throttle := NewCrawlThrottle(clock.at.Add(time.Hour), nil, nil)
	throttle.SetClock(clock.Now)
	if _, err := throttle.Backoff(context.Background(), 0, "120"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := throttle.State().PausedUntilMs; got != clock.at.Add(120*time.Second).UnixMilli() {
		t.Fatalf("pausedUntil = %d", got)
	}
}

func TestCrawlThrottleStopsAfterTooManyRateLimits(t *testing.T) {
	clock := &fakeClock{at: time.Unix(1_000_000, 0)}
	throttle := NewCrawlThrottle(clock.at.Add(time.Hour), nil, nil)
	throttle.SetClock(clock.Now)
	for attempt := 0; attempt <= throttleMaxRetries; attempt++ {
		if _, err := throttle.Backoff(context.Background(), attempt, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if !throttle.Stopped() {
		t.Fatal("expected the throttle to stop")
	}
}

func TestCrawlThrottleRecoveredResets(t *testing.T) {
	clock := &fakeClock{at: time.Unix(1_000_000, 0)}
	throttle := NewCrawlThrottle(clock.at.Add(time.Hour), nil, nil)
	throttle.SetClock(clock.Now)
	if _, err := throttle.Backoff(context.Background(), 0, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := throttle.Recovered(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := throttle.State().ConsecutiveRateLimits; got != 0 {
		t.Fatalf("consecutive = %d, want 0", got)
	}
}

func TestCrawlThrottleCheckpointFailureStops(t *testing.T) {
	clock := &fakeClock{at: time.Unix(1_000_000, 0)}
	throttle := NewCrawlThrottle(clock.at.Add(time.Hour), nil, func(context.Context, CrawlThrottleState) error {
		return errors.New("redis down")
	})
	throttle.SetClock(clock.Now)
	if _, err := throttle.Backoff(context.Background(), 0, ""); err == nil {
		t.Fatal("expected the checkpoint error to surface")
	}
	if !throttle.CheckpointFailed() || !throttle.Stopped() {
		t.Fatal("expected the throttle to be stopped after a checkpoint failure")
	}
}

func TestParseRetryAfterMs(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	if got, ok := parseRetryAfterMs("60", now); !ok || got != 60_000 {
		t.Fatalf("got %d ok=%v", got, ok)
	}
	if _, ok := parseRetryAfterMs("garbage", now); ok {
		t.Fatal("expected no value for garbage")
	}
	httpDate := now.Add(90 * time.Second).UTC().Format(time.RFC1123)
	if got, ok := parseRetryAfterMs(httpDate, now); !ok || got != 90_000 {
		t.Fatalf("got %d ok=%v", got, ok)
	}
	if got, ok := parseRetryAfterMs("999999999", now); !ok || got != throttleMaxCooldownMs+1 {
		t.Fatalf("expected over-budget values to clamp, got %d", got)
	}
}
