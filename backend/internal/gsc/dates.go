package gsc

import (
	"errors"
	"time"
)

// DateRange is an inclusive UTC interval sent to Search Console.
type DateRange struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

func subtractMonths(date time.Time, months int) time.Time {
	year, month, day := date.Date()
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).AddDate(0, -months, 0)
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC)
}

// ResolveDateRange applies GSC's three-day data lag and 16-month floor.
func ResolveDateRange(now time.Time, preset, explicitStart, explicitEnd string) (DateRange, error) {
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	floor := subtractMonths(today, 16)
	format := func(date time.Time) string { return date.Format("2006-01-02") }
	if explicitStart != "" || explicitEnd != "" {
		if !validDate(explicitStart) || !validDate(explicitEnd) || explicitStart > explicitEnd || explicitEnd > format(today) {
			return DateRange{}, errors.New("invalid search date range")
		}
		start, _ := time.Parse("2006-01-02", explicitStart)
		if start.Before(floor) {
			start = floor
		}
		return DateRange{StartDate: format(start), EndDate: explicitEnd}, nil
	}
	end := today.AddDate(0, 0, -3)
	var start time.Time
	switch preset {
	case "", "last_28_days":
		start = end.AddDate(0, 0, -28)
	case "last_7_days":
		start = end.AddDate(0, 0, -7)
	case "last_3_months":
		start = subtractMonths(end, 3)
	case "last_6_months":
		start = subtractMonths(end, 6)
	case "last_12_months":
		start = subtractMonths(end, 12)
	case "last_16_months":
		start = subtractMonths(end, 16)
	default:
		return DateRange{}, errors.New("invalid search date preset")
	}
	if start.Before(floor) {
		start = floor
	}
	return DateRange{StartDate: format(start), EndDate: format(end)}, nil
}

// PreviousPeriod returns the adjacent interval of the same inclusive length.
func PreviousPeriod(current DateRange) (DateRange, error) {
	if !validDate(current.StartDate) || !validDate(current.EndDate) || current.StartDate > current.EndDate {
		return DateRange{}, errors.New("invalid search date range")
	}
	start, _ := time.Parse("2006-01-02", current.StartDate)
	end, _ := time.Parse("2006-01-02", current.EndDate)
	length := int(end.Sub(start).Hours() / 24)
	prevEnd := start.AddDate(0, 0, -1)
	prevStart := prevEnd.AddDate(0, 0, -length)
	return DateRange{StartDate: prevStart.Format("2006-01-02"), EndDate: prevEnd.Format("2006-01-02")}, nil
}
