package audit

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// throttleMaxRetries is the number of retries per URL after its first 429.
	// Every 429 still pauses new URLs.
	throttleMaxRetries = 3
	// throttleFirstDelayMs is the first backoff delay for a 429 without a
	// Retry-After header.
	throttleFirstDelayMs = 30_000
	// throttleMaxIntervalMs caps the per-request interval.
	throttleMaxIntervalMs = 30_000
	// throttleMaxCooldownMs caps the total accumulated cooldown.
	throttleMaxCooldownMs = 30 * 60_000
)

// CrawlThrottleState is checkpointed between chunks so a new step does not
// forget the site's limits.
type CrawlThrottleState struct {
	IntervalMs            int64 `json:"intervalMs"`
	NextRequestAtMs       int64 `json:"nextRequestAt"`
	PausedUntilMs         int64 `json:"pausedUntil"`
	ConsecutiveRateLimits int   `json:"consecutiveRateLimits"`
	CooldownMs            int64 `json:"cooldownMs"`
}

// CrawlThrottle paces requests to one audit's origin. Long waits are resumed by
// the workflow rather than slept through.
type CrawlThrottle struct {
	mu               sync.Mutex
	saveMu           sync.Mutex
	state            CrawlThrottleState
	deadline         time.Time
	persist          func(context.Context, CrawlThrottleState) error
	checkpointFailed bool
	now              func() time.Time
}

// NewCrawlThrottle creates a throttle that refuses to start a request after
// deadlineAt. previous resumes a checkpointed state; persist may be nil.
func NewCrawlThrottle(deadlineAt time.Time, previous *CrawlThrottleState, persist func(context.Context, CrawlThrottleState) error) *CrawlThrottle {
	state := CrawlThrottleState{IntervalMs: 1_000}
	if previous != nil {
		state = *previous
	}
	return &CrawlThrottle{state: state, deadline: deadlineAt, persist: persist, now: time.Now}
}

// SetClock replaces the throttle's clock. Test-only.
func (t *CrawlThrottle) SetClock(now func() time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if now == nil {
		now = time.Now
	}
	t.now = now
}

// CheckpointFailed reports whether persisting state failed.
func (t *CrawlThrottle) CheckpointFailed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.checkpointFailed
}

// Stopped reports whether the throttle has given up.
func (t *CrawlThrottle) Stopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stoppedLocked()
}

// State returns a copy of the current throttle state.
func (t *CrawlThrottle) State() CrawlThrottleState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

func (t *CrawlThrottle) stoppedLocked() bool {
	return t.checkpointFailed ||
		t.state.ConsecutiveRateLimits > throttleMaxRetries ||
		t.state.CooldownMs > throttleMaxCooldownMs
}

func (t *CrawlThrottle) readyAtLocked() time.Time {
	readyAtMs := max(t.state.PausedUntilMs, t.state.NextRequestAtMs)
	return time.UnixMilli(readyAtMs)
}

// Ready blocks until this chunk may start a request, then reserves the slot by
// advancing NextRequestAt synchronously so waiters waking together take turns.
// It returns false when the throttle is stopped or the deadline has passed.
func (t *CrawlThrottle) Ready(ctx context.Context) (bool, error) {
	for {
		t.mu.Lock()
		if t.stoppedLocked() {
			t.mu.Unlock()
			return false, nil
		}
		now := t.now()
		readyAt := t.readyAtLocked()
		if !now.Before(t.deadline) || !readyAt.Before(t.deadline) {
			t.mu.Unlock()
			return false, nil
		}
		if !readyAt.After(now) {
			t.state.NextRequestAtMs = now.Add(time.Duration(t.state.IntervalMs) * time.Millisecond).UnixMilli()
			t.mu.Unlock()
			return true, nil
		}
		waitFor := readyAt.Sub(now)
		t.mu.Unlock()
		if err := sleepContext(ctx, waitFor); err != nil {
			return false, err
		}
	}
}

// Backoff pauses the origin on a 429 and reports whether this URL may retry.
func (t *CrawlThrottle) Backoff(ctx context.Context, attempt int, retryAfter string) (bool, error) {
	t.mu.Lock()
	t.state.ConsecutiveRateLimits++
	t.state.IntervalMs = min(t.state.IntervalMs*2, int64(throttleMaxIntervalMs))
	delayMs := t.state.IntervalMs
	if retryAfterDelay, ok := parseRetryAfterMs(retryAfter, t.now()); ok {
		delayMs = max(delayMs, retryAfterDelay)
	} else {
		delayMs = max(delayMs, throttleFirstDelayMs*int64(math.Pow(2, float64(t.state.ConsecutiveRateLimits-1))))
	}
	now := t.now()
	pausedUntil := max(t.state.PausedUntilMs, now.Add(time.Duration(delayMs)*time.Millisecond).UnixMilli())
	t.state.CooldownMs += pausedUntil - max(now.UnixMilli(), t.state.PausedUntilMs)
	t.state.PausedUntilMs = pausedUntil
	stopped := t.stoppedLocked()
	t.mu.Unlock()

	if err := t.save(ctx); err != nil {
		return false, err
	}
	return !stopped && attempt <= throttleMaxRetries, nil
}

// Recovered breaks a run of consecutive refusals after a non-429 response.
func (t *CrawlThrottle) Recovered(ctx context.Context) error {
	t.mu.Lock()
	if t.state.ConsecutiveRateLimits == 0 {
		t.mu.Unlock()
		return nil
	}
	t.state.ConsecutiveRateLimits = 0
	t.mu.Unlock()
	return t.save(ctx)
}

// save persists the current state, serialized so checkpoints land in order.
func (t *CrawlThrottle) save(ctx context.Context) error {
	t.saveMu.Lock()
	defer t.saveMu.Unlock()
	if t.persist == nil {
		return nil
	}
	snapshot := t.State()
	if err := t.persist(ctx, snapshot); err != nil {
		t.mu.Lock()
		t.checkpointFailed = true
		t.mu.Unlock()
		return err
	}
	return nil
}

// parseRetryAfterMs parses a Retry-After header as either delay-seconds or an
// HTTP-date. ok=false means no usable value.
func parseRetryAfterMs(header string, now time.Time) (int64, bool) {
	if header == "" {
		return 0, false
	}
	value := strings.TrimSpace(header)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		// Keep over-budget values finite for checkpoint arithmetic.
		return min(seconds*1_000, int64(throttleMaxCooldownMs)+1), true
	}
	// Retry-After may also be an HTTP-date in any of the formats browsers and
	// JavaScript's Date.parse accept.
	for _, layout := range []string{http.TimeFormat, time.RFC1123, time.RFC1123Z, time.RFC850, time.ANSIC} {
		if at, err := time.Parse(layout, value); err == nil {
			return max(int64(0), at.Sub(now).Milliseconds()), true
		}
	}
	return 0, false
}

// sleepContext sleeps for d, returning early with the context's error when the
// caller goes away.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
