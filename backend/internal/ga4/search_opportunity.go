package ga4

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/google"
	"github.com/toufiqqureshi/seomarine/backend/internal/gsc"
	"golang.org/x/net/idna"
)

const searchConsoleTimeZone = "America/Los_Angeles"

// SearchOpportunityInput bounds the combined Search Console and Analytics join.
type SearchOpportunityInput struct {
	StartDate string `json:"startDate,omitempty"`
	EndDate   string `json:"endDate,omitempty"`
	Limit     *int   `json:"limit,omitempty"`
}

type opportunityCandidate struct {
	page, joinStatus                   string
	normalizedPage                     any
	clicks, impressions, ctr, position float64
	analytics                          map[string]any
	score                              any
	components                         any
	originalIndex                      int
}

// GetSearchOpportunities joins high-impression, striking-distance GSC pages
// with organic landing-page Analytics data and ranks the joined results.
func (s *Service) GetSearchOpportunities(ctx context.Context, searchConsole *gsc.Service, organizationID, projectID string, input SearchOpportunityInput) (map[string]any, error) {
	limit := 50
	if input.Limit != nil {
		limit = *input.Limit
	}
	if limit < 1 || limit > 100 {
		return nil, invalid("limit must be an integer from 1 to 100.")
	}
	if s == nil || s.Connections == nil {
		return nil, reportErr("ga4_unavailable", "Google Analytics reporting is not set up on this server.")
	}
	if searchConsole == nil {
		return nil, reportErr("gsc_unavailable", "Search Console reporting is not set up on this server.")
	}
	connection, err := s.Connections.GetByProjectID(ctx, projectID)
	if err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			return nil, reportErr("ga4_not_connected", "Google Analytics is not connected for this project.")
		}
		return nil, fmt.Errorf("load GA4 connection: %w", err)
	}
	dates, err := resolveOpportunityDates(input, connection.PropertyTimeZone, s.now())
	if err != nil {
		return nil, err
	}
	gscConnection, gscRows, err := searchConsole.GetPageRows(ctx, organizationID, projectID, dates.StartDate, dates.EndDate)
	if err != nil {
		return nil, mapOpportunityGSCErr(err)
	}
	pageLimit := 1000
	ga4Report, err := s.RunReport(ctx, projectID, ReportInput{Kind: LandingPages, StartDate: dates.StartDate, EndDate: dates.EndDate, Limit: &pageLimit, Channel: "organic_search"})
	if err != nil {
		return nil, err
	}
	ga4Rows := mapRows(ga4Report["rows"])
	ga4ByPage := make(map[string]map[string]any, len(ga4Rows))
	invalidGA4Rows := 0
	for _, row := range ga4Rows {
		host, _ := row["hostName"].(string)
		landing, _ := row["landingPage"].(string)
		key, ok := normalizePageKey(host + landing)
		if !ok {
			invalidGA4Rows++
			continue
		}
		ga4ByPage[key] = row
	}
	candidates := make([]opportunityCandidate, 0, len(gscRows))
	for index, row := range gscRows {
		if row.Position < 4 || row.Position > 20 {
			continue
		}
		page := ""
		if len(row.Keys) > 0 {
			page = row.Keys[0]
		}
		key, valid := normalizePageKey(page)
		analytics := map[string]any(nil)
		if valid {
			analytics = ga4ByPage[key]
		}
		joinStatus := "gsc_only"
		if analytics != nil {
			joinStatus = "joined"
		}
		candidates = append(candidates, opportunityCandidate{page: page, normalizedPage: nullableKey(key, valid), clicks: row.Clicks,
			impressions: row.Impressions, ctr: row.CTR, position: row.Position, joinStatus: joinStatus, analytics: analytics,
			originalIndex: index})
	}
	joined := make([]*opportunityCandidate, 0, len(candidates))
	allEventsZero := true
	for i := range candidates {
		candidate := &candidates[i]
		if candidate.analytics == nil {
			continue
		}
		joined = append(joined, candidate)
		if numberField(candidate.analytics, "keyEvents") != 0 {
			allEventsZero = false
		}
	}
	engagementFallback := len(joined) > 0 && allEventsZero
	demandRanks := percentileRanks(opportunityValues(joined, func(c *opportunityCandidate) float64 { return math.Log1p(c.impressions) }))
	valueRanks := percentileRanks(opportunityValues(joined, func(c *opportunityCandidate) float64 {
		metric := "sessionKeyEventRate"
		if engagementFallback {
			metric = "engagementRate"
		}
		return numberField(c.analytics, metric)
	}))
	reachabilityRanks := percentileRanks(opportunityValues(joined, func(c *opportunityCandidate) float64 { return 20 - c.position }))
	for i, candidate := range joined {
		components := map[string]float64{"demand": roundScore(demandRanks[i]), "businessValue": roundScore(valueRanks[i]), "reachability": roundScore(reachabilityRanks[i])}
		candidate.components = components
		candidate.score = math.Round(100 * (.5*components["demand"] + .3*components["businessValue"] + .2*components["reachability"]))
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		as, aJoined := a.score.(float64)
		bs, bJoined := b.score.(float64)
		if aJoined != bJoined {
			return aJoined
		}
		if aJoined && as != bs {
			return as > bs
		}
		if a.impressions != b.impressions {
			return a.impressions > b.impressions
		}
		return a.originalIndex < b.originalIndex
	})
	matchedRows := len(joined)
	returned := candidates
	if len(returned) > limit {
		returned = returned[:limit]
	}
	rows := make([]map[string]any, len(returned))
	for i, c := range returned {
		var analytics any
		if c.analytics != nil {
			analytics = map[string]any{"sessions": numberField(c.analytics, "sessions"), "activeUsers": numberField(c.analytics, "activeUsers"),
				"engagedSessions": numberField(c.analytics, "engagedSessions"), "engagementRate": numberField(c.analytics, "engagementRate"),
				"keyEvents": numberField(c.analytics, "keyEvents"), "sessionKeyEventRate": numberField(c.analytics, "sessionKeyEventRate"),
				"transactions": numberField(c.analytics, "transactions"), "purchaseRevenue": numberOrNil(c.analytics["purchaseRevenue"])}
		}
		rows[i] = map[string]any{"page": c.page, "normalizedPage": c.normalizedPage, "clicks": c.clicks, "impressions": c.impressions,
			"ctr": c.ctr, "position": c.position, "joinStatus": c.joinStatus, "ga4": analytics, "score": c.score, "scoreComponents": c.components}
	}
	metadata, _ := ga4Report["reportMetadata"].(reportMetadata)
	timeZone := connection.PropertyTimeZone
	warnings, _ := ga4Report["warnings"].([]string)
	if timeZone != searchConsoleTimeZone {
		warnings = append(append([]string{}, warnings...), "source_time_zones_differ")
	}
	return map[string]any{
		"status":   "ok",
		"source":   map[string]any{"searchConsoleSiteUrl": gscConnection.SiteURL, "googleAnalyticsPropertyId": connection.PropertyID, "googleAnalyticsPropertyDisplayName": connection.PropertyDisplayName},
		"request":  map[string]any{"dateRange": dates, "limit": limit, "searchConsoleTimeZone": searchConsoleTimeZone, "googleAnalyticsTimeZone": timeZone},
		"rowCount": len(rows), "totalCandidateRows": len(candidates), "rows": rows,
		"scoring":   map[string]any{"formula": "round(100 * (0.5 * demand + 0.3 * businessValue + 0.2 * reachability))", "businessValueMetric": map[bool]string{true: "engagementRate", false: "sessionKeyEventRate"}[engagementFallback], "engagementFallback": engagementFallback, "scoreDataLimited": metadata.HasLimitedData},
		"coverage":  map[string]any{"gscRowsConsidered": len(gscRows), "ga4RowsConsidered": len(ga4Rows), "matchedRows": matchedRows, "unmatchedGscRows": len(candidates) - matchedRows, "unmatchedGa4Rows": max(len(ga4ByPage)-matchedRows, 0) + invalidGA4Rows},
		"truncated": map[string]any{"gsc": len(gscRows) >= 1000, "ga4": intField(ga4Report["totalRowCount"]) > len(ga4Rows), "candidates": len(rows) < len(candidates)},
		"warnings":  warnings, "reportMetadata": ga4Report["reportMetadata"], "quota": ga4Report["quota"],
	}, nil
}

