//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
)

func TestSnapshot_CreateRollbackDestroy(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
	loop1 := makeLoopback(t, 256<<20)
	pm := &pool.Manager{ZpoolBin: "/sbin/zpool"}
	dm := &dataset.Manager{ZFSBin: "/sbin/zfs"}
	sm := &snapshot.Manager{ZFSBin: "/sbin/zfs"}
	name := uniquePoolName(t)
	ctx := context.Background()
	if err := pm.Create(ctx, pool.CreateSpec{
		Name:  name,
		Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: []string{loop1}}},
	}); err != nil {
		t.Fatalf("pool create: %v", err)
	}
	defer pm.Destroy(ctx, name)

	full := name + "/data"
	if err := dm.Create(ctx, dataset.CreateSpec{Parent: name, Name: "data", Type: "filesystem"}); err != nil {
		t.Fatalf("dataset create: %v", err)
	}

	if err := sm.Create(ctx, full, "snap1", false); err != nil {
		t.Fatalf("snapshot create: %v", err)
	}
	snaps, err := sm.List(ctx, full)
	if err != nil {
		t.Fatalf("snapshot list: %v", err)
	}
	if len(snaps) != 1 || snaps[0].ShortName != "snap1" {
		t.Errorf("snaps=%+v", snaps)
	}

	if err := sm.Rollback(ctx, full+"@snap1"); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if err := sm.Destroy(ctx, full+"@snap1"); err != nil {
		t.Fatalf("snapshot destroy: %v", err)
	}
	_ = dm.Destroy(ctx, full, false)
}
