package ga4

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/google"
)

const (
	defaultLimit  = 100
	maxLimit      = 1000
	completeLimit = 1000
)

// ConnectionStore looks up the connected property's OAuth identity and details.
type ConnectionStore interface {
	GetByProjectID(context.Context, string) (Connection, error)
}

// JSONDoer is the bounded, token-aware Google API transport.
type JSONDoer interface {
	DoJSON(context.Context, google.APIRequest) error
}

// Service builds and executes GA4 reports while preserving their response contract.
type Service struct {
	Connections ConnectionStore
	Google      JSONDoer
	Now         func() time.Time
}

// RunReport runs one configured report for the authorized project.
func (s *Service) RunReport(ctx context.Context, projectID string, input ReportInput) (map[string]any, error) {
	if s == nil || s.Connections == nil {
		return nil, reportErr("ga4_unavailable", "Google Analytics reporting is not set up on this server.")
	}
	connection, err := s.Connections.GetByProjectID(ctx, projectID)
	if err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			return nil, reportErr("ga4_not_connected", "Google Analytics is not connected for this project.")
		}
		return nil, fmt.Errorf("load GA4 connection: %w", err)
	}
	limit := defaultLimit
	if input.Limit != nil {
		limit = *input.Limit
	}
	if limit < 1 || limit > maxLimit {
		return nil, invalid("limit must be an integer from 1 to 1000.")
	}
	offset := input.Offset
	if offset < 0 {
		return nil, invalid("offset must be a non-negative integer.")
	}
	if input.Channel == "" {
		input.Channel = "organic_search"
	}
	if input.Channel != "organic_search" && input.Channel != "all" {
		return nil, invalid("channel must be organic_search or all.")
	}
	definition, err := buildDefinition(input)
	if err != nil {
		return nil, invalid(err.Error())
	}
	if input.ComparePreviousPeriod && !comparisonSupported(input) {
		return nil, invalid("Previous-period comparison is only available for event key events, channel-group acquisition, device audiences, and new-versus-returning audiences.")
	}
	dates, err := resolveDates(input, connection.PropertyTimeZone, s.now())
	if err != nil {
		return nil, err
	}
	complete := input.ComparePreviousPeriod || input.Kind == EcommercePerformance || input.Kind == SiteSearch || input.Kind == TrafficAcquisition && input.effectiveBreakdown() == "source_medium"
	request := buildRequest(input, dates.Resolved, definition, limit, offset)
	if complete {
		request = buildRequest(input, dates.Resolved, definition, completeLimit, 0)
	}
	response, err := s.runProvider(ctx, connection, request)
	if err != nil {
		return nil, mapProviderError(err)
	}
	current, err := normalize(response, request)
	if err != nil {
		return nil, reportErr("ga4_malformed_response", "Google Analytics returned an invalid report.")
	}
	rows := current.Rows
	if complete {
		start := offset
		if start > len(rows) {
			start = len(rows)
		}
		end := start + limit
		if end > len(rows) {
			end = len(rows)
		}
		rows = rows[start:end]
		if offset+limit > len(current.Rows) && offset < current.TotalRowCount {
			pageRequest := buildRequest(input, dates.Resolved, definition, limit, offset)
			pageResponse, pageErr := s.runProvider(ctx, connection, pageRequest)
			if pageErr != nil {
				return nil, mapProviderError(pageErr)
			}
			page, normalizeErr := normalize(pageResponse, pageRequest)
			if normalizeErr != nil {
				return nil, reportErr("ga4_malformed_response", "Google Analytics returned an invalid report.")
			}
			rows = page.Rows
		}
		if len(rows) == 0 && offset < current.TotalRowCount {
			return nil, reportErr("ga4_malformed_response", "Google Analytics returned an invalid report.")
		}
	} else if len(rows) > limit {
		rows = rows[:limit]
	}
	if !complete && len(rows) == 0 && offset < current.TotalRowCount {
		return nil, reportErr("ga4_malformed_response", "Google Analytics returned an invalid report.")
	}
	rowCount := len(rows)
	next := offset + rowCount
	hasMore := next < current.TotalRowCount
	if complete && !hasMore {
		next = 0
	}
	result := map[string]any{
		"status": "ok", "source": map[string]any{"provider": "google_analytics", "propertyId": connection.PropertyID, "propertyDisplayName": connection.PropertyDisplayName},
		"request": map[string]any{"requestedDateRange": dates.Requested, "resolvedDateRange": dates.Resolved, "propertyTimeZone": connection.PropertyTimeZone, "currencyCode": connection.PropertyCurrencyCode, "channel": input.Channel,
			"reportKind": input.Kind, "breakdown": input.effectiveBreakdown(), "dimensions": definition.dimensions, "metrics": definition.metrics, "flags": map[string]bool{"includeDate": input.IncludeDate, "onlyWithTransactions": input.EcommerceOnlyWithTransactions}, "limit": limit, "offset": offset},
		"rowCount": rowCount, "totalRowCount": current.TotalRowCount, "rows": rows,
		"pageInfo":       map[string]any{"offset": offset, "limit": limit, "hasMore": hasMore, "nextOffset": any(nil)},
		"reportMetadata": current.Metadata, "quota": current.Quota, "warnings": dates.Warnings,
	}
	if hasMore {
		result["pageInfo"].(map[string]any)["nextOffset"] = next
	}
	if input.ComparePreviousPeriod {
		previousDates, err := previousPeriod(dates.Resolved)
		if err != nil {
			return nil, err
		}
		previousReq := buildRequest(input, previousDates, definition, completeLimit, 0)
		previousResponse, err := s.runProvider(ctx, connection, previousReq)
		if err != nil {
			return nil, mapProviderError(err)
		}
		previous, err := normalize(previousResponse, previousReq)
		if err != nil {
			return nil, reportErr("ga4_malformed_response", "Google Analytics returned an invalid report.")
		}
		comparison := makeComparison(current, previous, previousDates, definition)
		result["comparison"] = comparison
		if !comparison["coverage"].(map[string]any)["complete"].(bool) {
			result["warnings"] = append(dates.Warnings, "comparison_incomplete")
		}
	}
	for key, value := range reportEnhancements(current, input, dates.Resolved) {
		result[key] = value
	}
	return result, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func (s *Service) runProvider(ctx context.Context, c Connection, request APIRequest) (ProviderResponse, error) {
	if s.Google == nil {
		return ProviderResponse{}, reportErr("ga4_unavailable", "Google Analytics reporting is not set up on this server.")
	}
	if !validPropertyID(c.PropertyID) {
		return ProviderResponse{}, reportErr("ga4_malformed_response", "Google Analytics returned an invalid report.")
	}
	var response ProviderResponse
	urlPath := "https://analyticsdata.googleapis.com/v1beta/" + c.PropertyID + ":runReport"
	err := s.Google.DoJSON(ctx, google.APIRequest{UserID: c.ConnectedByUserID, Provider: "google-analytics", AccountID: c.GA4AccountID, Method: http.MethodPost, URL: urlPath, Body: request, Response: &response, Retryable: true, MaxResponseBytes: 2 << 20})
	return response, err
}

