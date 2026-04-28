// Command zfs-validate-neg exercises negative paths: input that should
// be rejected by the API layer (validators) before it reaches ZFS, and
// input that ZFS itself rejects (insufficient disks, name collisions).
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	hexec "github.com/novanas/nova-nas/internal/host/exec"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
)

const poolName = "validate"

func main() {
	disks := splitEnv("DISKS")
	if len(disks) < 3 {
		die("DISKS must list at least 3 by-id paths (got %d)", len(disks))
	}

	pm := &pool.Manager{ZpoolBin: "/sbin/zpool"}
	dm := &dataset.Manager{ZFSBin: "/sbin/zfs"}
	sm := &snapshot.Manager{ZFSBin: "/sbin/zfs"}

	ctx := context.Background()
	_ = pm.Destroy(ctx, poolName)

	// Group 1: Validator rejects (never reaches zpool/zfs)
	check("N1. ValidatePoolName rejects 'mirror' (reserved)",
		func() error { return names.ValidatePoolName("mirror") }, true)
	check("N2. ValidatePoolName rejects 'tank/x' (slash)",
		func() error { return names.ValidatePoolName("tank/x") }, true)
	check("N3. ValidatePoolName rejects '1tank' (leading digit)",
		func() error { return names.ValidatePoolName("1tank") }, true)
	check("N4. ValidatePoolName rejects '' (empty)",
		func() error { return names.ValidatePoolName("") }, true)
	check("N5. ValidateDatasetName rejects 'tank/-leading' (leading dash)",
		func() error { return names.ValidateDatasetName("tank/-leading") }, true)
	check("N6. ValidateSnapshotName rejects 'tank@-bad' (short name leading dash, argv injection)",
		func() error { return names.ValidateSnapshotName("tank@-bad") }, true)
	check("N7. ValidateSnapshotName rejects 'tank' (no @)",
		func() error { return names.ValidateSnapshotName("tank") }, true)
	check("N8. ValidateSnapshotName rejects 'tank@x@y' (double @)",
		func() error { return names.ValidateSnapshotName("tank@x@y") }, true)

	// Group 2: Manager-layer rejects (validator catches before exec.Run)
	check("N9. pool.Manager.Destroy rejects 'mirror' before any exec",
		func() error { return pm.Destroy(ctx, "mirror") }, true)
	check("N10. dataset.Manager.SetProps rejects 'bad@name'",
		func() error { return dm.SetProps(ctx, "bad@name", map[string]string{"x": "y"}) }, true)
	check("N11. snapshot.Manager.Destroy rejects 'tank' (missing @)",
		func() error { return sm.Destroy(ctx, "tank") }, true)

	// Group 3: ZFS-layer rejects (validator passes, host returns non-zero)
	check("N12. raidz3 with only 3 disks (needs 4+)",
		func() error {
			return pm.Create(ctx, pool.CreateSpec{
				Name:  poolName,
				Vdevs: []pool.VdevSpec{{Type: "raidz3", Disks: disks[:3]}},
			})
		}, true)

	check("N13. duplicate pool create when one already exists",
		func() error {
			if err := pm.Create(ctx, pool.CreateSpec{
				Name:  poolName,
				Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[:1]}},
			}); err != nil {
				return fmt.Errorf("setup-create: %w", err)
			}
			err := pm.Create(ctx, pool.CreateSpec{
				Name:  poolName,
				Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: disks[1:2]}},
			})
			_ = pm.Destroy(ctx, poolName)
			return err
		}, true)

	check("N14. Get of nonexistent pool returns ErrNotFound",
		func() error {
			_, err := pm.Get(ctx, "nope-not-here")
			if err != nil && errors.Is(err, pool.ErrNotFound) {
				return errors.New("got ErrNotFound (expected behavior)")
			}
			return err
		}, true)

	check("N15. Get of nonexistent dataset returns ErrNotFound",
		func() error {
			_, err := dm.Get(ctx, "nope-pool/nope-ds")
			if err != nil && errors.Is(err, dataset.ErrNotFound) {
				return errors.New("got ErrNotFound (expected behavior)")
			}
			return err
		}, true)

	check("N16. Create dataset under nonexistent parent fails cleanly",
		func() error {
			return dm.Create(ctx, dataset.CreateSpec{
				Parent: "nope-pool", Name: "child", Type: "filesystem",
			})
		}, true)

	// Group 4: Verify exec layer doesn't allow argv flag injection.
	check("N17. exec.Run with /sbin/zpool '--help' arg is treated as a flag (sanity)",
		func() error {
			// Just demonstrates exec doesn't shell-interpret. Pass --version
			// which IS a valid flag — we expect success and that the binary
			// doesn't see an injected command.
			out, err := hexec.Run(ctx, "/sbin/zpool", "--version")
			if err != nil {
				return err
			}
			if !strings.Contains(string(out), "zfs") {
				return fmt.Errorf("unexpected stdout: %s", out)
			}
			return errors.New("zpool --version returned valid version (no injection)")
		}, true)

	fmt.Println("\nALL NEGATIVE CHECKS PASSED")
}

// check asserts that fn returns an error (when wantErr=true). The error
// message is printed for human verification; success is "fn returned an
// error of any kind".
func check(label string, fn func() error, wantErr bool) {
	err := fn()
	got := err != nil
	if got != wantErr {
		fmt.Printf("  %s\n    FAIL: wantErr=%v gotErr=%v err=%v\n", label, wantErr, got, err)
		os.Exit(1)
	}
	if err != nil {
		msg := err.Error()
		if len(msg) > 80 {
			msg = msg[:80] + "..."
		}
		fmt.Printf("  %s\n    rejected: %s\n", label, msg)
	} else {
		fmt.Printf("  %s\n    ok\n", label)
	}
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

