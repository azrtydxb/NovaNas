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

// Group headers (logs/cache/spares) appear in zpool status with only the
// group name on a line — no STATE column. The parser must accept these.
func TestParseStatus_LogsAndCacheGroups(t *testing.T) {
	// Note: parser counts leading TABS for depth. Visual spaces inside the
	// row are ignored. Group headers (logs/cache) sit at depth=1 alongside
	// data vdevs; their leaves are at depth=2.
	in := []byte("  pool: tank\n state: ONLINE\nconfig:\n\n" +
		"\tNAME             STATE     READ WRITE CKSUM\n" +
		"\ttank             ONLINE       0     0     0\n" +
		"\tmirror-0         ONLINE       0     0     0\n" +
		"\t\t/dev/A         ONLINE       0     0     0\n" +
		"\t\t/dev/B         ONLINE       0     0     0\n" +
		"\tlogs\n" +
		"\t\t/dev/log1      ONLINE       0     0     0\n" +
		"\tcache\n" +
		"\t\t/dev/cache1    ONLINE       0     0     0\n" +
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
