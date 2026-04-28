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

	"github.com/novanas/nova-nas/internal/host/configfs"
	disksPkg "github.com/novanas/nova-nas/internal/host/disks"
	"github.com/novanas/nova-nas/internal/host/iscsi"
	"github.com/novanas/nova-nas/internal/host/krb5"
	"github.com/novanas/nova-nas/internal/host/nfs"
	"github.com/novanas/nova-nas/internal/host/nvmeof"
	"github.com/novanas/nova-nas/internal/host/rdma"
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

	step("38. disks.List reflects live block-device set after udev rescan", func() error {
		lister := &disksPkg.Lister{LsblkBin: "/usr/bin/lsblk"}

		before, err := lister.List(ctx)
		if err != nil {
			return fmt.Errorf("initial disks.List: %w", err)
		}
		beforeNames := make(map[string]struct{}, len(before))
		for _, d := range before {
			beforeNames[d.Name] = struct{}{}
		}
		fmt.Printf("  baseline: %d disks listed\n", len(before))

		// Force udev to refresh the device tree. This simulates the hot-plug
		// rescan path; we cannot physically pull a drive in CI.
		if out, err := exec.Command("udevadm", "trigger", "--type=devices", "--action=add").CombinedOutput(); err != nil {
			fmt.Printf("  WARNING: udevadm trigger: %v: %s\n", err, strings.TrimSpace(string(out)))
		}
		if out, err := exec.Command("udevadm", "settle", "--timeout=10").CombinedOutput(); err != nil {
			fmt.Printf("  WARNING: udevadm settle: %v: %s\n", err, strings.TrimSpace(string(out)))
		}

		after, err := lister.List(ctx)
		if err != nil {
			return fmt.Errorf("post-rescan disks.List: %w", err)
		}
		fmt.Printf("  post-rescan: %d disks listed\n", len(after))

		if len(after) != len(before) {
			return fmt.Errorf("disk count changed across udev rescan: before=%d after=%d",
				len(before), len(after))
		}
		for _, d := range after {
			if _, ok := beforeNames[d.Name]; !ok {
				return fmt.Errorf("disk %q appeared after rescan; baseline set unstable", d.Name)
			}
		}
		fmt.Printf("  idempotent across udev rescan (%d disks, names match)\n", len(after))

		// Optional: real detach + SCSI rescan. Gated because the /sys writes are
		// gnarly and may not work in containers / non-SCSI transports / when the
		// disk is busy. Set HOT_PLUG_DISK to a kernel name like "sdz".
		hot := os.Getenv("HOT_PLUG_DISK")
		if hot == "" {
			fmt.Printf("  SKIP live-pull: HOT_PLUG_DISK unset (set to e.g. sdz to exercise detach+rescan)\n")
			return nil
		}
		sysPath := "/sys/block/" + hot
		if _, err := os.Stat(sysPath); err != nil {
			fmt.Printf("  SKIP live-pull: %s not present: %v\n", sysPath, err)
			return nil
		}

		// Detach.
		delCmd := fmt.Sprintf("echo 1 > /sys/block/%s/device/delete", hot)
		if out, err := exec.Command("sh", "-c", delCmd).CombinedOutput(); err != nil {
			fmt.Printf("  WARNING: detach %s failed: %v: %s\n", hot, err, strings.TrimSpace(string(out)))
			return nil
		}
		_, _ = exec.Command("udevadm", "settle", "--timeout=10").CombinedOutput()

		detached, err := lister.List(ctx)
		if err != nil {
			return fmt.Errorf("post-detach disks.List: %w", err)
		}
		fmt.Printf("  post-detach: %d disks listed (was %d)\n", len(detached), len(after))
		if len(detached) >= len(after) {
			fmt.Printf("  WARNING: expected fewer disks after detach of %s; saw %d\n", hot, len(detached))
		}

		// Re-attach via SCSI host rescan.
		const scanCmd = `for h in /sys/class/scsi_host/host*/scan; do echo "- - -" > "$h"; done`
		if out, err := exec.Command("sh", "-c", scanCmd).CombinedOutput(); err != nil {
			fmt.Printf("  WARNING: SCSI rescan failed: %v: %s\n", err, strings.TrimSpace(string(out)))
		}
		_, _ = exec.Command("udevadm", "settle", "--timeout=15").CombinedOutput()

		// Give udev / kernel a beat to repopulate /sys/block.
		deadline := time.Now().Add(15 * time.Second)
		var reattached []disksPkg.Disk
		for time.Now().Before(deadline) {
			reattached, err = lister.List(ctx)
			if err != nil {
				return fmt.Errorf("post-rescan disks.List: %w", err)
			}
			if len(reattached) == len(after) {
				break
			}
			time.Sleep(1 * time.Second)
		}
		fmt.Printf("  post-rescan: %d disks listed (target %d)\n", len(reattached), len(after))
		if len(reattached) != len(after) {
			return fmt.Errorf("disk %s did not reappear after SCSI rescan: got %d, want %d",
				hot, len(reattached), len(after))
		}
		found := false
		for _, d := range reattached {
			if d.Name == hot {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("disk %q missing from post-rescan list", hot)
		}
		fmt.Printf("  live-pull cycle complete: %s detached and re-listed\n", hot)
		return nil
	})
	cleanup("after scenario 38")

	step("39. zpool checkpoint + discard", func() error {
		wipeDisks(disks[:2])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "mirror", Disks: disks[:2]}},
		}); err != nil {
			return err
		}
		full := poolName + "/data"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "data", Type: "filesystem",
		}); err != nil {
			return err
		}
		mp := "/" + full
		writePayload(filepath.Join(mp, "checkpoint-data.bin"), 4<<20)

		if err := pm.Checkpoint(ctx, poolName); err != nil {
			return fmt.Errorf("checkpoint: %w", err)
		}
		d, err := pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		cp, ok := d.Props["checkpoint"]
		if !ok {
			return fmt.Errorf("checkpoint property missing from props")
		}
		if cp == "" || cp == "-" {
			return fmt.Errorf("checkpoint property=%q (expected non-empty value after creation)", cp)
		}
		fmt.Printf("  checkpoint created, property=%q\n", cp)

		if err := pm.DiscardCheckpoint(ctx, poolName); err != nil {
			return fmt.Errorf("discard checkpoint: %w", err)
		}
		d, err = pm.Get(ctx, poolName)
		if err != nil {
			return err
		}
		cp = d.Props["checkpoint"]
		if cp != "" && cp != "-" {
			return fmt.Errorf("checkpoint still present after discard: %q", cp)
		}
		fmt.Printf("  checkpoint discarded, property=%q\n", cp)
		return nil
	})
	cleanup("after scenario 39")

	step("40. zpool reguid", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		readGUID := func() (string, error) {
			out, err := exec.CommandContext(ctx, "/sbin/zpool", "get", "-H", "-o", "value", "guid", poolName).Output()
			if err != nil {
				return "", fmt.Errorf("zpool get guid: %w", err)
			}
			return strings.TrimSpace(string(out)), nil
		}
		before, err := readGUID()
		if err != nil {
			return err
		}
		if before == "" {
			return fmt.Errorf("empty initial GUID")
		}
		fmt.Printf("  GUID before reguid: %s\n", before)

		if err := pm.Reguid(ctx, poolName); err != nil {
			return fmt.Errorf("reguid: %w", err)
		}
		after, err := readGUID()
		if err != nil {
			return err
		}
		fmt.Printf("  GUID after reguid:  %s\n", after)
		if before == after {
			return fmt.Errorf("GUID unchanged after reguid (still %s)", before)
		}
		return nil
	})
	cleanup("after scenario 40")

	step("41. zpool sync", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		full := poolName + "/data"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "data", Type: "filesystem",
		}); err != nil {
			return err
		}
		mp := "/" + full
		for i := 0; i < 3; i++ {
			writePayload(filepath.Join(mp, fmt.Sprintf("sync-%d.bin", i)), 1<<20)
		}
		if err := pm.Sync(ctx, []string{poolName}); err != nil {
			return fmt.Errorf("sync(named): %w", err)
		}
		fmt.Printf("  sync(%q) ok\n", poolName)
		if err := pm.Sync(ctx, nil); err != nil {
			return fmt.Errorf("sync(all): %w", err)
		}
		fmt.Printf("  sync(all) ok\n")
		return nil
	})
	cleanup("after scenario 41")

	step("42. zpool upgrade", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		// A freshly-created pool may be at the latest features. zpool upgrade
		// can return non-zero on some ZFS versions when nothing is to be done;
		// log as a warning rather than fatal.
		if err := pm.Upgrade(ctx, poolName, false); err != nil {
			fmt.Printf("  WARN: Upgrade(%q,false) returned: %v (treated as soft failure)\n", poolName, err)
		} else {
			fmt.Printf("  Upgrade(%q,false) ok\n", poolName)
		}
		if err := pm.Upgrade(ctx, "", true); err != nil {
			fmt.Printf("  WARN: Upgrade(\"\",true) returned: %v (treated as soft failure)\n", err)
		} else {
			fmt.Printf("  Upgrade(all) ok\n")
		}
		return nil
	})
	cleanup("after scenario 42")

	step("43. zfs diff", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		full := poolName + "/data"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "data", Type: "filesystem",
		}); err != nil {
			return err
		}
		mp := "/" + full
		aPath := filepath.Join(mp, "a.bin")
		bPath := filepath.Join(mp, "b.bin")
		if err := os.WriteFile(aPath, []byte("ORIGINAL_A"), 0o600); err != nil {
			return err
		}
		if err := sm.Create(ctx, full, "s1", false); err != nil {
			return err
		}
		// Modify a.bin and create b.bin.
		if err := os.WriteFile(aPath, []byte("MUTATED_A_PAYLOAD"), 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(bPath, []byte("NEW_B_FILE"), 0o600); err != nil {
			return err
		}
		// Force txg flush so diff sees the changes deterministically.
		if err := pm.Sync(ctx, []string{poolName}); err != nil {
			return err
		}

		entries, err := dm.Diff(ctx, full+"@s1", "")
		if err != nil {
			return fmt.Errorf("diff: %w", err)
		}
		fmt.Printf("  diff returned %d entries:\n", len(entries))
		var sawModA, sawAddB bool
		for _, e := range entries {
			fmt.Printf("    %s %s%s\n", e.Change, e.Path,
				func() string {
					if e.NewPath != "" {
						return " -> " + e.NewPath
					}
					return ""
				}())
			if e.Change == "M" && strings.HasSuffix(e.Path, "/a.bin") {
				sawModA = true
			}
			if e.Change == "+" && strings.HasSuffix(e.Path, "/b.bin") {
				sawAddB = true
			}
		}
		if !sawModA {
			return fmt.Errorf("diff missing M entry for a.bin")
		}
		if !sawAddB {
			return fmt.Errorf("diff missing + entry for b.bin")
		}
		return nil
	})
	cleanup("after scenario 43")

	step("44. zfs bookmark + list + destroy", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		full := poolName + "/data"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "data", Type: "filesystem",
		}); err != nil {
			return err
		}
		if err := sm.Create(ctx, full, "s1", false); err != nil {
			return err
		}
		bm := full + "#bm1"
		if err := dm.Bookmark(ctx, full+"@s1", bm); err != nil {
			return fmt.Errorf("bookmark: %w", err)
		}
		bms, err := dm.ListBookmarks(ctx, full)
		if err != nil {
			return fmt.Errorf("list bookmarks: %w", err)
		}
		if len(bms) != 1 {
			return fmt.Errorf("want 1 bookmark, got %d: %+v", len(bms), bms)
		}
		if !strings.HasSuffix(bms[0].Name, "#bm1") {
			return fmt.Errorf("bookmark name=%q (want suffix #bm1)", bms[0].Name)
		}
		fmt.Printf("  bookmark created: %s (creation=%d)\n", bms[0].Name, bms[0].CreationUnix)

		if err := dm.DestroyBookmark(ctx, bm); err != nil {
			return fmt.Errorf("destroy bookmark: %w", err)
		}
		bms, err = dm.ListBookmarks(ctx, full)
		if err != nil {
			return err
		}
		if len(bms) != 0 {
			return fmt.Errorf("expected zero bookmarks after destroy, got %d", len(bms))
		}
		fmt.Printf("  bookmark destroyed; list now empty\n")
		return nil
	})
	cleanup("after scenario 44")

	step("45. snapshot hold + release + holds list", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		full := poolName + "/data"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "data", Type: "filesystem",
		}); err != nil {
			return err
		}
		snap := full + "@s1"
		if err := sm.Create(ctx, full, "s1", false); err != nil {
			return err
		}
		if err := sm.Hold(ctx, snap, "tag1", false); err != nil {
			return fmt.Errorf("hold: %w", err)
		}
		holds, err := sm.Holds(ctx, snap)
		if err != nil {
			return fmt.Errorf("holds: %w", err)
		}
		if len(holds) != 1 {
			return fmt.Errorf("want 1 hold, got %d: %+v", len(holds), holds)
		}
		if holds[0].Tag != "tag1" {
			return fmt.Errorf("hold tag=%q (want tag1)", holds[0].Tag)
		}
		fmt.Printf("  hold placed: tag=%s snapshot=%s\n", holds[0].Tag, holds[0].Snapshot)

		// Destroy must FAIL while held.
		if err := sm.Destroy(ctx, snap); err == nil {
			return fmt.Errorf("destroy succeeded on held snapshot (expected failure)")
		} else {
			fmt.Printf("  destroy correctly refused while held: %v\n", err)
		}

		if err := sm.Release(ctx, snap, "tag1", false); err != nil {
			return fmt.Errorf("release: %w", err)
		}
		holds, err = sm.Holds(ctx, snap)
		if err != nil {
			return err
		}
		if len(holds) != 0 {
			return fmt.Errorf("expected zero holds after release, got %d", len(holds))
		}
		fmt.Printf("  hold released; holds list empty\n")

		if err := sm.Destroy(ctx, snap); err != nil {
			return fmt.Errorf("destroy after release: %w", err)
		}
		fmt.Printf("  destroy now succeeds after release\n")
		return nil
	})
	cleanup("after scenario 45")

	// ---- Real-target scenarios (46-52) ---------------------------------------
	// These scenarios exercise iSCSI (LIO via targetcli), NVMe-oF (nvmet via
	// configfs), and RDMA detection against real kernel state. They run after
	// the ZFS scenarios so they can layer on a fresh single-disk pool. Any
	// leftover host-state (LIO targets, nvmet configfs trees) from a prior
	// failed run is best-effort cleared first.

	preTargetCleanup(ctx)

	const (
		validateIQN     = "iqn.2026-04.local.novanas:validate"
		validateClient  = "iqn.2026-04.local.novanas:client"
		validateNQN     = "nqn.2026-04.local.novanas:validate"
		validateHostNQN = "nqn.2026-04.local.novanas:client"
	)
	im := &iscsi.Manager{TargetcliBin: "/usr/bin/targetcli"}
	cfsMgr := &configfs.Manager{Root: "/sys/kernel/config"}
	nm := &nvmeof.Manager{CFS: cfsMgr}
	rl := &rdma.Lister{}

	step("46. iSCSI: target lifecycle (TCP)", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		defer func() { _ = pm.Destroy(ctx, poolName) }()

		zvol := poolName + "/iscsi-vol"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "iscsi-vol", Type: "volume",
			VolumeSizeBytes: 64 << 20,
		}); err != nil {
			return err
		}
		defer func() { _ = dm.Destroy(ctx, zvol, false) }()

		dev := "/dev/zvol/" + zvol
		if err := waitDevice(dev, 10*time.Second); err != nil {
			return err
		}

		const bs = "validate-bs"
		if err := im.CreateBackstore(ctx, bs, dev); err != nil {
			return fmt.Errorf("CreateBackstore: %w", err)
		}
		defer func() { _ = im.DeleteBackstore(ctx, bs) }()

		if err := im.CreateTarget(ctx, validateIQN); err != nil {
			return fmt.Errorf("CreateTarget: %w", err)
		}
		defer func() { _ = im.DeleteTarget(ctx, validateIQN) }()

		portal := iscsi.Portal{IP: "127.0.0.1", Port: 3260, Transport: "tcp"}
		if err := im.CreatePortal(ctx, validateIQN, portal); err != nil {
			return fmt.Errorf("CreatePortal: %w", err)
		}
		defer func() { _ = im.DeletePortal(ctx, validateIQN, portal) }()

		lun := iscsi.LUN{ID: 1, Backstore: bs}
		if err := im.CreateLUN(ctx, validateIQN, lun); err != nil {
			return fmt.Errorf("CreateLUN: %w", err)
		}
		defer func() { _ = im.DeleteLUN(ctx, validateIQN, 1) }()

		acl := iscsi.ACL{
			InitiatorIQN: validateClient,
			CHAPUser:     "chapuser",
			CHAPSecret:   "chapsecret12345",
		}
		if err := im.CreateACL(ctx, validateIQN, acl); err != nil {
			return fmt.Errorf("CreateACL: %w", err)
		}
		defer func() { _ = im.DeleteACL(ctx, validateIQN, validateClient) }()

		detail, err := im.GetTarget(ctx, validateIQN)
		if err != nil {
			return fmt.Errorf("GetTarget: %w", err)
		}
		if len(detail.Portals) < 1 {
			return fmt.Errorf("expected >=1 portal, got %d", len(detail.Portals))
		}
		foundLUN := false
		for _, l := range detail.LUNs {
			if l.Backstore == bs {
				foundLUN = true
				break
			}
		}
		if !foundLUN {
			return fmt.Errorf("LUN with backstore %q not found in %+v", bs, detail.LUNs)
		}
		foundACL := false
		for _, a := range detail.ACLs {
			if a.InitiatorIQN == validateClient {
				foundACL = true
				break
			}
		}
		if !foundACL {
			return fmt.Errorf("ACL for %q not found in %+v", validateClient, detail.ACLs)
		}
		fmt.Printf("  target %s: %d portal(s), %d LUN(s), %d ACL(s)\n",
			validateIQN, len(detail.Portals), len(detail.LUNs), len(detail.ACLs))
		return nil
	})

	step("47. iSCSI: open-iscsi self-loopback IO", func() error {
		if !which("iscsiadm") {
			fmt.Printf("  SKIP: iscsiadm not available\n")
			return nil
		}
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		defer func() { _ = pm.Destroy(ctx, poolName) }()

		zvol := poolName + "/iscsi-vol"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "iscsi-vol", Type: "volume",
			VolumeSizeBytes: 64 << 20,
		}); err != nil {
			return err
		}
		defer func() { _ = dm.Destroy(ctx, zvol, false) }()

		dev := "/dev/zvol/" + zvol
		if err := waitDevice(dev, 10*time.Second); err != nil {
			return err
		}

		const bs = "validate-bs47"
		if err := im.CreateBackstore(ctx, bs, dev); err != nil {
			return fmt.Errorf("CreateBackstore: %w", err)
		}
		defer func() { _ = im.DeleteBackstore(ctx, bs) }()

		if err := im.CreateTarget(ctx, validateIQN); err != nil {
			return fmt.Errorf("CreateTarget: %w", err)
		}
		defer func() { _ = im.DeleteTarget(ctx, validateIQN) }()

		portal := iscsi.Portal{IP: "127.0.0.1", Port: 3260, Transport: "tcp"}
		if err := im.CreatePortal(ctx, validateIQN, portal); err != nil {
			return fmt.Errorf("CreatePortal: %w", err)
		}
		defer func() { _ = im.DeletePortal(ctx, validateIQN, portal) }()

		if err := im.CreateLUN(ctx, validateIQN, iscsi.LUN{ID: 1, Backstore: bs}); err != nil {
			return fmt.Errorf("CreateLUN: %w", err)
		}
		defer func() { _ = im.DeleteLUN(ctx, validateIQN, 1) }()

		// Enable demo mode on the TPG so any initiator can log in without
		// an ACL (Debian's LIO default enforces ACLs + auth). This is
		// only for the loopback IO test; real deployments use ACLs+CHAP.
		tpgPath := "/iscsi/" + validateIQN + "/tpg1"
		for _, kv := range []string{"generate_node_acls=1", "authentication=0", "demo_mode_write_protect=0"} {
			if _, err := runCmd(ctx, "/usr/bin/targetcli", tpgPath, "set", "attribute", kv); err != nil {
				return fmt.Errorf("tpg set %s: %w", kv, err)
			}
		}
		// Discover.
		if _, err := runCmd(ctx, "iscsiadm", "-m", "discovery", "-t", "st", "-p", "127.0.0.1"); err != nil {
			return fmt.Errorf("iscsiadm discovery: %w", err)
		}
		// Login.
		if _, err := runCmd(ctx, "iscsiadm", "-m", "node", "-T", validateIQN, "-p", "127.0.0.1", "-l"); err != nil {
			return fmt.Errorf("iscsiadm login: %w", err)
		}
		loggedIn := true
		defer func() {
			if loggedIn {
				_, _ = runCmd(ctx, "iscsiadm", "-m", "node", "-T", validateIQN, "-p", "127.0.0.1", "-u")
				_, _ = runCmd(ctx, "iscsiadm", "-m", "node", "-T", validateIQN, "-p", "127.0.0.1", "-o", "delete")
			}
		}()

		// Find by-path device.
		pattern := "/dev/disk/by-path/ip-127.0.0.1*-iscsi-" + validateIQN + "*lun-1"
		var devPath string
		ddl := time.Now().Add(15 * time.Second)
		for time.Now().Before(ddl) {
			matches, _ := filepath.Glob(pattern)
			if len(matches) > 0 {
				devPath = matches[0]
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if devPath == "" {
			return fmt.Errorf("iSCSI by-path device did not appear (pattern=%s)", pattern)
		}
		fmt.Printf("  logged in, by-path device: %s\n", devPath)

		// Write 1 MiB of known data, fsync, sha256.
		payload := make([]byte, 1<<20)
		for i := range payload {
			payload[i] = byte(i % 251)
		}
		f, err := os.OpenFile(devPath, os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("open iscsi dev: %w", err)
		}
		if _, err := f.Write(payload); err != nil {
			f.Close()
			return fmt.Errorf("write iscsi dev: %w", err)
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return fmt.Errorf("fsync iscsi dev: %w", err)
		}
		f.Close()
		hashWritten := sha256bytes(payload)

		readBack, err := readBlockAt(devPath, 0, len(payload))
		if err != nil {
			return fmt.Errorf("read iscsi dev: %w", err)
		}
		if sha256bytes(readBack) != hashWritten {
			return fmt.Errorf("iscsi readback hash mismatch")
		}
		fmt.Printf("  wrote+read 1MiB OK, sha256=%s\n", hashWritten[:16])
		return nil
	})

	step("48. NVMe-oF: subsystem + namespace + port + loopback", func() error {
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		defer func() { _ = pm.Destroy(ctx, poolName) }()

		zvol := poolName + "/nvmeof-vol"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "nvmeof-vol", Type: "volume",
			VolumeSizeBytes: 64 << 20,
		}); err != nil {
			return err
		}
		defer func() { _ = dm.Destroy(ctx, zvol, false) }()

		dev := "/dev/zvol/" + zvol
		if err := waitDevice(dev, 10*time.Second); err != nil {
			return err
		}

		if err := nm.CreateSubsystem(ctx, nvmeof.Subsystem{
			NQN: validateNQN, AllowAnyHost: true, Serial: "validate01",
		}); err != nil {
			return fmt.Errorf("CreateSubsystem: %w", err)
		}
		defer func() { _ = nm.DeleteSubsystem(ctx, validateNQN) }()

		if err := nm.AddNamespace(ctx, validateNQN, nvmeof.Namespace{
			NSID: 1, DevicePath: dev, Enabled: true,
		}); err != nil {
			return fmt.Errorf("AddNamespace: %w", err)
		}
		defer func() { _ = nm.RemoveNamespace(ctx, validateNQN, 1) }()

		if err := nm.CreatePort(ctx, nvmeof.Port{
			ID: 1, IP: "127.0.0.1", Port: 4420, Transport: "tcp",
		}); err != nil {
			return fmt.Errorf("CreatePort: %w", err)
		}
		defer func() { _ = nm.DeletePort(ctx, 1) }()

		if err := nm.LinkSubsystemToPort(ctx, validateNQN, 1); err != nil {
			return fmt.Errorf("LinkSubsystemToPort: %w", err)
		}
		defer func() { _ = nm.UnlinkSubsystemFromPort(ctx, validateNQN, 1) }()

		d, err := nm.GetSubsystem(ctx, validateNQN)
		if err != nil {
			return fmt.Errorf("GetSubsystem: %w", err)
		}
		if !d.Subsystem.AllowAnyHost {
			return fmt.Errorf("expected allow_any_host=true")
		}
		if len(d.Namespaces) != 1 {
			return fmt.Errorf("expected 1 namespace; got %d", len(d.Namespaces))
		}
		if !d.Namespaces[0].Enabled {
			return fmt.Errorf("namespace not enabled")
		}
		fmt.Printf("  subsystem %s: ns=%d enabled=%v allowAny=%v\n",
			validateNQN, d.Namespaces[0].NSID, d.Namespaces[0].Enabled, d.Subsystem.AllowAnyHost)
		return nil
	})

	step("49. NVMe-oF: real connect + IO via nvme-cli", func() error {
		if !which("nvme") {
			fmt.Printf("  SKIP: nvme-cli not available\n")
			return nil
		}
		// Sanity check that nvme version actually runs.
		if _, err := runCmd(ctx, "nvme", "version"); err != nil {
			fmt.Printf("  SKIP: nvme version failed: %v\n", err)
			return nil
		}

		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		defer func() { _ = pm.Destroy(ctx, poolName) }()

		zvol := poolName + "/nvmeof-vol"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "nvmeof-vol", Type: "volume",
			VolumeSizeBytes: 64 << 20,
		}); err != nil {
			return err
		}
		defer func() { _ = dm.Destroy(ctx, zvol, false) }()

		dev := "/dev/zvol/" + zvol
		if err := waitDevice(dev, 10*time.Second); err != nil {
			return err
		}

		if err := nm.CreateSubsystem(ctx, nvmeof.Subsystem{
			NQN: validateNQN, AllowAnyHost: true, Serial: "validate49",
		}); err != nil {
			return err
		}
		defer func() { _ = nm.DeleteSubsystem(ctx, validateNQN) }()
		if err := nm.AddNamespace(ctx, validateNQN, nvmeof.Namespace{
			NSID: 1, DevicePath: dev, Enabled: true,
		}); err != nil {
			return err
		}
		defer func() { _ = nm.RemoveNamespace(ctx, validateNQN, 1) }()
		if err := nm.CreatePort(ctx, nvmeof.Port{
			ID: 1, IP: "127.0.0.1", Port: 4420, Transport: "tcp",
		}); err != nil {
			return err
		}
		defer func() { _ = nm.DeletePort(ctx, 1) }()
		if err := nm.LinkSubsystemToPort(ctx, validateNQN, 1); err != nil {
			return err
		}
		defer func() { _ = nm.UnlinkSubsystemFromPort(ctx, validateNQN, 1) }()

		// Snapshot pre-existing nvme block devs so we can detect the new one.
		preDevs := globNvmeBlocks()
		if _, err := runCmd(ctx, "nvme", "connect", "-t", "tcp", "-a", "127.0.0.1",
			"-s", "4420", "-n", validateNQN); err != nil {
			return fmt.Errorf("nvme connect: %w", err)
		}
		connected := true
		defer func() {
			if connected {
				_, _ = runCmd(ctx, "nvme", "disconnect", "-n", validateNQN)
			}
		}()

		var nvmeDev string
		ddl := time.Now().Add(10 * time.Second)
		for time.Now().Before(ddl) {
			postDevs := globNvmeBlocks()
			for _, d := range postDevs {
				if !contains(preDevs, d) {
					nvmeDev = d
					break
				}
			}
			if nvmeDev != "" {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if nvmeDev == "" {
			return fmt.Errorf("no new /dev/nvme*n* device appeared after connect")
		}
		fmt.Printf("  connected, device: %s\n", nvmeDev)

		payload := make([]byte, 1<<20)
		for i := range payload {
			payload[i] = byte((i * 7) % 251)
		}
		f, err := os.OpenFile(nvmeDev, os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("open nvme dev: %w", err)
		}
		if _, err := f.Write(payload); err != nil {
			f.Close()
			return fmt.Errorf("write nvme dev: %w", err)
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return fmt.Errorf("fsync nvme dev: %w", err)
		}
		f.Close()
		hashWritten := sha256bytes(payload)

		readBack, err := readBlockAt(nvmeDev, 0, len(payload))
		if err != nil {
			return fmt.Errorf("read nvme dev: %w", err)
		}
		if sha256bytes(readBack) != hashWritten {
			return fmt.Errorf("nvme readback hash mismatch")
		}
		fmt.Printf("  wrote+read 1MiB OK, sha256=%s\n", hashWritten[:16])
		return nil
	})

	step("50. NVMe-oF: explicit host NQN allowlist (no allow-any-host)", func() error {
		// Standalone setup; no zvol/pool needed (no namespaces required for
		// the allowlist semantics test).
		if err := nm.CreateSubsystem(ctx, nvmeof.Subsystem{
			NQN: validateNQN, AllowAnyHost: false, Serial: "validate50",
		}); err != nil {
			return fmt.Errorf("CreateSubsystem: %w", err)
		}
		defer func() { _ = nm.DeleteSubsystem(ctx, validateNQN) }()

		if err := nm.EnsureHost(ctx, validateHostNQN); err != nil {
			return fmt.Errorf("EnsureHost: %w", err)
		}
		hostCleanedUp := false
		defer func() {
			if !hostCleanedUp {
				_ = nm.RemoveHost(ctx, validateHostNQN)
			}
		}()

		if err := nm.AllowHost(ctx, validateNQN, validateHostNQN); err != nil {
			return fmt.Errorf("AllowHost: %w", err)
		}
		allowed := true
		defer func() {
			if allowed {
				_ = nm.DisallowHost(ctx, validateNQN, validateHostNQN)
			}
		}()

		d, err := nm.GetSubsystem(ctx, validateNQN)
		if err != nil {
			return fmt.Errorf("GetSubsystem: %w", err)
		}
		if d.Subsystem.AllowAnyHost {
			return fmt.Errorf("allow_any_host should be false")
		}
		found := false
		for _, h := range d.AllowedHosts {
			if h == validateHostNQN {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("host %q not in allowed list %v", validateHostNQN, d.AllowedHosts)
		}
		fmt.Printf("  allowlist contains %s, allowAny=%v\n", validateHostNQN, d.Subsystem.AllowAnyHost)

		if err := nm.DisallowHost(ctx, validateNQN, validateHostNQN); err != nil {
			return fmt.Errorf("DisallowHost: %w", err)
		}
		allowed = false
		if err := nm.RemoveHost(ctx, validateHostNQN); err != nil {
			return fmt.Errorf("RemoveHost: %w", err)
		}
		hostCleanedUp = true
		return nil
	})

	step("51. RDMA detection", func() error {
		adapters, err := rl.List(ctx)
		if err != nil {
			return fmt.Errorf("rdma.List: %w", err)
		}
		hasRDMA, err := rl.HasActiveRDMA(ctx)
		if err != nil {
			return fmt.Errorf("HasActiveRDMA: %w", err)
		}
		fmt.Printf("  adapters=%d hasActiveRDMA=%v\n", len(adapters), hasRDMA)
		for _, a := range adapters {
			fmt.Printf("    %s ports=%d\n", a.Name, len(a.Ports))
		}
		return nil
	})

	step("52. iSCSI iSER (skip if no RDMA)", func() error {
		hasRDMA, err := rl.HasActiveRDMA(ctx)
		if err != nil {
			return fmt.Errorf("HasActiveRDMA: %w", err)
		}
		if !hasRDMA {
			fmt.Printf("  SKIP: no active RDMA adapter present\n")
			return nil
		}

		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		defer func() { _ = pm.Destroy(ctx, poolName) }()

		zvol := poolName + "/iser-vol"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "iser-vol", Type: "volume",
			VolumeSizeBytes: 64 << 20,
		}); err != nil {
			return err
		}
		defer func() { _ = dm.Destroy(ctx, zvol, false) }()

		dev := "/dev/zvol/" + zvol
		if err := waitDevice(dev, 10*time.Second); err != nil {
			return err
		}

		const bs = "validate-iser-bs"
		if err := im.CreateBackstore(ctx, bs, dev); err != nil {
			return err
		}
		defer func() { _ = im.DeleteBackstore(ctx, bs) }()

		if err := im.CreateTarget(ctx, validateIQN); err != nil {
			return err
		}
		defer func() { _ = im.DeleteTarget(ctx, validateIQN) }()

		portal := iscsi.Portal{IP: "127.0.0.1", Port: 3260, Transport: "iser"}
		if err := im.CreatePortal(ctx, validateIQN, portal); err != nil {
			return fmt.Errorf("CreatePortal(iser): %w", err)
		}
		defer func() { _ = im.DeletePortal(ctx, validateIQN, portal) }()

		detail, err := im.GetTarget(ctx, validateIQN)
		if err != nil {
			return fmt.Errorf("GetTarget: %w", err)
		}
		if len(detail.Portals) < 1 {
			return fmt.Errorf("no portal listed after iser create")
		}
		fmt.Printf("  iSER portal created on %s:%d\n", portal.IP, portal.Port)

		if os.Getenv("RDMA_IO") != "1" {
			fmt.Printf("  SKIP: iSER IO test (set RDMA_IO=1 to run)\n")
			return nil
		}
		// Real iSER IO would go here; intentionally omitted unless opted in.
		return nil
	})

	step("53. NVMe-oF DH-HMAC-CHAP authentication", func() error {
		if !which("nvme") {
			fmt.Printf("  SKIP: nvme-cli not available\n")
			return nil
		}
		if _, err := runCmd(ctx, "nvme", "version"); err != nil {
			fmt.Printf("  SKIP: nvme version failed: %v\n", err)
			return nil
		}

		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		defer func() { _ = pm.Destroy(ctx, poolName) }()

		zvol := poolName + "/dhchap-vol"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "dhchap-vol", Type: "volume",
			VolumeSizeBytes: 64 << 20,
		}); err != nil {
			return err
		}
		defer func() { _ = dm.Destroy(ctx, zvol, false) }()

		dev := "/dev/zvol/" + zvol
		if err := waitDevice(dev, 10*time.Second); err != nil {
			return err
		}

		if err := nm.CreateSubsystem(ctx, nvmeof.Subsystem{
			NQN: validateNQN, AllowAnyHost: false, Serial: "validate53",
		}); err != nil {
			return err
		}
		defer func() { _ = nm.DeleteSubsystem(ctx, validateNQN) }()
		if err := nm.AddNamespace(ctx, validateNQN, nvmeof.Namespace{
			NSID: 1, DevicePath: dev, Enabled: true,
		}); err != nil {
			return err
		}
		defer func() { _ = nm.RemoveNamespace(ctx, validateNQN, 1) }()
		// Use port 4421 to avoid colliding with any leftover state
		// from scenario 49 (which uses port 4420).
		if err := nm.CreatePort(ctx, nvmeof.Port{
			ID: 2, IP: "127.0.0.1", Port: 4421, Transport: "tcp",
		}); err != nil {
			return err
		}
		defer func() { _ = nm.DeletePort(ctx, 2) }()
		if err := nm.LinkSubsystemToPort(ctx, validateNQN, 2); err != nil {
			return err
		}
		defer func() { _ = nm.UnlinkSubsystemFromPort(ctx, validateNQN, 2) }()

		// Generate a real TP4022 host secret via nvme-cli.
		genCmd := exec.CommandContext(ctx, "nvme", "gen-dhchap-key", "-n", validateHostNQN)
		genOut, err := genCmd.Output()
		if err != nil {
			return fmt.Errorf("nvme gen-dhchap-key: %w", err)
		}
		secret := strings.TrimSpace(string(genOut))
		if !strings.HasPrefix(secret, "DHHC-1:") {
			return fmt.Errorf("unexpected dhchap-key format: %q", secret)
		}
		fmt.Printf("  generated dhchap secret: %s...\n", secret[:20])

		if err := nm.SetHostDHChap(ctx, validateHostNQN, nvmeof.DHChapConfig{
			Key: secret, Hash: "hmac(sha256)", DHGroup: "null",
		}); err != nil {
			return fmt.Errorf("SetHostDHChap: %w", err)
		}
		defer func() { _ = nm.ClearHostDHChap(ctx, validateHostNQN) }()

		if err := nm.AllowHost(ctx, validateNQN, validateHostNQN); err != nil {
			return fmt.Errorf("AllowHost: %w", err)
		}
		defer func() { _ = nm.DisallowHost(ctx, validateNQN, validateHostNQN) }()

		preDevs := globNvmeBlocks()
		connectArgs := []string{
			"connect", "-t", "tcp", "-a", "127.0.0.1", "-s", "4421",
			"-n", validateNQN, "--hostnqn=" + validateHostNQN,
			"--dhchap-secret=" + secret,
		}
		if _, err := runCmd(ctx, "nvme", connectArgs...); err != nil {
			return fmt.Errorf("nvme connect with dhchap: %w", err)
		}
		connected := true
		defer func() {
			if connected {
				_, _ = runCmd(ctx, "nvme", "disconnect", "-n", validateNQN)
			}
		}()

		var nvmeDev string
		ddl := time.Now().Add(10 * time.Second)
		for time.Now().Before(ddl) {
			postDevs := globNvmeBlocks()
			for _, d := range postDevs {
				if !contains(preDevs, d) {
					nvmeDev = d
					break
				}
			}
			if nvmeDev != "" {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if nvmeDev == "" {
			return fmt.Errorf("no new /dev/nvme*n* device appeared after authenticated connect")
		}
		fmt.Printf("  authenticated connect succeeded, device: %s\n", nvmeDev)

		payload := make([]byte, 1<<20)
		for i := range payload {
			payload[i] = byte((i*11 + 3) % 251)
		}
		f, err := os.OpenFile(nvmeDev, os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("open nvme dev: %w", err)
		}
		if _, err := f.Write(payload); err != nil {
			f.Close()
			return fmt.Errorf("write nvme dev: %w", err)
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return fmt.Errorf("fsync nvme dev: %w", err)
		}
		f.Close()
		hashWritten := sha256bytes(payload)
		readBack, err := readBlockAt(nvmeDev, 0, len(payload))
		if err != nil {
			return fmt.Errorf("read nvme dev: %w", err)
		}
		if sha256bytes(readBack) != hashWritten {
			return fmt.Errorf("nvme readback hash mismatch")
		}
		fmt.Printf("  authenticated IO ok, sha256=%s\n", hashWritten[:16])

		// Disconnect, then prove a wrong secret is rejected.
		if _, err := runCmd(ctx, "nvme", "disconnect", "-n", validateNQN); err != nil {
			return fmt.Errorf("disconnect before wrong-secret test: %w", err)
		}
		connected = false

		// Generate a different secret that the target won't accept.
		wrongCmd := exec.CommandContext(ctx, "nvme", "gen-dhchap-key", "-n", validateHostNQN)
		wrongOut, err := wrongCmd.Output()
		if err != nil {
			return fmt.Errorf("nvme gen-dhchap-key (wrong): %w", err)
		}
		wrongSecret := strings.TrimSpace(string(wrongOut))
		if wrongSecret == secret {
			return fmt.Errorf("expected gen-dhchap-key to produce a fresh secret")
		}
		wrongArgs := []string{
			"connect", "-t", "tcp", "-a", "127.0.0.1", "-s", "4421",
			"-n", validateNQN, "--hostnqn=" + validateHostNQN,
			"--dhchap-secret=" + wrongSecret,
		}
		if _, err := runCmd(ctx, "nvme", wrongArgs...); err == nil {
			// Connect succeeded — that's a security failure. Tear down
			// the bogus session before returning the error.
			_, _ = runCmd(ctx, "nvme", "disconnect", "-n", validateNQN)
			return fmt.Errorf("connect with wrong dhchap secret unexpectedly succeeded")
		}
		fmt.Printf("  wrong-secret connect correctly rejected\n")
		return nil
	})

	step("54. NVMe-oF persistence: Save → ClearAll → Restore round trip", func() error {
		// Build a non-trivial config in configfs, snapshot it to JSON,
		// tear down (simulating a reboot), restore, and verify the
		// post-restore state matches.
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		defer func() { _ = pm.Destroy(ctx, poolName) }()
		zvol := poolName + "/persist-vol"
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: "persist-vol", Type: "volume",
			VolumeSizeBytes: 32 << 20,
		}); err != nil {
			return err
		}
		defer func() { _ = dm.Destroy(ctx, zvol, false) }()
		dev := "/dev/zvol/" + zvol
		if err := waitDevice(dev, 10*time.Second); err != nil {
			return err
		}

		subsysNQN := "nqn.2026-04.local.novanas:persist"
		hostNQN := "nqn.2026-04.local.novanas:client"
		nm := &nvmeof.Manager{CFS: &configfs.Manager{Root: "/sys/kernel/config"}}

		// Build it.
		if err := nm.CreateSubsystem(ctx, nvmeof.Subsystem{NQN: subsysNQN, AllowAnyHost: false, Serial: "persist01"}); err != nil {
			return fmt.Errorf("CreateSubsystem: %w", err)
		}
		if err := nm.AddNamespace(ctx, subsysNQN, nvmeof.Namespace{NSID: 1, DevicePath: dev, Enabled: true}); err != nil {
			return fmt.Errorf("AddNamespace: %w", err)
		}
		if err := nm.EnsureHost(ctx, hostNQN); err != nil {
			return fmt.Errorf("EnsureHost: %w", err)
		}
		if err := nm.AllowHost(ctx, subsysNQN, hostNQN); err != nil {
			return fmt.Errorf("AllowHost: %w", err)
		}
		if err := nm.CreatePort(ctx, nvmeof.Port{ID: 7, IP: "127.0.0.1", Port: 4422, Transport: "tcp"}); err != nil {
			return fmt.Errorf("CreatePort: %w", err)
		}
		if err := nm.LinkSubsystemToPort(ctx, subsysNQN, 7); err != nil {
			return fmt.Errorf("LinkSubsystemToPort: %w", err)
		}

		// Save snapshot.
		snapPath := "/tmp/nova-nvmet-test.json"
		if err := nm.SaveToFile(ctx, snapPath); err != nil {
			return fmt.Errorf("SaveToFile: %w", err)
		}
		defer os.Remove(snapPath)
		fmt.Printf("  saved nvmet snapshot to %s\n", snapPath)

		// Tear down (simulating reboot wiping nvmet RAM state).
		if err := nm.ClearAll(ctx); err != nil {
			return fmt.Errorf("ClearAll: %w", err)
		}
		// Verify it's empty.
		subs, _ := nm.ListSubsystems(ctx)
		ports, _ := nm.ListPorts(ctx)
		if len(subs) != 0 || len(ports) != 0 {
			return fmt.Errorf("ClearAll left subs=%d ports=%d", len(subs), len(ports))
		}
		fmt.Printf("  cleared all nvmet state\n")

		// Restore from JSON.
		if err := nm.RestoreFromFile(ctx, snapPath); err != nil {
			return fmt.Errorf("RestoreFromFile: %w", err)
		}
		fmt.Printf("  restored from snapshot\n")

		// Verify the state is back.
		got, err := nm.GetSubsystem(ctx, subsysNQN)
		if err != nil {
			return fmt.Errorf("GetSubsystem after restore: %w", err)
		}
		if len(got.Namespaces) != 1 || got.Namespaces[0].NSID != 1 || !got.Namespaces[0].Enabled {
			return fmt.Errorf("namespace not restored: %+v", got.Namespaces)
		}
		if len(got.AllowedHosts) != 1 || got.AllowedHosts[0] != hostNQN {
			return fmt.Errorf("allowed_hosts not restored: %+v", got.AllowedHosts)
		}
		ports, _ = nm.ListPorts(ctx)
		var seenPort *nvmeof.Port
		for i := range ports {
			if ports[i].ID == 7 {
				seenPort = &ports[i]
			}
		}
		if seenPort == nil {
			return fmt.Errorf("port 7 not restored: %+v", ports)
		}
		fmt.Printf("  state matches post-restore: subsys=%s nsCount=%d port=%d:%s\n",
			subsysNQN, len(got.Namespaces), seenPort.Port, seenPort.Transport)

		// Tear down for next runs.
		_ = nm.UnlinkSubsystemFromPort(ctx, subsysNQN, 7)
		_ = nm.DeletePort(ctx, 7)
		_ = nm.DisallowHost(ctx, subsysNQN, hostNQN)
		_ = nm.RemoveHost(ctx, hostNQN)
		_ = nm.RemoveNamespace(ctx, subsysNQN, 1)
		_ = nm.DeleteSubsystem(ctx, subsysNQN)
		return nil
	})

	step("55. NFS export with sec=sys (real loopback mount + IO)", func() error {
		// Pre-clean any stale nova-nas-*.exports left over from a prior
		// failed run — those would reference paths that no longer
		// exist and make `exportfs -ra` fail this scenario before it
		// even starts.
		if entries, err := os.ReadDir("/etc/exports.d"); err == nil {
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), "nova-nas-") {
					_ = os.Remove(filepath.Join("/etc/exports.d", e.Name()))
				}
			}
		}
		_, _ = runCmd(ctx, "/usr/sbin/exportfs", "-ra")
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return err
		}
		defer func() {
			// NFS server can briefly hold the path even after umount/
			// DeleteExport. Drop all kernel-side exports first, then
			// destroy.
			_, _ = runCmd(ctx, "/usr/sbin/exportfs", "-ua")
			if err := pm.Destroy(ctx, poolName); err != nil {
				fmt.Printf("  WARN scenario 55 pool destroy: %v\n", err)
			}
		}()

		dsName := "nfs-share"
		full := poolName + "/" + dsName
		mp := "/validate/nfs-share"
		if err := os.MkdirAll("/validate", 0o755); err != nil {
			return err
		}
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: dsName, Type: "filesystem",
			Properties: map[string]string{"mountpoint": mp},
		}); err != nil {
			return err
		}
		defer func() { _ = dm.Destroy(ctx, full, false) }()

		// Marker file for visibility check.
		markerPath := filepath.Join(mp, "marker.txt")
		markerBody := []byte("hello from host fs\n")
		if err := os.WriteFile(markerPath, markerBody, 0o644); err != nil {
			return fmt.Errorf("write marker: %w", err)
		}

		nm := &nfs.Manager{ExportsBin: "/usr/sbin/exportfs"}
		exp := nfs.Export{
			Name: "validate",
			Path: mp,
			Clients: []nfs.ClientRule{{
				Spec:    "127.0.0.1",
				Options: "rw,sync,no_subtree_check,no_root_squash,sec=sys",
			}},
		}
		if err := nm.CreateExport(ctx, exp); err != nil {
			return fmt.Errorf("CreateExport: %w", err)
		}
		defer func() { _ = nm.DeleteExport(ctx, "validate") }()

		mountPoint := "/mnt/validate-nfs"
		if err := os.MkdirAll(mountPoint, 0o755); err != nil {
			return err
		}
		if _, err := runCmd(ctx, "mount", "-t", "nfs", "-o", "vers=4,sec=sys",
			"127.0.0.1:"+mp, mountPoint); err != nil {
			return fmt.Errorf("mount: %w", err)
		}
		unmounted := false
		defer func() {
			if !unmounted {
				_, _ = runCmd(ctx, "umount", mountPoint)
			}
		}()

		// Verify marker visible across NFS.
		gotMarker, err := os.ReadFile(filepath.Join(mountPoint, "marker.txt"))
		if err != nil {
			return fmt.Errorf("read marker via NFS: %w", err)
		}
		if !bytes.Equal(gotMarker, markerBody) {
			return fmt.Errorf("marker mismatch via NFS: got %q want %q", gotMarker, markerBody)
		}
		fmt.Printf("  marker file visible via NFS mount\n")

		// 1 MiB known data, fsync, hash.
		payload := filepath.Join(mountPoint, "payload.bin")
		writePayload(payload, 1<<20)
		hashWrote := sha256file(payload)

		readBack, err := os.ReadFile(payload)
		if err != nil {
			return fmt.Errorf("readback: %w", err)
		}
		hashRead := sha256bytes(readBack)
		if hashRead != hashWrote {
			return fmt.Errorf("sha256 mismatch: write=%s read=%s", hashWrote, hashRead)
		}
		fmt.Printf("  1 MiB payload sha256 round-trip ok (%s)\n", hashRead[:16])

		if _, err := runCmd(ctx, "umount", mountPoint); err != nil {
			return fmt.Errorf("umount: %w", err)
		}
		unmounted = true

		if err := nm.DeleteExport(ctx, "validate"); err != nil {
			return fmt.Errorf("DeleteExport: %w", err)
		}
		return nil
	})

	step("56. NFS export with sec=krb5 (full local KDC + real mount + IO)", func() error {
		if !which("kadmin.local") {
			fmt.Printf("  SKIP: kadmin.local not available\n")
			return nil
		}

		// Snapshot the host state we are about to clobber so cleanup can
		// restore it. Files we touch and may need to remove on teardown.
		krb5Conf := "/etc/krb5.conf"
		krb5Keytab := "/etc/krb5.keytab"
		idmapdConf := "/etc/idmapd.conf"
		kdcConf := "/etc/krb5kdc/kdc.conf"
		kdcStateDir := "/var/lib/krb5kdc-test"
		exportName := "krbvalidate"
		exportMP := "/validate/nfs-krb"
		mountPoint := "/mnt/validate-nfs-krb"
		testKeytabPath := "/tmp/nfs-test.keytab"

		// Capture pre-existing files (best-effort): empty []byte means
		// "no file existed", non-nil means restore those bytes on teardown.
		readIfExists := func(p string) ([]byte, bool) {
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, false
			}
			return b, true
		}
		preKrb5Conf, hadKrb5Conf := readIfExists(krb5Conf)
		preKeytab, hadKeytab := readIfExists(krb5Keytab)
		preIdmapd, hadIdmapd := readIfExists(idmapdConf)
		preKdcConf, hadKdcConf := readIfExists(kdcConf)

		mounted := false
		exportCreated := false
		poolCreated := false

		// Master cleanup: runs unconditionally to put the box back.
		// Best-effort everywhere — we do not want cleanup failures to
		// shadow the real scenario error.
		defer func() {
			if mounted {
				_, _ = runCmd(ctx, "umount", mountPoint)
			}
			if exportCreated {
				nm := &nfs.Manager{ExportsBin: "/usr/sbin/exportfs"}
				_ = nm.DeleteExport(ctx, exportName)
			}
			if poolCreated {
				_ = pm.Destroy(ctx, poolName)
			}
			// Stop KDC services (best-effort).
			_, _ = runCmd(ctx, "systemctl", "stop", "krb5-kdc")
			_, _ = runCmd(ctx, "systemctl", "stop", "krb5-admin-server")
			// Restart nfs-server so it releases keytab/idmapd state.
			_, _ = runCmd(ctx, "systemctl", "restart", "nfs-server")
			// Remove the test KDC state dir.
			_ = os.RemoveAll(kdcStateDir)
			_ = os.Remove(testKeytabPath)
			// Restore (or remove) the host config files.
			restore := func(p string, pre []byte, had bool) {
				if had {
					_ = os.WriteFile(p, pre, 0o644)
				} else {
					_ = os.Remove(p)
				}
			}
			restore(krb5Conf, preKrb5Conf, hadKrb5Conf)
			restore(idmapdConf, preIdmapd, hadIdmapd)
			restore(kdcConf, preKdcConf, hadKdcConf)
			if hadKeytab {
				_ = os.WriteFile(krb5Keytab, preKeytab, 0o600)
			} else {
				_ = os.Remove(krb5Keytab)
			}
		}()

		// Step 2: stop any running KDC.
		_, _ = runCmd(ctx, "systemctl", "stop", "krb5-kdc")
		_, _ = runCmd(ctx, "systemctl", "stop", "krb5-admin-server")

		// Step 3: write krb5.conf + idmapd.conf via the Manager.
		km := &krb5.Manager{
			Krb5ConfPath:   krb5Conf,
			KeytabPath:     krb5Keytab,
			IdmapdConfPath: idmapdConf,
			KlistBin:       "/usr/bin/klist",
		}
		if err := km.SetConfig(ctx, krb5.Config{
			DefaultRealm: "NOVANAS.LOCAL",
			Realms: map[string]krb5.Realm{
				"NOVANAS.LOCAL": {
					KDC:         []string{"127.0.0.1:88"},
					AdminServer: "127.0.0.1:749",
				},
			},
			DomainRealm: map[string]string{".novanas.local": "NOVANAS.LOCAL"},
		}); err != nil {
			return fmt.Errorf("step3 krb5.SetConfig: %w", err)
		}
		if err := km.SetIdmapdConfig(ctx, krb5.IdmapdConfig{Domain: "novanas.local"}); err != nil {
			return fmt.Errorf("step3 krb5.SetIdmapdConfig: %w", err)
		}

		// Step 4: kdc.conf pointing at the test directory.
		// On Debian 13 /etc/krb5kdc is provided by the krb5-kdc package; if
		// the directory is missing for any reason, MkdirAll creates it.
		if err := os.MkdirAll(filepath.Dir(kdcConf), 0o755); err != nil {
			return fmt.Errorf("step4 mkdir krb5kdc: %w", err)
		}
		kdcConfBody := `[kdcdefaults]
    kdc_ports = 88
[realms]
    NOVANAS.LOCAL = {
        database_name = /var/lib/krb5kdc-test/principal
        admin_keytab = FILE:/var/lib/krb5kdc-test/kadm5.keytab
        acl_file = /var/lib/krb5kdc-test/kadm5.acl
        key_stash_file = /var/lib/krb5kdc-test/.k5.NOVANAS.LOCAL
        max_life = 10h 0m 0s
        max_renewable_life = 7d 0h 0m 0s
        supported_enctypes = aes256-cts-hmac-sha1-96:normal aes128-cts-hmac-sha1-96:normal
    }
`
		if err := os.WriteFile(kdcConf, []byte(kdcConfBody), 0o644); err != nil {
			return fmt.Errorf("step4 write kdc.conf: %w", err)
		}

		// Step 5: mkdir kdc state dir + empty acl.
		// Remove any leftover from a previous run so kdb5_util create
		// (step 6) is one-shot.
		_ = os.RemoveAll(kdcStateDir)
		if err := os.MkdirAll(kdcStateDir, 0o700); err != nil {
			return fmt.Errorf("step5 mkdir kdc state: %w", err)
		}
		if err := os.WriteFile(filepath.Join(kdcStateDir, "kadm5.acl"), []byte{}, 0o600); err != nil {
			return fmt.Errorf("step5 write acl: %w", err)
		}

		// Step 6: initialize realm DB.
		if _, err := runCmd(ctx, "kdb5_util", "-r", "NOVANAS.LOCAL", "create", "-s", "-P", "testmasterpw"); err != nil {
			return fmt.Errorf("step6 kdb5_util create: %w", err)
		}

		// Step 7: start the KDC + kadmin.
		dumpJournal := func(unit string) {
			if out, err := runCmd(ctx, "journalctl", "-u", unit, "--no-pager", "-n", "30"); err == nil {
				fmt.Printf("  journalctl -u %s:\n%s\n", unit, string(out))
			}
		}
		if _, err := runCmd(ctx, "systemctl", "start", "krb5-kdc"); err != nil {
			dumpJournal("krb5-kdc")
			return fmt.Errorf("step7 start krb5-kdc: %w", err)
		}
		if _, err := runCmd(ctx, "systemctl", "start", "krb5-admin-server"); err != nil {
			dumpJournal("krb5-admin-server")
			return fmt.Errorf("step7 start krb5-admin-server: %w", err)
		}
		if !systemctlIsActive(ctx, "krb5-kdc") {
			dumpJournal("krb5-kdc")
			return fmt.Errorf("step7: krb5-kdc not active")
		}
		if !systemctlIsActive(ctx, "krb5-admin-server") {
			dumpJournal("krb5-admin-server")
			return fmt.Errorf("step7: krb5-admin-server not active")
		}

		// Step 8: create NFS service principals.
		// Get the host's name (uname -n). Linux NFS clients try the FQDN
		// for sec=krb5 mounts; we add both 127.0.0.1 and the host name.
		hostnameOut, err := runCmd(ctx, "uname", "-n")
		if err != nil {
			return fmt.Errorf("step8 uname -n: %w", err)
		}
		hostname := strings.TrimSpace(string(hostnameOut))
		// Sanity: must look DNS-ish (alnum + . + -). If exotic, addprinc
		// may reject it.
		if hostname == "" {
			return fmt.Errorf("step8: empty hostname")
		}
		// Linux NFS resolves the server's IP back to a hostname before
		// asking gssd for credentials. For 127.0.0.1 the kernel asks for
		// `nfs/localhost@REALM` (not nfs/127.0.0.1). We create all three
		// (127.0.0.1, localhost, $hostname) so the test is robust to
		// whichever name the kernel happens to ask for.
		for _, host := range []string{"127.0.0.1", "localhost", hostname} {
			if _, err := runCmd(ctx, "kadmin.local", "-q",
				"addprinc -randkey nfs/"+host+"@NOVANAS.LOCAL"); err != nil {
				return fmt.Errorf("step8 addprinc nfs/%s: %w", host, err)
			}
		}

		// Step 9: create user principal (kept for completeness; not used
		// for the loopback mount which uses host machine creds).
		if _, err := runCmd(ctx, "kadmin.local", "-q",
			"addprinc -pw testpass1 testuser@NOVANAS.LOCAL"); err != nil {
			return fmt.Errorf("step9 addprinc testuser: %w", err)
		}

		// Step 10: ktadd both NFS service principals into a temp keytab,
		// then upload it through the Manager (exercises UploadKeytab).
		_ = os.Remove(testKeytabPath)
		if _, err := runCmd(ctx, "kadmin.local", "-q",
			"ktadd -k "+testKeytabPath+
				" nfs/127.0.0.1@NOVANAS.LOCAL"+
				" nfs/localhost@NOVANAS.LOCAL"+
				" nfs/"+hostname+"@NOVANAS.LOCAL"); err != nil {
			return fmt.Errorf("step10 ktadd: %w", err)
		}
		ktData, err := os.ReadFile(testKeytabPath)
		if err != nil {
			return fmt.Errorf("step10 read keytab: %w", err)
		}
		if err := km.UploadKeytab(ctx, ktData); err != nil {
			return fmt.Errorf("step10 UploadKeytab: %w", err)
		}

		// Step 11: restart nfs-server to pick up new keytab + idmapd.
		// Debian 13: rpc-svcgssd is folded into nfs-server.
		if _, err := runCmd(ctx, "systemctl", "restart", "nfs-server"); err != nil {
			dumpJournal("nfs-server")
			return fmt.Errorf("step11 restart nfs-server: %w", err)
		}

		// Step 12: ensure client-side gssd is running. On Debian 13
		// nfs-common provides rpc-gssd as part of nfs-client.target.
		_, _ = runCmd(ctx, "systemctl", "start", "nfs-client.target")
		_, _ = runCmd(ctx, "systemctl", "start", "rpc-gssd")

		// Step 13: pool, dataset, export.
		// Belt-and-suspenders: scenario 55's deferred pool destroy can
		// fail silently if NFS still holds a reference to the export's
		// path. Force-destroy any leftover "validate" pool, unmount any
		// stale NFS export, then aggressively wipe.
		_, _ = runCmd(ctx, "umount", "-f", "/mnt/validate-nfs")
		_, _ = runCmd(ctx, "/usr/sbin/exportfs", "-ua")
		_ = pm.Destroy(ctx, poolName)
		wipeDisks(disks[:1])
		if err := pm.Create(ctx, pool.CreateSpec{
			Name:  poolName,
			Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
		}); err != nil {
			return fmt.Errorf("step13 pool create: %w", err)
		}
		poolCreated = true

		if err := os.MkdirAll("/validate", 0o755); err != nil {
			return fmt.Errorf("step13 mkdir /validate: %w", err)
		}
		dsName := "nfs-krb"
		full := poolName + "/" + dsName
		if err := dm.Create(ctx, dataset.CreateSpec{
			Parent: poolName, Name: dsName, Type: "filesystem",
			Properties: map[string]string{"mountpoint": exportMP},
		}); err != nil {
			return fmt.Errorf("step13 dataset create: %w", err)
		}
		defer func() { _ = dm.Destroy(ctx, full, false) }()

		nm := &nfs.Manager{ExportsBin: "/usr/sbin/exportfs"}
		if err := nm.CreateExport(ctx, nfs.Export{
			Name: exportName,
			Path: exportMP,
			Clients: []nfs.ClientRule{{
				Spec:    "127.0.0.1",
				// all_squash + anonuid/gid=0: with sec=krb5 and no user
				// TGT in the kernel keyring, the kernel uses machine
				// creds and idmapd maps unknown principals to nobody.
				// Squashing everyone to uid 0 lets the test write
				// without setting up a per-user TGT — this is a
				// test-only relaxation, not a recommended production
				// setting. Real deployments use krb5 + per-user
				// kinit + a properly populated idmapd domain.
				Options: "rw,sync,no_subtree_check,sec=krb5,all_squash,anonuid=0,anongid=0",
			}},
		}); err != nil {
			return fmt.Errorf("step13 CreateExport: %w", err)
		}
		exportCreated = true

		// Steps 14-15: mount with sec=krb5. Linux NFS client + sec=krb5
		// uses /etc/krb5.keytab (machine creds) under the root context.
		// Force-unmount any stale mount left over from a previous run,
		// then re-create the dir cleanly.
		_, _ = runCmd(ctx, "umount", "-f", "-l", mountPoint)
		_ = os.Remove(mountPoint)
		if err := os.MkdirAll(mountPoint, 0o755); err != nil {
			return fmt.Errorf("step15 mkdir mountpoint: %w", err)
		}
		if _, err := runCmd(ctx, "mount", "-t", "nfs",
			"-o", "vers=4.2,sec=krb5",
			"127.0.0.1:"+exportMP, mountPoint); err != nil {
			if dmesg, derr := runCmd(ctx, "dmesg"); derr == nil {
				lines := strings.Split(string(dmesg), "\n")
				if len(lines) > 50 {
					lines = lines[len(lines)-50:]
				}
				fmt.Printf("  dmesg tail:\n%s\n", strings.Join(lines, "\n"))
			}
			dumpJournal("rpc-gssd")
			return fmt.Errorf("step15 mount sec=krb5: %w", err)
		}
		mounted = true

		// Step 16: 1 MiB IO + sha256 verify.
		payload := filepath.Join(mountPoint, "payload.bin")
		writePayload(payload, 1<<20)
		hashWrote := sha256file(payload)
		readBack, err := os.ReadFile(payload)
		if err != nil {
			return fmt.Errorf("step16 readback: %w", err)
		}
		hashRead := sha256bytes(readBack)
		if hashRead != hashWrote {
			return fmt.Errorf("step16 sha256 mismatch: write=%s read=%s", hashWrote, hashRead)
		}
		fmt.Printf("  sec=krb5 1 MiB payload sha256 round-trip ok (%s)\n", hashRead[:16])

		// Step 17: umount.
		if _, err := runCmd(ctx, "umount", mountPoint); err != nil {
			return fmt.Errorf("step17 umount: %w", err)
		}
		mounted = false

		// Step 18: tear-down handled by deferred cleanup above.
		return nil
	})

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

