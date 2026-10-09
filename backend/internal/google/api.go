package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	googleAPITimeout = 8 * time.Second
	maxAPIAttempts   = 3
)

// APIError carries an HTTP status without exposing Google's response body.
type APIError struct{ Status int }

// Error implements error without including provider data or credentials.
func (e APIError) Error() string { return "google api request failed" }

// APIClient sends authorized, bounded Google API requests.
type APIClient struct {
	Tokens    TokenSource
	Client    *http.Client
	mu        sync.Mutex
	failures  int
	openUntil time.Time
}

// TokenSource supplies a fresh token and refreshes a token rejected with 401.
type TokenSource interface {
	AccessToken(context.Context, string, string, string, string) (string, error)
}

// APIRequest describes one Google API call. Retryable must only be true for
// read-only or otherwise idempotent operations, including read-only POST APIs.
type APIRequest struct {
	UserID           string
	Provider         string
	AccountID        string
	Method           string
	URL              string
	Body             any
	Response         any
	Retryable        bool
	MaxResponseBytes int64
}

func (c *APIClient) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{Timeout: googleAPITimeout}
}

func (c *APIClient) available(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !now.Before(c.openUntil)
}

func (c *APIClient) recordFailure(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures++
	if c.failures >= 5 {
		c.openUntil = now.Add(30 * time.Second)
		c.failures = 0
	}
}

func (c *APIClient) recordSuccess() {
	c.mu.Lock()
	c.failures = 0
	c.mu.Unlock()
}

// DoJSON performs a bounded API call, refreshes once after 401, and retries
// idempotent calls after quota or transient failures with capped jitter.
func (c *APIClient) DoJSON(ctx context.Context, input APIRequest) error {
	if c == nil || c.Tokens == nil || input.UserID == "" || input.URL == "" || input.Response == nil ||
		(input.Method != http.MethodGet && input.Method != http.MethodPost) ||
		(input.Method == http.MethodPost && input.Body == nil) {
		return errors.New("invalid google api request")
	}
	if !c.available(time.Now()) {
		return errors.New("google api temporarily unavailable")
	}
	token, err := c.Tokens.AccessToken(ctx, input.UserID, input.Provider, input.AccountID, "")
	if err != nil {
		return err
	}
	var body []byte
	if input.Body != nil {
		body, err = json.Marshal(input.Body)
		if err != nil {
			return errors.New("invalid google api request body")
		}
		if len(body) > 64<<10 {
			return errors.New("google api request body too large")
		}
	}
	maxBytes := input.MaxResponseBytes
	if maxBytes <= 0 || maxBytes > 8<<20 {
		maxBytes = 2 << 20
	}
	refreshed := false
	for attempt := 0; attempt < maxAPIAttempts; {
		if err := ctx.Err(); err != nil {
			return err
		}
		requestCtx, cancel := context.WithTimeout(ctx, googleAPITimeout)
		req, err := http.NewRequestWithContext(requestCtx, input.Method, input.URL, bytes.NewReader(body))
		if err != nil {
			cancel()
			return errors.New("invalid google api url")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.client().Do(req)
		if err != nil {
			cancel()
			c.recordFailure(time.Now())
			if !input.Retryable || attempt == maxAPIAttempts-1 {
				return errors.New("google api temporarily unavailable")
			}
			if err := apiBackoff(ctx, attempt, 0); err != nil {
				return err
			}
			attempt++
			continue
		}
		if resp.StatusCode == http.StatusUnauthorized && !refreshed {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			cancel()
			refreshed = true
			token, err = c.Tokens.AccessToken(ctx, input.UserID, input.Provider, input.AccountID, token)
			if err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			retryAfter := resp.Header.Get("Retry-After")
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			cancel()
			if resp.StatusCode >= 500 {
				c.recordFailure(time.Now())
			}
			if input.Retryable && attempt < maxAPIAttempts-1 {
				if err := apiBackoff(ctx, attempt, parseRetryAfter(retryAfter)); err != nil {
					return err
				}
				attempt++
				continue
			}
			return APIError{Status: resp.StatusCode}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			cancel()
			return APIError{Status: resp.StatusCode}
		}
		limited := io.LimitReader(resp.Body, maxBytes+1)
		data, readErr := io.ReadAll(limited)
		_ = resp.Body.Close()
		cancel()
		if readErr != nil || int64(len(data)) > maxBytes {
			return errors.New("google api response unavailable")
		}
		if err := json.Unmarshal(data, input.Response); err != nil {
			return errors.New("invalid google api response")
		}
		c.recordSuccess()
		return nil
	}
	return errors.New("google api retry limit reached")
}

func parseRetryAfter(raw string) time.Duration {
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return 0
	}
	if seconds > 2 {
		seconds = 2
	}
	return time.Duration(seconds) * time.Second
}

func apiBackoff(ctx context.Context, attempt int, retryAfter time.Duration) error {
	delay := time.Duration(200<<attempt)*time.Millisecond + time.Duration(rand.IntN(100))*time.Millisecond
	if retryAfter > delay {
		delay = retryAfter
	}
	if delay > 2*time.Second {
		delay = 2 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