func resolveOpportunityDates(input SearchOpportunityInput, timeZone string, now time.Time) (DateRange, error) {
	if input.StartDate != "" || input.EndDate != "" {
		resolved, err := resolveDates(ReportInput{StartDate: input.StartDate, EndDate: input.EndDate}, timeZone, now)
		return resolved.Resolved, err
	}
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		return DateRange{}, invalid("The property's time zone is invalid.")
	}
	// Compute the civil day in the property's time zone.
	year, month, day := now.In(location).Date()
	end := time.Date(year, month, day, 0, 0, 0, 0, location).AddDate(0, 0, -3)
	return DateRange{StartDate: end.AddDate(0, 0, -27).Format("2006-01-02"), EndDate: end.Format("2006-01-02")}, nil
}

func mapOpportunityGSCErr(err error) error {
	if errors.Is(err, gsc.ErrConnectionNotFound) {
		return reportErr("gsc_not_connected", "Search Console is not connected for this project.")
	}
	if errors.Is(err, google.ErrGrantUnavailable) {
		return reportErr("gsc_reconnect_required", "The Search Console connection has expired or was revoked.")
	}
	var apiErr google.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case 401, 403:
			return reportErr("gsc_reconnect_required", "The Search Console connection has expired or was revoked.")
		case 429:
			return reportErr("gsc_rate_limited", "Search Console rate limit reached. Retry shortly.")
		default:
			return reportErr("gsc_upstream_unavailable", "Search Console is temporarily unavailable.")
		}
	}
	return fmt.Errorf("read Search Console opportunity rows: %w", err)
}

