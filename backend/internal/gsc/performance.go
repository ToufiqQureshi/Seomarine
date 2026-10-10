package gsc

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/google"
)

const (
	defaultSearchRowLimit = 250
	maxSearchRowLimit     = 1000
	strikingDistanceLimit = 1000
	dailyRowLimit         = 200
	countryRowLimit       = 25
	exportRowLimit        = 1000
)

type dateRange string

const (
	last7Days    dateRange = "last_7_days"
	last28Days   dateRange = "last_28_days"
	last3Months  dateRange = "last_3_months"
	last6Months  dateRange = "last_6_months"
	last12Months dateRange = "last_12_months"
	last16Months dateRange = "last_16_months"
)

// PerformanceInput describes the public search-performance filters and range.
type PerformanceInput struct {
	DateRange   dateRange   `json:"dateRange,omitempty"`
	StartDate   string      `json:"-"`
	EndDate     string      `json:"-"`
	Device      string      `json:"device,omitempty"`
	Country     string      `json:"country,omitempty"`
	PageFilter  *TextFilter `json:"pageFilter,omitempty"`
	QueryFilter *TextFilter `json:"queryFilter,omitempty"`
}

// TextFilter restricts query or page dimension rows.
type TextFilter struct {
	Operator   string `json:"operator"`
	Expression string `json:"expression"`
}

// TableInput selects one page of the query or page table.
type TableInput struct {
	PerformanceInput
	Dimension string          `json:"dimension"`
	Page      OptionalPageInt `json:"page,omitempty"`
	PageSize  OptionalPageInt `json:"pageSize,omitempty"`
}

// OptionalPageInt preserves omitted versus explicit numeric paging fields.
type OptionalPageInt struct {
	Value   int
	Present bool
}

// UnmarshalJSON rejects null and non-integer values while preserving omission.
func (v *OptionalPageInt) UnmarshalJSON(data []byte) error {
	if strings.TrimSpace(string(data)) == "null" {
		return errors.New("page values cannot be null")
	}
	if err := json.Unmarshal(data, &v.Value); err != nil {
		return err
	}
	v.Present = true
	return nil
}

// ExportInput selects the complete capped query or page table.
type ExportInput struct {
	PerformanceInput
	Dimension string `json:"dimension"`
}

// MCPPerformanceInput preserves the public MCP Search Console query contract.
type MCPPerformanceInput struct {
	Dimensions     []string          `json:"dimensions"`
	DateRange      string            `json:"dateRange"`
	StartDate      string            `json:"startDate"`
	EndDate        string            `json:"endDate"`
	Filters        []DimensionFilter `json:"filters"`
	RowLimit       *int              `json:"rowLimit"`
	StartRow       *int              `json:"startRow"`
	MinPosition    *float64          `json:"minPosition"`
	MaxPosition    *float64          `json:"maxPosition"`
	MinImpressions *int              `json:"minImpressions"`
	Type           string            `json:"type"`
	DataState      string            `json:"dataState"`
}

// SearchClient is the read-only part of the Search Console client.
type SearchClient interface {
	QuerySearchAnalytics(context.Context, string, SearchRequest) ([]SearchRow, error)
}

// Service owns the Search Performance report rules.
type Service struct {
	Connections ConnectionReader
	NewClient   func(userID, accountID string) SearchClient
	Now         func() time.Time
}

type dateWindow struct {
	StartDate string
	EndDate   string
}