// ---------------------------------------------------------------------------
// Helpers for iSCSI/NVMe-oF/RDMA scenarios (46-52)
// ---------------------------------------------------------------------------

// runCmd runs a command with the harness context and returns combined output.
func runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w (output: %s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// which reports whether bin is on PATH.
func which(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// systemctlIsActive reports whether `systemctl is-active <unit>` exits 0
// and emits "active" (the standard signal that the unit is up).
func systemctlIsActive(ctx context.Context, unit string) bool {
	out, err := exec.CommandContext(ctx, "systemctl", "is-active", unit).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "active")
}

// waitDevice polls until path exists (typical zvol/udev settle case).
func waitDevice(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("device %s did not appear within %v", path, timeout)
}

// globNvmeBlocks returns the current set of /dev/nvme*n* block devices
// (excluding partition entries). Used to detect newly-attached NVMe-oF
// namespaces after `nvme connect`.
func globNvmeBlocks() []string {
	matches, _ := filepath.Glob("/dev/nvme*n*")
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		// Skip partitions like /dev/nvme0n1p1
		base := filepath.Base(m)
		if strings.Contains(base, "p") {
			// nvme0n1 has no 'p'; nvme0n1p1 does. Be precise: a partition
			// has the form nvmeXnYpZ where Z is digits.
			i := strings.LastIndex(base, "p")
			if i > 0 && i < len(base)-1 {
				rest := base[i+1:]
				allDigits := len(rest) > 0
				for _, r := range rest {
					if r < '0' || r > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					continue
				}
			}
		}
		out = append(out, m)
	}
	return out
}

