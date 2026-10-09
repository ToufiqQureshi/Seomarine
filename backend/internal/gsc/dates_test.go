package gsc

import (
	"testing"
	"time"
)

func TestResolveDateRange(t *testing.T) {
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ preset, start, end string }{
		{"last_7_days", "2026-03-21", "2026-03-28"},
		{"last_3_months", "2025-12-28", "2026-03-28"},
		{"last_16_months", "2024-11-30", "2026-03-28"},
	} {
		got, err := ResolveDateRange(now, tc.preset, "", "")
		if err != nil || got.StartDate != tc.start || got.EndDate != tc.end {
			t.Errorf("%s: %+v, %v; want %s..%s", tc.preset, got, err, tc.start, tc.end)
		}
	}
	if _, err := ResolveDateRange(now, "", "2026-04-01", "2026-04-02"); err == nil {
		t.Fatal("future date accepted")
	}
}

func TestPreviousPeriod(t *testing.T) {
	got, err := PreviousPeriod(DateRange{StartDate: "2026-02-01", EndDate: "2026-02-28"})
	if err != nil || got.StartDate != "2026-01-04" || got.EndDate != "2026-01-31" {
		t.Fatalf("previous = %+v, %v", got, err)
	}
}
