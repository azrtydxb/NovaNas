package dataset

import (
	"os"
	"testing"
)

func TestParseList(t *testing.T) {
	data, err := os.ReadFile("../../../../test/fixtures/zfs_list_datasets.txt")
	if err != nil {
		t.Fatal(err)
	}
	ds, err := parseList(data)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}
	if len(ds) != 3 {
		t.Fatalf("want 3, got %d", len(ds))
	}
	if ds[0].Name != "tank" || ds[0].Type != "filesystem" || ds[0].UsedBytes != 123456789 {
		t.Errorf("ds[0]=%+v", ds[0])
	}
	if ds[2].Type != "volume" || ds[2].Mountpoint != "" {
		t.Errorf("ds[2]=%+v", ds[2])
	}
}
