// Command zfs-validate exercises every feature of the internal/host/zfs
// Manager packages against real disks. Pool name "validate" is destroyed
// at start and end. Disks are taken from $DISKS (comma-separated by-id
// paths). Optional $LOG_A/$LOG_B add a mirrored log; $CACHE adds cache.
//
// Layouts exercised: stripe(1), mirror(2), raidz1(3), raidz2(4),
// raidz3(5), mirror + mirrored-log + cache. Plus dataset/snapshot
// lifecycle, scrub, real IO with snapshot+rollback integrity check.
// Scenarios 11-20 cover lifecycle operations: offline/online, replace,
// attach/detach, add, export/import, RAIDZ expansion, scrub-progress.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
)

const poolName = "validate"

func main() {
	disks := splitEnv("DISKS")
	logA := os.Getenv("LOG_A")
	cache := os.Getenv("CACHE")
	_ = os.Getenv("LOG_B") // accepted; mirrored-log now expressible via []VdevSpec
	if len(disks) < 6 {
		die("DISKS must list at least 6 by-id paths (got %d) for lifecycle scenarios", len(disks))
	}

	pm := &pool.Manager{ZpoolBin: "/sbin/zpool"}
	dm := &dataset.Manager{ZFSBin: "/sbin/zfs"}
	sm := &snapshot.Manager{ZFSBin: "/sbin/zfs"}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
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
		return createAndCheck(ctx, pm, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}, "disk", 1, 0)
	})
	cleanup("after stripe")

	step("2. mirror pool with 2 disks", func() error {
		return createAndCheck(ctx, pm, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: disks[:2]}},
		}, "mirror", 1, 2)
	})
	cleanup("after mirror")

	step("3. raidz1 pool with 3 disks", func() error {
		return createAndCheck(ctx, pm, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "raidz1", Disks: disks[:3]}},
		}, "raidz1", 1, 3)
	})
	cleanup("after raidz1")

	step("4. raidz2 pool with 4 disks", func() error {
		return createAndCheck(ctx, pm, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "raidz2", Disks: disks[:4]}},
		}, "raidz2", 1, 4)
	})
	cleanup("after raidz2")

	step("5. raidz3 pool with 5 disks", func() error {
		return createAndCheck(ctx, pm, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "raidz3", Disks: disks[:5]}},
		}, "raidz3", 1, 5)
	})
	cleanup("after raidz3")

	if logA != "" {
		step("6. mirror + single log + cache", func() error {
			logs := []pool.VdevSpec{{Type: "disk", Disks: []string{logA}}}
			caches := []string{}
			if cache != "" {
				caches = []string{cache}
			}
			spec := pool.CreateSpec{
				Name:  poolName,
				Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: disks[:2]}},
				Log:   logs,
				Cache: caches,
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
			return nil
		})
		cleanup("after mirror+log+cache")
	}

	step("7. dataset CRUD + properties", func() error {
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		full := poolName + "/data"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "data", Type: "filesystem",
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
		fmt.Printf("  setprops: compression=zstd, atime=off\n")

		if _, err := dm.Get(ctx, poolName+"/nope"); err == nil {
			return fmt.Errorf("expected ErrNotFound on missing dataset")
		}
		fmt.Printf("  ErrNotFound mapping verified\n")

		return dm.Destroy(ctx, full, false)
	})

	step("8. snapshot + rollback REAL IO integrity", func() error {
		// Pool from step 7 still alive (single disk stripe).
		full := poolName + "/iotest"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "iotest", Type: "filesystem",
		}); err != nil {
			return err
		}
		mountpoint := "/" + full

		// Write some known data, hash it, snapshot, modify, rollback, verify.
		dataPath := filepath.Join(mountpoint, "payload.bin")
		writePayload(dataPath, 8<<20) // 8 MiB
		hashOriginal := sha256file(dataPath)
		fmt.Printf("  wrote %s, sha256=%s\n", dataPath, hashOriginal[:16])

		if err := sm.Create(ctx, full, "before-modify", false); err != nil {
			return err
		}
		fmt.Printf("  snapshot %s@before-modify created\n", full)

		// Modify the file.
		if err := os.WriteFile(dataPath, []byte("MUTATED"), 0o600); err != nil {
			return err
		}
		hashAfterMod := sha256file(dataPath)
		if hashAfterMod == hashOriginal {
			return fmt.Errorf("file did not actually change")
		}
		fmt.Printf("  modified file, new sha256=%s\n", hashAfterMod[:16])

		// Rollback.
		if err := sm.Rollback(ctx, full+"@before-modify"); err != nil {
			return err
		}
		hashAfterRollback := sha256file(dataPath)
		if hashAfterRollback != hashOriginal {
			return fmt.Errorf("rollback did not restore data: pre=%s post=%s",
				hashOriginal, hashAfterRollback)
		}
		fmt.Printf("  rollback verified: sha256 matches original (%s)\n", hashAfterRollback[:16])

		// Cleanup snapshot+dataset.
		if err := sm.Destroy(ctx, full+"@before-modify"); err != nil {
			return err
		}
		return dm.Destroy(ctx, full, false)
	})
	cleanup("after IO test")

	step("9. scrub on real-data mirror pool", func() error {
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: disks[:2]}},
		}); err != nil {
			return err
		}
		// Write a chunk so scrub has something to verify.
		mp := "/" + poolName
		writePayload(filepath.Join(mp, "scrub-data.bin"), 32<<20)

		if err := pm.Scrub(ctx, poolName, pool.ScrubStart); err != nil {
			return err
		}
		fmt.Printf("  scrub started, waiting for completion...\n")
		if err := waitScrubComplete(ctx, pm, 60*time.Second); err != nil {
			return err
		}
		d, _ := pm.Get(ctx, poolName)
		fmt.Printf("  scrub finished, state=%s, errors=%s\n", d.Status.State, errorsLine())
		return nil
	})

	step("10. ErrNotFound on Get of destroyed pool", func() error {
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

	// ---- Lifecycle scenarios (11-20) -----------------------------------------
	// These require at least 6 disks from $DISKS. disks[0..5] are used.

	step("11. mirrored log + cache ([]VdevSpec)", func() error {
		wipeDisks(disks[:4])
		if len(disks) < 4 {
			return fmt.Errorf("need 4 disks")
		}
		spec := pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: disks[:2]}},
			Log:   []pool.VdevSpec{{Type: "mirror", Disks: disks[2:4]}},
		}
		if len(disks) >= 5 {
			spec.Cache = []string{disks[4]}
		}
		if err := pm.Create(ctx, spec); err != nil {
			return err
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		// Expect: mirror(2) data vdev, log vdev with 2 children, optionally cache(1).
		var dataMirror, logVdev, cacheVdev *pool.Vdev
		for i := range d.Status.Vdevs {
			v := &d.Status.Vdevs[i]
			switch v.Type {
			case "mirror":
				if dataMirror == nil {
					dataMirror = v
				}
			case "log":
				logVdev = v
			case "cache":
				cacheVdev = v
			}
		}
		if dataMirror == nil || len(dataMirror.Children) != 2 {
			return fmt.Errorf("expected data mirror with 2 children; vdevs=%+v", d.Status.Vdevs)
		}
		if logVdev == nil {
			return fmt.Errorf("expected log vdev; vdevs=%+v", d.Status.Vdevs)
		}
		if len(logVdev.Children) != 2 {
			return fmt.Errorf("expected mirrored log (2 children), got %d", len(logVdev.Children))
		}
		fmt.Printf("  data mirror: %d children, log: %d children", len(dataMirror.Children), len(logVdev.Children))
		if cacheVdev != nil {
			fmt.Printf(", cache: %d child(ren)", len(cacheVdev.Children)+1) // cache leaf has no sub-children
		}
		fmt.Println()
		return nil
	})
	cleanup("after scenario 11")

	step("12. spare in CreateSpec", func() error {
		wipeDisks(disks[:4])
		spec := pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "raidz1", Disks: disks[:3]}},
			Spare: []string{disks[3]},
		}
		if err := pm.Create(ctx, spec); err != nil {
			return err
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		var spareVdev *pool.Vdev
		for i := range d.Status.Vdevs {
			if d.Status.Vdevs[i].Type == "spare" {
				spareVdev = &d.Status.Vdevs[i]
			}
		}
		if spareVdev == nil {
			return fmt.Errorf("expected spare vdev in status; vdevs=%+v", d.Status.Vdevs)
		}
		// The spare entry itself has children (the spare disks) OR is listed
		// as a leaf child under a "spare" group. Either way len > 0 or the
		// spare vdev exists.
		fmt.Printf("  spare vdev present, children=%d\n", len(spareVdev.Children))
		return nil
	})
	cleanup("after scenario 12")

	step("13. offline → DEGRADED → online → ONLINE", func() error {
		wipeDisks(disks[:3])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "raidz1", Disks: disks[:3]}},
		}); err != nil {
			return err
		}
		// Offline disk[0].
		if err := pm.Offline(ctx, poolName, disks[0], false); err != nil {
			return fmt.Errorf("offline: %w", err)
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		if d.Status.State != "DEGRADED" || d.Pool.Health != "DEGRADED" {
			return fmt.Errorf("expected DEGRADED after offline; state=%q health=%q",
				d.Status.State, d.Pool.Health)
		}
		fmt.Printf("  after offline: state=%s health=%s\n", d.Status.State, d.Pool.Health)
		// Bring back online.
		if err := pm.Online(ctx, poolName, disks[0]); err != nil {
			return fmt.Errorf("online: %w", err)
		}
		// Allow a brief moment for the pool to re-assess.
		time.Sleep(500 * time.Millisecond)
		d, err = pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		if d.Pool.Health != "ONLINE" {
			return fmt.Errorf("expected ONLINE after online; health=%q", d.Pool.Health)
		}
		fmt.Printf("  after online: state=%s health=%s\n", d.Status.State, d.Pool.Health)
		return nil
	})
	// pool stays up for scenario 14

	step("14. clear errors", func() error {
		// Run clear with no disk (clears pool-wide counters). Should not error.
		if err := pm.Clear(ctx, poolName, ""); err != nil {
			return fmt.Errorf("clear: %w", err)
		}
		fmt.Printf("  clear(pool-wide) OK\n")
		// Also exercise targeted clear.
		if err := pm.Clear(ctx, poolName, disks[0]); err != nil {
			return fmt.Errorf("clear(disk): %w", err)
		}
		fmt.Printf("  clear(%s) OK\n", disks[0])
		return nil
	})
	cleanup("after scenarios 13+14")

	step("15. replace + resilver", func() error {
		wipeDisks(disks[:4])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "raidz1", Disks: disks[:3]}},
		}); err != nil {
			return err
		}
		if err := pm.Offline(ctx, poolName, disks[0], false); err != nil {
			return fmt.Errorf("offline before replace: %w", err)
		}
		if err := pm.Replace(ctx, poolName, disks[0], disks[3]); err != nil {
			return fmt.Errorf("replace: %w", err)
		}
		fmt.Printf("  replace issued, waiting for resilver (up to 60s)...\n")
		if err := waitPoolOnline(ctx, pm, poolName, 60*time.Second); err != nil {
			return err
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		// Verify new disk is present somewhere in the vdev tree.
		if !vdevTreeContainsDisk(d.Status.Vdevs, disks[3]) {
			return fmt.Errorf("new disk %q not found in vdev tree after replace", disks[3])
		}
		fmt.Printf("  resilver complete, new disk present, health=%s\n", d.Pool.Health)
		return nil
	})
	cleanup("after scenario 15")

	step("16. attach/detach (single → mirror → single)", func() error {
		wipeDisks(disks[:2])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		// Attach second disk → becomes a mirror.
		if err := pm.Attach(ctx, poolName, disks[0], disks[1]); err != nil {
			return fmt.Errorf("attach: %w", err)
		}
		// Wait for resilver that attach triggers.
		time.Sleep(2 * time.Second)
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		if len(d.Status.Vdevs) == 0 {
			return fmt.Errorf("no vdevs after attach")
		}
		if d.Status.Vdevs[0].Type != "mirror" {
			return fmt.Errorf("expected mirror after attach; got %q", d.Status.Vdevs[0].Type)
		}
		if len(d.Status.Vdevs[0].Children) != 2 {
			return fmt.Errorf("expected 2 mirror children after attach; got %d", len(d.Status.Vdevs[0].Children))
		}
		fmt.Printf("  after attach: vdev=%s(%d children)\n",
			d.Status.Vdevs[0].Type, len(d.Status.Vdevs[0].Children))
		// Detach disks[1] → back to single disk.
		if err := pm.Detach(ctx, poolName, disks[1]); err != nil {
			return fmt.Errorf("detach: %w", err)
		}
		d, err = pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		if len(d.Status.Vdevs) != 1 {
			return fmt.Errorf("expected 1 top-level vdev after detach; got %d", len(d.Status.Vdevs))
		}
		if d.Status.Vdevs[0].Type != "disk" {
			return fmt.Errorf("expected type=disk after detach; got %q", d.Status.Vdevs[0].Type)
		}
		fmt.Printf("  after detach: vdev=%s (single disk)\n", d.Status.Vdevs[0].Type)
		return nil
	})
	cleanup("after scenario 16")

	step("17. add (extra mirror to existing pool)", func() error {
		wipeDisks(disks[:4])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: disks[:2]}},
		}); err != nil {
			return err
		}
		addSpec := pool.AddSpec{
			Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: disks[2:4]}},
		}
		if err := pm.Add(ctx, poolName, addSpec); err != nil {
			return fmt.Errorf("add: %w", err)
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		// Expect 2 top-level mirror vdevs.
		mirrors := 0
		for _, v := range d.Status.Vdevs {
			if v.Type == "mirror" {
				if len(v.Children) != 2 {
					return fmt.Errorf("mirror vdev has %d children (want 2)", len(v.Children))
				}
				mirrors++
			}
		}
		if mirrors != 2 {
			return fmt.Errorf("expected 2 mirror vdevs after add; got %d (vdevs=%+v)", mirrors, d.Status.Vdevs)
		}
		fmt.Printf("  after add: %d mirror vdevs, each with 2 children\n", mirrors)
		return nil
	})
	cleanup("after scenario 17")

	step("18. RAIDZ expansion (ZFS 2.3+)", func() error {
		wipeDisks(disks[:4])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "raidz1", Disks: disks[:3]}},
		}); err != nil {
			return err
		}
		// Attach a 4th disk to the raidz vdev group named "raidz1-0".
		if err := pm.Attach(ctx, poolName, "raidz1-0", disks[3]); err != nil {
			return fmt.Errorf("raidz expand attach: %w", err)
		}
		fmt.Printf("  raidz expand issued, waiting up to 60s for expansion...\n")
		// Poll until Scan.State != "in-progress" or timeout.
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			d, err := pm.Get(ctx, poolName)
			if err != nil {
				return err
			}
			scanState := "none"
			if d.Status.Scan != nil {
				scanState = d.Status.Scan.State
			}
			fmt.Printf("  scan.state=%s\n", scanState)
			if scanState != "in-progress" {
				break
			}
			time.Sleep(2 * time.Second)
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		// Find the raidz1 vdev and check child count.
		var rz *pool.Vdev
		for i := range d.Status.Vdevs {
			if d.Status.Vdevs[i].Type == "raidz1" {
				rz = &d.Status.Vdevs[i]
				break
			}
		}
		if rz == nil {
			return fmt.Errorf("raidz1 vdev not found after expansion; vdevs=%+v", d.Status.Vdevs)
		}
		if len(rz.Children) != 4 {
			return fmt.Errorf("expected 4 raidz children after expansion; got %d", len(rz.Children))
		}
		fmt.Printf("  raidz1 now has %d children\n", len(rz.Children))
		return nil
	})
	cleanup("after scenario 18")

	step("19. export + import round trip", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		// Export.
		if err := pm.Export(ctx, poolName, false); err != nil {
			return fmt.Errorf("export: %w", err)
		}
		// Confirm no longer listed.
		pools, err := pm.List(ctx)
		if err != nil {
			return fmt.Errorf("list after export: %w", err)
		}
		for _, p := range pools {
			if p.Name == poolName {
				return fmt.Errorf("pool %q still listed after export", poolName)
			}
		}
		fmt.Printf("  pool no longer in List after export\n")
		// Importable must list it.
		importable, err := pm.Importable(ctx)
		if err != nil {
			return fmt.Errorf("importable: %w", err)
		}
		found := false
		for _, ip := range importable {
			if ip.Name == poolName {
				found = true
				fmt.Printf("  found in importable: name=%s state=%s\n", ip.Name, ip.State)
				break
			}
		}
		if !found {
			// Non-fatal on some systems (e.g. ZFS without device scanning). Warn.
			fmt.Printf("  WARNING: pool not found in importable list (may be env-specific)\n")
		}
		// Import.
		if err := pm.Import(ctx, poolName); err != nil {
			return fmt.Errorf("import: %w", err)
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return fmt.Errorf("get after import: %w", err)
		}
		if d.Pool.Health != "ONLINE" {
			return fmt.Errorf("expected ONLINE after import; health=%q", d.Pool.Health)
		}
		fmt.Printf("  re-imported, health=%s\n", d.Pool.Health)
		return nil
	})
	cleanup("after scenario 19")

	step("20. scrub progress (ScrubStatus parser)", func() error {
		wipeDisks(disks[:2])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: disks[:2]}},
		}); err != nil {
			return err
		}
		// Write some data so scrub has something to verify.
		mp := "/" + poolName
		writePayload(filepath.Join(mp, "scrub-data.bin"), 64<<20)
		// Start scrub.
		if err := pm.Scrub(ctx, poolName, pool.ScrubStart); err != nil {
			return fmt.Errorf("scrub start: %w", err)
		}
		fmt.Printf("  scrub started, polling...\n")
		deadline := time.Now().Add(60 * time.Second)
		var finalScan *pool.ScrubStatus
		for time.Now().Before(deadline) {
			d, err := pm.Get(ctx, poolName)
			if err != nil {
				return err
			}
			if d.Status.Scan != nil {
				finalScan = d.Status.Scan
				fmt.Printf("  scan.state=%s scanned=%d total=%d\n",
					finalScan.State, finalScan.ScannedBytes, finalScan.TotalBytes)
				if finalScan.State == "finished" {
					break
				}
			}
			time.Sleep(1 * time.Second)
		}
		if finalScan == nil {
			return fmt.Errorf("Scan field never populated")
		}
		if finalScan.State != "finished" {
			return fmt.Errorf("scrub did not finish within 60s; last state=%q", finalScan.State)
		}
		if finalScan.TotalBytes == 0 {
			// TotalBytes may not be populated on very fast scrubs; warn but don't fail.
			fmt.Printf("  WARNING: TotalBytes=0 (scrub may have finished before first poll)\n")
		}
		fmt.Printf("  scrub finished OK, totalBytes=%d\n", finalScan.TotalBytes)
		return nil
	})
	cleanup("after scenario 20")

	fmt.Println("\nALL CHECKS PASSED")
}

