package ranktracking

import (
	"errors"
	"fmt"
	"time"
)

// Interval is how often a config is checked automatically.
type Interval string

// Schedule intervals. Manual configs never run on their own.
const (
	Daily   Interval = "daily"
	Weekly  Interval = "weekly"
	Monthly Interval = "monthly"
	Manual  Interval = "manual"
)

// ScheduleTime is a run time the user picked in their own timezone.
// Weekday is 0 = Sunday and only matters for weekly schedules.
type ScheduleTime struct {
	Weekday  *int
	Hour     int
	Minute   int
	Location *time.Location // nil means UTC
}

// Validate rejects values the scheduler cannot honour.
func (t ScheduleTime) Validate() error {
	switch {
	case t.Hour < 0 || t.Hour > 23:
		return errors.New("hour must be 0-23")
	case t.Minute < 0 || t.Minute > 59:
		return errors.New("minute must be 0-59")
	case t.Weekday != nil && (*t.Weekday < 0 || *t.Weekday > 6):
		return errors.New("weekday must be 0-6")
	}
	return nil
}

// Scheduler computes anchors with an injectable clock and randomness, so tests
// are deterministic. Rand returns a value in [0, n).
type Scheduler struct {
	Now  func() time.Time
	Rand func(n int) int
}

// ResolveNext validates a schedule change and returns the first anchor.
// A nil result means manual (no anchor). It mirrors the legacy rules: a weekly
// time needs a weekday, and a time needs a non-manual interval.
func (s Scheduler) ResolveNext(interval Interval, chosen *ScheduleTime) (*time.Time, error) {
	if chosen != nil {
		if err := chosen.Validate(); err != nil {
			return nil, err
		}
	}
	switch interval {
	case Daily, Weekly, Monthly:
		if interval == Weekly && chosen != nil && chosen.Weekday == nil {
			return nil, errors.New("a weekly schedule time needs a weekday")
		}
		next := s.Next(interval, nil, chosen)
		return &next, nil
	case Manual:
		if chosen != nil {
			return nil, errors.New("a schedule time needs a daily, weekly, or monthly schedule")
		}
		return nil, nil
	}
	return nil, fmt.Errorf("unknown schedule interval %q", interval)
}

// Next returns the next check time for a scheduled config.
//
// With a previous anchor it advances in fixed steps until the result is in the
// future, so a delayed run does not drift the schedule. Without one, the first
// check lands on the chosen time, or on a random time between 04:00 and 09:59
// UTC to spread load across scheduler ticks.
func (s Scheduler) Next(interval Interval, previous *time.Time, chosen *ScheduleTime) time.Time {
	now := s.Now().UTC()
	var utc *utcTime
	if chosen != nil {
		u := toUTC(*chosen, now)
		utc = &u
	}
	if interval == Monthly {
		return s.nextMonthly(now, previous, utc)
	}

	step := 24 * time.Hour
	if interval == Weekly {
		step = 7 * 24 * time.Hour
	}
	if previous != nil {
		anchor := previous.UTC()
		steps := int64(1)
		if now.After(anchor) {
			steps += int64(now.Sub(anchor) / step)
		}
		return anchor.Add(time.Duration(steps) * step)
	}
	if utc != nil {
		next := time.Date(now.Year(), now.Month(), now.Day(), utc.hour, utc.minute, 0, 0, time.UTC)
		days := 1
		if interval == Weekly && utc.weekday != nil {
			next = next.AddDate(0, 0, (*utc.weekday-int(next.Weekday())+7)%7)
			days = 7
		}
		if !next.After(now) {
			next = next.AddDate(0, 0, days)
		}
		return next
	}
	next := now.AddDate(0, 0, int(step/(24*time.Hour)))
	return time.Date(next.Year(), next.Month(), next.Day(), 4+s.Rand(6), s.Rand(60), 0, 0, time.UTC)
}

func (s Scheduler) nextMonthly(now time.Time, previous *time.Time, utc *utcTime) time.Time {
	if previous != nil {
		anchor := previous.UTC()
		shift := monthlyDayShift(anchor)
		month := int(anchor.Month()) - 1
		if shift != 1 {
			month++ // a +1 anchor sits on the 1st, which belongs to the month before it
		}
		return monthEnd(anchor.Year(), month, shift, anchor.Hour(), anchor.Minute(), now)
	}
	hour, minute, shift := 4+s.Rand(6), s.Rand(60), 0
	if utc != nil {
		hour, minute, shift = utc.hour, utc.minute, utc.dayShift
	}
	// Start a month back: a +1 shift puts last month's run early in this one.
	return monthEnd(now.Year(), int(now.Month())-2, shift, hour, minute, now)
}

// monthEnd finds the first month-end anchor after now, starting at month
// (0-based, may be out of range). Month-end is the last day of the month in the
// user's zone, which can sit one day either side of the UTC month end.
func monthEnd(year, month, shift, hour, minute int, now time.Time) time.Time {
	for {
		// Day 0 of the following month is the last day of this one.
		t := time.Date(year, time.Month(month+2), shift, hour, minute, 0, 0, time.UTC)
		if t.After(now) {
			return t
		}
		month++
	}
}

// monthlyDayShift reads the shift back off a stored anchor: 1 on the 1st,
// -1 on the day before the last day, else 0. Months have at least 28 days, so
// these three never collide.
func monthlyDayShift(anchor time.Time) int {
	if anchor.Day() == 1 {
		return 1
	}
	lastDay := time.Date(anchor.Year(), anchor.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if anchor.Day() == lastDay-1 {
		return -1
	}
	return 0
}

type utcTime struct {
	weekday  *int
	hour     int
	minute   int
	dayShift int // days the chosen date moved when converting to UTC: -1, 0, +1
}

// toUTC shifts the chosen time to UTC using the zone's offset right now. The
// anchor is fixed in UTC afterwards, so a pick made just before a clock change
// is one hour off from its first run on.
func toUTC(t ScheduleTime, now time.Time) utcTime {
	if t.Location == nil || t.Location == time.UTC {
		return utcTime{weekday: t.Weekday, hour: t.Hour, minute: t.Minute}
	}
	_, offsetSeconds := now.In(t.Location).Zone()
	minutes := t.Hour*60 + t.Minute - offsetSeconds/60
	shift := floorDiv(minutes, 1440)
	minutes -= shift * 1440
	out := utcTime{hour: minutes / 60, minute: minutes % 60, dayShift: shift}
	if t.Weekday != nil {
		w := ((*t.Weekday+shift)%7 + 7) % 7
		out.weekday = &w
	}
	return out
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}
