//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

func TestDataset_CreateGetDestroy(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
	loop1 := makeLoopback(t, 256<<20)
	pm := &pool.Manager{ZpoolBin: "/sbin/zpool"}
	name := uniquePoolName(t)
	ctx := context.Background()
	if err := pm.Create(ctx, pool.CreateSpec{
		Name:  name,
		Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: []string{loop1}}},
	}); err != nil {
		t.Fatalf("pool create: %v", err)
	}

	dm := &dataset.Manager{ZFSBin: "/sbin/zfs"}
	full := name + "/data"
	if err := dm.Create(ctx, dataset.CreateSpec{
		Parent:     name,
		Name:       "data",
		Type:       "filesystem",
		Properties: map[string]string{"compression": "lz4"},
	}); err != nil {
		t.Fatalf("dataset create: %v", err)
	}

	d, err := dm.Get(ctx, full)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.Props["compression"] != "lz4" {
		t.Errorf("compression=%q", d.Props["compression"])
	}

	if err := dm.SetProps(ctx, full, map[string]string{"compression": "zstd"}); err != nil {
		t.Fatalf("SetProps: %v", err)
	}
	d, _ = dm.Get(ctx, full)
	if d.Props["compression"] != "zstd" {
		t.Errorf("compression after set=%q", d.Props["compression"])
	}

	if err := dm.Destroy(ctx, full, false); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	_ = pm.Destroy(ctx, name)
}