func normalizePageKey(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "(not set)" {
		return "", false
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	host, err := idna.Lookup.ToASCII(strings.ToLower(parsed.Hostname()))
	if err != nil || host == "" {
		return "", false
	}
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return host + path, true
}

func nullableKey(value string, valid bool) any {
	if !valid {
		return nil
	}
	return value
}

func mapRows(value any) []map[string]any {
	rows, ok := value.([]map[string]any)
	if !ok || rows == nil {
		return []map[string]any{}
	}
	return rows
}

func numberField(row map[string]any, name string) float64 {
	value, ok := row[name].(float64)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func numberOrNil(value any) any {
	if number, ok := value.(float64); ok && !math.IsNaN(number) && !math.IsInf(number, 0) {
		return number
	}
	return nil
}

func intField(value any) int {
	number, _ := value.(int)
	return number
}

func opportunityValues(candidates []*opportunityCandidate, value func(*opportunityCandidate) float64) []float64 {
	values := make([]float64, len(candidates))
	for i, candidate := range candidates {
		values[i] = value(candidate)
	}
	return values
}

func percentileRanks(values []float64) []float64 {
	if len(values) == 0 {
		return []float64{}
	}
	if len(values) == 1 {
		return []float64{1}
	}
	ranks := make([]float64, len(values))
	for i, value := range values {
		for _, candidate := range values {
			if candidate < value {
				ranks[i]++
			}
		}
		ranks[i] /= float64(len(values) - 1)
	}
	return ranks
}

func roundScore(value float64) float64 { return math.Round(value*10000) / 10000 }
