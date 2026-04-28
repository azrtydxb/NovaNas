//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

func TestPool_CreateListDestroy(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root for losetup/zpool")
	}
	loop1 := makeLoopback(t, 256<<20) // 256 MiB
	loop2 := makeLoopback(t, 256<<20)

	mgr := &pool.Manager{ZpoolBin: "/sbin/zpool"}
	name := uniquePoolName(t)

	ctx := context.Background()
	if err := mgr.Create(ctx, pool.CreateSpec{
		Name: name,
		Vdevs: []pool.VdevSpec{{
			Type:  "mirror",
			Disks: []string{loop1, loop2},
		}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	pools, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, p := range pools {
		if p.Name == name {
			found = true
			if p.Health != "ONLINE" {
				t.Errorf("health=%q", p.Health)
			}
		}
	}
	if !found {
		t.Fatalf("pool %s not in list", name)
	}

	d, err := mgr.Get(ctx, name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.Status == nil || d.Status.State != "ONLINE" {
		t.Errorf("status=%+v", d.Status)
	}

	if err := mgr.Destroy(ctx, name); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
}
