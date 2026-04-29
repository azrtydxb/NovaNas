package scheduler

import (
	"sort"
	"testing"
	"time"
)

// mkSnap makes a SnapInfo at "tank/data@auto-<ts>".
func mkSnap(ts time.Time) SnapInfo {
	return SnapInfo{
		Name: "tank/data@" + FormatSnapTime("auto", ts),
		Time: ts,
	}
}

func keepNames(snaps []SnapInfo) []string {
	out := make([]string, len(snaps))
	for i, s := range snaps {
		out[i] = s.Name
	}
	sort.Strings(out)
	return out
}

func TestPartitionRetention_DailyKeepsRecent(t *testing.T) {
	loc := time.UTC
	var snaps []SnapInfo
	// 10 daily snapshots, one per day at noon.
	for d := 0; d < 10; d++ {
		snaps = append(snaps, mkSnap(time.Date(2026, 4, 20+d, 12, 0, 0, 0, loc)))
	}
	keep, drop := PartitionRetention(snaps, RetentionPolicy{Daily: 3})
	if len(keep) != 3 {
		t.Errorf("keep=%d, want 3 (%v)", len(keep), keepNames(keep))
	}
	if len(drop) != 7 {
		t.Errorf("drop=%d, want 7", len(drop))
	}
	// The 3 kept should be the latest 3 days.
	for _, s := range keep {
		if s.Time.Day() < 27 {
			t.Errorf("kept too-old snap %v", s.Name)
		}
	}
}

func TestPartitionRetention_HourlyKeepsLatestPerHour(t *testing.T) {
	loc := time.UTC
	var snaps []SnapInfo
	// 4 snapshots in hour 12, 4 in hour 13. With Hourly=2, only the
	// EARLIEST snap from each of the latest 2 hour-buckets should be
	// kept.
	for m := 0; m < 4; m++ {
		snaps = append(snaps, mkSnap(time.Date(2026, 4, 28, 12, m*15, 0, 0, loc)))
		snaps = append(snaps, mkSnap(time.Date(2026, 4, 28, 13, m*15, 0, 0, loc)))
	}
	keep, _ := PartitionRetention(snaps, RetentionPolicy{Hourly: 2})
	if len(keep) != 2 {
		t.Fatalf("keep=%d, want 2: %v", len(keep), keepNames(keep))
	}
	// Both kept should be at minute 0 of their hour.
	for _, k := range keep {
		if k.Time.Minute() != 0 {
			t.Errorf("expected minute=0 (earliest), got %v", k.Name)
		}
	}
}

func TestPartitionRetention_Combined(t *testing.T) {
	loc := time.UTC
	// One snap per day for 14 days at midnight.
	var snaps []SnapInfo
	for d := 0; d < 14; d++ {
		snaps = append(snaps, mkSnap(time.Date(2026, 4, 1+d, 0, 0, 0, 0, loc)))
	}
	keep, _ := PartitionRetention(snaps, RetentionPolicy{Daily: 3, Weekly: 2})
	// Daily keeps 3 most-recent days; weekly keeps earliest of each ISO
	// week, latest 2. Some overlap; union should be > 3.
	if len(keep) < 3 || len(keep) > 5 {
		t.Errorf("unexpected keep count %d (%v)", len(keep), keepNames(keep))
	}
}

func TestPartitionRetention_AllZeroPolicyKeepsNone(t *testing.T) {
	loc := time.UTC
	snaps := []SnapInfo{mkSnap(time.Date(2026, 4, 28, 12, 0, 0, 0, loc))}
	keep, drop := PartitionRetention(snaps, RetentionPolicy{})
	if len(keep) != 0 || len(drop) != 1 {
		t.Errorf("zero policy: keep=%d drop=%d (manager bypasses this case)", len(keep), len(drop))
	}
}

func TestParsedSnapTime(t *testing.T) {
	loc := time.UTC
	got, ok := ParsedSnapTime("auto-2026-04-28-1430", "auto", loc)
	if !ok {
		t.Fatal("expected parse ok")
	}
	want := time.Date(2026, 4, 28, 14, 30, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Wrong prefix.
	if _, ok := ParsedSnapTime("repl-2026-04-28-1430", "auto", loc); ok {
		t.Error("expected mismatch on prefix")
	}
	// Bad format.
	if _, ok := ParsedSnapTime("auto-not-a-date", "auto", loc); ok {
		t.Error("expected mismatch on format")
	}
}

func TestFormatSnapTime(t *testing.T) {
	loc := time.UTC
	t0 := time.Date(2026, 4, 28, 14, 30, 0, 0, loc)
	got := FormatSnapTime("auto", t0)
	if got != "auto-2026-04-28-1430" {
		t.Errorf("got %q", got)
	}
}
