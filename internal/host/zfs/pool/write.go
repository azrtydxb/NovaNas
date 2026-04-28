package pool

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/novanas/nova-nas/internal/host/exec"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
)

type CreateSpec struct {
	Name  string     `json:"name"`
	Vdevs []VdevSpec `json:"vdevs"`
	// Special vdevs hold metadata/small-blocks. Same shape as data vdevs.
	Special []VdevSpec `json:"special,omitempty"`
	// Dedup vdevs hold the dedup table. Same shape as data vdevs.
	Dedup []VdevSpec `json:"dedup,omitempty"`
	// Log accepts one or more log vdevs. Each log vdev is a single disk
	// (Type="disk") or a mirror (Type="mirror" with two disks). Other
	// types (raidz/draid) are rejected — ZFS does not support them for
	// log vdevs.
	Log []VdevSpec `json:"log,omitempty"`
	// Cache disks are non-redundant by design; expressed as a flat list.
	Cache []string `json:"cache,omitempty"`
	// Spare disks are individual hot-spares.
	Spare []string `json:"spare,omitempty"`
}

type VdevSpec struct {
	Type  string   `json:"type"` // mirror|raidz1|raidz2|raidz3|stripe|disk
	Disks []string `json:"disks"`
}

// AddSpec is the shape for `zpool add`. Same vdev shape as CreateSpec
// minus the pool name (the pool already exists).
type AddSpec struct {
	Vdevs   []VdevSpec `json:"vdevs,omitempty"`
	Special []VdevSpec `json:"special,omitempty"`
	Dedup   []VdevSpec `json:"dedup,omitempty"`
	Log     []VdevSpec `json:"log,omitempty"`
	Cache   []string   `json:"cache,omitempty"`
	Spare   []string   `json:"spare,omitempty"`
}

var validVdevTypes = map[string]struct{}{
	"mirror": {}, "raidz1": {}, "raidz2": {}, "raidz3": {}, "stripe": {}, "disk": {},
}

// validLogTypes is the subset of vdev types ZFS accepts for log vdevs.
// raidz/draid are NOT valid for log.
var validLogTypes = map[string]struct{}{
	"mirror": {}, "stripe": {}, "disk": {},
}

func emitVdevs(args []string, vdevs []VdevSpec, allowed map[string]struct{}) ([]string, error) {
	for _, v := range vdevs {
		isDraid := strings.HasPrefix(v.Type, "draid")
		if !isDraid {
			if _, ok := allowed[v.Type]; !ok {
				return nil, fmt.Errorf("invalid vdev type %q for this position", v.Type)
			}
		}
		if len(v.Disks) == 0 {
			return nil, fmt.Errorf("vdev %q has no disks", v.Type)
		}
		// "stripe" and "disk" are emitted as bare disks (no group token).
		// All other types (including draid<spec>) prefix with the type name.
		if v.Type != "stripe" && v.Type != "disk" {
			args = append(args, v.Type)
		}
		args = append(args, v.Disks...)
	}
	return args, nil
}

func buildCreateArgs(spec CreateSpec) ([]string, error) {
	if err := names.ValidatePoolName(spec.Name); err != nil {
		return nil, err
	}
	if len(spec.Vdevs) == 0 {
		return nil, fmt.Errorf("pool create: at least one vdev required")
	}
	args := []string{"create", "-f", spec.Name}
	var err error
	if args, err = emitVdevs(args, spec.Vdevs, validVdevTypes); err != nil {
		return nil, err
	}
	if len(spec.Special) > 0 {
		args = append(args, "special")
		if args, err = emitVdevs(args, spec.Special, validVdevTypes); err != nil {
			return nil, fmt.Errorf("special: %w", err)
		}
	}
	if len(spec.Dedup) > 0 {
		args = append(args, "dedup")
		if args, err = emitVdevs(args, spec.Dedup, validVdevTypes); err != nil {
			return nil, fmt.Errorf("dedup: %w", err)
		}
	}
	if len(spec.Log) > 0 {
		args = append(args, "log")
		if args, err = emitVdevs(args, spec.Log, validLogTypes); err != nil {
			return nil, fmt.Errorf("log: %w", err)
		}
	}
	if len(spec.Cache) > 0 {
		args = append(args, "cache")
		args = append(args, spec.Cache...)
	}
	if len(spec.Spare) > 0 {
		args = append(args, "spare")
		args = append(args, spec.Spare...)
	}
	return args, nil
}

