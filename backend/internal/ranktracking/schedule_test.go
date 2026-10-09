package ranktracking

import (
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC) // a Friday

func newScheduler() Scheduler {
	return Scheduler{Now: func() time.Time { return testNow }, Rand: func(int) int { return 0 }}
}

func ptr[T any](v T) *T { return &v }

func utc(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, time.UTC)
}

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func TestNext(t *testing.T) {
	kolkata := mustZone(t, "Asia/Kolkata")
	tests := []struct {
		name     string
		interval Interval
		previous *time.Time
		chosen   *ScheduleTime
		want     time.Time
	}{
		{"daily advances from a stale anchor without drift", Daily, ptr(utc(2026, 10, 7, 5, 30)), nil, utc(2026, 10, 10, 5, 30)},
		{"weekly keeps its weekday after a late run", Weekly, ptr(utc(2026, 10, 5, 5, 30)), nil, utc(2026, 10, 12, 5, 30)},
		{"anchor in the future still moves one step", Daily, ptr(utc(2026, 10, 20, 5, 30)), nil, utc(2026, 10, 21, 5, 30)},
		{"daily random default lands tomorrow in the 04-09 window", Daily, nil, nil, utc(2026, 10, 10, 4, 0)},
		{"weekly random default lands a week out", Weekly, nil, nil, utc(2026, 10, 16, 4, 0)},
		{"daily pick later today runs today", Daily, nil, &ScheduleTime{Hour: 11}, utc(2026, 10, 9, 11, 0)},
		{"daily pick already passed runs tomorrow", Daily, nil, &ScheduleTime{Hour: 9}, utc(2026, 10, 10, 9, 0)},
		{"daily pick exactly now runs tomorrow", Daily, nil, &ScheduleTime{Hour: 10}, utc(2026, 10, 10, 10, 0)},
		{"weekly pick on the next Monday", Weekly, nil, &ScheduleTime{Weekday: ptr(1), Hour: 9}, utc(2026, 10, 12, 9, 0)},
		{"weekly pick on today's weekday but passed waits a week", Weekly, nil, &ScheduleTime{Weekday: ptr(5), Hour: 9}, utc(2026, 10, 16, 9, 0)},
		{"weekly pick on today's weekday later today runs today", Weekly, nil, &ScheduleTime{Weekday: ptr(5), Hour: 18}, utc(2026, 10, 9, 18, 0)},
		{"IST early Monday is Sunday evening UTC", Weekly, nil, &ScheduleTime{Weekday: ptr(1), Hour: 2, Location: kolkata}, utc(2026, 10, 11, 20, 30)},
		{"monthly picks this month's end", Monthly, nil, &ScheduleTime{Hour: 6}, utc(2026, 10, 31, 6, 0)},
		{"monthly IST early morning lands the day before UTC month end", Monthly, nil, &ScheduleTime{Hour: 2, Location: kolkata}, utc(2026, 10, 30, 20, 30)},
		{"monthly keeps month-end across a 30-day month", Monthly, ptr(utc(2026, 10, 31, 6, 0)), nil, utc(2026, 11, 30, 6, 0)},
		{"monthly from a 30-day month anchor reaches the 31st", Monthly, ptr(utc(2026, 9, 30, 6, 0)), nil, utc(2026, 10, 31, 6, 0)},
		{"monthly from a stale anchor skips to the next future month end", Monthly, ptr(utc(2026, 2, 28, 6, 0)), nil, utc(2026, 10, 31, 6, 0)},
		{"monthly -1 anchor stays the day before month end", Monthly, ptr(utc(2026, 10, 30, 20, 30)), nil, utc(2026, 11, 29, 20, 30)},
		{"monthly +1 anchor on the 1st stays on the 1st", Monthly, ptr(utc(2026, 11, 1, 6, 0)), nil, utc(2026, 12, 1, 6, 0)},
		{"monthly random default is this month's end", Monthly, nil, nil, utc(2026, 10, 31, 4, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newScheduler().Next(tt.interval, tt.previous, tt.chosen)
			if !got.Equal(tt.want) {
				t.Fatalf("got %s, want %s", got.Format(time.RFC3339), tt.want.Format(time.RFC3339))
			}
		})
	}
}

func TestResolveNext(t *testing.T) {
	tests := []struct {
		name     string
		interval Interval
		chosen   *ScheduleTime
		wantNil  bool
		wantErr  bool
	}{
		{"manual has no anchor", Manual, nil, true, false},
		{"manual with a time is rejected", Manual, &ScheduleTime{Hour: 5}, false, true},
		{"weekly time without weekday is rejected", Weekly, &ScheduleTime{Hour: 5}, false, true},
		{"weekly without a time is fine", Weekly, nil, false, false},
		{"hour out of range", Daily, &ScheduleTime{Hour: 24}, false, true},
		{"negative minute", Daily, &ScheduleTime{Hour: 1, Minute: -1}, false, true},
		{"weekday out of range", Weekly, &ScheduleTime{Weekday: ptr(7), Hour: 1}, false, true},
		{"unknown interval", Interval("hourly"), nil, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newScheduler().ResolveNext(tt.interval, tt.chosen)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (got == nil) != tt.wantNil {
				t.Fatalf("got %v, wantNil %v", got, tt.wantNil)
			}
		})
	}
}
