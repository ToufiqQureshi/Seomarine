package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const (
	autumnEventsURL = "https://api.useautumn.com/v1/events.list"
	eventPageLimit  = 1000
	maxRetryDelay   = 5 * time.Second
)

type usageRange struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// UsageEvent is a DataForSEO or LLM credit event returned by Autumn.
type UsageEvent struct {
	Timestamp  float64                    `json:"timestamp"`
	Value      float64                    `json:"value"`
	Properties map[string]json.RawMessage `json:"properties"`
}

type autumnEventsPage struct {
	List         []UsageEvent `json:"list"`
	HasMore      *bool        `json:"has_more"`
	HasMoreCamel *bool        `json:"hasMore"`
}

func usageEventsHandler(logger *slog.Logger, secret string, hosted bool) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	client := &http.Client{Timeout: 15 * time.Second}
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.UserFromContext(r.Context())
		if !ok || user.OrganizationID == "" {
			httpx.WriteError(w, http.StatusForbidden, "no_organization", "Open a workspace first, then try again.")
			return
		}
		var in usageRange
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil || in.Start < 0 || in.End < in.Start {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid billing usage date range.")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
			return
		}
		if !hosted {
			httpx.WriteJSON(w, http.StatusOK, []UsageEvent{})
			return
		}
		if strings.TrimSpace(secret) == "" {
			httpx.WriteError(w, http.StatusServiceUnavailable, "billing_usage_unavailable", "Billing usage history is not configured on this server.")
			return
		}
		events, err := listUsageEvents(r.Context(), client, secret, user.OrganizationID, in)
		if err != nil {
			logger.ErrorContext(r.Context(), "load billing usage events", "err", err)
			httpx.WriteError(w, http.StatusBadGateway, "billing_usage_unavailable", "Could not load billing usage history. Please try again.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, events)
	}
}

func listUsageEvents(ctx context.Context, client *http.Client, secret, customerID string, period usageRange) ([]UsageEvent, error) {
	events := make([]UsageEvent, 0)
	for offset := 0; ; {
		page, err := fetchAutumnEventsPage(ctx, client, secret, customerID, period, offset)
		if err != nil {
			return nil, err
		}
		events = append(events, page.List...)
		hasMore := false
		if page.HasMore != nil {
			hasMore = *page.HasMore
		} else if page.HasMoreCamel != nil {
			hasMore = *page.HasMoreCamel
		}
		if !hasMore || len(page.List) == 0 {
			return events, nil
		}
		offset += len(page.List)
	}
}

func fetchAutumnEventsPage(ctx context.Context, client *http.Client, secret, customerID string, period usageRange, offset int) (autumnEventsPage, error) {
	body, err := json.Marshal(map[string]any{
		"customer_id":  customerID,
		"custom_range": map[string]float64{"start": period.Start, "end": period.End},
		"feature_id":   []string{"usage_credits", "topup_credits"},
		"limit":        eventPageLimit,
		"offset":       offset,
	})
	if err != nil {
		return autumnEventsPage{}, err
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, autumnEventsURL, bytes.NewReader(body))
		if err != nil {
			return autumnEventsPage{}, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return autumnEventsPage{}, err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := retryDelay(resp.Header.Get("Retry-After"), attempt)
			resp.Body.Close()
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return autumnEventsPage{}, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			status := resp.Status
			resp.Body.Close()
			return autumnEventsPage{}, fmt.Errorf("Autumn events.list returned %s", status)
		}
		var page autumnEventsPage
		err = json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return autumnEventsPage{}, fmt.Errorf("decode Autumn events.list: %w", err)
		}
		for i := range page.List {
			if page.List[i].Properties == nil {
				page.List[i].Properties = map[string]json.RawMessage{}
			}
		}
		return page, nil
	}
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.ParseFloat(header, 64); err == nil && seconds >= 0 {
		return min(time.Duration(seconds*float64(time.Second)), maxRetryDelay)
	}
	return 250 * time.Millisecond * time.Duration(attempt+1)
}
