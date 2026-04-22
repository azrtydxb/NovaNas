package tests

import (
	"testing"

	"github.com/azrtydxb/novanas/installer/internal/disks"
)

const lsblkFixture = `{
  "blockdevices": [
    {"name":"sda","size":2000398934016,"model":"Samsung SSD 870","serial":"S123","type":"disk","rota":false,"tran":"sata","rm":false},
    {"name":"sdb","size":7814037168128,"model":"WD Red","serial":"W456","type":"disk","rota":true,"tran":"sata","rm":false},
    {"name":"sdc","size":16008609792,"model":"USB Stick","serial":"U789","type":"disk","rota":false,"tran":"usb","rm":true},
    {"name":"loop0","size":4096,"model":null,"serial":null,"type":"loop","rota":false,"tran":null,"rm":false},
    {"name":"sr0","size":0,"model":"DVD","serial":null,"type":"rom","rota":true,"tran":"sata","rm":true},
    {"name":"sdd","size":8000000000,"model":"Tiny","serial":"T0","type":"disk","rota":true,"tran":"sata","rm":false}
  ]
}`

func TestParseLsblkFiltersCandidates(t *testing.T) {
	got, err := disks.ParseLsblk([]byte(lsblkFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 candidate disks (sda, sdb); got %d: %+v", len(got), got)
	}
	names := map[string]bool{}
	for _, d := range got {
		names[d.Name] = true
		if d.Path == "" || d.Path[:5] != "/dev/" {
			t.Errorf("disk %s missing /dev/ path: %q", d.Name, d.Path)
		}
	}
	if !names["sda"] || !names["sdb"] {
		t.Errorf("missing expected sda/sdb: %+v", names)
	}
	if names["sdc"] {
		t.Error("removable USB sdc should have been filtered out")
	}
	if names["sdd"] {
		t.Error("sub-16GB sdd should have been filtered out")
	}
	if names["loop0"] || names["sr0"] {
		t.Error("loop/rom should have been filtered out")
	}
}

func TestPartNameNVMe(t *testing.T) {
	cases := map[string]string{
		"/dev/sda":      "/dev/sda1",
		"/dev/nvme0n1":  "/dev/nvme0n1p1",
		"/dev/md0":      "/dev/md0p1",
	}
	for dev, want := range cases {
		if got := disks.PartName(dev, 1); got != want {
			t.Errorf("PartName(%q,1) = %q, want %q", dev, got, want)
		}
	}
}

func TestBuildPartitionPlan(t *testing.T) {
	plan := disks.BuildPartitionPlan("/dev/sda", disks.DefaultLayout())
	if plan.Device != "/dev/sda" {
		t.Errorf("device = %q", plan.Device)
	}
	if len(plan.Commands) == 0 {
		t.Fatal("no commands")
	}
	if plan.Commands[0][0] != "parted" {
		t.Errorf("first cmd = %v, want parted", plan.Commands[0])
	}
	// Expect 1 parted + 5 mkfs.
	if len(plan.Commands) != 6 {
		t.Errorf("expected 6 commands, got %d", len(plan.Commands))
	}
}

func TestBuildMirrorPlanRequiresTwo(t *testing.T) {
	if _, err := disks.BuildMirrorPlan("/dev/md0", []string{"/dev/sda"}); err == nil {
		t.Error("expected error for single disk")
	}
	plan, err := disks.BuildMirrorPlan("/dev/md0", []string{"/dev/sda", "/dev/sdb"})
	if err != nil {
		t.Fatalf("mirror plan: %v", err)
	}
	if plan.MDDevice != "/dev/md0" {
		t.Errorf("md device = %q", plan.MDDevice)
	}
	if len(plan.Commands) != 3 {
		t.Errorf("expected 2 zero-superblock + 1 create = 3 cmds, got %d", len(plan.Commands))
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		500:            "500 B",
		1500:           "1.5 kB",
		2_000_000:      "2.0 MB",
		2_000_000_000:  "2.0 GB",
	}
	for in, want := range cases {
		if got := disks.HumanSize(in); got != want {
			t.Errorf("HumanSize(%d) = %q, want %q", in, got, want)
		}
	}
}
