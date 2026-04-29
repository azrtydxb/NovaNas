package scheduler

import (
	"strings"
	"testing"
	"time"
)

func TestParseCron_Valid(t *testing.T) {
	cases := []struct {
		expr string
	}{
		{"* * * * *"},
		{"0 0 * * *"},
		{"*/5 * * * *"},
		{"0,15,30,45 * * * *"},
		{"0 9-17 * * 1-5"},
		{"0 0-23/2 * * *"},
	}
	for _, tc := range cases {
		if _, err := ParseCron(tc.expr); err != nil {
			t.Errorf("ParseCron(%q) failed: %v", tc.expr, err)
		}
	}
}

func TestParseCron_Invalid(t *testing.T) {
	cases := []string{
		"",
		"* * * *",     // 4 fields
		"* * * * * *", // 6 fields
		"60 * * * *",  // minute out of range
		"-1 * * * *",  // negative
		"* 24 * * *",  // hour out of range
		"* * 0 * *",   // dom too low
		"* * 32 * *",  // dom too high
		"* * * 0 *",   // month too low
		"* * * 13 *",  // month too high
		"* * * * 7",   // dow too high
		"*/0 * * * *", // step zero
		"5-2 * * * *", // reversed range
		"abc * * * *", // bad
		"1,, * * * *", // empty list item
	}
	for _, tc := range cases {
		if _, err := ParseCron(tc); err == nil {
			t.Errorf("ParseCron(%q) should have failed", tc)
		}
	}
}

func TestNextAfter_EveryMinute(t *testing.T) {
	c, _ := ParseCron("* * * * *")
	now := time.Date(2026, 4, 28, 12, 30, 15, 0, time.UTC)
	got := c.NextAfter(now, time.UTC)
	want := time.Date(2026, 4, 28, 12, 31, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNextAfter_DailyMidnight(t *testing.T) {
	c, _ := ParseCron("0 0 * * *")
	now := time.Date(2026, 4, 28, 12, 30, 0, 0, time.UTC)
	got := c.NextAfter(now, time.UTC)
	want := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNextAfter_Feb29Leap(t *testing.T) {
	// "0 0 29 2 *" — Feb 29: only fires in leap years.
	c, _ := ParseCron("0 0 29 2 *")
	// Starting in a non-leap year, next Feb 29 is 2028.
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	got := c.NextAfter(now, time.UTC)
	want := time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNextAfter_EndOfMonth(t *testing.T) {
	// "0 0 31 * *" only fires in 31-day months.
	c, _ := ParseCron("0 0 31 * *")
	now := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	got := c.NextAfter(now, time.UTC)
	want := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNextAfter_DOMorDOW(t *testing.T) {
	// Vixie cron: when both dom and dow are restricted, fire if either matches.
	// "0 0 1 * 0" → 1st of month OR Sunday.
	c, _ := ParseCron("0 0 1 * 0")
	// 2026-04-30 (Thursday) → next match is 2026-05-01 (Friday) by DOM.
	now := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	got := c.NextAfter(now, time.UTC)
	want := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNextAfter_DOWOnly(t *testing.T) {
	// Sunday 03:00.
	c, _ := ParseCron("0 3 * * 0")
	// 2026-04-28 is a Tuesday → next Sunday is 2026-05-03.
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	got := c.NextAfter(now, time.UTC)
	want := time.Date(2026, 5, 3, 3, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestShouldFireBetween(t *testing.T) {
	c, _ := ParseCron("0 0 * * *") // daily midnight
	loc := time.UTC
	prev := time.Date(2026, 4, 27, 0, 0, 0, 0, loc) // last fired yesterday midnight
	now := time.Date(2026, 4, 28, 0, 0, 30, 0, loc) // 30s after today's midnight
	if !c.ShouldFireBetween(prev, now, loc) {
		t.Errorf("expected fire between %v and %v", prev, now)
	}
	// 23:59 — not yet fired today.
	now2 := time.Date(2026, 4, 27, 23, 59, 0, 0, loc)
	if c.ShouldFireBetween(prev, now2, loc) {
		t.Errorf("did not expect fire at %v", now2)
	}
}

func TestShouldFireBetween_FirstRun(t *testing.T) {
	c, _ := ParseCron("0 0 * * *")
	// prev zero — only fires if "now" itself matches.
	loc := time.UTC
	if !c.ShouldFireBetween(time.Time{}, time.Date(2026, 4, 28, 0, 0, 30, 0, loc), loc) {
		t.Error("first run at midnight should fire")
	}
	if c.ShouldFireBetween(time.Time{}, time.Date(2026, 4, 28, 12, 30, 0, 0, loc), loc) {
		t.Error("first run at 12:30 should not fire")
	}
}

func TestShouldFireBetween_NotAfter(t *testing.T) {
	c, _ := ParseCron("* * * * *")
	loc := time.UTC
	t0 := time.Date(2026, 4, 28, 12, 0, 0, 0, loc)
	if c.ShouldFireBetween(t0, t0, loc) {
		t.Error("now == prev should not fire")
	}
}

func TestParseCron_StepWildcard(t *testing.T) {
	c, err := ParseCron("*/15 * * * *")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{0, 15, 30, 45}
	if len(c.Minute.values) != len(want) {
		t.Fatalf("got %v, want %v", c.Minute.values, want)
	}
	for i, v := range want {
		if c.Minute.values[i] != v {
			t.Errorf("got %v, want %v", c.Minute.values, want)
			break
		}
	}
}

func TestParseCron_RangeStep(t *testing.T) {
	c, err := ParseCron("0 0-23/6 * * *")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{0, 6, 12, 18}
	if len(c.Hour.values) != len(want) {
		t.Fatalf("got %v, want %v", c.Hour.values, want)
	}
	for i, v := range want {
		if c.Hour.values[i] != v {
			t.Errorf("got %v, want %v", c.Hour.values, want)
			break
		}
	}
}

func TestParseCron_DSTSpringForward(t *testing.T) {
	// US Eastern DST 2026 starts 2026-03-08 02:00 → jumps to 03:00.
	// "30 2 * * *" — 02:30 doesn't exist on that day.
	// We're a wall-clock cron: NextAfter() steps minute-by-minute in the
	// given location, and time.Date in NY normalizes 02:30 to 03:30 on
	// the spring-forward day. So we should not infinite-loop and we
	// should fire on a real instant.
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata")
	}
	c, _ := ParseCron("30 2 * * *")
	// Day before DST.
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, loc)
	got := c.NextAfter(now, loc)
	if got.IsZero() {
		t.Fatal("NextAfter returned zero time")
	}
	// Should be "the next 02:30 wall clock" — on 2026-03-07 evening
	// before DST jump that's the 8th, but 02:30 is skipped → time.Date
	// normalizes to 03:30 EDT. Either way: must be after `now`.
	if !got.After(now) {
		t.Errorf("NextAfter returned non-progressing time %v", got)
	}
}

func TestParseCron_HumanError(t *testing.T) {
	_, err := ParseCron("* * 32 * *")
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range err, got %v", err)
	}
}