func resolveDateRange(input PerformanceInput, today time.Time) (dateWindow, error) {
	today = today.UTC()
	floor := subtractUTCMonths(today, 16).Format("2006-01-02")
	if input.StartDate != "" || input.EndDate != "" {
		if input.StartDate == "" || input.EndDate == "" {
			return dateWindow{}, validationError("Provide both startDate and endDate, or neither.")
		}
		start, ok1 := parseDate(input.StartDate)
		end, ok2 := parseDate(input.EndDate)
		if !ok1 || !ok2 || start.After(end) {
			return dateWindow{}, validationError("Dates must be valid YYYY-MM-DD values with startDate on or before endDate.")
		}
		startDate := input.StartDate
		if startDate < floor {
			startDate = floor
		}
		return dateWindow{StartDate: startDate, EndDate: input.EndDate}, nil
	}
	end := today.AddDate(0, 0, -3)
	rangeValue := input.DateRange
	if rangeValue == "" {
		rangeValue = last28Days
	}
	var start time.Time
	switch rangeValue {
	case last7Days:
		start = end.AddDate(0, 0, -7)
	case last28Days:
		start = end.AddDate(0, 0, -28)
	case last3Months:
		start = subtractUTCMonths(end, 3)
	case last6Months:
		start = subtractUTCMonths(end, 6)
	case last12Months:
		start = subtractUTCMonths(end, 12)
	case last16Months:
		start = subtractUTCMonths(end, 16)
	default:
		return dateWindow{}, validationError("dateRange must be one of the supported Search Console ranges.")
	}
	if start.Format("2006-01-02") < floor {
		start, _ = parseDate(floor)
	}
	return dateWindow{StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02")}, nil
}

func parseDate(value string) (time.Time, bool) {
	d, err := time.Parse("2006-01-02", value)
	return d, err == nil && d.Format("2006-01-02") == value
}
func subtractUTCMonths(day time.Time, months int) time.Time {
	year, month, date := day.Date()
	target := time.Date(year, month-time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if date > last {
		date = last
	}
	return time.Date(target.Year(), target.Month(), date, 0, 0, 0, 0, time.UTC)
}

// BuildSearchAnalyticsRequest resolves dates and builds a provider-valid read request.
func BuildSearchAnalyticsRequest(input PerformanceInput, dimensions []string, rowLimit, startRow int, today time.Time) (SearchRequest, error) {
	rangeDates, err := resolveDateRange(input, today)
	if err != nil {
		return SearchRequest{}, err
	}
	if rowLimit < 1 {
		rowLimit = 1
	}
	if rowLimit > maxSearchRowLimit {
		rowLimit = maxSearchRowLimit
	}
	if startRow < 0 || startRow > 1_000_000 {
		return SearchRequest{}, validationError("startRow must be between 0 and 1000000.")
	}
	if len(dimensions) == 0 {
		dimensions = []string{"query"}
	}
	request := SearchRequest{StartDate: rangeDates.StartDate, EndDate: rangeDates.EndDate, Dimensions: dimensions, RowLimit: rowLimit, Type: "web", DataState: "all"}
	if startRow > 0 {
		request.StartRow = startRow
	}
	filters := []DimensionFilter{}
	if input.Device != "" {
		filters = append(filters, DimensionFilter{Dimension: "device", Operator: "equals", Expression: input.Device})
	}
	if input.PageFilter != nil {
		filters = append(filters, DimensionFilter{Dimension: "page", Operator: input.PageFilter.Operator, Expression: input.PageFilter.Expression})
	}
	if input.QueryFilter != nil {
		filters = append(filters, DimensionFilter{Dimension: "query", Operator: input.QueryFilter.Operator, Expression: input.QueryFilter.Expression})
	}
	if input.Country != "" {
		filters = append(filters, DimensionFilter{Dimension: "country", Operator: "equals", Expression: input.Country})
	}
	if len(filters) > 0 {
		request.DimensionFilterGroups = []FilterGroup{{GroupType: "and", Filters: filters}}
	}
	return request, nil
}

func (s *Service) client(ctx context.Context, organizationID, projectID string) (Connection, SearchClient, error) {
	if s == nil || s.Connections == nil || s.NewClient == nil {
		return Connection{}, nil, errors.New("search console reporting is not configured")
	}
	connection, err := s.Connections.GetByProjectID(ctx, organizationID, projectID)
	if err != nil {
		return Connection{}, nil, err
	}
	accountID := ""
	if connection.GSCAccountID != nil {
		accountID = *connection.GSCAccountID
	}
	return connection, s.NewClient(connection.ConnectedByUserID, accountID), nil
}

// GetPerformance returns totals, comparisons, striking-distance rows and country options.
func (s *Service) GetPerformance(ctx context.Context, organizationID, projectID string, input PerformanceInput) (map[string]any, error) {
	connection, client, err := s.client(ctx, organizationID, projectID)
	if err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			return map[string]any{"connected": false}, nil
		}
		return nil, err
	}
	now := s.now()
	window, err := resolveDateRange(input, now)
	if err != nil {
		return nil, err
	}
	previous, err := previousPeriod(window.StartDate, window.EndDate)
	if err != nil {
		return nil, err
	}
	currentInput := input
	currentInput.StartDate = window.StartDate
	currentInput.EndDate = window.EndDate
	currentInput.DateRange = ""
	nonCountry := withoutCountry(currentInput)
	filtersRequest, err := BuildSearchAnalyticsRequest(currentInput, []string{"date"}, dailyRowLimit, 0, now)
	if err != nil {
		return nil, err
	}
	previousInput := currentInput
	previousInput.StartDate = previous.StartDate
	previousInput.EndDate = previous.EndDate
	previousInput.DateRange = ""
	previousRequest, err := BuildSearchAnalyticsRequest(previousInput, []string{"date"}, dailyRowLimit, 0, now)
	if err != nil {
		return nil, err
	}
	queryPages, err := BuildSearchAnalyticsRequest(currentInput, []string{"query", "page"}, strikingDistanceLimit, 0, now)
	if err != nil {
		return nil, err
	}
	countryRequest, err := BuildSearchAnalyticsRequest(nonCountry, []string{"country"}, countryRowLimit, 0, now)
	if err != nil {
		return nil, err
	}
	rowsByRequest, err := s.fetchFour(ctx, client, connection.SiteURL, filtersRequest, previousRequest, queryPages, countryRequest)
	if err != nil {
		mapped := mapProviderError(err)
		if expectedGrantFailure(mapped) {
			return map[string]any{"connected": false}, nil
		}
		return nil, mapped
	}
	currentRows, previousRows, queryRows, countryRows := rowsByRequest[0], rowsByRequest[1], rowsByRequest[2], rowsByRequest[3]
	return map[string]any{"connected": true, "range": map[string]string{"startDate": window.StartDate, "endDate": window.EndDate, "prevStartDate": previous.StartDate, "prevEndDate": previous.EndDate}, "totals": sumSearchTotals(currentRows), "prevTotals": sumSearchTotals(previousRows), "strikingDistance": buildStrikingDistanceRows(queryRows, 100), "countries": toDimensionRows(countryRows)}, nil
}

