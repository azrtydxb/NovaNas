package pool

import (
	"context"
	"testing"
	"time"
)

func TestBuildCreateArgs_Draid(t *testing.T) {
	disks := []string{
		"/dev/d0", "/dev/d1", "/dev/d2", "/dev/d3", "/dev/d4", "/dev/d5",
		"/dev/d6", "/dev/d7", "/dev/d8", "/dev/d9", "/dev/d10",
	}
	spec := CreateSpec{
		Name:  "tank",
		Vdevs: []VdevSpec{{Type: "draid2:8d:1s", Disks: disks}},
	}
	args, err := buildCreateArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string{"create", "-f", "tank", "draid2:8d:1s"}, disks...)
	if !equal(args, want) {
		t.Errorf("args=%v want=%v", args, want)
	}
}

func TestBuildCreateArgs_SpecialAndDedup(t *testing.T) {
	spec := CreateSpec{
		Name:    "tank",
		Vdevs:   []VdevSpec{{Type: "mirror", Disks: []string{"/dev/A", "/dev/B"}}},
		Special: []VdevSpec{{Type: "mirror", Disks: []string{"/dev/SP1", "/dev/SP2"}}},
		Dedup:   []VdevSpec{{Type: "mirror", Disks: []string{"/dev/DD1", "/dev/DD2"}}},
		Log:     []VdevSpec{{Type: "disk", Disks: []string{"/dev/log1"}}},
		Cache:   []string{"/dev/cache1"},
		Spare:   []string{"/dev/spare1"},
	}
	args, err := buildCreateArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"create", "-f", "tank",
		"mirror", "/dev/A", "/dev/B",
		"special", "mirror", "/dev/SP1", "/dev/SP2",
		"dedup", "mirror", "/dev/DD1", "/dev/DD2",
		"log", "/dev/log1",
		"cache", "/dev/cache1",
		"spare", "/dev/spare1",
	}
	if !equal(args, want) {
		t.Errorf("args=%v want=%v", args, want)
	}
}

func TestBuildAddArgs_Special(t *testing.T) {
	args, err := buildAddArgs("tank", AddSpec{
		Special: []VdevSpec{{Type: "mirror", Disks: []string{"/dev/SP1", "/dev/SP2"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"add", "-f", "tank", "special", "mirror", "/dev/SP1", "/dev/SP2"}
	if !equal(args, want) {
		t.Errorf("args=%v want=%v", args, want)
	}
}

// captureRunner records the last argv it was called with and returns
// empty output. Used to assert that Manager methods build the right
// zpool invocation without actually executing anything.
type captureRunner struct {
	calls [][]string
}

func (c *captureRunner) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	cp := append([]string(nil), args...)
	c.calls = append(c.calls, cp)
	return nil, nil
}

func TestTrim_Args(t *testing.T) {
	cases := []struct {
		name    string
		action  TrimAction
		disk    string
		want    []string
		wantErr bool
	}{
		{"start whole", TrimStart, "", []string{"trim", "tank"}, false},
		{"stop whole", TrimStop, "", []string{"trim", "-c", "tank"}, false},
		{"start scoped", TrimStart, "/dev/sda", []string{"trim", "tank", "/dev/sda"}, false},
		{"stop scoped", TrimStop, "/dev/sda", []string{"trim", "-c", "tank", "/dev/sda"}, false},
		{"reject dash disk", TrimStart, "-bad", nil, true},
		{"reject bogus action", "burn", "", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cap := &captureRunner{}
			m := &Manager{ZpoolBin: "/sbin/zpool", Runner: cap.run}
			err := m.Trim(context.Background(), "tank", tc.action, tc.disk)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(cap.calls) != 1 || !equal(cap.calls[0], tc.want) {
				t.Errorf("got=%v want=%v", cap.calls, tc.want)
			}
		})
	}
}

func TestSetPoolProps_Args(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: cap.run}
	err := m.SetProps(context.Background(), "tank", map[string]string{
		"comment":  "hello",
		"autotrim": "on",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Sorted: autotrim, comment.
	want := [][]string{
		{"set", "autotrim=on", "tank"},
		{"set", "comment=hello", "tank"},
	}
	if len(cap.calls) != len(want) {
		t.Fatalf("got %d calls, want %d (%v)", len(cap.calls), len(want), cap.calls)
	}
	for i := range want {
		if !equal(cap.calls[i], want[i]) {
			t.Errorf("call %d: got=%v want=%v", i, cap.calls[i], want[i])
		}
	}
}

func TestSetPoolProps_RejectsBadKey(t *testing.T) {
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: (&captureRunner{}).run}
	if err := m.SetProps(context.Background(), "tank", map[string]string{"-evil": "x"}); err == nil {
		t.Error("expected error for leading-dash key")
	}
	if err := m.SetProps(context.Background(), "tank", map[string]string{"": "x"}); err == nil {
		t.Error("expected error for empty key")
	}
}

func TestWait_Args(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: cap.run}
	if err := m.Wait(context.Background(), "tank", "scrub", 0); err != nil {
		t.Fatal(err)
	}
	want := []string{"wait", "-t", "scrub", "tank"}
	if len(cap.calls) != 1 || !equal(cap.calls[0], want) {
		t.Errorf("got=%v want=%v", cap.calls, want)
	}
}

func TestWait_RejectsBogusActivity(t *testing.T) {
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: (&captureRunner{}).run}
	if err := m.Wait(context.Background(), "tank", "bogus", 0); err == nil {
		t.Error("expected error for bogus activity")
	}
}

func TestWait_TimeoutHonored(t *testing.T) {
	// The runner inspects the ctx deadline to verify Wait wires timeout.
	var sawDeadline bool
	runner := func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		_, sawDeadline = ctx.Deadline()
		return nil, nil
	}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: runner}
	if err := m.Wait(context.Background(), "tank", "scrub", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if !sawDeadline {
		t.Error("expected ctx deadline to be set when timeout > 0")
	}
}
