package replication

import (
	"sort"
	"time"
)

// RunRecord is the minimal shape RetentionApply operates on. It is
// generic over what a "run" is: pass in successful Run rows for
// run-level retention, or remote backup-object timestamps for
// destination-side retention.
type RunRecord struct {
	ID   string
	Time time.Time
}

// RetentionApply partitions records into "keep" and "drop" sets based
// on the policy. Records are not modified; the caller is responsible
// for actually destroying anything in drop.
//
// The algorithm is sanoid-style: bucket entries by day/week/month/year
// and keep the newest N per bucket. KeepLastN is applied as a
// supplementary global "keep at least N most recent" floor.
func RetentionApply(records []RunRecord, policy RetentionPolicy) (keep, drop []RunRecord) {
	if policy.IsZero() || len(records) == 0 {
		// "no retention" means keep everything.
		return records, nil
	}
	sorted := make([]RunRecord, len(records))
	copy(sorted, records)
	// Newest first.
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Time.After(sorted[j].Time)
	})

	keepSet := make(map[string]struct{}, len(sorted))
	mark := func(id string) { keepSet[id] = struct{}{} }

	// Simple "keep last N" bucket.
	if policy.KeepLastN > 0 {
		for i := 0; i < len(sorted) && i < policy.KeepLastN; i++ {
			mark(sorted[i].ID)
		}
	}

	// Calendar buckets. We pick the newest entry whose calendar bucket
	// hasn't already been claimed.
	pickByBucket := func(want int, bucketKey func(t time.Time) string) {
		if want <= 0 {
			return
		}
		seen := map[string]struct{}{}
		picked := 0
		for _, r := range sorted {
			k := bucketKey(r.Time)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			mark(r.ID)
			picked++
			if picked >= want {
				return
			}
		}
	}

	pickByBucket(policy.KeepDaily, func(t time.Time) string {
		y, m, d := t.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, t.Location()).Format("2006-01-02")
	})
	pickByBucket(policy.KeepWeekly, func(t time.Time) string {
		y, w := t.ISOWeek()
		return formatWeekKey(y, w)
	})
	pickByBucket(policy.KeepMonthly, func(t time.Time) string {
		return t.Format("2006-01")
	})
	pickByBucket(policy.KeepYearly, func(t time.Time) string {
		return t.Format("2006")
	})

	for _, r := range sorted {
		if _, ok := keepSet[r.ID]; ok {
			keep = append(keep, r)
		} else {
			drop = append(drop, r)
		}
	}
	return keep, drop
}

func formatWeekKey(year, week int) string {
	// Simple stable encoding; the exact string doesn't matter, just
	// that two times in the same ISO week produce the same key.
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006") +
		"-W" + twoDigit(week)
}

func twoDigit(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