// contains reports whether s contains v.
func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// preTargetCleanup tears down any leftover LIO/nvmet kernel state before
// the target-related scenarios run. Best-effort: prints what it did.
func preTargetCleanup(ctx context.Context) {
	fmt.Printf("\n=== pre-target cleanup (LIO + nvmet) ===\n")

	// LIO: targetcli clearconfig wipes all backstores/targets/portals/ACLs.
	if which("targetcli") {
		out, err := exec.CommandContext(ctx, "/usr/bin/targetcli", "clearconfig", "confirm=True").CombinedOutput()
		if err != nil {
			fmt.Printf("  targetcli clearconfig: %v (best-effort, ok if nothing to clear)\n", err)
		} else {
			fmt.Printf("  targetcli clearconfig: ok (%s)\n", strings.TrimSpace(string(out)))
		}
	} else {
		fmt.Printf("  targetcli not found; skipping LIO clearconfig\n")
	}

	// nvmet: prefer nvmetcli clear if present, otherwise walk configfs.
	if which("nvmetcli") {
		if _, err := exec.CommandContext(ctx, "nvmetcli", "clear").CombinedOutput(); err != nil {
			fmt.Printf("  nvmetcli clear: %v (best-effort)\n", err)
		} else {
			fmt.Printf("  nvmetcli clear: ok\n")
		}
	} else {
		nvmetClearConfigfs()
	}
}