// GetMCPPerformance runs one bounded Search Analytics query for the MCP tool.
func (s *Service) GetMCPPerformance(ctx context.Context, organizationID, projectID string, input MCPPerformanceInput) (map[string]any, error) {
	if len(input.Dimensions) == 0 {
		input.Dimensions = []string{"query"}
	}
	if len(input.Dimensions) > 4 {
		return nil, validationError("dimensions must contain between 1 and 4 values.")
	}
	for _, dimension := range input.Dimensions {
		if !validDimension(dimension) {
			return nil, validationError("dimensions contains an unsupported value.")
		}
	}
	for _, dimension := range input.Dimensions {
		if dimension == "searchAppearance" && len(input.Dimensions) != 1 {
			return nil, validationError("searchAppearance must be the only dimension when used.")
		}
	}
	if len(input.Filters) > 5 {
		return nil, validationError("filters must contain at most 5 values.")
	}
	filters := append([]DimensionFilter(nil), input.Filters...)
	for i := range filters {
		if filters[i].Operator == "" {
			filters[i].Operator = "equals"
		}
	}
	for _, filter := range filters {
		if !validFilter(filter) {
			return nil, validationError("filters contain an unsupported dimension, operator, or expression.")
		}
	}
	if (input.StartDate == "") != (input.EndDate == "") {
		return nil, validationError("Provide both startDate and endDate, or neither (use dateRange instead).")
	}
	dateInput := PerformanceInput{StartDate: input.StartDate, EndDate: input.EndDate, DateRange: dateRange(input.DateRange)}
	window, err := resolveDateRange(dateInput, s.now())
	if err != nil {
		return nil, err
	}
	rowLimit := defaultSearchRowLimit
	if input.RowLimit != nil {
		rowLimit = *input.RowLimit
	}
	if rowLimit < 1 || rowLimit > maxSearchRowLimit {
		return nil, validationError("rowLimit must be an integer from 1 to 1000.")
	}
	startRow := 0
	if input.StartRow != nil {
		startRow = *input.StartRow
	}
	if startRow < 0 || startRow > 1_000_000 {
		return nil, validationError("startRow must be a non-negative integer.")
	}
	if input.MinPosition != nil && *input.MinPosition < 1 || input.MaxPosition != nil && *input.MaxPosition < 1 || input.MinImpressions != nil && *input.MinImpressions < 0 {
		return nil, validationError("Metric filters must use positions of at least 1 and non-negative impressions.")
	}
	typeValue := input.Type
	if typeValue == "" {
		typeValue = "web"
	}
	dataState := input.DataState
	if dataState == "" {
		dataState = "all"
	}
	request := SearchRequest{StartDate: window.StartDate, EndDate: window.EndDate, Dimensions: input.Dimensions, RowLimit: rowLimit, StartRow: startRow, Type: typeValue, DataState: dataState}
	if len(filters) > 0 {
		request.DimensionFilterGroups = []FilterGroup{{GroupType: "and", Filters: filters}}
	}
	if err := ValidateSearchRequest(request); err != nil {
		return nil, validationError("Search Console request is not valid.")
	}
	connection, client, err := s.client(ctx, organizationID, projectID)
	if err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			return map[string]any{"connected": false}, nil
		}
		return nil, err
	}
	fetchLimit := rowLimit
	metricFilter := input.MinPosition != nil || input.MaxPosition != nil || input.MinImpressions != nil
	if metricFilter {
		fetchLimit = maxSearchRowLimit
	}
	request.RowLimit = fetchLimit
	fetched, err := client.QuerySearchAnalytics(ctx, connection.SiteURL, request)
	if err != nil {
		return nil, mapProviderError(err)
	}
	kept := make([]SearchRow, 0, len(fetched))
	keptIndices := make([]int, 0, len(fetched))
	for index, row := range fetched {
		if input.MinImpressions != nil && row.Impressions < float64(*input.MinImpressions) {
			continue
		}
		if input.MinPosition != nil && (typeValue == "discover" || typeValue == "googleNews" || row.Position < *input.MinPosition) {
			continue
		}
		if input.MaxPosition != nil && (typeValue == "discover" || typeValue == "googleNews" || row.Position > *input.MaxPosition) {
			continue
		}
		kept = append(kept, row)
		keptIndices = append(keptIndices, index)
	}
	rows := kept
	if len(rows) > rowLimit {
		rows = rows[:rowLimit]
	}
	serialized := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{"keys": row.Keys, "clicks": row.Clicks, "impressions": row.Impressions, "ctr": mathRound(row.CTR, 4)}
		if typeValue != "discover" && typeValue != "googleNews" {
			item["position"] = mathRound(row.Position, 1)
		}
		serialized = append(serialized, item)
	}
	truncated := len(kept) > len(rows)
	hasMore := truncated || len(fetched) >= fetchLimit
	nextStartRow := startRow + len(fetched)
	if truncated {
		nextStartRow = startRow + keptIndices[len(rows)-1] + 1
	}
	return map[string]any{"connected": true, "siteUrl": connection.SiteURL, "startDate": window.StartDate, "endDate": window.EndDate, "dimensions": input.Dimensions, "rowCount": len(serialized), "rows": serialized, "hasMore": hasMore, "nextStartRow": nextStartRow}, nil
}