func buildAddArgs(name string, spec AddSpec) ([]string, error) {
	if err := names.ValidatePoolName(name); err != nil {
		return nil, err
	}
	if len(spec.Vdevs) == 0 && len(spec.Special) == 0 && len(spec.Dedup) == 0 &&
		len(spec.Log) == 0 && len(spec.Cache) == 0 && len(spec.Spare) == 0 {
		return nil, fmt.Errorf("add: nothing to add")
	}
	args := []string{"add", "-f", name}
	var err error
	if args, err = emitVdevs(args, spec.Vdevs, validVdevTypes); err != nil {
		return nil, err
	}
	if len(spec.Special) > 0 {
		args = append(args, "special")
		if args, err = emitVdevs(args, spec.Special, validVdevTypes); err != nil {
			return nil, fmt.Errorf("special: %w", err)
		}
	}
	if len(spec.Dedup) > 0 {
		args = append(args, "dedup")
		if args, err = emitVdevs(args, spec.Dedup, validVdevTypes); err != nil {
			return nil, fmt.Errorf("dedup: %w", err)
		}
	}
	if len(spec.Log) > 0 {
		args = append(args, "log")
		if args, err = emitVdevs(args, spec.Log, validLogTypes); err != nil {
			return nil, fmt.Errorf("log: %w", err)
		}
	}
	if len(spec.Cache) > 0 {
		args = append(args, "cache")
		args = append(args, spec.Cache...)
	}
	if len(spec.Spare) > 0 {
		args = append(args, "spare")
		args = append(args, spec.Spare...)
	}
	return args, nil
}

func (m *Manager) run(ctx context.Context, args ...string) ([]byte, error) {
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	return runner(ctx, m.ZpoolBin, args...)
}

func (m *Manager) Create(ctx context.Context, spec CreateSpec) error {
	args, err := buildCreateArgs(spec)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

func (m *Manager) Destroy(ctx context.Context, name string) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	_, err := m.run(ctx, "destroy", "-f", name)
	return err
}

type ScrubAction string

const (
	ScrubStart ScrubAction = "start"
	ScrubStop  ScrubAction = "stop"
)

func (m *Manager) Scrub(ctx context.Context, name string, action ScrubAction) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	args := []string{"scrub"}
	if action == ScrubStop {
		args = append(args, "-s")
	}
	args = append(args, name)
	_, err := m.run(ctx, args...)
	return err
}

// Replace replaces oldDisk in pool with newDisk and resilvers.
func (m *Manager) Replace(ctx context.Context, name, oldDisk, newDisk string) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	if oldDisk == "" || newDisk == "" {
		return fmt.Errorf("old and new disk paths required")
	}
	if strings.HasPrefix(oldDisk, "-") || strings.HasPrefix(newDisk, "-") {
		return fmt.Errorf("disk path cannot start with '-' (argv injection guard)")
	}
	_, err := m.run(ctx, "replace", "-f", name, oldDisk, newDisk)
	return err
}

// Offline marks a disk offline within a pool. If temporary is true, the
// disk comes back online on the next reboot.
func (m *Manager) Offline(ctx context.Context, name, disk string, temporary bool) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	if disk == "" || strings.HasPrefix(disk, "-") {
		return fmt.Errorf("disk path required and cannot start with '-'")
	}
	args := []string{"offline"}
	if temporary {
		args = append(args, "-t")
	}
	args = append(args, name, disk)
	_, err := m.run(ctx, args...)
	return err
}

