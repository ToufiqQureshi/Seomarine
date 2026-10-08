// Package dataforseo contains the authenticated transport shared by DataForSEO
// feature clients. Endpoint-specific validation and response mapping belong to
// their feature packages.
package dataforseo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// APIBaseURL is the production DataForSEO API origin.
	APIBaseURL       = "https://api.dataforseo.com"
	requestTimeout   = 60 * time.Second
	maxResponseBytes = 64 << 20
)

// ErrInvalidRequest marks a request the client refuses to send.
var ErrInvalidRequest = errors.New("invalid DataForSEO request")

// Cost is the provider-reported price of one DataForSEO task.
type Cost struct {
	Path []string
	USD  float64
}

// CostRecorder persists the cost of a DataForSEO task against an organization.
type CostRecorder interface {
	RecordDataForSEO(ctx context.Context, organizationID string, cost Cost) error
}

// CostRecorderFunc adapts a function to CostRecorder.
type CostRecorderFunc func(context.Context, string, Cost) error

// RecordDataForSEO calls f.
func (f CostRecorderFunc) RecordDataForSEO(ctx context.Context, organizationID string, cost Cost) error {
	return f(ctx, organizationID, cost)
}

// Options configures a Client. Zero values select safe defaults.
type Options struct {
	BaseURL      string
	APIKey       string
	HTTPClient   *http.Client
	Recorder     CostRecorder
	Timeout      time.Duration
	MaxRetries   int
	RetryBackoff time.Duration
}

// Client is an authenticated DataForSEO transport that records task costs.
type Client struct {
	baseURL    string
	apiKey     string
	http       *http.Client
	recorder   CostRecorder
	timeout    time.Duration
	maxRetries int
	backoff    time.Duration
}

// CostRecordingError reports that a billed task could not be recorded.
type CostRecordingError struct {
	OrganizationID string
	Cost           Cost
	Cause          error
}

func (e *CostRecordingError) Error() string {
	return fmt.Sprintf("record DataForSEO cost for organization %q: %v", e.OrganizationID, e.Cause)
}

func (e *CostRecordingError) Unwrap() error { return e.Cause }

// HTTPError is a non-2xx DataForSEO response.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("DataForSEO HTTP %d: %s", e.StatusCode, e.Body)
}

// NewClient validates opts and returns a Client.
func NewClient(opts Options) (*Client, error) {
	if opts.APIKey == "" {
		return nil, errors.New("DataForSEO API key is required")
	}
	decoded, err := base64.StdEncoding.DecodeString(opts.APIKey)
	if err != nil || !strings.Contains(string(decoded), ":") {
		return nil, errors.New("DataForSEO API key must be base64-encoded credentials")
	}
	if opts.Recorder == nil {
		return nil, errors.New("DataForSEO cost recorder is required")
	}
	if opts.BaseURL == "" {
		opts.BaseURL = APIBaseURL
	}
	base, err := url.Parse(opts.BaseURL)
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("DataForSEO base URL must be an absolute HTTP(S) URL without credentials or query")
	}
	if opts.Timeout == 0 {
		opts.Timeout = requestTimeout
	}
	if opts.Timeout < time.Second || opts.Timeout > 5*time.Minute {
		return nil, errors.New("DataForSEO timeout must be between 1 second and 5 minutes")
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = 2
	}
	if opts.MaxRetries < 0 || opts.MaxRetries > 5 {
		return nil, errors.New("DataForSEO max retries must be between 0 and 5")
	}
	if opts.RetryBackoff == 0 {
		opts.RetryBackoff = 250 * time.Millisecond
	}
	if opts.RetryBackoff < 0 || opts.RetryBackoff > 10*time.Second {
		return nil, errors.New("DataForSEO retry backoff must be between 0 and 10 seconds")
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{}
	}
	clientCopy := *opts.HTTPClient
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{baseURL: strings.TrimRight(opts.BaseURL, "/"), apiKey: opts.APIKey, http: &clientCopy,
		recorder: opts.Recorder, timeout: opts.Timeout, maxRetries: opts.MaxRetries, backoff: opts.RetryBackoff}, nil
}

// Do sends one request. Set retrySafe only for operations that are safe to
// replay: billed task-post calls must pass false to avoid duplicate charges.
// Actual provider task costs are recorded against organizationID before the
// response is returned, including task-level failures carried by HTTP 200.
func (c *Client) Do(ctx context.Context, organizationID, method, path string, body []byte, retrySafe bool) (json.RawMessage, error) {
	if strings.TrimSpace(organizationID) == "" || len(organizationID) > 128 {
		return nil, fmt.Errorf("%w: organization id is required", ErrInvalidRequest)
	}
	if method != http.MethodGet && method != http.MethodPost {
		return nil, fmt.Errorf("%w: only GET and POST are supported", ErrInvalidRequest)
	}
	endpoint, err := c.endpoint(path)
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	for attempt := 0; ; attempt++ {
		var requestBody io.Reader
		if body != nil {
			requestBody = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(requestCtx, method, endpoint, requestBody)
		if err != nil {
			return nil, fmt.Errorf("build DataForSEO request: %w", err)
		}
		req.Header.Set("Authorization", "Basic "+c.apiKey)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		response, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send DataForSEO request: %w", err)
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		closeErr := response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read DataForSEO response: %w", readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close DataForSEO response: %w", closeErr)
		}
		if len(payload) > maxResponseBytes {
			return nil, fmt.Errorf("DataForSEO response exceeds %d bytes", maxResponseBytes)
		}
		if response.StatusCode >= 500 && retrySafe && attempt < c.maxRetries {
			if err := wait(requestCtx, c.backoff*time.Duration(attempt+1)); err != nil {
				return nil, err
			}
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, &HTTPError{StatusCode: response.StatusCode, Body: truncate(string(payload), 1600)}
		}
		if len(payload) == 0 {
			return nil, nil
		}
		if !json.Valid(payload) {
			return nil, errors.New("DataForSEO returned invalid JSON")
		}
		costs, err := responseCosts(payload)
		if err != nil {
			return nil, err
		}
		for _, cost := range costs {
			recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			err := c.recorder.RecordDataForSEO(recordCtx, organizationID, cost)
			recordCancel()
			if err != nil {
				return nil, &CostRecordingError{OrganizationID: organizationID, Cost: cost, Cause: err}
			}
		}
		return json.RawMessage(payload), nil
	}
}

func (c *Client) endpoint(path string) (string, error) {
	parsed, err := url.ParseRequestURI(path)
	if err != nil || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || parsed.IsAbs() || parsed.Host != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: path must be an absolute API path", ErrInvalidRequest)
	}
	return c.baseURL + path, nil
}

func responseCosts(payload []byte) ([]Cost, error) {
	var envelope struct {
		Tasks []struct {
			Path []string `json:"path"`
			Cost *float64 `json:"cost"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("decode DataForSEO billing metadata: %w", err)
	}
	costs := make([]Cost, 0, len(envelope.Tasks))
	for _, task := range envelope.Tasks {
		if task.Cost == nil {
			continue
		}
		if len(task.Path) == 0 || math.IsNaN(*task.Cost) || math.IsInf(*task.Cost, 0) || *task.Cost < 0 {
			return nil, errors.New("DataForSEO returned invalid billing metadata")
		}
		costs = append(costs, Cost{Path: task.Path, USD: *task.Cost})
	}
	return costs, nil
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func truncate(value string, limit int) string {
	if len(value) > limit {
		return value[:limit] + "... [truncated]"
	}
	return value
}
