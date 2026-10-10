package ga4

import (
	"context"
	"errors"
	"fmt"
)

func overviewMetrics() []string {
	return []string{"sessions", "activeUsers", "engagedSessions", "engagementRate", "keyEvents", "transactions", "purchaseRevenue"}
}

// OrganicOverviewInput selects the date range and trend granularity.
type OrganicOverviewInput struct {
	StartDate string `json:"startDate,omitempty"`
	EndDate   string `json:"endDate,omitempty"`
	Trend     string `json:"trend,omitempty"`
}

// GetOrganicOverview returns current, previous-period and trend reports for
// organic search, preserving the legacy overview response contract.
func (s *Service) GetOrganicOverview(ctx context.Context, projectID string, input OrganicOverviewInput) (map[string]any, error) {
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
	trend := input.Trend
	if trend == "" {
		trend = "daily"
	}
	if trend != "daily" && trend != "weekly" {
		return nil, invalid("trend must be daily or weekly.")
	}
	dates, err := resolveDates(ReportInput{StartDate: input.StartDate, EndDate: input.EndDate}, connection.PropertyTimeZone, s.now())
	if err != nil {
		return nil, err
	}
	previousDates, err := previousPeriod(dates.Resolved)
	if err != nil {
		return nil, err
	}
	currentRequest := overviewRequest(dates.Resolved, "")
	previousRequest := overviewRequest(previousDates, "")
	trendDimension := "date"
	if trend == "weekly" {
		trendDimension = "yearWeek"
	}
	trendRequest := overviewRequest(dates.Resolved, trendDimension)
	current, err := s.overviewReport(ctx, connection, currentRequest)
	if err != nil {
		return nil, mapProviderError(err)
	}
	previous, err := s.overviewReport(ctx, connection, previousRequest)
	if err != nil {
		return nil, mapProviderError(err)
	}
	trendReport, err := s.overviewReport(ctx, connection, trendRequest)
	if err != nil {
		return nil, mapProviderError(err)
	}
	var currentSummary, previousSummary map[string]any
	if len(current.Rows) > 0 {
		currentSummary = current.Rows[0]
	}
	if len(previous.Rows) > 0 {
		previousSummary = previous.Rows[0]
	}
	limited := current.Metadata.HasLimitedData || previous.Metadata.HasLimitedData || trendReport.Metadata.HasLimitedData
	return map[string]any{
		"status":  "ok",
		"source":  map[string]any{"provider": "google_analytics", "propertyId": connection.PropertyID, "propertyDisplayName": connection.PropertyDisplayName},
		"request": map[string]any{"requestedDateRange": dates.Requested, "resolvedDateRange": dates.Resolved, "previousDateRange": previousDates, "propertyTimeZone": connection.PropertyTimeZone, "currencyCode": connection.PropertyCurrencyCode, "channel": "organic_search", "trend": trend},
		"current": currentSummary, "previous": previousSummary,
		"comparison":     overviewComparison(currentSummary, previousSummary),
		"trend":          trendReport.Rows,
		"diagnostics":    overviewDiagnostics(currentSummary, previousSummary, limited),
		"reportMetadata": map[string]any{"hasLimitedData": limited, "reports": []reportMetadata{current.Metadata, previous.Metadata, trendReport.Metadata}},
		"quota":          firstQuota(trendReport.Quota, current.Quota),
		"warnings":       overviewWarnings(dates.Warnings, trendReport),
	}, nil
}

func (s *Service) overviewReport(ctx context.Context, connection Connection, request APIRequest) (normalizedReport, error) {
	response, err := s.runProvider(ctx, connection, request)
	if err != nil {
		return normalizedReport{}, err
	}
	result, err := normalize(response, request)
	if err != nil {
		return normalizedReport{}, reportErr("ga4_malformed_response", "Google Analytics returned an invalid report.")
	}
	return result, nil
}

func overviewRequest(dates DateRange, dimension string) APIRequest {
	request := APIRequest{DateRanges: []DateRange{dates}, Offset: "0", Limit: "1", KeepEmptyRows: false, ReturnPropertyQuota: true,
		DimensionFilter: map[string]any{"filter": map[string]any{"fieldName": "sessionDefaultChannelGroup", "stringFilter": map[string]any{"matchType": "EXACT", "value": "Organic Search"}}}}
	if dimension != "" {
		request.Dimensions = []Name{{Name: dimension}}
		request.Limit = "1000"
		request.OrderBys = []OrderBy{{Dimension: &DimensionOrder{DimensionName: dimension}}}
	}
	for _, name := range overviewMetrics() {
		request.Metrics = append(request.Metrics, Name{Name: name})
	}
	return request
}

func overviewComparison(current, previous map[string]any) map[string]any {
	result := make(map[string]any, len(overviewMetrics()))
	for _, name := range overviewMetrics() {
		cv, cok := current[name].(float64)
		pv, pok := previous[name].(float64)
		var absolute, percent any
		if cok && pok {
			absolute = cv - pv
			if pv != 0 {
				percent = (cv - pv) / pv
			}
		}
		result[name] = map[string]any{"current": nullable(cv, cok), "previous": nullable(pv, pok), "absoluteChange": absolute, "percentChange": percent}
	}
	return result
}

func overviewDiagnostics(current, previous map[string]any, limited bool) []any {
	if limited {
		return []any{}
	}
	cv, cok := current["keyEvents"].(float64)
	pv, pok := previous["keyEvents"].(float64)
	if !cok || !pok || pv < 5 || (cv-pv)/pv > -0.5 {
		return []any{}
	}
	return []any{map[string]any{"code": "key_events_sharp_decline", "severity": "warning", "message": "Organic key events declined sharply compared with the previous equal-length period.",
		"evidence":  map[string]any{"current": cv, "previous": pv, "percentChange": (cv - pv) / pv},
		"threshold": map[string]any{"minimumPreviousKeyEvents": 5, "percentChange": -0.5}}}
}

func overviewWarnings(warnings []string, trend normalizedReport) []string {
	result := append([]string{}, warnings...)
	if trend.TotalRowCount > len(trend.Rows) {
		result = append(result, "trend_truncated")
	}
	return result
}

func firstQuota(preferred, fallback *Quota) *Quota {
	if preferred != nil {
		return preferred
	}
	return fallback
}
