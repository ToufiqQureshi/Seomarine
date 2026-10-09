package ga4

import (
	"fmt"
	"time"
)

func parseDate(value string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", value)
	return t, err == nil && t.Format("2006-01-02") == value
}
func resolveDates(input ReportInput, zone string, now time.Time) (dateResolution, error) {
	if (input.StartDate == "") != (input.EndDate == "") {
		return dateResolution{}, invalid("Provide both startDate and endDate, or neither.")
	}
	var requested *DateRange
	if input.StartDate != "" {
		start, ok1 := parseDate(input.StartDate)
		end, ok2 := parseDate(input.EndDate)
		if !ok1 || !ok2 || start.After(end) {
			return dateResolution{}, invalid("Dates must be valid YYYY-MM-DD values with startDate on or before endDate.")
		}
		requested = &DateRange{StartDate: input.StartDate, EndDate: input.EndDate}
	}
	localDay, err := dayInZone(now, zone)
	if err != nil {
		return dateResolution{}, fmt.Errorf("load GA4 property timezone: %w", err)
	}
	last, _ := parseDate(localDay)
	last = last.AddDate(0, 0, -1)
	end := last.Format("2006-01-02")
	start := last.AddDate(0, 0, -27).Format("2006-01-02")
	if requested != nil {
		start = requested.StartDate
		end = requested.EndDate
	}
	warnings := []string{}
	if end > last.Format("2006-01-02") {
		end = last.Format("2006-01-02")
		warnings = append(warnings, "end_date_clamped")
	}
	if start > end {
		return dateResolution{}, invalid("The resolved startDate is after the last complete Analytics day.")
	}
	return dateResolution{Requested: requested, Resolved: DateRange{StartDate: start, EndDate: end}, Warnings: warnings}, nil
}
func previousPeriod(r DateRange) (DateRange, error) {
	start, ok1 := parseDate(r.StartDate)
	end, ok2 := parseDate(r.EndDate)
	if !ok1 || !ok2 || start.After(end) {
		return DateRange{}, invalid("Invalid date range.")
	}
	days := int(end.Sub(start).Hours()/24) + 1
	previousEnd := start.AddDate(0, 0, -1)
	previousStart := previousEnd.AddDate(0, 0, -(days - 1))
	return DateRange{StartDate: previousStart.Format("2006-01-02"), EndDate: previousEnd.Format("2006-01-02")}, nil
}
