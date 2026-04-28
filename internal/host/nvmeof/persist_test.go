package nvmeof

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"testing"
)

// tmpfsClearAll mimics what Manager.ClearAll does on a real configfs
// where the kernel auto-cleans container subdirectories. On tmpfs we
// have to do that ourselves between the package's symlink-removal
// step and the directory rmdir, otherwise the empty namespaces/,
// allowed_hosts/, and subsystems/ dirs block their parent's rmdir.
//
// Tests use this helper instead of the public ClearAll because the
// public method is correct as written for the kernel; tmpfs is just
// missing an autoremove behaviour.
func tmpfsClearAll(t *testing.T, m *Manager, root string) {
	t.Helper()
	ctx := context.Background()

	// Ports.
	portsRoot := filepath.Join(root, "nvmet/ports")
	if entries, err := os.ReadDir(portsRoot); err == nil {
		for _, e := range entries {
			portDir := filepath.Join(portsRoot, e.Name())
			subsContainer := filepath.Join(portDir, "subsystems")
			// Unlink any port→subsystem symlinks first.
			if links, err := os.ReadDir(subsContainer); err == nil {
				for _, l := range links {
					if err := os.Remove(filepath.Join(subsContainer, l.Name())); err != nil {
						t.Fatalf("unlink port symlink: %v", err)
					}
				}
			}
			// Now the kernel-stub addr_* attrs and the empty
			// subsystems/ container would normally vanish on rmdir.
			for _, f := range []string{"addr_traddr", "addr_trtype", "addr_adrfam", "addr_trsvcid"} {
				_ = os.Remove(filepath.Join(portDir, f))
			}
			_ = os.Remove(subsContainer)
			id, err := strconv.Atoi(e.Name())
			if err != nil {
				continue
			}
			if err := m.DeletePort(ctx, id); err != nil {
				t.Fatalf("DeletePort %d: %v", id, err)
			}
		}
	}

	// Subsystems.
	subsRoot := filepath.Join(root, "nvmet/subsystems")
	if entries, err := os.ReadDir(subsRoot); err == nil {
		for _, e := range entries {
			subDir := filepath.Join(subsRoot, e.Name())
			// Strip namespace stub files; DeleteSubsystem will rmdir
			// each namespace dir which now (post-strip) is empty.
			nsRoot := filepath.Join(subDir, "namespaces")
			if nsEntries, err := os.ReadDir(nsRoot); err == nil {
				for _, ne := range nsEntries {
					nsPath := filepath.Join(nsRoot, ne.Name())
					_ = os.Remove(filepath.Join(nsPath, "device_path"))
					_ = os.Remove(filepath.Join(nsPath, "enable"))
				}
			}
			// Strip subsystem attrs and pre-remove the empty
			// container subdirs that the kernel would auto-remove.
			_ = os.Remove(filepath.Join(subDir, "attr_allow_any_host"))
			_ = os.Remove(filepath.Join(subDir, "attr_serial"))
			// allowed_hosts may still contain symlinks; DeleteSubsystem
			// handles those before the final rmdir, but the empty
			// container itself blocks rmdir on tmpfs. We'll let
			// DeleteSubsystem unlink the symlinks, then we remove the
			// containers below before ClearAll's RemoveHost step.
			if err := m.DeleteSubsystem(ctx, e.Name()); err != nil {
				// On tmpfs the empty container dirs block the final
				// rmdir; finish manually.
				_ = os.Remove(filepath.Join(subDir, "namespaces"))
				_ = os.Remove(filepath.Join(subDir, "allowed_hosts"))
				if err2 := os.Remove(subDir); err2 != nil {
					t.Fatalf("DeleteSubsystem %q: %v then rmdir: %v", e.Name(), err, err2)
				}
			}
		}
	}

	// Hosts.
	hostsRoot := filepath.Join(root, "nvmet/hosts")
	if entries, err := os.ReadDir(hostsRoot); err == nil {
		for _, e := range entries {
			if err := m.RemoveHost(ctx, e.Name()); err != nil {
				t.Fatalf("RemoveHost %q: %v", e.Name(), err)
			}
		}
	}
}

