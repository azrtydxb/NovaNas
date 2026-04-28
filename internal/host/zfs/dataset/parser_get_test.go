package dataset

import (
	"os"
	"testing"
)

func TestParseProps(t *testing.T) {
	data, err := os.ReadFile("../../../../test/fixtures/zfs_get.txt")
	if err != nil {
		t.Fatal(err)
	}
	props, err := parseProps(data)
	if err != nil {
		t.Fatalf("parseProps: %v", err)
	}
	if props["compression"] != "lz4" {
		t.Errorf("compression=%q", props["compression"])
	}
	if props["mountpoint"] != "/tank/home" {
		t.Errorf("mountpoint=%q", props["mountpoint"])
	}
}
