package pool

import (
	"os"
	"testing"
)

func TestParseProps(t *testing.T) {
	data, err := os.ReadFile("../../../../test/fixtures/zpool_get.txt")
	if err != nil {
		t.Fatal(err)
	}
	props, err := parseProps(data)
	if err != nil {
		t.Fatalf("parseProps: %v", err)
	}
	if props["health"] != "ONLINE" {
		t.Errorf("health=%q", props["health"])
	}
	if props["ashift"] != "12" {
		t.Errorf("ashift=%q", props["ashift"])
	}
}

func TestParseStatus_VdevTree(t *testing.T) {
	data, err := os.ReadFile("../../../../test/fixtures/zpool_status.txt")
	if err != nil {
		t.Fatal(err)
	}
	st, err := parseStatus(data)
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if st.State != "ONLINE" {
		t.Errorf("state=%q", st.State)
	}
	if len(st.Vdevs) == 0 {
		t.Fatal("no vdevs")
	}
	if st.Vdevs[0].Type != "mirror" {
		t.Errorf("vdev0=%+v", st.Vdevs[0])
	}
	if len(st.Vdevs[0].Children) != 2 {
		t.Errorf("mirror children=%d", len(st.Vdevs[0].Children))
	}
}

// TestParseStatus_RealZpoolFormat exercises the actual `zpool status -P`
// output shape: one leading TAB, then SPACES for visual nesting (2 per
// level). Captured from a live mirror pool — historically the parser
// only counted tabs, which collapsed every row to depth 1 and lost the
// vdev tree. Regression guard.
func TestParseStatus_RealZpoolFormat(t *testing.T) {
	in := []byte("  pool: tank\n state: ONLINE\nconfig:\n\n" +
		"\tNAME                STATE     READ WRITE CKSUM\n" +
		"\ttank                ONLINE       0     0     0\n" +
		"\t  mirror-0          ONLINE       0     0     0\n" +
		"\t    /dev/sda        ONLINE       0     0     0\n" +
		"\t    /dev/sdb        ONLINE       0     0     0\n" +
		"\nerrors: No known data errors\n")
	st, err := parseStatus(in)
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if len(st.Vdevs) != 1 || st.Vdevs[0].Type != "mirror" {
		t.Fatalf("want one mirror vdev, got %+v", st.Vdevs)
	}
	if len(st.Vdevs[0].Children) != 2 {
		t.Errorf("mirror children=%d (want 2)", len(st.Vdevs[0].Children))
	}
}

// --- scrub / scan tests -------------------------------------------------------

func makeStatusInput(scanLine string) []byte {
	base := "  pool: tank\n state: ONLINE\n" + scanLine + "\nconfig:\n\n" +
		"\tNAME        STATE     READ WRITE CKSUM\n" +
		"\ttank        ONLINE       0     0     0\n" +
		"\t  /dev/sda  ONLINE       0     0     0\n" +
		"\nerrors: No known data errors\n"
	return []byte(base)
}

func TestParseScrubStatus_None(t *testing.T) {
	st, err := parseStatus(makeStatusInput("scan: none requested"))
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if st.Scan == nil {
		t.Fatal("Scan is nil")
	}
	if st.Scan.State != "none" {
		t.Errorf("State=%q want %q", st.Scan.State, "none")
	}
	if st.Scan.RawLine != "none requested" {
		t.Errorf("RawLine=%q", st.Scan.RawLine)
	}
}

func TestParseScrubStatus_Finished(t *testing.T) {
	line := "scan: scrub repaired 0B in 00:01:30 with 0 errors on Tue Apr 28 18:30:00 2026"
	st, err := parseStatus(makeStatusInput(line))
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if st.Scan == nil {
		t.Fatal("Scan is nil")
	}
	if st.Scan.State != "finished" {
		t.Errorf("State=%q want %q", st.Scan.State, "finished")
	}
	wantRaw := "scrub repaired 0B in 00:01:30 with 0 errors on Tue Apr 28 18:30:00 2026"
	if st.Scan.RawLine != wantRaw {
		t.Errorf("RawLine=%q", st.Scan.RawLine)
	}
}

