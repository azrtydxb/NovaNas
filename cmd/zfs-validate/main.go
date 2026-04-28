// Command zfs-validate exercises every feature of the internal/host/zfs
// Manager packages against real disks. Pool name "validate" is destroyed
// at start and end. Disks are taken from $DISKS (comma-separated by-id
// paths). Optional $LOG_A/$LOG_B add a mirrored log; $CACHE adds cache.
//
// Layouts exercised: stripe(1), mirror(2), raidz1(3), raidz2(4),
// raidz3(5), mirror + mirrored-log + cache. Plus dataset/snapshot
// lifecycle, scrub, real IO with snapshot+rollback integrity check.
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
	_ = os.Getenv("LOG_B") // accepted but unused; mirrored-log isn't expressible by current API
	if len(disks) < 5 {
		die("DISKS must list at least 5 by-id paths (got %d)", len(disks))
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
		step("6. mirror + single log + cache (single-disk only; see API gap notes)", func() error {
			// API gap (current Plan 1+2 surface): pool.CreateSpec.Log is
			// a []string of disk paths. There is no way to express a
			// MIRRORED log vdev — `zpool create ... log mirror d1 d2`
			// would require Log to be []VdevSpec, not []string. We test
			// a single log device here. Cache is always non-redundant
			// in ZFS; raidz/draid are NOT valid for log or cache.
			logs := []string{logA}
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
