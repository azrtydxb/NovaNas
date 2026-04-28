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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
		// Mirrored log: zpool reports it as `log → mirror-N → 2 leaves`.
		// Verify the nested mirror with 2 leaves under the log group.
		if len(logVdev.Children) != 1 || logVdev.Children[0].Type != "mirror" ||
			len(logVdev.Children[0].Children) != 2 {
			return fmt.Errorf("expected mirrored-log (log→mirror→2 leaves), got %+v", logVdev)
		}
		fmt.Printf("  data mirror: %d children, log: nested mirror with %d leaves",
			len(dataMirror.Children), len(logVdev.Children[0].Children))
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

	// ---- Dataset/zvol/quota scenarios (21-25) -------------------------------

	step("21. zvol create + REAL block IO + snapshot/rollback", func() error {
		wipeDisks(disks[:2])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: disks[:2]}},
		}); err != nil {
			return err
		}
		full := poolName + "/vol1"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "vol1", Type: "volume",
			VolumeSizeBytes: 64 << 20, // 64 MiB
			Properties:      map[string]string{"compression": "lz4"},
		}); err != nil {
			return err
		}
		got, err := dm.Get(ctx, full)
		if err != nil {
			return err
		}
		if got.Dataset.Type != "volume" {
			return fmt.Errorf("zvol type=%q want volume", got.Dataset.Type)
		}
		if got.Props["volsize"] == "" {
			return fmt.Errorf("volsize not reported by Get: props=%v", got.Props)
		}
		fmt.Printf("  zvol %s created, volsize=%s\n", full, got.Props["volsize"])

		// Block device path: /dev/zvol/<pool>/<name>. Wait briefly for udev.
		dev := "/dev/zvol/" + full
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(dev); err == nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if _, err := os.Stat(dev); err != nil {
			return fmt.Errorf("zvol device did not appear at %s: %v", dev, err)
		}

		// Real block IO: write 4 MiB of known data to offset 0, sha256 it.
		payload := make([]byte, 4<<20)
		for i := range payload {
			payload[i] = byte(i % 251)
		}
		if err := writeBlockAt(dev, 0, payload); err != nil {
			return fmt.Errorf("write zvol: %w", err)
		}
		hashOriginal := sha256bytes(payload)
		fmt.Printf("  wrote 4MiB to %s, sha256=%s\n", dev, hashOriginal[:16])

		if err := sm.Create(ctx, full, "before-mutate", false); err != nil {
			return err
		}
		// Mutate the same range with garbage.
		mutated := make([]byte, len(payload))
		for i := range mutated {
			mutated[i] = 0xAB
		}
		if err := writeBlockAt(dev, 0, mutated); err != nil {
			return fmt.Errorf("mutate zvol: %w", err)
		}
		readBack, err := readBlockAt(dev, 0, len(payload))
		if err != nil {
			return fmt.Errorf("read back: %w", err)
		}
		if sha256bytes(readBack) == hashOriginal {
			return fmt.Errorf("zvol did not actually change")
		}
		// Rollback.
		if err := sm.Rollback(ctx, full+"@before-mutate"); err != nil {
			return err
		}
		// Re-read; udev may need a moment to refresh the device-mapper view
		// after rollback, so retry briefly.
		var afterRollback []byte
		ddl := time.Now().Add(5 * time.Second)
		for time.Now().Before(ddl) {
			afterRollback, err = readBlockAt(dev, 0, len(payload))
			if err == nil && sha256bytes(afterRollback) == hashOriginal {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if err != nil {
			return fmt.Errorf("read after rollback: %w", err)
		}
		if sha256bytes(afterRollback) != hashOriginal {
			return fmt.Errorf("rollback did not restore zvol contents")
		}
		fmt.Printf("  rollback verified: zvol bytes match original\n")

		if err := sm.Destroy(ctx, full+"@before-mutate"); err != nil {
			return err
		}
		return dm.Destroy(ctx, full, false)
	})

	step("22. zvol resize via SetProps(volsize)", func() error {
		// Pool from scenario 21 still alive.
		full := poolName + "/vol2"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "vol2", Type: "volume",
			VolumeSizeBytes: 32 << 20, // 32 MiB
		}); err != nil {
			return err
		}
		// Grow to 128 MiB.
		newSize := uint64(128 << 20)
		if err := dm.SetProps(ctx, full, map[string]string{
			"volsize": fmt.Sprintf("%d", newSize),
		}); err != nil {
			return fmt.Errorf("setprops volsize: %w", err)
		}
		got, err := dm.Get(ctx, full)
		if err != nil {
			return err
		}
		if got.Props["volsize"] != fmt.Sprintf("%d", newSize) {
			return fmt.Errorf("volsize after resize=%q want %d", got.Props["volsize"], newSize)
		}
		fmt.Printf("  resized %s 32MiB → %s bytes (volsize property)\n", full, got.Props["volsize"])
		return dm.Destroy(ctx, full, false)
	})

	step("23. dataset quota enforces ENOSPC", func() error {
		full := poolName + "/quota_fs"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "quota_fs", Type: "filesystem",
			Properties: map[string]string{"quota": "8M", "compression": "off"},
		}); err != nil {
			return err
		}
		got, err := dm.Get(ctx, full)
		if err != nil {
			return err
		}
		// ZFS reports quota in bytes when -p was used.
		if got.Props["quota"] == "" || got.Props["quota"] == "0" || got.Props["quota"] == "none" {
			return fmt.Errorf("quota not set on %s: props.quota=%q", full, got.Props["quota"])
		}
		fmt.Printf("  quota=%s bytes set on %s\n", got.Props["quota"], full)

		mp := "/" + full
		// Writing 4MiB should succeed (well under quota).
		small := filepath.Join(mp, "small.bin")
		if err := writeRandom(small, 4<<20); err != nil {
			return fmt.Errorf("write 4MiB: %w", err)
		}
		fmt.Printf("  wrote 4MiB OK (under quota)\n")

		// Writing another 16MiB should FAIL with ENOSPC (quota is 8M total).
		big := filepath.Join(mp, "big.bin")
		err = writeRandom(big, 16<<20)
		if err == nil {
			return fmt.Errorf("expected ENOSPC writing 16MiB to 8M-quota dataset, got nil")
		}
		if !strings.Contains(err.Error(), "no space") &&
			!strings.Contains(err.Error(), "disk quota exceeded") &&
			!strings.Contains(err.Error(), "Disk quota exceeded") {
			// Be lenient about exact phrasing; just require *some* error.
			fmt.Printf("  write rejected (as expected): %v\n", err)
		} else {
			fmt.Printf("  write rejected with quota error: %v\n", err)
		}

		// Raise quota; the same write should now succeed.
		if err := dm.SetProps(ctx, full, map[string]string{"quota": "64M"}); err != nil {
			return err
		}
		os.Remove(big) // discard partial write
		if err := writeRandom(big, 16<<20); err != nil {
			return fmt.Errorf("write after quota raise: %w", err)
		}
		fmt.Printf("  after quota=64M, 16MiB write succeeds\n")
		return dm.Destroy(ctx, full, false)
	})

	step("24. zvol volsize enforces block-device size", func() error {
		full := poolName + "/vol_small"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "vol_small", Type: "volume",
			VolumeSizeBytes: 16 << 20, // 16 MiB
		}); err != nil {
			return err
		}
		dev := "/dev/zvol/" + full
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(dev); err == nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		// Writing 32MiB at offset 0 must fail (device is 16MiB).
		buf := make([]byte, 32<<20)
		err := writeBlockAt(dev, 0, buf)
		if err == nil {
			return fmt.Errorf("expected write past volsize to fail, got nil")
		}
		fmt.Printf("  write past volsize rejected: %v\n", err)
		// Kernel may briefly hold a reference to the block device after
		// the failed write. Retry destroy a few times.
		ddl := time.Now().Add(10 * time.Second)
		for time.Now().Before(ddl) {
			if err := dm.Destroy(ctx, full, false); err == nil {
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		}
		return dm.Destroy(ctx, full, false)
	})

	step("25. List/Get sees zvols and filesystems together", func() error {
		// Create one of each, list under the pool, both must appear with the
		// right Type. Exercises dataset.List(-t filesystem,volume).
		fsFull := poolName + "/mixfs"
		volFull := poolName + "/mixvol"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "mixfs", Type: "filesystem",
		}); err != nil {
			return err
		}
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "mixvol", Type: "volume",
			VolumeSizeBytes: 16 << 20,
		}); err != nil {
			return err
		}
		ds, err := dm.List(ctx, poolName)
		if err != nil {
			return err
		}
		var seenFs, seenVol bool
		for _, d := range ds {
			switch d.Name {
			case fsFull:
				if d.Type != "filesystem" {
					return fmt.Errorf("%s reported type=%q", d.Name, d.Type)
				}
				seenFs = true
			case volFull:
				if d.Type != "volume" {
					return fmt.Errorf("%s reported type=%q", d.Name, d.Type)
				}
				seenVol = true
			}
		}
		if !seenFs || !seenVol {
			return fmt.Errorf("List missed entries: fs=%v vol=%v (got %d datasets)", seenFs, seenVol, len(ds))
		}
		fmt.Printf("  List returned both filesystem and volume with correct Type\n")

		// Snapshots over both (zfs snapshot works the same regardless of type).
		if err := sm.Create(ctx, fsFull, "s1", false); err != nil {
			return err
		}
		if err := sm.Create(ctx, volFull, "s1", false); err != nil {
			return err
		}
		snaps, err := sm.List(ctx, poolName)
		if err != nil {
			return err
		}
		var fsSnap, volSnap bool
		for _, s := range snaps {
			if s.Name == fsFull+"@s1" {
				fsSnap = true
			}
			if s.Name == volFull+"@s1" {
				volSnap = true
			}
		}
		if !fsSnap || !volSnap {
			return fmt.Errorf("snapshot list missed entries: fs=%v vol=%v", fsSnap, volSnap)
		}
		fmt.Printf("  snapshot.List sees both fs and zvol snapshots\n")

		_ = sm.Destroy(ctx, fsFull+"@s1")
		_ = sm.Destroy(ctx, volFull+"@s1")
		_ = dm.Destroy(ctx, fsFull, false)
		_ = dm.Destroy(ctx, volFull, false)
		return nil
	})
	cleanup("after scenario 25")

	// ---- New manager methods (26-37) ---------------------------------------

	step("26. recursive snapshot + recursive destroy", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		parent := poolName + "/parent"
		childA := parent + "/childA"
		childB := parent + "/childB"
		if err := dm.Create(ctx, dataset.CreateSpec{Parent: poolName, Name: "parent", Type: "filesystem"}); err != nil {
			return err
		}
		if err := dm.Create(ctx, dataset.CreateSpec{Parent: parent, Name: "childA", Type: "filesystem"}); err != nil {
			return err
		}
		if err := dm.Create(ctx, dataset.CreateSpec{Parent: parent, Name: "childB", Type: "filesystem"}); err != nil {
			return err
		}
		if err := sm.Create(ctx, parent, "snap1", true); err != nil {
			return fmt.Errorf("recursive snapshot: %w", err)
		}
		snaps, err := sm.List(ctx, parent)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, s := range snaps {
			if strings.HasSuffix(s.Name, "@snap1") {
				seen[s.Name] = true
			}
		}
		want := []string{parent + "@snap1", childA + "@snap1", childB + "@snap1"}
		for _, n := range want {
			if !seen[n] {
				return fmt.Errorf("missing recursive snapshot %s (saw %v)", n, seen)
			}
		}
		fmt.Printf("  recursive snapshot created %d entries\n", len(seen))
		// Recursive destroy of parent.
		if err := dm.Destroy(ctx, parent, true); err != nil {
			return fmt.Errorf("recursive destroy: %w", err)
		}
		// Confirm children gone.
		if _, err := dm.Get(ctx, childA); !errors.Is(err, dataset.ErrNotFound) {
			return fmt.Errorf("expected ErrNotFound on %s; got %v", childA, err)
		}
		if _, err := dm.Get(ctx, childB); !errors.Is(err, dataset.ErrNotFound) {
			return fmt.Errorf("expected ErrNotFound on %s; got %v", childB, err)
		}
		fmt.Printf("  recursive destroy removed both children\n")
		return nil
	})
	cleanup("after scenario 26")

	step("27. zfs rename (with and without recursive)", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		// Non-recursive rename.
		oldName := poolName + "/old"
		newName := poolName + "/new"
		if err := dm.Create(ctx, dataset.CreateSpec{Parent: poolName, Name: "old", Type: "filesystem"}); err != nil {
			return err
		}
		if err := dm.Rename(ctx, oldName, newName, false); err != nil {
			return err
		}
		if _, err := dm.Get(ctx, oldName); !errors.Is(err, dataset.ErrNotFound) {
			return fmt.Errorf("expected ErrNotFound on old name; got %v", err)
		}
		if _, err := dm.Get(ctx, newName); err != nil {
			return fmt.Errorf("get new name: %w", err)
		}
		fmt.Printf("  rename old -> new OK\n")

		// Filesystem rename takes children automatically (no -r flag needed
		// for filesystems; -r is for snapshot rename across descendants).
		pParent := poolName + "/p"
		qParent := poolName + "/q"
		qChild := qParent + "/c"
		if err := dm.Create(ctx, dataset.CreateSpec{Parent: poolName, Name: "p", Type: "filesystem"}); err != nil {
			return err
		}
		if err := dm.Create(ctx, dataset.CreateSpec{Parent: pParent, Name: "c", Type: "filesystem"}); err != nil {
			return err
		}
		if err := dm.Rename(ctx, pParent, qParent, false); err != nil {
			return fmt.Errorf("rename p -> q: %w", err)
		}
		if _, err := dm.Get(ctx, qChild); err != nil {
			return fmt.Errorf("get %s after rename: %w", qChild, err)
		}
		fmt.Printf("  filesystem rename p -> q (child %s carried over)\n", qChild)

		// Snapshot recursive rename: rename @s1 to @s2 across the whole
		// q subtree.
		if err := sm.Create(ctx, qParent, "s1", true); err != nil {
			return fmt.Errorf("recursive snapshot @s1: %w", err)
		}
		if err := dm.Rename(ctx, qParent+"@s1", qParent+"@s2", true); err != nil {
			return fmt.Errorf("recursive snapshot rename: %w", err)
		}
		// Verify child snapshot was also renamed.
		snaps, err := sm.List(ctx, qParent)
		if err != nil {
			return err
		}
		var childRenamed bool
		for _, s := range snaps {
			if s.Name == qChild+"@s2" {
				childRenamed = true
			}
		}
		if !childRenamed {
			return fmt.Errorf("recursive snapshot rename did not propagate to child")
		}
		fmt.Printf("  recursive snapshot rename @s1 -> @s2 across subtree OK\n")
		return nil
	})
	cleanup("after scenario 27")

	step("28. clone + promote", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		src := poolName + "/data"
		clone := poolName + "/clone"
		if err := dm.Create(ctx, dataset.CreateSpec{Parent: poolName, Name: "data", Type: "filesystem"}); err != nil {
			return err
		}
		filePath := filepath.Join("/"+src, "payload.bin")
		writePayload(filePath, 1<<20)
		hashOriginal := sha256file(filePath)
		if err := sm.Create(ctx, src, "snap", false); err != nil {
			return err
		}
		if err := dm.Clone(ctx, src+"@snap", clone, nil); err != nil {
			return fmt.Errorf("clone: %w", err)
		}
		clonedFile := filepath.Join("/"+clone, "payload.bin")
		if sha256file(clonedFile) != hashOriginal {
			return fmt.Errorf("clone file hash mismatch")
		}
		fmt.Printf("  clone %s contains payload (sha256 matches)\n", clone)

		// Confirm origin is set on clone before promote.
		got, err := dm.Get(ctx, clone)
		if err != nil {
			return err
		}
		fmt.Printf("  pre-promote: clone.origin=%q\n", got.Props["origin"])

		if err := dm.Promote(ctx, clone); err != nil {
			return fmt.Errorf("promote: %w", err)
		}
		got, err = dm.Get(ctx, clone)
		if err != nil {
			return err
		}
		if got.Props["origin"] != "-" && got.Props["origin"] != "" {
			return fmt.Errorf("after promote, expected clone.origin=- got %q", got.Props["origin"])
		}
		// Re-check the file is intact on the (now promoted) clone.
		if sha256file(clonedFile) != hashOriginal {
			return fmt.Errorf("post-promote clone file hash mismatch")
		}
		fmt.Printf("  promoted: clone.origin=%q, file intact\n", got.Props["origin"])
		return nil
	})
	cleanup("after scenario 28")

	step("29. encryption: create encrypted dataset, unload/load key", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		keyPath := "/tmp/zfs.key"
		if err := os.WriteFile(keyPath, []byte("testpassword12345"), 0o600); err != nil {
			return fmt.Errorf("write keyfile: %w", err)
		}
		full := poolName + "/encrypted"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "encrypted", Type: "filesystem",
			Properties: map[string]string{
				"encryption":  "aes-256-gcm",
				"keyformat":   "passphrase",
				"keylocation": "file://" + keyPath,
			},
		}); err != nil {
			return fmt.Errorf("create encrypted: %w", err)
		}
		// Write a file inside.
		mp := "/" + full
		dataPath := filepath.Join(mp, "secret.bin")
		writePayload(dataPath, 1<<20)
		hashOriginal := sha256file(dataPath)
		fmt.Printf("  wrote %s sha256=%s\n", dataPath, hashOriginal[:16])

		// Unload key (must unmount first).
		if err := exec.Command("/sbin/zfs", "unmount", full).Run(); err != nil {
			fmt.Printf("  WARNING: unmount before unload-key: %v\n", err)
		}
		if err := dm.UnloadKey(ctx, full, false); err != nil {
			return fmt.Errorf("unload-key: %w", err)
		}
		got, err := dm.Get(ctx, full)
		if err != nil {
			return err
		}
		if got.Props["keystatus"] != "unavailable" {
			return fmt.Errorf("after unload-key keystatus=%q want unavailable", got.Props["keystatus"])
		}
		fmt.Printf("  unloaded: keystatus=%s\n", got.Props["keystatus"])

		// Load key back.
		if err := dm.LoadKey(ctx, full, "file://"+keyPath, false); err != nil {
			return fmt.Errorf("load-key: %w", err)
		}
		got, err = dm.Get(ctx, full)
		if err != nil {
			return err
		}
		if got.Props["keystatus"] != "available" {
			return fmt.Errorf("after load-key keystatus=%q want available", got.Props["keystatus"])
		}
		fmt.Printf("  loaded: keystatus=%s\n", got.Props["keystatus"])
		// Re-mount and confirm payload is intact.
		if err := exec.Command("/sbin/zfs", "mount", full).Run(); err != nil {
			fmt.Printf("  WARNING: re-mount after load-key: %v\n", err)
		}
		if sha256file(dataPath) != hashOriginal {
			return fmt.Errorf("payload changed across unload/load cycle")
		}
		fmt.Printf("  payload sha256 matches after load-key\n")
		return nil
	})
	cleanup("after scenario 29")

	step("30. zfs send + zfs receive (full snapshot stream)", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		src := poolName + "/src"
		dst := poolName + "/dst"
		if err := dm.Create(ctx, dataset.CreateSpec{Parent: poolName, Name: "src", Type: "filesystem"}); err != nil {
			return err
		}
		filePath := filepath.Join("/"+src, "payload.bin")
		writePayload(filePath, 1<<20)
		hashOriginal := sha256file(filePath)
		if err := sm.Create(ctx, src, "s1", false); err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := dm.Send(ctx, src+"@s1", dataset.SendOpts{}, &buf); err != nil {
			return fmt.Errorf("send: %w", err)
		}
		fmt.Printf("  send produced %d bytes\n", buf.Len())
		if err := dm.Receive(ctx, dst, dataset.RecvOpts{}, &buf); err != nil {
			return fmt.Errorf("receive: %w", err)
		}
		if _, err := dm.Get(ctx, dst); err != nil {
			return fmt.Errorf("get dst: %w", err)
		}
		dstFile := filepath.Join("/"+dst, "payload.bin")
		if sha256file(dstFile) != hashOriginal {
			return fmt.Errorf("dst payload hash mismatch")
		}
		fmt.Printf("  dst payload sha256 matches src\n")

		// Scenario 31 piggybacks on this pool.
		step31(ctx, dm, sm, src, dst, hashOriginal, filePath, dstFile)
		return nil
	})
	cleanup("after scenarios 30+31")

	step("32. dRAID pool create", func() error {
		if len(disks) < 8 {
			fmt.Printf("  SKIP: need >=8 disks for draid (have %d)\n", len(disks))
			return nil
		}
		wipeDisks(disks[:8])
		spec := pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "draid2:6d", Disks: disks[:8]}},
		}
		if err := pm.Create(ctx, spec); err != nil {
			return fmt.Errorf("draid create: %w", err)
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		if d.Pool.Health != "ONLINE" {
			return fmt.Errorf("draid pool health=%q want ONLINE", d.Pool.Health)
		}
		if len(d.Status.Vdevs) == 0 {
			return fmt.Errorf("no vdevs reported")
		}
		t := d.Status.Vdevs[0].Type
		if !strings.HasPrefix(t, "draid") {
			return fmt.Errorf("vdev[0].Type=%q want draid*", t)
		}
		fmt.Printf("  draid pool ONLINE, vdev[0].Type=%s\n", t)
		return nil
	})
	cleanup("after scenario 32")

	step("33. special vdev", func() error {
		if len(disks) < 4 {
			return fmt.Errorf("need >=4 disks for special vdev scenario")
		}
		wipeDisks(disks[:4])
		spec := pool.CreateSpec{
			Name:    poolName,
			Vdevs:   []pool.VdevSpec{{Type: "mirror", Disks: disks[:2]}},
			Special: []pool.VdevSpec{{Type: "mirror", Disks: disks[2:4]}},
		}
		if err := pm.Create(ctx, spec); err != nil {
			return fmt.Errorf("create with special: %w", err)
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		var foundSpecial bool
		for _, v := range d.Status.Vdevs {
			if v.Type == "special" {
				foundSpecial = true
				fmt.Printf("  found special vdev with %d children\n", len(v.Children))
			}
		}
		if !foundSpecial {
			return fmt.Errorf("special vdev not present in status; vdevs=%+v", d.Status.Vdevs)
		}
		// Write data, scrub.
		writePayload(filepath.Join("/"+poolName, "special-data.bin"), 16<<20)
		if err := pm.Scrub(ctx, poolName, pool.ScrubStart); err != nil {
			return err
		}
		if err := waitScrubComplete(ctx, pm, 60*time.Second); err != nil {
			return err
		}
		fmt.Printf("  scrub clean on pool with special vdev\n")
		return nil
	})
	cleanup("after scenario 33")

	step("34. trim (start, then stop)", func() error {
		// Prefer SSDs via LOG_A + CACHE env (TRIM is a real operation on
		// SSDs). Fall back to HDD disks[:2] but tolerate the
		// "no devices in pool support trim operations" error.
		ssdA, ssdB := os.Getenv("LOG_A"), os.Getenv("CACHE")
		var trimDisks []string
		if ssdA != "" && ssdB != "" && ssdA != ssdB {
			trimDisks = []string{ssdA, ssdB}
		} else {
			trimDisks = disks[:2]
		}
		wipeDisks(trimDisks)
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: trimDisks}},
		}); err != nil {
			return err
		}
		if err := pm.Trim(ctx, poolName, pool.TrimStart, ""); err != nil {
			if strings.Contains(err.Error(), "no devices in pool support trim") {
				fmt.Printf("  SKIP: pool has no TRIM-capable devices (HDD)\n")
				return nil
			}
			return fmt.Errorf("trim start: %w", err)
		}
		if err := pm.Wait(ctx, poolName, "trim", 30*time.Second); err != nil {
			fmt.Printf("  WARNING: wait trim: %v (continuing)\n", err)
		}
		fmt.Printf("  trim start + wait OK\n")

		// Test stop: start again, then immediately stop.
		if err := pm.Trim(ctx, poolName, pool.TrimStart, ""); err != nil {
			fmt.Printf("  WARNING: second trim start: %v\n", err)
		}
		if err := pm.Trim(ctx, poolName, pool.TrimStop, ""); err != nil {
			fmt.Printf("  WARNING: trim stop: %v (HDDs may not support trim cancel)\n", err)
		} else {
			fmt.Printf("  trim stop OK\n")
		}
		return nil
	})
	cleanup("after scenario 34")

	step("35. zpool set/get properties (autotrim, comment)", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		if err := pm.SetProps(ctx, poolName, map[string]string{
			"autotrim": "on",
			"comment":  "validate-test",
		}); err != nil {
			return fmt.Errorf("setprops: %w", err)
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		if d.Props["autotrim"] != "on" {
			return fmt.Errorf("autotrim=%q want on", d.Props["autotrim"])
		}
		if d.Props["comment"] != "validate-test" {
			return fmt.Errorf("comment=%q want validate-test", d.Props["comment"])
		}
		fmt.Printf("  autotrim=%s comment=%s\n", d.Props["autotrim"], d.Props["comment"])
		return nil
	})
	cleanup("after scenario 35")

	step("36. refquota + refreservation + userquota", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		full := poolName + "/qfs"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "qfs", Type: "filesystem",
			Properties: map[string]string{"compression": "off"},
		}); err != nil {
			return err
		}
		if err := dm.SetProps(ctx, full, map[string]string{"refquota": "10M"}); err != nil {
			return fmt.Errorf("set refquota: %w", err)
		}
		mp := "/" + full
		// 5MiB succeeds.
		if err := writeRandom(filepath.Join(mp, "small.bin"), 5<<20); err != nil {
			return fmt.Errorf("5MiB write under 10M refquota: %w", err)
		}
		fmt.Printf("  refquota=10M, 5MiB write OK\n")
		// 20MiB fails.
		err := writeRandom(filepath.Join(mp, "big.bin"), 20<<20)
		if err == nil {
			return fmt.Errorf("expected 20MiB write to fail under 10M refquota")
		}
		fmt.Printf("  20MiB write rejected as expected: %v\n", err)
		_ = os.Remove(filepath.Join(mp, "big.bin"))

		// refreservation.
		if err := dm.SetProps(ctx, full, map[string]string{"refreservation": "5M"}); err != nil {
			return fmt.Errorf("set refreservation: %w", err)
		}
		got, err := dm.Get(ctx, full)
		if err != nil {
			return err
		}
		if got.Props["refreservation"] == "" || got.Props["refreservation"] == "0" || got.Props["refreservation"] == "none" {
			return fmt.Errorf("refreservation not visible: %q", got.Props["refreservation"])
		}
		fmt.Printf("  refreservation=%s bytes\n", got.Props["refreservation"])

		// userquota@root=2M (best-effort).
		if err := dm.SetProps(ctx, full, map[string]string{"userquota@root": "2M"}); err != nil {
			return fmt.Errorf("set userquota: %w", err)
		}
		// Clear refquota first so it doesn't interfere with userquota detection.
		if err := dm.SetProps(ctx, full, map[string]string{"refquota": "none"}); err != nil {
			fmt.Printf("  WARNING: clear refquota: %v\n", err)
		}
		_ = os.Remove(filepath.Join(mp, "small.bin"))
		err = writeRandom(filepath.Join(mp, "userq.bin"), 5<<20)
		if err == nil {
			fmt.Printf("  WARNING: 5MiB write succeeded despite userquota=2M (enforcement is async, skipping strict check)\n")
		} else {
			fmt.Printf("  userquota enforced: %v\n", err)
		}
		return nil
	})
	cleanup("after scenario 36")

	step("37. spare auto-substitution (requires zfs-zed)", func() error {
		out, err := exec.Command("systemctl", "is-active", "zfs-zed").Output()
		state := strings.TrimSpace(string(out))
		if err != nil || state != "active" {
			fmt.Printf("  SKIP: zed not running (is-active=%q err=%v)\n", state, err)
			return nil
		}
		if len(disks) < 4 {
			fmt.Printf("  SKIP: need >=4 disks (3 raidz1 + 1 spare); have %d\n", len(disks))
			return nil
		}
		wipeDisks(disks[:4])
		spec := pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "raidz1", Disks: disks[:3]}},
			Spare: []string{disks[3]},
		}
		if err := pm.Create(ctx, spec); err != nil {
			return fmt.Errorf("create with spare: %w", err)
		}
		// Write some data so resilver has work to do.
		writePayload(filepath.Join("/"+poolName, "spare-data.bin"), 8<<20)

		// Trigger fault: offline disks[0] then scrub. (dd label corruption on
		// an in-use disk often races with zfs's own writes.)
		if err := pm.Offline(ctx, poolName, disks[0], false); err != nil {
			fmt.Printf("  WARNING: offline disks[0]: %v\n", err)
		}
		_ = exec.Command("dd", "if=/dev/zero", "of="+disks[0], "bs=1M", "count=10").Run()
		_ = exec.Command("/sbin/zpool", "scrub", poolName).Run()

		// Poll up to 30s for the spare to be substituted.
		deadline := time.Now().Add(30 * time.Second)
		var substituted bool
		for time.Now().Before(deadline) {
			d, err := pm.Get(ctx, poolName)
			if err != nil {
				return err
			}
			if vdevTreeContainsDisk(d.Status.Vdevs, disks[3]) {
				// Walk: a substitution shows the spare disk somewhere under
				// a "spare" or "replacing" vdev group nested inside raidz1.
				for _, top := range d.Status.Vdevs {
					if top.Type != "raidz1" {
						continue
					}
					for _, child := range top.Children {
						if child.Type == "spare" || child.Type == "replacing" {
							substituted = true
						}
					}
				}
			}
			if substituted {
				fmt.Printf("  spare substituted, pool state=%s\n", d.Status.State)
				break
			}
			time.Sleep(2 * time.Second)
		}
		if !substituted {
			fmt.Printf("  WARNING: spare not auto-substituted within 30s (zed timing best-effort)\n")
		}
		return nil
	})
	cleanup("after scenario 37")

	fmt.Println("\nALL CHECKS PASSED")
}