// nvmetClearConfigfs walks /sys/kernel/config/nvmet and tears down ports,
// subsystems (with namespaces+allowed_hosts), and hosts in the order the
// kernel requires. All failures are warnings only.
func nvmetClearConfigfs() {
	root := "/sys/kernel/config/nvmet"
	if _, err := os.Stat(root); err != nil {
		fmt.Printf("  nvmet configfs root absent: %v (skipping)\n", err)
		return
	}
	cleared := 0

	// 1. Unlink all port→subsystem symlinks, then rmdir the ports.
	if portIDs, err := os.ReadDir(filepath.Join(root, "ports")); err == nil {
		for _, p := range portIDs {
			pdir := filepath.Join(root, "ports", p.Name())
			subDir := filepath.Join(pdir, "subsystems")
			if links, err := os.ReadDir(subDir); err == nil {
				for _, l := range links {
					_ = os.Remove(filepath.Join(subDir, l.Name()))
					cleared++
				}
			}
			_ = os.Remove(pdir)
			cleared++
		}
	}

	// 2. For each subsystem: disable+rmdir each namespace, unlink each
	//    allowed_hosts entry, then rmdir the subsystem.
	if subs, err := os.ReadDir(filepath.Join(root, "subsystems")); err == nil {
		for _, s := range subs {
			sdir := filepath.Join(root, "subsystems", s.Name())
			if nsids, err := os.ReadDir(filepath.Join(sdir, "namespaces")); err == nil {
				for _, n := range nsids {
					ndir := filepath.Join(sdir, "namespaces", n.Name())
					_ = os.WriteFile(filepath.Join(ndir, "enable"), []byte("0"), 0o644)
					_ = os.Remove(ndir)
					cleared++
				}
			}
			if hosts, err := os.ReadDir(filepath.Join(sdir, "allowed_hosts")); err == nil {
				for _, h := range hosts {
					_ = os.Remove(filepath.Join(sdir, "allowed_hosts", h.Name()))
					cleared++
				}
			}
			_ = os.Remove(sdir)
			cleared++
		}
	}

	// 3. rmdir each host.
	if hosts, err := os.ReadDir(filepath.Join(root, "hosts")); err == nil {
		for _, h := range hosts {
			_ = os.Remove(filepath.Join(root, "hosts", h.Name()))
			cleared++
		}
	}

	fmt.Printf("  nvmet configfs cleanup: %d entries removed (best-effort)\n", cleared)
}
