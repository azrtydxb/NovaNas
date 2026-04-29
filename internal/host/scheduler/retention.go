package scheduler

import (
	"sort"
	"strings"
	"time"
)

// RetentionPolicy is the per-bucket keep counts. Zero means "do not keep
// any from this class" — but if every class is zero, the manager treats
// it as "keep nothing managed by this prefix" and prunes all matches. Use
// a high number (e.g. math.MaxInt32) to mean "keep all".
type RetentionPolicy struct {
	Hourly, Daily, Weekly, Monthly, Yearly int
}

// SnapInfo is a minimal description of a snapshot used by retention.
// Name is the full "<dataset>@<short>" name; Time is parsed from the
// short-name suffix (preferred) or falls back to a creation time.
type SnapInfo struct {
	Name string
	Time time.Time
}

// ParsedSnapTime extracts the timestamp portion from a snapshot short name
// matching "<prefix>-<YYYY-MM-DD-HHMM>". Returns ok=false if the format
// doesn't match.
func ParsedSnapTime(short, prefix string, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = time.UTC
	}
	wantPrefix := prefix + "-"
	if !strings.HasPrefix(short, wantPrefix) {
		return time.Time{}, false
	}
	suffix := short[len(wantPrefix):]
	t, err := time.ParseInLocation("2006-01-02-1504", suffix, loc)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// FormatSnapTime renders a snapshot short name as "<prefix>-YYYY-MM-DD-HHMM".
func FormatSnapTime(prefix string, t time.Time) string {
	return prefix + "-" + t.Format("2006-01-02-1504")
}

// classifyKeep takes a slice of SnapInfo (with Time populated) and a
// RetentionPolicy, and returns the set of names to KEEP. Anything not in
// the keep set should be destroyed.
//
// Bucketing rules (sanoid-style):
//   - hourly  : first snapshot in each (year, day-of-year, hour) bucket
//   - daily   : first snapshot in each (year, day-of-year) bucket
//   - weekly  : first snapshot in each ISO (year, week) bucket
//   - monthly : first snapshot in each (year, month) bucket
//   - yearly  : first snapshot in each year
//
// "first snapshot" = chronologically earliest in that bucket. Then we
// keep the most-recent N buckets per class. A snapshot kept by ANY class
// is preserved.
func classifyKeep(snaps []SnapInfo, p RetentionPolicy) map[string]struct{} {
	keep := map[string]struct{}{}
	if len(snaps) == 0 {
		return keep
	}
	// Stable order: oldest → newest. The "first" snapshot in a bucket is
	// the earliest one we encounter.
	sorted := make([]SnapInfo, len(snaps))
	copy(sorted, snaps)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Time.Before(sorted[j].Time)
	})
	hourly := bucketEarliest(sorted, hourKey)
	daily := bucketEarliest(sorted, dayKey)
	weekly := bucketEarliest(sorted, weekKey)
	monthly := bucketEarliest(sorted, monthKey)
	yearly := bucketEarliest(sorted, yearKey)
	keepN(hourly, p.Hourly, keep)
	keepN(daily, p.Daily, keep)
	keepN(weekly, p.Weekly, keep)
	keepN(monthly, p.Monthly, keep)
	keepN(yearly, p.Yearly, keep)
	return keep
}

// keyFn maps a snapshot time to a bucket key.
type keyFn func(time.Time) string

func hourKey(t time.Time) string  { return t.UTC().Format("2006-01-02-15") }
func dayKey(t time.Time) string   { return t.UTC().Format("2006-01-02") }
func monthKey(t time.Time) string { return t.UTC().Format("2006-01") }
func yearKey(t time.Time) string  { return t.UTC().Format("2006") }

func weekKey(t time.Time) string {
	y, w := t.UTC().ISOWeek()
	return formatWeek(y, w)
}

func formatWeek(y, w int) string {
	// Stable two-digit week to keep string sort identical to time sort.
	ws := strings.Repeat("0", 2-len(itoa(w))) + itoa(w)
	return itoa(y) + "-W" + ws
}

func itoa(n int) string { return formatInt(n) }

func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// bucketEarliest returns the earliest snapshot per bucket, in chronological
// order (oldest bucket first).
func bucketEarliest(sortedAsc []SnapInfo, k keyFn) []SnapInfo {
	seen := map[string]struct{}{}
	var out []SnapInfo
	for _, s := range sortedAsc {
		key := k(s.Time)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

// keepN adds the most recent n entries from the bucket-earliest list to
// keep. The list is in oldest-first order, so we take the tail.
func keepN(b []SnapInfo, n int, keep map[string]struct{}) {
	if n <= 0 || len(b) == 0 {
		return
	}
	start := len(b) - n
	if start < 0 {
		start = 0
	}
	for _, s := range b[start:] {
		keep[s.Name] = struct{}{}
	}
}

// PartitionRetention sorts snaps into kept and dropped using the policy.
// Useful for tests and callers that want both sides.
func PartitionRetention(snaps []SnapInfo, p RetentionPolicy) (keep, drop []SnapInfo) {
	keepSet := classifyKeep(snaps, p)
	for _, s := range snaps {
		if _, ok := keepSet[s.Name]; ok {
			keep = append(keep, s)
		} else {
			drop = append(drop, s)
		}
	}
	return keep, drop
}