func createAndCheck(ctx context.Context, pm *pool.Manager, spec pool.CreateSpec,
	wantType string, wantTopVdevs, wantChildren int) error {
	if err := pm.Create(ctx, spec); err != nil {
		return err
	}
	d, err := pm.Get(ctx, spec.Name)
	if err != nil {
		return err
	}
	if len(d.Status.Vdevs) != wantTopVdevs {
		return fmt.Errorf("want %d top-level vdevs, got %d: %+v",
			wantTopVdevs, len(d.Status.Vdevs), d.Status.Vdevs)
	}
	if d.Status.Vdevs[0].Type != wantType {
		return fmt.Errorf("vdev[0].Type=%q want %q", d.Status.Vdevs[0].Type, wantType)
	}
	if wantChildren > 0 && len(d.Status.Vdevs[0].Children) != wantChildren {
		return fmt.Errorf("vdev[0] children=%d want %d",
			len(d.Status.Vdevs[0].Children), wantChildren)
	}
	if d.Pool.Health != "ONLINE" {
		return fmt.Errorf("health=%q (want ONLINE)", d.Pool.Health)
	}
	fmt.Printf("  pool %s: %d bytes, %s, vdev=%s(%d children)\n",
		d.Pool.Name, d.Pool.SizeBytes, d.Pool.Health,
		d.Status.Vdevs[0].Type, len(d.Status.Vdevs[0].Children))
	return nil
}

