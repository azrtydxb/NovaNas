package nvmeof

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/novanas/nova-nas/internal/host/configfs"
)

// SavedConfigVersion is the current serialised schema version. Bump on
// any incompatible change to SavedConfig and reject older/newer versions
// in Restore so operators get a clear error rather than silent drift.
const SavedConfigVersion = 1

// SavedConfig is the serialised NVMe-oF target configuration. Format
// version is bumped on incompatible schema changes.
type SavedConfig struct {
	Version    int              `json:"version"`
	Subsystems []SavedSubsystem `json:"subsystems"`
	Ports      []SavedPort      `json:"ports"`
	Hosts      []string         `json:"hosts"` // host NQNs (just the directory presence)
}

// SavedSubsystem captures the per-subsystem attributes plus its
// namespaces and allowed_hosts symlink targets.
type SavedSubsystem struct {
	NQN          string           `json:"nqn"`
	AllowAnyHost bool             `json:"allowAnyHost"`
	Serial       string           `json:"serial,omitempty"`
	Namespaces   []SavedNamespace `json:"namespaces"`
	AllowedHosts []string         `json:"allowedHosts"`
}

// SavedNamespace mirrors Namespace but is named separately so future
// schema evolution doesn't bind serialised form to in-memory shape.
type SavedNamespace struct {
	NSID       int    `json:"nsid"`
	DevicePath string `json:"devicePath"`
	Enabled    bool   `json:"enabled"`
}

// SavedPort mirrors Port and additionally records which subsystems are
// linked under the port at snapshot time.
type SavedPort struct {
	ID         int      `json:"id"`
	IP         string   `json:"ip"`
	Port       int      `json:"port"`
	Transport  string   `json:"transport"`
	Subsystems []string `json:"subsystems"` // NQNs linked under this port
}

// Save walks the live nvmet configfs tree and returns a snapshot.
// The snapshot is sorted (subsystems by NQN, ports by ID, link/host
// lists alphabetically) so byte-identical output is produced for
// identical state, which makes diffing and round-trip testing
// straightforward.
func (m *Manager) Save(ctx context.Context) (*SavedConfig, error) {
	c := m.cfs()
	cfg := &SavedConfig{Version: SavedConfigVersion}

	// Subsystems (with namespaces + allowed hosts).
	subs, err := m.ListSubsystems(ctx)
	if err != nil {
		return nil, fmt.Errorf("save: list subsystems: %w", err)
	}
	sort.Slice(subs, func(i, j int) bool { return subs[i].NQN < subs[j].NQN })
	for _, s := range subs {
		d, err := m.GetSubsystem(ctx, s.NQN)
		if err != nil {
			return nil, fmt.Errorf("save: get subsystem %q: %w", s.NQN, err)
		}
		ss := SavedSubsystem{
			NQN:          d.Subsystem.NQN,
			AllowAnyHost: d.Subsystem.AllowAnyHost,
			Serial:       d.Subsystem.Serial,
		}
		for _, ns := range d.Namespaces {
			ss.Namespaces = append(ss.Namespaces, SavedNamespace{
				NSID: ns.NSID, DevicePath: ns.DevicePath, Enabled: ns.Enabled,
			})
		}
		sort.Slice(ss.Namespaces, func(i, j int) bool {
			return ss.Namespaces[i].NSID < ss.Namespaces[j].NSID
		})
		ss.AllowedHosts = append([]string(nil), d.AllowedHosts...)
		sort.Strings(ss.AllowedHosts)
		cfg.Subsystems = append(cfg.Subsystems, ss)
	}

	// Ports (with linked subsystem NQNs).
	ports, err := m.ListPorts(ctx)
	if err != nil {
		return nil, fmt.Errorf("save: list ports: %w", err)
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i].ID < ports[j].ID })
	for _, p := range ports {
		sp := SavedPort{
			ID:        p.ID,
			IP:        p.IP,
			Port:      p.Port,
			Transport: p.Transport,
		}
		linkDir := path.Join(portDir(p.ID), "subsystems")
		links, err := c.ListDir(linkDir)
		if err != nil && !errors.Is(err, configfs.ErrNotExist) {
			return nil, fmt.Errorf("save: list port %d subsystems: %w", p.ID, err)
		}
		sort.Strings(links)
		sp.Subsystems = links
		cfg.Ports = append(cfg.Ports, sp)
	}

	// Hosts: every host directory under nvmet/hosts. We persist host
	// NQNs even if no subsystem references them, because operators may
	// have pre-provisioned host identities (e.g. for DH-HMAC-CHAP keys
	// to be filled in later).
	hosts, err := c.ListDir("nvmet/hosts")
	if err != nil && !errors.Is(err, configfs.ErrNotExist) {
		return nil, fmt.Errorf("save: list hosts: %w", err)
	}
	sort.Strings(hosts)
	cfg.Hosts = hosts

	return cfg, nil
}

