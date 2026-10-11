// Package email contains Go clients for outbound email providers.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const loopsTransactionalURL = "https://app.loops.so/api/v1/transactional"

var ErrLoopsUnavailable = errors.New("Loops email provider unavailable")

// LoopsClient sends transactional messages through Loops. Keep the HTTP client
// injectable so callers can share transport settings without sharing secrets.
type LoopsClient struct {
	APIKey string
	HTTP   *http.Client
}

// SendTransactional sends one Loops transactional email without adding the
// recipient to the marketing audience.
func (c LoopsClient) SendTransactional(ctx context.Context, email, templateID string, dataVariables map[string]string) error {
	if strings.TrimSpace(c.APIKey) == "" || strings.TrimSpace(templateID) == "" {
		return ErrLoopsUnavailable
	}

	body, err := json.Marshal(map[string]any{
		"transactionalId": templateID,
		"email":           email,
		"addToAudience":   false,
		"dataVariables":   dataVariables,
	})
	if err != nil {
		return fmt.Errorf("encode Loops transactional email: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, loopsTransactionalURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build Loops transactional email request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send Loops transactional email: %w", errors.Join(ErrLoopsUnavailable, err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Loops transactional email returned %s: %w", resp.Status, ErrLoopsUnavailable)
	}
	return nil
}