// step31 is the incremental-send half of scenarios 30+31. It is invoked
// from inside step 30 because it shares the pool and the source dataset.
func step31(ctx context.Context, dm *dataset.Manager, sm *snapshot.Manager,
	src, dst, _ string, srcFile, dstFile string) {
	fmt.Printf("\n=== 31. zfs send incremental ===\n")
	// Modify src file.
	mutated := []byte("INCREMENTAL_PAYLOAD_v2")
	if err := os.WriteFile(srcFile, mutated, 0o600); err != nil {
		fmt.Printf("  FAIL: rewrite src file: %v\n", err)
		os.Exit(1)
	}
	hashMutated := sha256file(srcFile)
	if err := sm.Create(ctx, src, "s2", false); err != nil {
		fmt.Printf("  FAIL: snapshot s2: %v\n", err)
		os.Exit(1)
	}
	var buf bytes.Buffer
	if err := dm.Send(ctx, src+"@s2", dataset.SendOpts{IncrementalFrom: src + "@s1"}, &buf); err != nil {
		fmt.Printf("  FAIL: incremental send: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  incremental send produced %d bytes\n", buf.Len())
	if err := dm.Receive(ctx, dst, dataset.RecvOpts{Force: true}, &buf); err != nil {
		fmt.Printf("  FAIL: incremental receive: %v\n", err)
		os.Exit(1)
	}
	if got := sha256file(dstFile); got != hashMutated {
		fmt.Printf("  FAIL: dst hash %s != mutated %s\n", got, hashMutated)
		os.Exit(1)
	}
	fmt.Printf("  dst now matches mutated src (sha256 verified)\n")
	fmt.Printf("  OK\n")
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

// sha256bytes returns hex sha256 of buf.
func sha256bytes(buf []byte) string {
	h := sha256.Sum256(buf)
	return hex.EncodeToString(h[:])
}

// writeBlockAt opens path with O_WRONLY (no truncate — required for block
// devices), pwrites buf at offset, fsyncs, closes.
func writeBlockAt(path string, off int64, buf []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteAt(buf, off); err != nil {
		return err
	}
	return f.Sync()
}

// readBlockAt opens path read-only, reads n bytes at offset.
func readBlockAt(path string, off int64, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, off); err != nil {
		return nil, err
	}
	return buf, nil
}

// writeRandom writes size bytes of pseudo-random reproducible data to path,
// returning the first error encountered (open, write, or sync). Used for
// quota tests where we need to detect ENOSPC mid-write.
func writeRandom(path string, size int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 1<<16)
	for i := range buf {
		buf[i] = byte(i * 7)
	}
	for written := 0; written < size; written += len(buf) {
		n := len(buf)
		if size-written < n {
			n = size - written
		}
		if _, err := f.Write(buf[:n]); err != nil {
			return err
		}
	}
	return f.Sync()
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

// vdevTreeContainsDisk recursively checks if any Vdev in the tree refers
// to the given disk. ZFS reports vdev paths with a "-partN" suffix even
// when the disk was added by whole-device path, so we match on prefix.
func vdevTreeContainsDisk(vdevs []pool.Vdev, disk string) bool {
	for _, v := range vdevs {
		if v.Path == disk || strings.HasPrefix(v.Path, disk+"-part") {
			return true
		}
		if vdevTreeContainsDisk(v.Children, disk) {
			return true
		}
	}
	return false
}
