package pool

import (
	"os"
	"testing"
)

func TestParseList(t *testing.T) {
	data, err := os.ReadFile("../../../../test/fixtures/zpool_list.txt")
	if err != nil {
		t.Fatal(err)
	}
	pools, err := parseList(data)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}
	if len(pools) != 2 {
		t.Fatalf("want 2, got %d", len(pools))
	}
	tank := pools[0]
	if tank.Name != "tank" || tank.SizeBytes != 1000204886016 || tank.Health != "ONLINE" {
		t.Errorf("tank=%+v", tank)
	}
	if tank.Allocated != 123456789012 || tank.Free != 876748097004 {
		t.Errorf("tank alloc/free=%+v", tank)
	}
}
