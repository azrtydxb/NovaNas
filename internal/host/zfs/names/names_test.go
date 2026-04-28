package names

import "testing"

func strings(s string, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = s[0]
	}
	return string(out)
}

func TestValidatePoolName(t *testing.T) {
	good := []string{"tank", "ssd", "p1", "Pool-A", "tank_01"}
	bad := []string{"", "tank/x", "tank@x", "1tank", "log", "mirror", strings("a", 256)}
	for _, n := range good {
		if err := ValidatePoolName(n); err != nil {
			t.Errorf("good %q: %v", n, err)
		}
	}
	for _, n := range bad {
		if err := ValidatePoolName(n); err == nil {
			t.Errorf("bad %q: expected error", n)
		}
	}
}

func TestValidateDatasetName(t *testing.T) {
	good := []string{"tank/home", "tank/home/alice", "tank"}
	bad := []string{"", "/tank", "tank/", "tank//x", "tank@snap", "tank/-leadingdash"}
	for _, n := range good {
		if err := ValidateDatasetName(n); err != nil {
			t.Errorf("good %q: %v", n, err)
		}
	}
	for _, n := range bad {
		if err := ValidateDatasetName(n); err == nil {
			t.Errorf("bad %q: expected error", n)
		}
	}
}

func TestValidateSnapshotName(t *testing.T) {
	good := []string{"tank@a", "tank/home@daily-2026-04-27", "p/v@x_1"}
	bad := []string{"", "tank", "tank@", "@x", "tank/@x", "tank@x@y"}
	for _, n := range good {
		if err := ValidateSnapshotName(n); err != nil {
			t.Errorf("good %q: %v", n, err)
		}
	}
	for _, n := range bad {
		if err := ValidateSnapshotName(n); err == nil {
			t.Errorf("bad %q: expected error", n)
		}
	}
}