func mathRound(value float64, digits int) float64 {
	factor := math.Pow10(digits)
	return math.Round(value*factor) / factor
}

func (s *Service) fetchFour(ctx context.Context, client SearchClient, siteURL string, requests ...SearchRequest) ([][]SearchRow, error) {
	type answer struct {
		index int
		rows  []SearchRow
		err   error
	}
	ch := make(chan answer, len(requests))
	for i, req := range requests {
		go func(index int, request SearchRequest) {
			rows, err := client.QuerySearchAnalytics(ctx, siteURL, request)
			ch <- answer{index: index, rows: rows, err: err}
		}(i, req)
	}
	results := make([][]SearchRow, len(requests))
	for range requests {
		a := <-ch
		if a.err != nil {
			return nil, a.err
		}
		results[a.index] = a.rows
	}
	return results, nil
}

// GetTable returns one page of query or page performance rows.
func (s *Service) GetTable(ctx context.Context, organizationID, projectID string, input TableInput) (map[string]any, error) {
	if input.Dimension != "query" && input.Dimension != "page" {
		return nil, validationError("dimension must be query or page.")
	}
	page := 1
	if input.Page.Present {
		page = input.Page.Value
	}
	if page < 1 {
		return nil, validationError("page must be a positive integer.")
	}
	size := 25
	if input.PageSize.Present {
		size = input.PageSize.Value
	}
	if size != 25 && size != 50 && size != 100 {
		return nil, validationError("pageSize must be 25, 50, or 100.")
	}
	if page-1 > 1_000_000/size {
		return nil, validationError("page offset is too large.")
	}
	offset := (page - 1) * size
	connection, client, err := s.client(ctx, organizationID, projectID)
	if err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			return map[string]any{"connected": false}, nil
		}
		return nil, err
	}
	request, err := BuildSearchAnalyticsRequest(input.PerformanceInput, []string{input.Dimension}, size+1, offset, s.now())
	if err != nil {
		return nil, err
	}
	rows, err := client.QuerySearchAnalytics(ctx, connection.SiteURL, request)
	if err != nil {
		mapped := mapProviderError(err)
		if expectedGrantFailure(mapped) {
			return map[string]any{"connected": false}, nil
		}
		return nil, mapped
	}
	fetched := toDimensionRows(rows)
	hasNext := len(fetched) > size
	if hasNext {
		fetched = fetched[:size]
	}
	var total any
	if !hasNext && (offset == 0 || len(fetched) > 0) {
		total = offset + len(fetched)
	}
	return map[string]any{"connected": true, "dimension": input.Dimension, "page": page, "pageSize": size, "hasNextPage": hasNext, "totalCount": total, "rows": fetched}, nil
}

