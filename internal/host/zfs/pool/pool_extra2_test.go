package pool

import (
	"context"
	"testing"
)

func TestBuildCheckpointArgs(t *testing.T) {
	cases := []struct {
		name    string
		pool    string
		discard bool
		want    []string
		wantErr bool
	}{
		{"create", "tank", false, []string{"checkpoint", "tank"}, false},
		{"discard", "tank", true, []string{"checkpoint", "-d", "tank"}, false},
		{"bad name", "-evil", false, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildCheckpointArgs(tc.pool, tc.discard)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !equal(got, tc.want) {
				t.Errorf("got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestCheckpoint_Args(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: cap.run}
	if err := m.Checkpoint(context.Background(), "tank"); err != nil {
		t.Fatal(err)
	}
	want := []string{"checkpoint", "tank"}
	if len(cap.calls) != 1 || !equal(cap.calls[0], want) {
		t.Errorf("got=%v want=%v", cap.calls, want)
	}
}

func TestCheckpoint_RejectsBadName(t *testing.T) {
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: (&captureRunner{}).run}
	if err := m.Checkpoint(context.Background(), "-evil"); err == nil {
		t.Error("expected error for bad pool name")
	}
}

func TestDiscardCheckpoint_Args(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: cap.run}
	if err := m.DiscardCheckpoint(context.Background(), "tank"); err != nil {
		t.Fatal(err)
	}
	want := []string{"checkpoint", "-d", "tank"}
	if len(cap.calls) != 1 || !equal(cap.calls[0], want) {
		t.Errorf("got=%v want=%v", cap.calls, want)
	}
}

func TestDiscardCheckpoint_RejectsBadName(t *testing.T) {
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: (&captureRunner{}).run}
	if err := m.DiscardCheckpoint(context.Background(), "-evil"); err == nil {
		t.Error("expected error for bad pool name")
	}
}

func TestBuildUpgradeArgs(t *testing.T) {
	cases := []struct {
		desc    string
		name    string
		all     bool
		want    []string
		wantErr bool
	}{
		{"single pool", "tank", false, []string{"upgrade", "tank"}, false},
		{"all pools", "", true, []string{"upgrade", "-a"}, false},
		{"all ignores name", "tank", true, []string{"upgrade", "-a"}, false},
		{"empty name without all", "", false, nil, true},
		{"bad name", "-evil", false, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := buildUpgradeArgs(tc.name, tc.all)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !equal(got, tc.want) {
				t.Errorf("got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestUpgrade_Args(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: cap.run}
	if err := m.Upgrade(context.Background(), "tank", false); err != nil {
		t.Fatal(err)
	}
	if err := m.Upgrade(context.Background(), "", true); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"upgrade", "tank"},
		{"upgrade", "-a"},
	}
	if len(cap.calls) != len(want) {
		t.Fatalf("got %d calls want %d (%v)", len(cap.calls), len(want), cap.calls)
	}
	for i := range want {
		if !equal(cap.calls[i], want[i]) {
			t.Errorf("call %d: got=%v want=%v", i, cap.calls[i], want[i])
		}
	}
}

func TestUpgrade_RejectsBadName(t *testing.T) {
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: (&captureRunner{}).run}
	if err := m.Upgrade(context.Background(), "-evil", false); err == nil {
		t.Error("expected error for bad pool name")
	}
	if err := m.Upgrade(context.Background(), "", false); err == nil {
		t.Error("expected error for empty name without all")
	}
}

func TestBuildReguidArgs(t *testing.T) {
	got, err := buildReguidArgs("tank")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"reguid", "tank"}
	if !equal(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
	if _, err := buildReguidArgs("-evil"); err == nil {
		t.Error("expected error for bad name")
	}
}

func TestReguid_Args(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: cap.run}
	if err := m.Reguid(context.Background(), "tank"); err != nil {
		t.Fatal(err)
	}
	want := []string{"reguid", "tank"}
	if len(cap.calls) != 1 || !equal(cap.calls[0], want) {
		t.Errorf("got=%v want=%v", cap.calls, want)
	}
}

func TestReguid_RejectsBadName(t *testing.T) {
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: (&captureRunner{}).run}
	if err := m.Reguid(context.Background(), "-evil"); err == nil {
		t.Error("expected error for bad pool name")
	}
}

func TestBuildSyncArgs(t *testing.T) {
	cases := []struct {
		desc    string
		names   []string
		want    []string
		wantErr bool
	}{
		{"all pools", nil, []string{"sync"}, false},
		{"empty list", []string{}, []string{"sync"}, false},
		{"single", []string{"tank"}, []string{"sync", "tank"}, false},
		{"multiple", []string{"tank", "rpool"}, []string{"sync", "tank", "rpool"}, false},
		{"bad name", []string{"tank", "-evil"}, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := buildSyncArgs(tc.names)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !equal(got, tc.want) {
				t.Errorf("got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestSync_Args(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: cap.run}
	if err := m.Sync(context.Background(), []string{"tank", "rpool"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"sync", "tank", "rpool"},
		{"sync"},
	}
	if len(cap.calls) != len(want) {
		t.Fatalf("got %d calls want %d (%v)", len(cap.calls), len(want), cap.calls)
	}
	for i := range want {
		if !equal(cap.calls[i], want[i]) {
			t.Errorf("call %d: got=%v want=%v", i, cap.calls[i], want[i])
		}
	}
}

func TestSync_RejectsBadName(t *testing.T) {
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: (&captureRunner{}).run}
	if err := m.Sync(context.Background(), []string{"-evil"}); err == nil {
		t.Error("expected error for bad pool name")
	}
}
