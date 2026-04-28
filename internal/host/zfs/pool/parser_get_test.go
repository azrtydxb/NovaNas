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