func TestParseScrubStatus_InProgress(t *testing.T) {
	line := "scan: scrub in progress since Tue Apr 28 18:30:00 2026, 1.50G scanned at 1.00G/s, 0B issued at 0B/s, 5.00G total"
	st, err := parseStatus(makeStatusInput(line))
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if st.Scan == nil {
		t.Fatal("Scan is nil")
	}
	if st.Scan.State != "in-progress" {
		t.Errorf("State=%q want %q", st.Scan.State, "in-progress")
	}
	wantScanned := uint64(1.50 * 1024 * 1024 * 1024)
	if st.Scan.ScannedBytes != wantScanned {
		t.Errorf("ScannedBytes=%d want %d", st.Scan.ScannedBytes, wantScanned)
	}
	wantTotal := uint64(5.00 * 1024 * 1024 * 1024)
	if st.Scan.TotalBytes != wantTotal {
		t.Errorf("TotalBytes=%d want %d", st.Scan.TotalBytes, wantTotal)
	}
}

func TestParseScrubStatus_Resilver(t *testing.T) {
	line := "scan: resilvered 752K in 00:00:01 with 0 errors on Tue Apr 28 18:28:47 2026"
	st, err := parseStatus(makeStatusInput(line))
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if st.Scan == nil {
		t.Fatal("Scan is nil")
	}
	if st.Scan.State != "resilver" {
		t.Errorf("State=%q want %q", st.Scan.State, "resilver")
	}
	wantRaw := "resilvered 752K in 00:00:01 with 0 errors on Tue Apr 28 18:28:47 2026"
	if st.Scan.RawLine != wantRaw {
		t.Errorf("RawLine=%q", st.Scan.RawLine)
	}
}

// Group headers (logs/cache/spares) appear in zpool status with only the
// group name on a line — no STATE column. The parser must accept these.
func TestParseStatus_LogsAndCacheGroups(t *testing.T) {
	// Real `zpool status -P` indent: one leading TAB plus 2 spaces per
	// nesting level. Pool root has no extra spaces, top-level vdevs
	// (mirror/log/cache) get 2 spaces, leaves get 4.
	in := []byte("  pool: tank\n state: ONLINE\nconfig:\n\n" +
		"\tNAME             STATE     READ WRITE CKSUM\n" +
		"\ttank             ONLINE       0     0     0\n" +
		"\t  mirror-0       ONLINE       0     0     0\n" +
		"\t    /dev/A       ONLINE       0     0     0\n" +
		"\t    /dev/B       ONLINE       0     0     0\n" +
		"\t  logs\n" +
		"\t    /dev/log1    ONLINE       0     0     0\n" +
		"\t  cache\n" +
		"\t    /dev/cache1  ONLINE       0     0     0\n" +
		"\nerrors: No known data errors\n")
	st, err := parseStatus(in)
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	// Top-level: mirror, logs, cache (3 entries)
	if len(st.Vdevs) != 3 {
		t.Fatalf("want 3 top-level vdevs, got %d: %+v", len(st.Vdevs), st.Vdevs)
	}
	types := []string{st.Vdevs[0].Type, st.Vdevs[1].Type, st.Vdevs[2].Type}
	wantTypes := []string{"mirror", "log", "cache"}
	for i, want := range wantTypes {
		if types[i] != want {
			t.Errorf("vdev[%d].Type=%q want %q", i, types[i], want)
		}
	}
	if len(st.Vdevs[1].Children) != 1 || st.Vdevs[1].Children[0].Path != "/dev/log1" {
		t.Errorf("log children=%+v", st.Vdevs[1].Children)
	}
	if len(st.Vdevs[2].Children) != 1 || st.Vdevs[2].Children[0].Path != "/dev/cache1" {
		t.Errorf("cache children=%+v", st.Vdevs[2].Children)
	}
}
