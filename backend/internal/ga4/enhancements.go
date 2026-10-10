package ga4

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func comparisonSupported(i ReportInput) bool {
	switch i.Kind {
	case KeyEvents:
		return i.effectiveBreakdown() == "event"
	case TrafficAcquisition:
		return i.effectiveBreakdown() == "channel_group"
	case AudienceBreakdown:
		return i.effectiveBreakdown() == "device" || i.effectiveBreakdown() == "new_vs_returning"
	default:
		return false
	}
}
func metric(row map[string]any, name string) float64 { v, _ := row[name].(float64); return v }
func rowKey(row map[string]any, dims []string) string {
	var parts []string
	for _, d := range dims {
		parts = append(parts, fmt.Sprint(row[d]))
	}
	return strings.Join(parts, "\x00")
}
func makeComparison(current, previous normalizedReport, rangeDates DateRange, d reportDefinition) map[string]any {
	cur, prev := map[string]map[string]any{}, map[string]map[string]any{}
	for _, r := range current.Rows {
		cur[rowKey(r, d.dimensions)] = r
	}
	for _, r := range previous.Rows {
		prev[rowKey(r, d.dimensions)] = r
	}
	keys := []string{}
	seen := map[string]bool{}
	for _, row := range current.Rows {
		k := rowKey(row, d.dimensions)
		if !seen[k] {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	for _, row := range previous.Rows {
		k := rowKey(row, d.dimensions)
		if !seen[k] {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	rows := make([]any, 0, len(keys))
	for _, k := range keys {
		c, p := cur[k], prev[k]
		dimensions := map[string]any{}
		metrics := map[string]any{}
		for _, d := range d.dimensions {
			if c[d] != nil {
				dimensions[d] = c[d]
			} else {
				dimensions[d] = p[d]
			}
		}
		for _, m := range d.metrics {
			cv, cok := c[m].(float64)
			pv, pok := p[m].(float64)
			var absolute, percent any
			if cok && pok {
				absolute = cv - pv
				if pv != 0 {
					percent = (cv - pv) / pv
				}
			}
			metrics[m] = map[string]any{"current": nullable(cv, cok), "previous": nullable(pv, pok), "absoluteChange": absolute, "percentChange": percent}
		}
		rows = append(rows, map[string]any{"dimensions": dimensions, "metrics": metrics})
	}
	return map[string]any{"previousDateRange": rangeDates, "dimensions": d.dimensions, "metrics": d.metrics, "rows": rows,
		"coverage":       map[string]any{"complete": len(current.Rows) == current.TotalRowCount && len(previous.Rows) == previous.TotalRowCount, "current": map[string]int{"fetchedRowCount": len(current.Rows), "totalRowCount": current.TotalRowCount}, "previous": map[string]int{"fetchedRowCount": len(previous.Rows), "totalRowCount": previous.TotalRowCount}},
		"reportMetadata": map[string]any{"hasLimitedData": current.Metadata.HasLimitedData || previous.Metadata.HasLimitedData, "current": current.Metadata, "previous": previous.Metadata}, "quota": previous.Quota}
}
func nullable(v float64, ok bool) any {
	if !ok {
		return nil
	}
	return v
}

func attributionDiagnostics(r normalizedReport) map[string]any {
	complete := len(r.Rows) == r.TotalRowCount
	coverage := map[string]any{"complete": complete, "limitedData": r.Metadata.HasLimitedData, "fetchedRowCount": len(r.Rows), "totalRowCount": r.TotalRowCount}
	diagnostics := []any{}
	if !complete || r.Metadata.HasLimitedData {
		return map[string]any{"diagnostics": diagnostics, "diagnosticCoverage": coverage}
	}
	total, notSet, internal := 0.0, 0.0, 0.0
	internalSources := []any{}
	groups := map[string][]string{}
	for _, row := range r.Rows {
		sessions := metric(row, "sessions")
		total += sessions
		source, _ := row["sessionSourceMedium"].(string)
		if source == "(not set)" {
			notSet += sessions
		}
		if isInternalSource(source) {
			internal += sessions
			internalSources = append(internalSources, source)
		}
		key := strings.ToLower(source)
		seen := false
		for _, v := range groups[key] {
			if v == source {
				seen = true
			}
		}
		if !seen && source != "" {
			groups[key] = append(groups[key], source)
		}
	}
	if total > 0 && notSet/total >= .05 {
		diagnostics = append(diagnostics, map[string]any{"code": "attribution_not_set_share_high", "severity": "warning", "message": "A notable share of sessions has no source/medium attribution.", "evidence": map[string]any{"sessions": notSet, "totalSessions": total, "share": notSet / total}, "threshold": map[string]float64{"share": .05}})
	}
	if internal > 0 {
		diagnostics = append(diagnostics, map[string]any{"code": "internal_referral_traffic_detected", "severity": "warning", "message": "Local or private-network referral sources appear in acquisition data.", "evidence": map[string]any{"sessions": internal, "sources": internalSources}, "threshold": map[string]int{"sessions": 0}})
	}
	variants := [][]string{}
	for _, vs := range groups {
		if len(vs) > 1 {
			variants = append(variants, vs)
		}
	}
	if len(variants) > 0 {
		diagnostics = append(diagnostics, map[string]any{"code": "source_medium_case_variants_detected", "severity": "info", "message": "Source/medium values differ only by letter casing.", "evidence": map[string]any{"variantGroups": variants}, "threshold": map[string]int{"variantGroups": 0}})
	}
	return map[string]any{"diagnostics": diagnostics, "diagnosticCoverage": coverage}
}
func isInternalSource(value string) bool {
	source := strings.ToLower(strings.Split(value, " / ")[0])
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		if u, e := url.Parse(source); e == nil {
			source = u.Hostname()
		}
	}
	if source == "::1" || strings.HasPrefix(source, "[::1]") {
		return true
	}
	if host, _, ok := strings.Cut(source, ":"); ok && !strings.Contains(host, "]") {
		source = host
	}
	if source == "localhost" || strings.HasPrefix(source, "127.") {
		return true
	}
	ip := net.ParseIP(source)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}
func activityStatus(r normalizedReport, detected bool) string {
	if r.Metadata.HasLimitedData || len(r.Rows) != r.TotalRowCount {
		return "unknown"
	}
	if detected {
		return "detected"
	}
	return "none"
}
func activitySummary(r normalizedReport, input ReportInput, dateRange DateRange) map[string]any {
	if input.Kind == EcommercePerformance {
		breakdown := input.effectiveBreakdown()
		metrics := []string{"itemsViewed", "itemsAddedToCart", "itemsPurchased", "itemRevenue"}
		if breakdown == "landing_page" {
			metrics = []string{"transactions", "purchaseRevenue"}
		}
		totals := map[string]float64{}
		detected := false
		for _, m := range metrics {
			for _, row := range r.Rows {
				totals[m] += metric(row, m)
			}
			detected = detected || totals[m] > 0
		}
		status := activityStatus(r, detected)
		var reason any
		switch status {
		case "none":
			reason = "No matching ecommerce activity was reported for this period and channel."
		case "unknown":
			reason = "The fetched report is incomplete or limited, so ecommerce activity cannot be determined."
		}
		return map[string]any{"status": status, "dateRange": dateRange, "channel": input.Channel, "breakdown": breakdown, "evidence": totals, "reason": reason}
	}
	events := 0.0
	for _, row := range r.Rows {
		events += metric(row, "eventCount")
	}
	status := activityStatus(r, events > 0)
	var reason any
	switch status {
	case "none":
		reason = "No measured site-search terms were reported for this period."
	case "unknown":
		reason = "The fetched report is incomplete or limited, so site-search activity cannot be determined."
	}
	return map[string]any{"status": status, "dateRange": dateRange, "searchTermCount": r.TotalRowCount, "searchEventCount": events, "reason": reason}
}

func reportEnhancements(r normalizedReport, input ReportInput, dateRange DateRange) map[string]any {
	if input.Kind == TrafficAcquisition && input.effectiveBreakdown() == "source_medium" {
		return attributionDiagnostics(r)
	}
	if input.Kind == EcommercePerformance {
		activity := activitySummary(r, input, dateRange)
		diagnostics := []any{}
		if activity["status"] == "none" {
			diagnostics = append(diagnostics, map[string]any{"code": "no_ecommerce_activity", "severity": "info", "message": activity["reason"], "evidence": activity["evidence"], "threshold": map[string]int{"matchingActivity": 0}})
		}
		return map[string]any{"diagnostics": diagnostics, "ecommerceActivity": activity}
	}
	if input.Kind == SiteSearch {
		activity := activitySummary(r, input, dateRange)
		diagnostics := []any{}
		if activity["status"] == "none" {
			diagnostics = append(diagnostics, map[string]any{"code": "no_site_search_activity", "severity": "info", "message": activity["reason"], "evidence": map[string]any{"searchTermCount": activity["searchTermCount"], "searchEventCount": activity["searchEventCount"]}, "threshold": map[string]int{"searchEvents": 0}})
		}
		return map[string]any{"diagnostics": diagnostics, "siteSearchActivity": activity}
	}
	return map[string]any{"diagnostics": []any{}}
}
