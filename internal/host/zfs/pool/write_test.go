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
			Type: "raidz1",
			Disks: []string{"/dev/A", "/dev/B", "/dev/C"},
		}},
		Log:   []string{"/dev/log1"},
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