// Online brings a previously-offline disk back into service.
func (m *Manager) Online(ctx context.Context, name, disk string) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	if disk == "" || strings.HasPrefix(disk, "-") {
		return fmt.Errorf("disk path required and cannot start with '-'")
	}
	_, err := m.run(ctx, "online", name, disk)
	return err
}

// Clear resets error counters for the pool, optionally scoped to a disk.
func (m *Manager) Clear(ctx context.Context, name, disk string) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	args := []string{"clear", name}
	if disk != "" {
		if strings.HasPrefix(disk, "-") {
			return fmt.Errorf("disk path cannot start with '-'")
		}
		args = append(args, disk)
	}
	_, err := m.run(ctx, args...)
	return err
}

// Attach grows a single-disk vdev into a mirror, or extends an existing
// mirror with another disk. Also used for RAIDZ expansion (ZFS 2.3+):
// pass the raidz vdev name (e.g. "raidz1-0") as existing.
func (m *Manager) Attach(ctx context.Context, name, existing, newDisk string) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	if existing == "" || newDisk == "" {
		return fmt.Errorf("existing and new disk required")
	}
	if strings.HasPrefix(existing, "-") || strings.HasPrefix(newDisk, "-") {
		return fmt.Errorf("disk path cannot start with '-'")
	}
	_, err := m.run(ctx, "attach", "-f", name, existing, newDisk)
	return err
}

// Detach removes a disk from a mirror vdev (or from a single-disk vdev,
// breaking it back to no redundancy).
func (m *Manager) Detach(ctx context.Context, name, disk string) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	if disk == "" || strings.HasPrefix(disk, "-") {
		return fmt.Errorf("disk path required and cannot start with '-'")
	}
	_, err := m.run(ctx, "detach", name, disk)
	return err
}

// Add adds new top-level vdev(s) to an existing pool: data vdevs, log,
// cache, or spare. Use to expand a pool's storage or attach SSD log/cache.
func (m *Manager) Add(ctx context.Context, name string, spec AddSpec) error {
	args, err := buildAddArgs(name, spec)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

// Export marks a pool exported (offline) so it can be moved to another
// host or reimported. With force=true, busy filesystems are torn down.
func (m *Manager) Export(ctx context.Context, name string, force bool) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	args := []string{"export"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)
	_, err := m.run(ctx, args...)
	return err
}

// Import imports a previously-exported pool by name.
func (m *Manager) Import(ctx context.Context, name string) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	_, err := m.run(ctx, "import", name)
	return err
}

// ImportablePool describes a pool available for import.
type ImportablePool struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Status string `json:"status,omitempty"`
}

// Importable lists pools available for import (e.g. unimported pools on
// disks that this host can see). Empty result is normal.
func (m *Manager) Importable(ctx context.Context) ([]ImportablePool, error) {
	out, err := m.run(ctx, "import")
	if err != nil {
		// `zpool import` with no available pools returns exit 0 + the
		// message "no pools available to import" on stderr OR 0 with
		// nothing. Some implementations return non-zero when no pools
		// are available. Treat that case as empty.
		var he *exec.HostError
		if errors.As(err, &he) && strings.Contains(he.Stderr, "no pools available") {
			return nil, nil
		}
		return nil, err
	}
	return parseImportable(out), nil
}

// parseImportable walks the `zpool import` (no args) output and pulls
// out (name, state) tuples. Best-effort — output format isn't formal.
func parseImportable(data []byte) []ImportablePool {
	var out []ImportablePool
	var cur *ImportablePool
	for _, line := range strings.Split(string(data), "\n") {
		trim := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trim, "pool: "):
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &ImportablePool{Name: strings.TrimSpace(strings.TrimPrefix(trim, "pool:"))}
		case strings.HasPrefix(trim, "state: ") && cur != nil:
			cur.State = strings.TrimSpace(strings.TrimPrefix(trim, "state:"))
		case strings.HasPrefix(trim, "status: ") && cur != nil:
			cur.Status = strings.TrimSpace(strings.TrimPrefix(trim, "status:"))
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}
