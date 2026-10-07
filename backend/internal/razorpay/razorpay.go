// Package razorpay is the only place that speaks Razorpay's REST API and
// webhook format, so the payment provider can be swapped without touching
// the billing logic.
package razorpay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BaseURL is Razorpay's production API.
const BaseURL = "https://api.razorpay.com/v1"

const (
	// requestTimeout bounds every API call, including reading the response.
	requestTimeout = 10 * time.Second
	// maxResponseBody bounds what is read from Razorpay; real responses are
	// a few kilobytes.
	maxResponseBody = 1 << 20
)

// Client calls the Razorpay API with a key pair.
type Client struct {
	baseURL   string
	keyID     string
	keySecret string
	http      *http.Client
}

// NewClient returns a Client for the API at baseURL (BaseURL in production).
func NewClient(baseURL, keyID, keySecret string) *Client {
	return &Client{
		baseURL:   baseURL,
		keyID:     keyID,
		keySecret: keySecret,
		http:      &http.Client{Timeout: requestTimeout},
	}
}

// KeyID is the public key id, which Razorpay Checkout in the browser needs.
func (c *Client) KeyID() string { return c.keyID }

// Subscription is the part of a Razorpay subscription entity the app uses.
type Subscription struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	// CurrentEnd is the end of the paid billing cycle in Unix seconds, nil
	// before the first charge.
	CurrentEnd *int64 `json:"current_end"`
	// CreatedAt is in Unix seconds.
	CreatedAt int64 `json:"created_at"`
	// Notes is kept raw: Razorpay sends an object, or [] when there are none.
	Notes json.RawMessage `json:"notes"`
}

// Note returns the subscription's note key, or "" when it has none.
func (s Subscription) Note(key string) string {
	var notes map[string]string
	if err := json.Unmarshal(s.Notes, &notes); err != nil {
		return "" // [] or missing notes: there are none
	}
	return notes[key]
}

// APIError is a non-2xx answer from the API.
type APIError struct {
	StatusCode  int
	Code        string
	Description string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("razorpay: HTTP %d %s: %s", e.StatusCode, e.Code, e.Description)
}

// CreateSubscription subscribes a new customer to planID for totalCount
// billing cycles. notes are stored on the subscription and come back in its
// webhooks.
func (c *Client) CreateSubscription(ctx context.Context, planID string, totalCount int, notes map[string]string) (Subscription, error) {
	body, err := json.Marshal(map[string]any{
		"plan_id":         planID,
		"total_count":     totalCount,
		"customer_notify": true,
		"notes":           notes,
	})
	if err != nil {
		return Subscription{}, fmt.Errorf("encode subscription request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/subscriptions", bytes.NewReader(body))
	if err != nil {
		return Subscription{}, fmt.Errorf("build subscription request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.keyID, c.keySecret)

	var sub Subscription
	if err := c.do(req, &sub); err != nil {
		return Subscription{}, fmt.Errorf("create subscription: %w", err)
	}
	if sub.ID == "" {
		return Subscription{}, errors.New("create subscription: response has no subscription id")
	}
	return sub, nil
}

// do sends req and decodes a 2xx JSON answer into dst, or returns an
// *APIError for any other status.
func (c *Client) do(req *http.Request, dst any) (err error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close response: %w", closeErr))
		}
	}()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &APIError{StatusCode: resp.StatusCode, Description: http.StatusText(resp.StatusCode)}
		var body struct {
			Error struct {
				Code        string `json:"code"`
				Description string `json:"description"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &body) == nil && body.Error.Code != "" {
			apiErr.Code, apiErr.Description = body.Error.Code, body.Error.Description
		}
		return apiErr
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// Event is a webhook delivery. Subscription is set for subscription.* events.
type Event struct {
	Event   string `json:"event"`
	Payload struct {
		Subscription *struct {
			Entity Subscription `json:"entity"`
		} `json:"subscription"`
	} `json:"payload"`
	// CreatedAt is in Unix seconds.
	CreatedAt int64 `json:"created_at"`
}

// ValidSignature reports whether signature, the X-Razorpay-Signature header,
// is the hex HMAC-SHA256 of the raw webhook body under secret. The
// comparison is constant-time.
func ValidSignature(body []byte, signature, secret string) bool {
	got, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}