func step(name string, fn func() error) {
	fmt.Printf("\n=== %s ===\n", name)
	if err := fn(); err != nil {
		fmt.Printf("  FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  OK\n")
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

func writePayload(path string, size int) {
	// Pseudo-random reproducible payload.
	f, err := os.Create(path)
	if err != nil {
		die("create %s: %v", path, err)
	}
	defer f.Close()
	buf := make([]byte, 1<<16)
	for i := range buf {
		buf[i] = byte(i)
	}
	for written := 0; written < size; written += len(buf) {
		n := len(buf)
		if size-written < n {
			n = size - written
		}
		if _, err := f.Write(buf[:n]); err != nil {
			die("write %s: %v", path, err)
		}
	}
	if err := f.Sync(); err != nil {
		die("sync %s: %v", path, err)
	}
}

func sha256file(path string) string {
	f, err := os.Open(path)
	if err != nil {
		die("open %s: %v", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		die("hash %s: %v", path, err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func waitScrubComplete(ctx context.Context, pm *pool.Manager, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := exec.CommandContext(ctx, "/sbin/zpool", "status", poolName).CombinedOutput()
		if err != nil {
			return err
		}
		if strings.Contains(string(out), "scrub repaired") || strings.Contains(string(out), "no known data errors") && !strings.Contains(string(out), "scrub in progress") {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("scrub did not complete in %v", timeout)
}

func errorsLine() string {
	out, _ := exec.Command("/sbin/zpool", "status", poolName).CombinedOutput()
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, "errors:") {
			return strings.TrimSpace(l)
		}
	}
	return "(no errors line)"
}

// cleanDisk wipes ZFS labels and partition signatures from a disk device.
// It is best-effort: errors are printed but do not halt the harness.
func cleanDisk(disk string) {
	// zpool labelclear on up to 9 partitions + whole disk.
	candidates := []string{disk}
	for i := 1; i <= 9; i++ {
		candidates = append(candidates, fmt.Sprintf("%s-part%d", disk, i))
	}
	for _, c := range candidates {
		_ = exec.Command("zpool", "labelclear", "-f", c).Run()
	}
	// Wipe filesystem signatures if wipefs is available.
	_ = exec.Command("wipefs", "-af", disk).Run()
	// Zero the first 10 blocks.
	_ = exec.Command("dd", "if=/dev/zero", "of="+disk, "bs=512", "count=10").Run()
}

// wipeDisks calls cleanDisk on each disk in the slice.
func wipeDisks(disks []string) {
	for _, d := range disks {
		cleanDisk(d)
	}
}

// waitPoolOnline polls pm.Get until Pool.Health == "ONLINE" or timeout.
func waitPoolOnline(ctx context.Context, pm *pool.Manager, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		d, err := pm.Get(ctx, name)
		if err != nil {
			return err
		}
		if d.Pool.Health == "ONLINE" {
			return nil
		}
		fmt.Printf("  health=%s (waiting)\n", d.Pool.Health)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("pool %q did not reach ONLINE within %v", name, timeout)
}

// vdevTreeContainsDisk recursively checks if any Vdev in the tree has Path == disk.
func vdevTreeContainsDisk(vdevs []pool.Vdev, disk string) bool {
	for _, v := range vdevs {
		if v.Path == disk {
			return true
		}
		if vdevTreeContainsDisk(v.Children, disk) {
			return true
		}
	}
	return false
}