// SaveToFile snapshots state and writes it atomically to path with
// mode 0600. The temp file is fsynced before rename so that a crash
// after rename leaves the file fully on disk; mode 0600 because the
// snapshot may include serial numbers and operator-chosen NQNs that
// are useful for fingerprinting the host.
func (m *Manager) SaveToFile(ctx context.Context, p string) error {
	cfg, err := m.Save(ctx)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("save: marshal: %w", err)
	}
	// Ensure trailing newline so editors and POSIX tools are happy.
	data = append(data, '\n')

	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("save: mkdir %q: %w", dir, err)
	}
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("save: create temp: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("save: write temp: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("save: fsync temp: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("save: close temp: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("save: rename: %w", err)
	}
	// Best-effort: ensure the rename is durable. Errors here aren't
	// fatal; callers checking for "did the file end up on disk" should
	// rely on the rename having completed.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Restore re-applies the saved state. Caller is responsible for
// ensuring the configfs tree is in a known-clean state (typically by
// running ClearAll first, or by virtue of being on a freshly booted
// system after `modprobe nvmet`).
//
// On the first error, Restore returns immediately and does not attempt
// to roll back. The operator's logs combined with a re-run after
// fixing the underlying issue is sufficient: nvmet config is small
// and re-applying is idempotent at the directory-creation level.
func (m *Manager) Restore(ctx context.Context, cfg SavedConfig) error {
	if cfg.Version != SavedConfigVersion {
		return fmt.Errorf("restore: unsupported config version %d (want %d)",
			cfg.Version, SavedConfigVersion)
	}
	// 1) Hosts first so allowed_hosts symlinks below have a target.
	for _, h := range cfg.Hosts {
		if err := m.EnsureHost(ctx, h); err != nil {
			return fmt.Errorf("restore: host %q: %w", h, err)
		}
	}
	// 2) Subsystems, namespaces, allowed_hosts.
	for _, s := range cfg.Subsystems {
		sub := Subsystem{NQN: s.NQN, AllowAnyHost: s.AllowAnyHost, Serial: s.Serial}
		if err := m.CreateSubsystem(ctx, sub); err != nil {
			return fmt.Errorf("restore: subsystem %q: %w", s.NQN, err)
		}
		for _, ns := range s.Namespaces {
			if err := m.AddNamespace(ctx, s.NQN, Namespace{
				NSID: ns.NSID, DevicePath: ns.DevicePath, Enabled: ns.Enabled,
			}); err != nil {
				return fmt.Errorf("restore: namespace %d on %q: %w", ns.NSID, s.NQN, err)
			}
		}
		for _, h := range s.AllowedHosts {
			if err := m.AllowHost(ctx, s.NQN, h); err != nil {
				return fmt.Errorf("restore: allow host %q on %q: %w", h, s.NQN, err)
			}
		}
	}
	// 3) Ports, then port→subsystem links.
	for _, p := range cfg.Ports {
		if err := m.CreatePort(ctx, Port{
			ID: p.ID, IP: p.IP, Port: p.Port, Transport: p.Transport,
		}); err != nil {
			return fmt.Errorf("restore: port %d: %w", p.ID, err)
		}
		for _, nqn := range p.Subsystems {
			if err := m.LinkSubsystemToPort(ctx, nqn, p.ID); err != nil {
				return fmt.Errorf("restore: link %q to port %d: %w", nqn, p.ID, err)
			}
		}
	}
	return nil
}

// RestoreFromFile reads p, unmarshals, and applies the snapshot. If p
// does not exist, the returned error wraps os.ErrNotExist so callers
// can branch on first-boot ("nothing to restore") without it being a
// hard failure.
func (m *Manager) RestoreFromFile(ctx context.Context, p string) error {
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("restore: read %q: %w", p, os.ErrNotExist)
		}
		return fmt.Errorf("restore: read %q: %w", p, err)
	}
	var cfg SavedConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("restore: unmarshal %q: %w", p, err)
	}
	return m.Restore(ctx, cfg)
}

// ClearAll tears down all current state in the order required by
// nvmet's lifecycle rules:
//
//  1. For each port: unlink all subsystem symlinks, then rmdir the port.
//  2. For each subsystem: disable+rmdir all namespaces, unlink all
//     allowed_hosts entries, then rmdir the subsystem.
//  3. For each host: rmdir.
//
// Mirrors the harness teardown logic. Safe to call on an empty tree
// (returns nil if nvmet/* directories don't exist).
func (m *Manager) ClearAll(ctx context.Context) error {
	c := m.cfs()

	// Ports first: a subsystem symlinked under a port cannot have its
	// directory removed while the link exists (kernel keeps a refcount).
	portIDs, err := c.ListDir("nvmet/ports")
	if err != nil && !errors.Is(err, configfs.ErrNotExist) {
		return fmt.Errorf("clear: list ports: %w", err)
	}
	for _, name := range portIDs {
		id, convErr := strconv.Atoi(name)
		if convErr != nil {
			continue
		}
		if err := m.DeletePort(ctx, id); err != nil {
			return fmt.Errorf("clear: delete port %d: %w", id, err)
		}
	}

	// Subsystems: DeleteSubsystem already handles namespaces +
	// allowed_hosts unlinks in the correct order.
	subs, err := c.ListDir("nvmet/subsystems")
	if err != nil && !errors.Is(err, configfs.ErrNotExist) {
		return fmt.Errorf("clear: list subsystems: %w", err)
	}
	for _, nqn := range subs {
		if err := m.DeleteSubsystem(ctx, nqn); err != nil {
			return fmt.Errorf("clear: delete subsystem %q: %w", nqn, err)
		}
	}

	// Hosts last: a host directory cannot be removed while any
	// subsystem references it; by now all such symlinks are gone.
	hosts, err := c.ListDir("nvmet/hosts")
	if err != nil && !errors.Is(err, configfs.ErrNotExist) {
		return fmt.Errorf("clear: list hosts: %w", err)
	}
	for _, h := range hosts {
		if err := m.RemoveHost(ctx, h); err != nil {
			return fmt.Errorf("clear: remove host %q: %w", h, err)
		}
	}
	return nil
}