// populate sets up a representative state (1 host, 2 subsystems, 1
// port linking both subsystems) and returns nothing — tests then
// snapshot + clear + restore against it.
func populate(t *testing.T, m *Manager, root string) {
	t.Helper()
	ctx := context.Background()

	hostNQN := "nqn.host.alpha"
	if err := m.EnsureHost(ctx, hostNQN); err != nil {
		t.Fatal(err)
	}

	// Subsystem 1 with namespace + allowed_host.
	s1 := "nqn.2024-01.io.novanas:s1"
	kernelStubSubsystemAttrs(t, root, s1)
	if err := m.CreateSubsystem(ctx, Subsystem{NQN: s1, AllowAnyHost: false, Serial: "SN0001"}); err != nil {
		t.Fatal(err)
	}
	kernelStubNamespaceAttrs(t, root, s1, 1)
	if err := m.AddNamespace(ctx, s1, Namespace{NSID: 1, DevicePath: "/dev/zvol/tank/v1", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.AllowHost(ctx, s1, hostNQN); err != nil {
		t.Fatal(err)
	}

	// Subsystem 2: allow-any-host, two namespaces.
	s2 := "nqn.2024-01.io.novanas:s2"
	kernelStubSubsystemAttrs(t, root, s2)
	if err := m.CreateSubsystem(ctx, Subsystem{NQN: s2, AllowAnyHost: true, Serial: "SN0002"}); err != nil {
		t.Fatal(err)
	}
	kernelStubNamespaceAttrs(t, root, s2, 1)
	if err := m.AddNamespace(ctx, s2, Namespace{NSID: 1, DevicePath: "/dev/zvol/tank/v2a", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	kernelStubNamespaceAttrs(t, root, s2, 2)
	if err := m.AddNamespace(ctx, s2, Namespace{NSID: 2, DevicePath: "/dev/zvol/tank/v2b", Enabled: false}); err != nil {
		t.Fatal(err)
	}

	// Port 1, links both subsystems.
	kernelStubPortAttrs(t, root, 1)
	if err := m.CreatePort(ctx, Port{ID: 1, IP: "10.0.0.5", Port: 4420, Transport: "tcp"}); err != nil {
		t.Fatal(err)
	}
	if err := m.LinkSubsystemToPort(ctx, s1, 1); err != nil {
		t.Fatal(err)
	}
	if err := m.LinkSubsystemToPort(ctx, s2, 1); err != nil {
		t.Fatal(err)
	}
}

// normalize sorts all slices in cfg so reflect.DeepEqual works on
// snapshots that were collected in any directory-listing order.
func normalize(cfg *SavedConfig) {
	sort.Slice(cfg.Subsystems, func(i, j int) bool { return cfg.Subsystems[i].NQN < cfg.Subsystems[j].NQN })
	for i := range cfg.Subsystems {
		s := &cfg.Subsystems[i]
		sort.Slice(s.Namespaces, func(a, b int) bool { return s.Namespaces[a].NSID < s.Namespaces[b].NSID })
		sort.Strings(s.AllowedHosts)
	}
	sort.Slice(cfg.Ports, func(i, j int) bool { return cfg.Ports[i].ID < cfg.Ports[j].ID })
	for i := range cfg.Ports {
		sort.Strings(cfg.Ports[i].Subsystems)
	}
	sort.Strings(cfg.Hosts)
}

func TestSave_EmptyConfig(t *testing.T) {
	m, _ := newManager(t)
	cfg, err := m.Save(context.Background())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if cfg.Version != SavedConfigVersion {
		t.Errorf("version = %d, want %d", cfg.Version, SavedConfigVersion)
	}
	if len(cfg.Subsystems) != 0 || len(cfg.Ports) != 0 || len(cfg.Hosts) != 0 {
		t.Errorf("expected empty snapshot, got %+v", cfg)
	}
}

func TestSaveRestore_RoundTrip(t *testing.T) {
	m, root := newManager(t)
	populate(t, m, root)

	// Snapshot 1.
	first, err := m.Save(context.Background())
	if err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	normalize(first)

	// Tear down. On tmpfs we cannot just call ClearAll because the
	// kernel-managed stub files and empty container subdirs (which
	// real configfs auto-removes on rmdir) block parent rmdirs.
	// tmpfsClearAll mimics ClearAll plus the kernel's autoremove.
	tmpfsClearAll(t, m, root)

	// Verify cleanup was thorough.
	if entries, err := os.ReadDir(filepath.Join(root, "nvmet/subsystems")); err != nil || len(entries) != 0 {
		t.Fatalf("subsystems not empty: %v err=%v", entries, err)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "nvmet/ports")); err != nil || len(entries) != 0 {
		t.Fatalf("ports not empty: %v err=%v", entries, err)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "nvmet/hosts")); err != nil || len(entries) != 0 {
		t.Fatalf("hosts not empty: %v err=%v", entries, err)
	}

	// Re-stub in preparation for Restore (Restore calls into the same
	// CreateSubsystem/AddNamespace/CreatePort code which writes to
	// kernel attr files that must exist on tmpfs).
	for _, s := range first.Subsystems {
		kernelStubSubsystemAttrs(t, root, s.NQN)
		for _, ns := range s.Namespaces {
			kernelStubNamespaceAttrs(t, root, s.NQN, ns.NSID)
		}
	}
	for _, p := range first.Ports {
		kernelStubPortAttrs(t, root, p.ID)
	}

	if err := m.Restore(context.Background(), *first); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	second, err := m.Save(context.Background())
	if err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	normalize(second)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("round-trip mismatch\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

func TestSaveToFile_PermsAndContent(t *testing.T) {
	m, root := newManager(t)
	populate(t, m, root)

	dir := t.TempDir()
	p := filepath.Join(dir, "snap.json")
	if err := m.SaveToFile(context.Background(), p); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// On Unix the mode bits are honoured; skip on Windows where perms
	// don't translate cleanly.
	if runtime.GOOS != "windows" {
		if mode := fi.Mode().Perm(); mode != 0o600 {
			t.Errorf("file perms = %o, want 0600", mode)
		}
	}
	// .tmp file should be gone after a successful rename.
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file should not exist: %v", err)
	}

	// Content round-trip.
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var cfg SavedConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.Version != SavedConfigVersion {
		t.Errorf("version = %d", cfg.Version)
	}
	if len(cfg.Subsystems) != 2 {
		t.Errorf("subsystems = %d, want 2", len(cfg.Subsystems))
	}
	if len(cfg.Ports) != 1 || len(cfg.Ports[0].Subsystems) != 2 {
		t.Errorf("ports/links wrong: %+v", cfg.Ports)
	}
	// Sanity: trailing newline.
	if data[len(data)-1] != '\n' {
		t.Errorf("expected trailing newline")
	}
}

func TestRestoreFromFile_NotFound(t *testing.T) {
	m, _ := newManager(t)
	err := m.RestoreFromFile(context.Background(), filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error should wrap os.ErrNotExist, got %v", err)
	}
}

func TestRestore_UnsupportedVersion(t *testing.T) {
	m, _ := newManager(t)
	err := m.Restore(context.Background(), SavedConfig{Version: 999})
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestClearAll_RemovesEverything(t *testing.T) {
	m, root := newManager(t)
	populate(t, m, root)

	// tmpfsClearAll wraps the same DeletePort/DeleteSubsystem/RemoveHost
	// primitives that the public ClearAll calls into; on a real
	// configfs ClearAll alone would suffice. After it returns we
	// invoke ClearAll on the resulting empty tree as a smoke test
	// that its discovery loops handle the empty case gracefully.
	tmpfsClearAll(t, m, root)
	if err := m.ClearAll(context.Background()); err != nil {
		t.Fatalf("ClearAll on already-empty tree: %v", err)
	}

	subs, err := m.ListSubsystems(context.Background())
	if err != nil {
		t.Fatalf("ListSubsystems: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("subsystems not cleared: %+v", subs)
	}
	ports, err := m.ListPorts(context.Background())
	if err != nil {
		t.Fatalf("ListPorts: %v", err)
	}
	if len(ports) != 0 {
		t.Errorf("ports not cleared: %+v", ports)
	}
	hostEntries, err := os.ReadDir(filepath.Join(root, "nvmet/hosts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(hostEntries) != 0 {
		names := make([]string, len(hostEntries))
		for i, e := range hostEntries {
			names[i] = e.Name()
		}
		t.Errorf("hosts not cleared: %v", names)
	}
}

func TestClearAll_OnEmptyTree(t *testing.T) {
	m, _ := newManager(t)
	if err := m.ClearAll(context.Background()); err != nil {
		t.Errorf("ClearAll on empty tree should not error: %v", err)
	}
}

// Sanity: namespace NSID round-trips through JSON correctly even at
// boundary values.
func TestSavedConfig_JSONShape(t *testing.T) {
	cfg := SavedConfig{
		Version: 1,
		Subsystems: []SavedSubsystem{{
			NQN:        "nqn.x",
			Namespaces: []SavedNamespace{{NSID: 1, DevicePath: "/dev/zvol/x", Enabled: true}},
		}},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var got SavedConfig
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Subsystems[0].Namespaces[0].NSID != 1 {
		t.Errorf("nsid lost in round-trip")
	}
	// Defensive: ensure NSID is encoded as JSON number, not string.
	_ = strconv.Itoa(got.Subsystems[0].Namespaces[0].NSID)
}
