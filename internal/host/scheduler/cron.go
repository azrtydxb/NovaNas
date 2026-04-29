// Package scheduler implements cron-driven snapshot and replication
// orchestration. cron.go is a small, dependency-free 5-field cron parser.
//
// Format: "minute hour day-of-month month day-of-week"
//
//	minute       0-59
//	hour         0-23
//	day-of-month 1-31
//	month        1-12
//	day-of-week  0-6 (0 == Sunday)
//
// Each field accepts:
//   - any value
//     N           a single value
//     N,M,P       a list
//     A-B         a range (inclusive)
//     */S         step (every S, starting at field min)
//     A-B/S       stepped range
//
// Names (jan, mon, etc) are not supported.
package scheduler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CronField is a parsed single field. wildcard means "match all values in
// the field's natural range"; otherwise values is a sorted, deduplicated
// set of allowed integers.
type CronField struct {
	wildcard bool
	values   []int
}

// CronExpr is a parsed 5-field cron expression.
type CronExpr struct {
	Minute, Hour, Dom, Mon, Dow CronField
	raw                         string
}

// String returns the original expression.
func (c *CronExpr) String() string { return c.raw }

type fieldRange struct{ min, max int }

var (
	rngMinute = fieldRange{0, 59}
	rngHour   = fieldRange{0, 23}
	rngDom    = fieldRange{1, 31}
	rngMon    = fieldRange{1, 12}
	rngDow    = fieldRange{0, 6}
)

// ParseCron parses a 5-field cron expression. Whitespace between fields is
// any run of spaces or tabs.
func ParseCron(expr string) (*CronExpr, error) {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d in %q", len(parts), expr)
	}
	mi, err := parseField(parts[0], rngMinute)
	if err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}
	hr, err := parseField(parts[1], rngHour)
	if err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}
	dom, err := parseField(parts[2], rngDom)
	if err != nil {
		return nil, fmt.Errorf("cron dom: %w", err)
	}
	mon, err := parseField(parts[3], rngMon)
	if err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}
	dow, err := parseField(parts[4], rngDow)
	if err != nil {
		return nil, fmt.Errorf("cron dow: %w", err)
	}
	return &CronExpr{Minute: mi, Hour: hr, Dom: dom, Mon: mon, Dow: dow, raw: expr}, nil
}

func parseField(s string, r fieldRange) (CronField, error) {
	if s == "" {
		return CronField{}, fmt.Errorf("empty field")
	}
	// "*" alone is wildcard; "*/N" is a stepped wildcard, expanded to a value list.
	if s == "*" {
		return CronField{wildcard: true}, nil
	}
	set := map[int]struct{}{}
	for _, part := range strings.Split(s, ",") {
		if part == "" {
			return CronField{}, fmt.Errorf("empty list item in %q", s)
		}
		// Parse step suffix.
		step := 1
		body := part
		if i := strings.Index(part, "/"); i >= 0 {
			body = part[:i]
			stepStr := part[i+1:]
			n, err := strconv.Atoi(stepStr)
			if err != nil || n <= 0 {
				return CronField{}, fmt.Errorf("bad step %q", stepStr)
			}
			step = n
		}
		// Body is *, single value, or range.
		var lo, hi int
		switch {
		case body == "*":
			lo, hi = r.min, r.max
		case strings.Contains(body, "-"):
			rp := strings.SplitN(body, "-", 2)
			a, err := strconv.Atoi(rp[0])
			if err != nil {
				return CronField{}, fmt.Errorf("bad range start %q", rp[0])
			}
			b, err := strconv.Atoi(rp[1])
			if err != nil {
				return CronField{}, fmt.Errorf("bad range end %q", rp[1])
			}
			if a > b {
				return CronField{}, fmt.Errorf("range %d-%d is reversed", a, b)
			}
			lo, hi = a, b
		default:
			n, err := strconv.Atoi(body)
			if err != nil {
				return CronField{}, fmt.Errorf("bad value %q", body)
			}
			lo, hi = n, n
		}
		if lo < r.min || hi > r.max {
			return CronField{}, fmt.Errorf("value out of range [%d,%d]: %s", r.min, r.max, part)
		}
		for v := lo; v <= hi; v += step {
			set[v] = struct{}{}
		}
	}
	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Ints(out)
	return CronField{values: out}, nil
}

// matches checks whether v is in the field's allowed set.
func (f CronField) matches(v int) bool {
	if f.wildcard {
		return true
	}
	// Linear scan is fine for these tiny sets.
	for _, x := range f.values {
		if x == v {
			return true
		}
		if x > v {
			return false
		}
	}
	return false
}