func validPropertyID(id string) bool {
	if len(id) < 12 || len(id) > 30 || id[:11] != "properties/" {
		return false
	}
	_, err := strconv.ParseUint(id[11:], 10, 64)
	return err == nil
}
func reportErr(code, message string) *reportError { return &reportError{Code: code, Message: message} }
func invalid(message string) *reportError         { return reportErr("validation_error", message) }

func mapProviderError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, google.ErrGrantUnavailable) {
		return reportErr("ga4_reconnect_required", "The Google Analytics connection has expired or was revoked.")
	}
	if errors.Is(err, google.ErrInvalidAPIResponse) {
		return reportErr("ga4_malformed_response", "Google Analytics returned an invalid report.")
	}
	var apiErr google.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case 400:
			return reportErr("ga4_report_incompatible", "This report is not compatible with the selected Analytics property.")
		case 401:
			return reportErr("ga4_reconnect_required", "The Google Analytics connection has expired or was revoked.")
		case 403:
			if apiErr.Reason == "SERVICE_DISABLED" {
				return reportErr("ga4_upstream_unavailable", "The Google Analytics Data API is not enabled for this OAuth application.")
			}
			return reportErr("ga4_property_inaccessible", "The connected Google account can no longer access this property.")
		case 404:
			return reportErr("ga4_property_inaccessible", "The selected Google Analytics property is no longer available.")
		case 429:
			quotaErr := reportErr("ga4_quota_exhausted", "Google Analytics reporting quota is exhausted. Try again later.")
			if apiErr.RetryAfterSeconds > 0 {
				seconds := apiErr.RetryAfterSeconds
				quotaErr.RetryAfterSeconds = &seconds
			}
			return quotaErr
		}
	}
	return reportErr("ga4_upstream_unavailable", "Google Analytics reporting is temporarily unavailable.")
}
