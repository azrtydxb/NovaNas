package snapshot

import (
	"os"
	"testing"
)

func TestParseList(t *testing.T) {
	data, err := os.ReadFile("../../../../test/fixtures/zfs_snap_list.txt")
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := parseList(data)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}
	if len(snaps) != 3 {
		t.Fatalf("want 3, got %d", len(snaps))
	}
	if snaps[0].Name != "tank/home@daily-2026-04-27" {
		t.Errorf("snap0=%+v", snaps[0])
	}
	if snaps[0].Dataset != "tank/home" || snaps[0].ShortName != "daily-2026-04-27" {
		t.Errorf("split wrong: %+v", snaps[0])
	}
	if snaps[2].Dataset != "tank/vol1" || snaps[2].ShortName != "pre-upgrade" {
		t.Errorf("split wrong: %+v", snaps[2])
	}
}
