package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	autumnCheckURL = "https://api.useautumn.com/v1/check"

	// Autumn credit features used by hosted SEO and agent usage.
	AutumnSEODataBalanceFeatureID     = "usage_credits"
	AutumnSEOTopupBalanceFeatureID    = "topup_credits"
	maxAutumnCheckBody                = 1 << 20
)

// AutumnCreditClient reads a hosted customer's remaining credit balance.
type AutumnCreditClient struct {
	secret string
	client *http.Client
}

// NewAutumnCreditClient returns nil when Autumn is not configured.
func NewAutumnCreditClient(secret string) *AutumnCreditClient {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	return &AutumnCreditClient{
		secret: secret,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Balance returns the remaining balance for one Autumn feature. A missing
// balance is nil; provider and network failures are returned to the caller.
func (c *AutumnCreditClient) Balance(ctx context.Context, customerID, featureID string) (*float64, error) {
	if c == nil {
		return nil, fmt.Errorf("Autumn credit client is not configured")
	}
	body, err := json.Marshal(struct {
		CustomerID string `json:"customer_id"`
		FeatureID  string `json:"feature_id"`
	}{CustomerID: customerID, FeatureID: featureID})
	if err != nil {
		return nil, fmt.Errorf("encode Autumn balance request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, autumnCheckURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create Autumn balance request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Autumn balance: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Autumn balance request returned %s", resp.Status)
	}
	var result struct {
		Balance *struct {
			Remaining *float64 `json:"remaining"`
		} `json:"balance"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAutumnCheckBody)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Autumn balance response: %w", err)
	}
	if result.Balance == nil {
		return nil, nil
	}
	return result.Balance.Remaining, nil
}
