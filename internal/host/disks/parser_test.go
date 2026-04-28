package disks

import (
	"os"
	"testing"
)

func TestParseLsblk(t *testing.T) {
	data, err := os.ReadFile("../../../test/fixtures/lsblk.json")
	if err != nil {
		t.Fatal(err)
	}
	disks, err := parseLsblk(data)
	if err != nil {
		t.Fatalf("parseLsblk: %v", err)
	}
	if len(disks) != 2 {
		t.Fatalf("want 2 disks, got %d", len(disks))
	}
	sda := disks[0]
	if sda.Name != "sda" || sda.SizeBytes != 1000204886016 || !sda.Rotational {
		t.Errorf("sda parsed wrong: %+v", sda)
	}
	if !sda.InUseByPool {
		t.Errorf("sda has zfs_member child; should be InUseByPool")
	}
	sdb := disks[1]
	if sdb.Rotational || sdb.InUseByPool {
		t.Errorf("sdb parsed wrong: %+v", sdb)
	}
}
