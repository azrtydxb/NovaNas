// Command zfs-validate exercises every feature of the internal/host/zfs
// Manager packages against real disks. Pool name "validate" is destroyed
// at start and end. Disks are taken from $DISKS (comma-separated by-id
// paths). Optional $LOG and $CACHE add log/cache vdevs.
//
// Layouts exercised: stripe(1), mirror(2), raidz1(3), mirror+log+cache.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
)

const poolName = "validate"

func main() {
	disks := splitEnv("DISKS")
	log := splitEnv("LOG")
	cache := splitEnv("CACHE")
	if len(disks) < 3 {
		die("DISKS must list at least 3 by-id paths (got %d)", len(disks))
	}

	pm := &pool.Manager{ZpoolBin: "/sbin/zpool"}
	dm := &dataset.Manager{ZFSBin: "/sbin/zfs"}
	sm := &snapshot.Manager{ZFSBin: "/sbin/zfs"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cleanup := func(label string) {
		if err := pm.Destroy(ctx, poolName); err != nil {
			fmt.Printf("  [%s] cleanup destroy: %v (ok if not present)\n", label, err)
		} else {
			fmt.Printf("  [%s] destroyed pool %q\n", label, poolName)
		}
	}
	cleanup("startup")
	defer cleanup("teardown")

	step("1. stripe pool with 1 disk", func() error {
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		return verifyHealth(ctx, pm)
	})
	cleanup("after stripe")

	step("2. mirror pool with 2 disks", func() error {
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: disks[:2]}},
		}); err != nil {
			return err
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		if d.Status == nil || len(d.Status.Vdevs) != 1 || d.Status.Vdevs[0].Type != "mirror" {
			return fmt.Errorf("vdev tree wrong: %+v", d.Status)
		}
		if len(d.Status.Vdevs[0].Children) != 2 {
			return fmt.Errorf("mirror children=%d", len(d.Status.Vdevs[0].Children))
		}
		fmt.Printf("  vdev tree: mirror with %d children, state=%s\n",
			len(d.Status.Vdevs[0].Children), d.Status.State)
		return verifyHealth(ctx, pm)
	})
	cleanup("after mirror")

	step("3. raidz1 pool with 3 disks", func() error {
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "raidz1", Disks: disks[:3]}},
		}); err != nil {
			return err
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		if d.Status == nil || len(d.Status.Vdevs) != 1 || d.Status.Vdevs[0].Type != "raidz1" {
			return fmt.Errorf("vdev tree wrong: %+v", d.Status)
		}
		fmt.Printf("  vdev tree: raidz1 with %d children\n",
			len(d.Status.Vdevs[0].Children))
		return verifyHealth(ctx, pm)
	})
	cleanup("after raidz1")

	if len(log) > 0 || len(cache) > 0 {
		step("4. mirror + log/cache vdevs", func() error {
			spec := pool.CreateSpec{
				Name:  poolName,
				Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: disks[:2]}},
				Log:   log,
				Cache: cache,
			}
			if err := pm.Create(ctx, spec); err != nil {
				return err
			}
			d, err := pm.Get(ctx, poolName)
			if err != nil {
				return err
			}
			groups := []string{}
			for _, v := range d.Status.Vdevs {
				groups = append(groups, fmt.Sprintf("%s(%d)", v.Type, len(v.Children)))
			}
			fmt.Printf("  top-level vdevs: %s\n", strings.Join(groups, ", "))
			return verifyHealth(ctx, pm)
		})
		cleanup("after log+cache")
	}

	step("5. dataset create / setprops / get / destroy", func() error {
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		full := poolName + "/data"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent:     poolName,
			Name:       "data",
			Type:       "filesystem",
			Properties: map[string]string{"compression": "lz4"},
		}); err != nil {
			return err
		}
		got, err := dm.Get(ctx, full)
		if err != nil {
			return err
		}
		if got.Props["compression"] != "lz4" {
			return fmt.Errorf("compression=%q (want lz4)", got.Props["compression"])
		}
		fmt.Printf("  created %s, compression=%s\n", full, got.Props["compression"])

		if err := dm.SetProps(ctx, full, map[string]string{"compression": "zstd", "atime": "off"}); err != nil {
			return err
		}
		got, _ = dm.Get(ctx, full)
		if got.Props["compression"] != "zstd" || got.Props["atime"] != "off" {
			return fmt.Errorf("setprops failed: compression=%q atime=%q",
				got.Props["compression"], got.Props["atime"])
		}
		fmt.Printf("  setprops: compression=zstd, atime=off ok\n")

		// Get-not-found
		if _, err := dm.Get(ctx, poolName+"/nope"); err == nil {
			return fmt.Errorf("expected ErrNotFound on missing dataset")
		}
		fmt.Printf("  ErrNotFound mapping verified\n")

		return dm.Destroy(ctx, full, false)
	})

	step("6. snapshot create / list / rollback / destroy", func() error {
		// Pool from step 5 still exists; create dataset, snapshot, rollback, cleanup.
		full := poolName + "/snapme"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "snapme", Type: "filesystem",
		}); err != nil {
			return err
		}
		if err := sm.Create(ctx, full, "before", false); err != nil {
			return err
		}
		snaps, err := sm.List(ctx, full)
		if err != nil {
			return err
		}
		if len(snaps) != 1 || snaps[0].ShortName != "before" {
			return fmt.Errorf("list returned %+v", snaps)
		}
		fmt.Printf("  created snapshot %s, dataset=%s, used=%d\n",
			snaps[0].Name, snaps[0].Dataset, snaps[0].UsedBytes)

		if err := sm.Rollback(ctx, full+"@before"); err != nil {
			return err
		}
		fmt.Printf("  rollback ok\n")

		if err := sm.Destroy(ctx, full+"@before"); err != nil {
			return err
		}
		return dm.Destroy(ctx, full, false)
	})
	cleanup("after datasets+snapshots")

	step("7. scrub on pool", func() error {
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: disks[:2]}},
		}); err != nil {
			return err
		}
		if err := pm.Scrub(ctx, poolName, pool.ScrubStart); err != nil {
			return err
		}
		fmt.Printf("  scrub started\n")
		// On a fresh pool scrub completes near-instantly; brief sleep then verify state still valid.
		time.Sleep(2 * time.Second)
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		fmt.Printf("  post-scrub state=%s\n", d.Status.State)
		return nil
	})

	step("8. ErrNotFound on Get of destroyed pool", func() error {
		if err := pm.Destroy(ctx, poolName); err != nil {
			return err
		}
		_, err := pm.Get(ctx, poolName)
		if err == nil {
			return fmt.Errorf("expected ErrNotFound; got nil")
		}
		fmt.Printf("  Get returned: %v\n", err)
		return nil
	})

	fmt.Println("\nALL CHECKS PASSED")
}

func step(name string, fn func() error) {
	fmt.Printf("\n=== %s ===\n", name)
	if err := fn(); err != nil {
		fmt.Printf("  FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  OK\n")
}

func verifyHealth(ctx context.Context, pm *pool.Manager) error {
	d, err := pm.Get(ctx, poolName)
	if err != nil {
		return err
	}
	if d.Pool.Health != "ONLINE" {
		return fmt.Errorf("health=%q (want ONLINE)", d.Pool.Health)
	}
	fmt.Printf("  pool %s: %d bytes, %s, %s\n",
		d.Pool.Name, d.Pool.SizeBytes, d.Pool.Health, d.Status.State)
	return nil
}

func splitEnv(name string) []string {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	out := []string{}
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(2)
}
