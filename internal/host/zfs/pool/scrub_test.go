package pool

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// scriptedRunner returns a different output per zpool sub-command. Keys
// are matched against args[0]; if multiple subcommands need different
// pool names the value can encode a switch on args themselves via
// scriptedFunc.
type scriptedRunner struct {
	scripts map[string][]byte
	calls   [][]string
}

func (s *scriptedRunner) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	cp := append([]string(nil), args...)
	s.calls = append(s.calls, cp)
	if len(args) == 0 {
		return nil, nil
	}
	if out, ok := s.scripts[args[0]]; ok {
		return out, nil
	}
	return nil, nil
}

// fakePoolListLine is a single tab-separated zpool list -p row that
// matches parser.parseList expectations (9 fields).
const fakePoolListLine = "tank\t1000204886016\t123456789012\t876748097004\tONLINE\toff\t5\t12\t1.00x\n"

func fakeZpoolGetAll(pool string) []byte {
	// Three-column tab-separated rows: pool\tprop\tvalue\tsource (parser
	// only requires 3+).
	return []byte(strings.Join([]string{
		pool + "\tsize\t1000204886016\tdefault",
		pool + "\thealth\tONLINE\t-",
		"",
	}, "\n"))
}

// fakeZpoolStatusInProgress emits a status output where the pool is
// mid-scrub. The format mirrors real `zpool status -P` carefully enough
// for parseStatus to produce a Scan{State:"in-progress"} block.
func fakeZpoolStatusInProgress(pool string) []byte {
	var b bytes.Buffer
	b.WriteString("  pool: " + pool + "\n")
	b.WriteString(" state: ONLINE\n")
	b.WriteString("  scan: scrub in progress since now, 1.50G scanned at 1.00G/s, 5.00G total\n")
	b.WriteString("config:\n")
	b.WriteString("\tNAME        STATE     READ WRITE CKSUM\n")
	b.WriteString("\t" + pool + "      ONLINE       0     0     0\n")
	b.WriteString("\t  /dev/sda  ONLINE       0     0     0\n")
	b.WriteString("errors: No known data errors\n")
	return b.Bytes()
}

func fakeZpoolStatusFinished(pool string) []byte {
	var b bytes.Buffer
	b.WriteString("  pool: " + pool + "\n")
	b.WriteString(" state: ONLINE\n")
	b.WriteString("  scan: scrub repaired 0B in 1h0m with 0 errors on Mon Jan  1 00:00:00 2024\n")
	b.WriteString("config:\n")
	b.WriteString("\tNAME        STATE     READ WRITE CKSUM\n")
	b.WriteString("\t" + pool + "      ONLINE       0     0     0\n")
	b.WriteString("\t  /dev/sda  ONLINE       0     0     0\n")
	b.WriteString("errors: No known data errors\n")
	return b.Bytes()
}

func fakeZpoolStatusResilver(pool string) []byte {
	var b bytes.Buffer
	b.WriteString("  pool: " + pool + "\n")
	b.WriteString(" state: ONLINE\n")
	b.WriteString("  scan: resilvered 5.00G in 0h10m with 0 errors on Mon Jan  1 00:00:00 2024\n")
	b.WriteString("config:\n")
	b.WriteString("\tNAME        STATE     READ WRITE CKSUM\n")
	b.WriteString("\t" + pool + "      ONLINE       0     0     0\n")
	b.WriteString("errors: No known data errors\n")
	return b.Bytes()
}

func TestStartStopScrub_Args(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: cap.run}

	if err := m.StartScrub(context.Background(), "tank"); err != nil {
		t.Fatalf("StartScrub: %v", err)
	}
	if err := m.StopScrub(context.Background(), "tank"); err != nil {
		t.Fatalf("StopScrub: %v", err)
	}
	if len(cap.calls) != 2 {
		t.Fatalf("want 2 calls, got %d", len(cap.calls))
	}
	if cap.calls[0][0] != "scrub" || cap.calls[0][len(cap.calls[0])-1] != "tank" {
		t.Errorf("start args=%v", cap.calls[0])
	}
	// stop must include "-s"
	found := false
	for _, a := range cap.calls[1] {
		if a == "-s" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("stop args missing -s: %v", cap.calls[1])
	}
}

func TestScrubStatus_InProgress(t *testing.T) {
	r := &scriptedRunner{scripts: map[string][]byte{
		"list":   []byte(fakePoolListLine),
		"get":    fakeZpoolGetAll("tank"),
		"status": fakeZpoolStatusInProgress("tank"),
	}}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: r.run}

	info, err := m.ScrubStatus(context.Background(), "tank")
	if err != nil {
		t.Fatalf("ScrubStatus: %v", err)
	}
	if info.State != "in-progress" {
		t.Errorf("state=%q want in-progress", info.State)
	}
	in, err := m.IsScrubInProgress(context.Background(), "tank")
	if err != nil || !in {
		t.Errorf("IsScrubInProgress=%v err=%v", in, err)
	}
}

func TestScrubStatus_Finished(t *testing.T) {
	r := &scriptedRunner{scripts: map[string][]byte{
		"list":   []byte(fakePoolListLine),
		"get":    fakeZpoolGetAll("tank"),
		"status": fakeZpoolStatusFinished("tank"),
	}}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: r.run}

	info, err := m.ScrubStatus(context.Background(), "tank")
	if err != nil {
		t.Fatalf("ScrubStatus: %v", err)
	}
	if info.State != "finished" {
		t.Errorf("state=%q want finished", info.State)
	}
}

func TestIsResilverInProgress(t *testing.T) {
	r := &scriptedRunner{scripts: map[string][]byte{
		"list":   []byte(fakePoolListLine),
		"get":    fakeZpoolGetAll("tank"),
		"status": fakeZpoolStatusResilver("tank"),
	}}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: r.run}

	in, err := m.IsResilverInProgress(context.Background(), "tank")
	if err != nil || !in {
		t.Errorf("IsResilverInProgress=%v err=%v", in, err)
	}
}

func TestPoolNames(t *testing.T) {
	r := &scriptedRunner{scripts: map[string][]byte{
		"list": []byte(fakePoolListLine + "scratch\t500107862016\t1024\t499999999\tONLINE\toff\t0\t0\t1.00x\n"),
	}}
	m := &Manager{ZpoolBin: "/sbin/zpool", Runner: r.run}

	names, err := m.PoolNames(context.Background())
	if err != nil {
		t.Fatalf("PoolNames: %v", err)
	}
	if len(names) != 2 || names[0] != "tank" || names[1] != "scratch" {
		t.Errorf("names=%v", names)
	}
}

func TestSumVdevErrors(t *testing.T) {
	v := []Vdev{
		{Type: "mirror", Children: []Vdev{
			{Type: "disk", Path: "/dev/a", ReadErr: 1, WriteErr: 2, CksumErr: 3},
			{Type: "disk", Path: "/dev/b", ReadErr: 0, WriteErr: 0, CksumErr: 1},
		}},
	}
	if got := sumVdevErrors(v); got != 7 {
		t.Errorf("sum=%d want 7", got)
	}
}
