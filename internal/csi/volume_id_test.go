package csi

import "testing"

func TestVolumeIDRoundTrip(t *testing.T) {
	cases := []struct {
		full, pool, parent, leaf string
	}{
		{"tank/csi/pvc-abc", "tank", "tank/csi", "pvc-abc"},
		{"tank/pvc-x", "tank", "tank", "pvc-x"},
		{"pool1/data/sub/pvc-y", "pool1", "pool1/data/sub", "pvc-y"},
	}
	for _, tc := range cases {
		id, err := ParseVolumeID(tc.full)
		if err != nil {
			t.Fatalf("%s: %v", tc.full, err)
		}
		if id.Pool != tc.pool || id.Parent != tc.parent || id.Leaf != tc.leaf {
			t.Errorf("%s: got %+v", tc.full, id)
		}
		if got := EncodeVolumeID(tc.parent, tc.leaf); got != tc.full {
			t.Errorf("encode mismatch: %s vs %s", got, tc.full)
		}
	}
}

func TestParseVolumeID_Errors(t *testing.T) {
	for _, bad := range []string{"", "single", "a//b", "a/"} {
		if _, err := ParseVolumeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestParseSnapshotID(t *testing.T) {
	id, err := ParseSnapshotID("tank/csi/pvc-x@snap1")
	if err != nil {
		t.Fatal(err)
	}
	if id.Dataset != "tank/csi/pvc-x" || id.ShortTag != "snap1" {
		t.Fatalf("unexpected: %+v", id)
	}
	for _, bad := range []string{"no-at", "@noprefix", "noversion@"} {
		if _, err := ParseSnapshotID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestZvolDevicePath(t *testing.T) {
	id, _ := ParseVolumeID("tank/csi/pvc-abc")
	if got := ZvolDevicePath(id); got != "/dev/zvol/tank/csi/pvc-abc" {
		t.Fatalf("unexpected: %s", got)
	}
}
