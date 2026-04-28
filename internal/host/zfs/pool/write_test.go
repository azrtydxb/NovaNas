package pool

import "testing"

func TestBuildCreateArgs_Mirror(t *testing.T) {
	spec := CreateSpec{
		Name: "tank",
		Vdevs: []VdevSpec{{
			Type: "mirror",
			Disks: []string{"/dev/disk/by-id/wwn-0xA", "/dev/disk/by-id/wwn-0xB"},
		}},
	}
	args, err := buildCreateArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"create", "-f", "tank", "mirror",
		"/dev/disk/by-id/wwn-0xA", "/dev/disk/by-id/wwn-0xB"}
	if !equal(args, want) {
		t.Errorf("args=%v want=%v", args, want)
	}
}

func TestBuildCreateArgs_RaidzPlusLog(t *testing.T) {
	spec := CreateSpec{
		Name: "tank",
		Vdevs: []VdevSpec{{
			Type:  "raidz1",
			Disks: []string{"/dev/A", "/dev/B", "/dev/C"},
		}},
		Log:   []VdevSpec{{Type: "disk", Disks: []string{"/dev/log1"}}},
		Cache: []string{"/dev/cache1"},
	}
	args, err := buildCreateArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"create", "-f", "tank",
		"raidz1", "/dev/A", "/dev/B", "/dev/C",
		"log", "/dev/log1",
		"cache", "/dev/cache1"}
	if !equal(args, want) {
		t.Errorf("args=%v want=%v", args, want)
	}
}

func TestBuildCreateArgs_MirroredLog(t *testing.T) {
	spec := CreateSpec{
		Name:  "tank",
		Vdevs: []VdevSpec{{Type: "mirror", Disks: []string{"/dev/A", "/dev/B"}}},
		Log:   []VdevSpec{{Type: "mirror", Disks: []string{"/dev/L1", "/dev/L2"}}},
	}
	args, err := buildCreateArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"create", "-f", "tank",
		"mirror", "/dev/A", "/dev/B",
		"log", "mirror", "/dev/L1", "/dev/L2"}
	if !equal(args, want) {
		t.Errorf("args=%v want=%v", args, want)
	}
}

func TestBuildCreateArgs_LogRejectsRaidz(t *testing.T) {
	spec := CreateSpec{
		Name:  "tank",
		Vdevs: []VdevSpec{{Type: "mirror", Disks: []string{"/dev/A", "/dev/B"}}},
		Log:   []VdevSpec{{Type: "raidz1", Disks: []string{"/dev/L1", "/dev/L2", "/dev/L3"}}},
	}
	if _, err := buildCreateArgs(spec); err == nil {
		t.Error("expected error: raidz not valid for log vdev")
	}
}

func TestBuildAddArgs_DataAndSpare(t *testing.T) {
	args, err := buildAddArgs("tank", AddSpec{
		Vdevs: []VdevSpec{{Type: "mirror", Disks: []string{"/dev/D1", "/dev/D2"}}},
		Spare: []string{"/dev/S1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"add", "-f", "tank",
		"mirror", "/dev/D1", "/dev/D2",
		"spare", "/dev/S1"}
	if !equal(args, want) {
		t.Errorf("args=%v want=%v", args, want)
	}
}

func TestBuildAddArgs_Empty(t *testing.T) {
	if _, err := buildAddArgs("tank", AddSpec{}); err == nil {
		t.Error("expected error for empty AddSpec")
	}
}

func TestBuildCreateArgs_BadVdevType(t *testing.T) {
	spec := CreateSpec{Name: "tank", Vdevs: []VdevSpec{{Type: "bogus", Disks: []string{"/dev/a"}}}}
	if _, err := buildCreateArgs(spec); err == nil {
		t.Error("expected error")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