// ExportTable returns at most 1000 query or page rows.
func (s *Service) ExportTable(ctx context.Context, organizationID, projectID string, input ExportInput) (map[string]any, error) {
	if input.Dimension != "query" && input.Dimension != "page" {
		return nil, validationError("dimension must be query or page.")
	}
	connection, client, err := s.client(ctx, organizationID, projectID)
	if err != nil {
		return nil, err
	}
	request, err := BuildSearchAnalyticsRequest(input.PerformanceInput, []string{input.Dimension}, exportRowLimit, 0, s.now())
	if err != nil {
		return nil, err
	}
	rows, err := client.QuerySearchAnalytics(ctx, connection.SiteURL, request)
	if err != nil {
		return nil, mapProviderError(err)
	}
	return map[string]any{"dimension": input.Dimension, "rows": toDimensionRows(rows)}, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func withoutCountry(input PerformanceInput) PerformanceInput { input.Country = ""; return input }
func previousPeriod(startDate, endDate string) (dateWindow, error) {
	start, ok1 := parseDate(startDate)
	end, ok2 := parseDate(endDate)
	if !ok1 || !ok2 {
		return dateWindow{}, errors.New("invalid Search Console date range")
	}
	length := end.Sub(start)
	if length < 0 {
		length = 0
	}
	prevEnd := start.AddDate(0, 0, -1)
	prevStart := prevEnd.Add(-length)
	return dateWindow{StartDate: prevStart.Format("2006-01-02"), EndDate: prevEnd.Format("2006-01-02")}, nil
}

// SearchTotals contains aggregate Search Console metrics.
type SearchTotals struct {
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

// DimensionRow contains metrics for one query or page key.
type DimensionRow struct {
	Key         string  `json:"key"`
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

// StrikingDistanceRow contains the best-ranking page for one query.
type StrikingDistanceRow struct {
	Query       string  `json:"query"`
	Page        string  `json:"page"`
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	Position    float64 `json:"position"`
}

func sumSearchTotals(rows []SearchRow) SearchTotals {
	out := SearchTotals{}
	weighted := 0.0
	for _, row := range rows {
		out.Clicks += row.Clicks
		out.Impressions += row.Impressions
		weighted += row.Position * row.Impressions
	}
	if out.Impressions > 0 {
		out.CTR = out.Clicks / out.Impressions
		out.Position = weighted / out.Impressions
	}
	return out
}
func toDimensionRows(rows []SearchRow) []DimensionRow {
	out := []DimensionRow{}
	for _, row := range rows {
		if len(row.Keys) == 0 || row.Keys[0] == "" {
			continue
		}
		out = append(out, DimensionRow{Key: row.Keys[0], Clicks: row.Clicks, Impressions: row.Impressions, CTR: row.CTR, Position: row.Position})
	}
	return out
}
func buildStrikingDistanceRows(rows []SearchRow, limit int) []StrikingDistanceRow {
	byQuery := map[string]StrikingDistanceRow{}
	order := []string{}
	for _, row := range rows {
		if len(row.Keys) < 2 || row.Keys[0] == "" || row.Keys[1] == "" {
			continue
		}
		current, ok := byQuery[row.Keys[0]]
		if !ok {
			order = append(order, row.Keys[0])
		}
		if !ok || row.Position < current.Position || row.Position == current.Position && row.Impressions > current.Impressions {
			byQuery[row.Keys[0]] = StrikingDistanceRow{Query: row.Keys[0], Page: row.Keys[1], Clicks: row.Clicks, Impressions: row.Impressions, Position: row.Position}
		}
	}
	out := []StrikingDistanceRow{}
	for _, query := range order {
		row := byQuery[query]
		if row.Position >= 5 && row.Position <= 20 {
			out = append(out, row)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Impressions > out[j].Impressions })
	if limit < 0 {
		limit = 0
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func mapProviderError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, google.ErrGrantUnavailable) {
		return reportError{status: http.StatusUnauthorized, code: "gsc_reconnect_required", message: "The Search Console connection has expired or was revoked."}
	}
	var apiError google.APIError
	if errors.As(err, &apiError) {
		if apiError.Status == http.StatusUnauthorized || apiError.Status == http.StatusForbidden {
			return reportError{status: http.StatusUnauthorized, code: "gsc_reconnect_required", message: "The Search Console connection has expired or was revoked."}
		}
		if apiError.Status == http.StatusTooManyRequests {
			return reportError{status: http.StatusTooManyRequests, code: "gsc_rate_limited", message: "Search Console rate limit reached. Retry shortly."}
		}
		return reportError{status: http.StatusBadGateway, code: "gsc_upstream_unavailable", message: "Search Console reporting is temporarily unavailable."}
	}
	return err
}
func expectedGrantFailure(err error) bool {
	var typed reportError
	return errors.As(err, &typed) && (typed.code == "gsc_reconnect_required")
}

type reportError struct {
	status        int
	code, message string
}

func (e reportError) Error() string { return e.message }