// minute-resolution match. Cron evaluates at minute granularity.
func (c *CronExpr) matchTime(t time.Time) bool {
	if !c.Minute.matches(t.Minute()) {
		return false
	}
	if !c.Hour.matches(t.Hour()) {
		return false
	}
	if !c.Mon.matches(int(t.Month())) {
		return false
	}
	// Vixie cron semantics: when both DOM and DOW are restricted, fire if
	// EITHER matches. When one is wildcard and the other restricted, only
	// the restricted one applies. When both are wildcard, both match.
	domWild := c.Dom.wildcard
	dowWild := c.Dow.wildcard
	dom := c.Dom.matches(t.Day())
	dow := c.Dow.matches(int(t.Weekday()))
	switch {
	case domWild && dowWild:
		return true
	case domWild:
		return dow
	case dowWild:
		return dom
	default:
		return dom || dow
	}
}

// NextAfter returns the smallest time t' > t (truncated to minute) at
// which the cron expression fires, evaluated in loc. If loc is nil, UTC.
//
// Implementation: skip-ahead by field. We bump the largest mismatched
// unit at each step (year > month > day > hour > minute) instead of
// always stepping by one minute, so leap-year-only schedules like
// "0 0 29 2 *" terminate quickly. A hard cap of 8 years guards against
// pathological / impossible expressions.
func (c *CronExpr) NextAfter(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	t = t.In(loc).Truncate(time.Minute).Add(time.Minute)
	endYear := t.Year() + 8
	for t.Year() <= endYear {
		// Month
		if !c.Mon.matches(int(t.Month())) {
			// Advance to the 1st of next month, midnight.
			t = nextMonthStart(t, loc)
			continue
		}
		// Day (DOM/DOW combined per Vixie semantics)
		domWild := c.Dom.wildcard
		dowWild := c.Dow.wildcard
		dom := c.Dom.matches(t.Day())
		dow := c.Dow.matches(int(t.Weekday()))
		var dayOK bool
		switch {
		case domWild && dowWild:
			dayOK = true
		case domWild:
			dayOK = dow
		case dowWild:
			dayOK = dom
		default:
			dayOK = dom || dow
		}
		if !dayOK {
			t = nextDayStart(t, loc)
			continue
		}
		// Hour
		if !c.Hour.matches(t.Hour()) {
			t = nextHourStart(t, loc)
			continue
		}
		// Minute
		if !c.Minute.matches(t.Minute()) {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	return time.Time{}
}

// nextMonthStart returns the 1st of the next calendar month at 00:00 in loc.
func nextMonthStart(t time.Time, loc *time.Location) time.Time {
	y, m, _ := t.Date()
	m++
	if m > time.December {
		m = time.January
		y++
	}
	return time.Date(y, m, 1, 0, 0, 0, 0, loc)
}

// nextDayStart returns the next calendar day at 00:00 in loc. We advance
// at least 24 wall-hours from t and snap back to midnight; the +24h step
// guarantees forward progress across DST transitions where time.Date(y,
// m, d+1, 0,0,0,0, loc) could otherwise re-emit the same instant.
func nextDayStart(t time.Time, loc *time.Location) time.Time {
	t2 := t.Add(24 * time.Hour).In(loc)
	y, m, d := t2.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// nextHourStart returns t advanced to the next hour boundary in loc.
// Implemented as Add(+1h) then truncate-to-hour: time.Date(y,m,d,h+1,...)
// can fall into a DST "skip" hole and re-emit the same instant (Go
// normalizes back into the nearest valid wall-clock minute), causing
// infinite loops.
func nextHourStart(t time.Time, loc *time.Location) time.Time {
	t2 := t.Add(time.Hour).In(loc)
	y, m, d := t2.Date()
	return time.Date(y, m, d, t2.Hour(), 0, 0, 0, loc)
}

// ShouldFireBetween reports whether the cron should have fired at any
// minute m in (prev, now]. If prev is the zero time, it's treated as
// "never fired" and we evaluate from one tick window before now (the
// caller's responsibility to bound first-tick behavior).
//
// This is the predicate the tick loop uses: "did we miss a fire since
// last check?"
func (c *CronExpr) ShouldFireBetween(prev, now time.Time, loc *time.Location) bool {
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc).Truncate(time.Minute)
	if prev.IsZero() {
		// First evaluation: only fire if "now" itself matches.
		return c.matchTime(now)
	}
	prev = prev.In(loc).Truncate(time.Minute)
	if !now.After(prev) {
		return false
	}
	// Use NextAfter(prev) and check whether it falls on or before now.
	next := c.NextAfter(prev, loc)
	if next.IsZero() {
		return false
	}
	return !next.After(now)
}
