package nvmeof

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/novanas/nova-nas/internal/host/configfs"
)

// newManager builds a Manager rooted in a fresh temp dir with the
// nvmet directory tree already in place, mirroring how a real system
// looks immediately after `modprobe nvmet`.
func newManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{
		"nvmet/subsystems",
		"nvmet/ports",
		"nvmet/hosts",
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatalf("setup mkdir: %v", err)
		}
	}
	m := &Manager{CFS: &configfs.Manager{Root: root}}
	return m, root
}

// kernelStubAttrs creates the kernel-managed attribute files that nvmet
// auto-populates when a subsystem/namespace/port directory is created.
// Real configfs creates these on mkdir; in tests against a temp dir we
// have to do it ourselves before the package writes to them.
func kernelStubSubsystemAttrs(t *testing.T, root, nqn string) {
	t.Helper()
	base := filepath.Join(root, "nvmet/subsystems", nqn)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "namespaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "allowed_hosts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"attr_allow_any_host", "attr_serial"} {
		touch(t, filepath.Join(base, f))
	}
}

func kernelStubNamespaceAttrs(t *testing.T, root, nqn string, nsid int) {
	t.Helper()
	base := filepath.Join(root, "nvmet/subsystems", nqn, "namespaces", strconv.Itoa(nsid))
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"device_path", "enable"} {
		touch(t, filepath.Join(base, f))
	}
}

func kernelStubPortAttrs(t *testing.T, root string, id int) {
	t.Helper()
	base := filepath.Join(root, "nvmet/ports", strconv.Itoa(id))
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "subsystems"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"addr_traddr", "addr_trtype", "addr_adrfam", "addr_trsvcid"} {
		touch(t, filepath.Join(base, f))
	}
}

func touch(t *testing.T, p string) {
	t.Helper()
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("touch %q: %v", p, err)
	}
	_ = f.Close()
}

// simulateKernelCleanup removes the stub attribute files (and any empty
// child dirs) in dir so that rmdir(dir) succeeds on a regular tmpfs. On
// real configfs the kernel removes its managed attribute files on
// rmdir; this helper mimics that.
func simulateKernelCleanup(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
			t.Fatalf("cleanup %q: %v", p, err)
		}
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %q: %v", p, err)
	}
	return strings.TrimSpace(string(data))
}

// ----------------------- NQN validation -----------------------

func TestValidateNQN(t *testing.T) {
	good := []string{
		"nqn.2014-08.org.nvmexpress:uuid:11223344",
		"nqn.2024-01.io.novanas:subsys-1",
		"nqn.foo.bar_baz",
	}
	for _, n := range good {
		if err := validateNQN(n); err != nil {
			t.Errorf("expected %q valid, got %v", n, err)
		}
	}
	bad := []string{
		"",
		"foo",                            // missing prefix
		"nqn.",                           // empty after prefix
		"nqn.-bad",                       // leading dash
		"nqn.has space",                  // space
		"nqn.has/slash",                  // slash
		"nqn.has\x00null",                // NUL
		"nqn." + strings.Repeat("x", 220), // too long (224)
	}
	for _, n := range bad {
		if err := validateNQN(n); err == nil {
			t.Errorf("expected %q invalid", n)
		}
	}
}

// ----------------------- Subsystems -----------------------

func TestCreateSubsystem(t *testing.T) {
	m, root := newManager(t)
	nqn := "nqn.2024-01.io.novanas:s1"
	kernelStubSubsystemAttrs(t, root, nqn)

	sub := Subsystem{NQN: nqn, AllowAnyHost: true, Serial: "SN12345"}
	if err := m.CreateSubsystem(context.Background(), sub); err != nil {
		t.Fatalf("CreateSubsystem: %v", err)
	}
	dir := filepath.Join(root, "nvmet/subsystems", nqn)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("dir missing: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "attr_allow_any_host")); got != "1" {
		t.Errorf("attr_allow_any_host = %q, want 1", got)
	}
	if got := readFile(t, filepath.Join(dir, "attr_serial")); got != "SN12345" {
		t.Errorf("attr_serial = %q, want SN12345", got)
	}
}

func TestCreateSubsystem_BadNQN(t *testing.T) {
	m, _ := newManager(t)
	if err := m.CreateSubsystem(context.Background(), Subsystem{NQN: "bogus"}); err == nil {
		t.Fatal("expected error for bad NQN")
	}
}

func TestListSubsystems(t *testing.T) {
	m, root := newManager(t)
	for _, n := range []string{"nqn.a.one", "nqn.a.two"} {
		kernelStubSubsystemAttrs(t, root, n)
		if err := m.CreateSubsystem(context.Background(), Subsystem{NQN: n, Serial: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := m.ListSubsystems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 subsystems, got %d", len(got))
	}
	names := []string{got[0].NQN, got[1].NQN}
	sort.Strings(names)
	if names[0] != "nqn.a.one" || names[1] != "nqn.a.two" {
		t.Errorf("unexpected subsystems: %v", names)
	}
}

func TestGetSubsystem(t *testing.T) {
	m, root := newManager(t)
	nqn := "nqn.x.y"
	kernelStubSubsystemAttrs(t, root, nqn)
	if err := m.CreateSubsystem(context.Background(), Subsystem{NQN: nqn, AllowAnyHost: false, Serial: "abc"}); err != nil {
		t.Fatal(err)
	}
	kernelStubNamespaceAttrs(t, root, nqn, 1)
	if err := m.AddNamespace(context.Background(), nqn, Namespace{NSID: 1, DevicePath: "/dev/zvol/tank/v1", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	hostNQN := "nqn.host.alpha"
	if err := m.AllowHost(context.Background(), nqn, hostNQN); err != nil {
		t.Fatal(err)
	}

	d, err := m.GetSubsystem(context.Background(), nqn)
	if err != nil {
		t.Fatal(err)
	}
	if d.Subsystem.Serial != "abc" {
		t.Errorf("serial = %q", d.Subsystem.Serial)
	}
	if len(d.Namespaces) != 1 || d.Namespaces[0].NSID != 1 || d.Namespaces[0].DevicePath != "/dev/zvol/tank/v1" || !d.Namespaces[0].Enabled {
		t.Errorf("namespaces wrong: %+v", d.Namespaces)
	}
	if len(d.AllowedHosts) != 1 || d.AllowedHosts[0] != hostNQN {
		t.Errorf("allowed hosts wrong: %v", d.AllowedHosts)
	}
}

func TestDeleteSubsystem(t *testing.T) {
	m, root := newManager(t)
	nqn := "nqn.del.me"
	kernelStubSubsystemAttrs(t, root, nqn)
	if err := m.CreateSubsystem(context.Background(), Subsystem{NQN: nqn}); err != nil {
		t.Fatal(err)
	}
	kernelStubNamespaceAttrs(t, root, nqn, 1)
	if err := m.AddNamespace(context.Background(), nqn, Namespace{NSID: 1, DevicePath: "/dev/zvol/tank/v", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	hostNQN := "nqn.host.beta"
	if err := m.AllowHost(context.Background(), nqn, hostNQN); err != nil {
		t.Fatal(err)
	}

	// Real configfs: rmdir of a subsystem auto-removes its kernel-managed
	// attribute files and container subdirs (namespaces/, allowed_hosts/).
	// On tmpfs we must mimic that by clearing stubs before rmdir.
	subsysPath := filepath.Join(root, "nvmet/subsystems", nqn)
	simulateKernelCleanup(t,
		filepath.Join(subsysPath, "namespaces", "1"),
		"device_path", "enable")
	simulateKernelCleanup(t, subsysPath,
		"attr_allow_any_host", "attr_serial")
	if err := m.DeleteSubsystem(context.Background(), nqn); err != nil {
		// DeleteSubsystem will remove namespaces/1 successfully (its
		// stubs were cleaned), and remove the allowed_hosts symlink.
		// The final rmdir of the subsystem dir fails on tmpfs because
		// the empty container subdirs remain. Verify the substeps ran.
		if _, err2 := os.Stat(filepath.Join(subsysPath, "namespaces", "1")); !os.IsNotExist(err2) {
			t.Errorf("namespace 1 should have been removed: %v", err2)
		}
		hostLink := filepath.Join(subsysPath, "allowed_hosts", hostNQN)
		if _, err2 := os.Lstat(hostLink); !os.IsNotExist(err2) {
			t.Errorf("allowed_hosts symlink should have been removed: %v", err2)
		}
		// The remaining error is expected on tmpfs only (empty
		// container dirs); it would not occur on real configfs.
		if !strings.Contains(err.Error(), "directory not empty") {
			t.Errorf("unexpected error: %v", err)
		}
		return
	}
	// If somehow the rmdir succeeded (e.g. some FS quirk), verify gone.
	if _, err := os.Stat(subsysPath); !os.IsNotExist(err) {
		t.Errorf("subsystem dir should be gone")
	}
	if _, err := os.Stat(filepath.Join(root, "nvmet/subsystems", nqn)); !os.IsNotExist(err) {
		t.Errorf("subsystem dir should be gone, stat err = %v", err)
	}
}

// ----------------------- Namespaces -----------------------

func TestAddNamespace(t *testing.T) {
	m, root := newManager(t)
	nqn := "nqn.add.ns"
	kernelStubSubsystemAttrs(t, root, nqn)
	if err := m.CreateSubsystem(context.Background(), Subsystem{NQN: nqn}); err != nil {
		t.Fatal(err)
	}
	kernelStubNamespaceAttrs(t, root, nqn, 7)
	ns := Namespace{NSID: 7, DevicePath: "/dev/zvol/tank/foo", Enabled: true}
	if err := m.AddNamespace(context.Background(), nqn, ns); err != nil {
		t.Fatalf("AddNamespace: %v", err)
	}
	base := filepath.Join(root, "nvmet/subsystems", nqn, "namespaces", "7")
	if got := readFile(t, filepath.Join(base, "device_path")); got != "/dev/zvol/tank/foo" {
		t.Errorf("device_path = %q", got)
	}
	if got := readFile(t, filepath.Join(base, "enable")); got != "1" {
		t.Errorf("enable = %q", got)
	}
}

func TestAddNamespace_BadInput(t *testing.T) {
	m, _ := newManager(t)
	nqn := "nqn.x.y"
	if err := m.AddNamespace(context.Background(), nqn, Namespace{NSID: 0, DevicePath: "/dev/zvol/x"}); err == nil {
		t.Error("expected error for nsid=0")
	}
	if err := m.AddNamespace(context.Background(), nqn, Namespace{NSID: 1, DevicePath: "relative/path"}); err == nil {
		t.Error("expected error for non-/dev path")
	}
}

func TestRemoveNamespace(t *testing.T) {
	m, root := newManager(t)
	nqn := "nqn.rm.ns"
	kernelStubSubsystemAttrs(t, root, nqn)
	if err := m.CreateSubsystem(context.Background(), Subsystem{NQN: nqn}); err != nil {
		t.Fatal(err)
	}
	kernelStubNamespaceAttrs(t, root, nqn, 3)
	if err := m.AddNamespace(context.Background(), nqn, Namespace{NSID: 3, DevicePath: "/dev/zvol/tank/x", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// Real configfs auto-removes namespace attribute files on rmdir.
	// On tmpfs we mimic that by clearing them after the package wrote
	// "0" to enable.
	nsPath := filepath.Join(root, "nvmet/subsystems", nqn, "namespaces", "3")
	// Verify enable was set to 0, then simulate kernel cleanup, then
	// re-run the rmdir step. To do this, we monkey with the FS between
	// the writes and the rmdir, which the public API doesn't expose;
	// instead we run RemoveNamespace and accept the rmdir error, then
	// verify enable=0 and finish the cleanup ourselves.
	err := m.RemoveNamespace(context.Background(), nqn, 3)
	if err == nil {
		// If somehow it succeeded, great.
		if _, err := os.Stat(nsPath); !os.IsNotExist(err) {
			t.Errorf("namespace dir should be gone")
		}
		return
	}
	// Expected on tmpfs: enable was written to 0, rmdir failed because
	// stub attr files remain.
	if got := readFile(t, filepath.Join(nsPath, "enable")); got != "0" {
		t.Errorf("enable = %q, want 0", got)
	}
	if !strings.Contains(err.Error(), "directory not empty") {
		t.Errorf("unexpected error: %v", err)
	}
	simulateKernelCleanup(t, nsPath, "device_path", "enable")
	if err := m.cfs().Rmdir("nvmet/subsystems/" + nqn + "/namespaces/3"); err != nil {
		t.Fatalf("final rmdir: %v", err)
	}
}

// ----------------------- Hosts & Allow -----------------------

func TestAllowAndDisallowHost(t *testing.T) {
	m, root := newManager(t)
	nqn := "nqn.s.one"
	kernelStubSubsystemAttrs(t, root, nqn)
	if err := m.CreateSubsystem(context.Background(), Subsystem{NQN: nqn}); err != nil {
		t.Fatal(err)
	}
	hostNQN := "nqn.host.gamma"
	if err := m.AllowHost(context.Background(), nqn, hostNQN); err != nil {
		t.Fatalf("AllowHost: %v", err)
	}
	link := filepath.Join(root, "nvmet/subsystems", nqn, "allowed_hosts", hostNQN)
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink at %q (err=%v)", link, err)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	wantTarget := filepath.Join(root, "nvmet/hosts", hostNQN)
	if target != wantTarget {
		t.Errorf("symlink target = %q, want %q", target, wantTarget)
	}
	if _, err := os.Stat(filepath.Join(root, "nvmet/hosts", hostNQN)); err != nil {
		t.Errorf("host dir not created: %v", err)
	}

	if err := m.DisallowHost(context.Background(), nqn, hostNQN); err != nil {
		t.Fatalf("DisallowHost: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("symlink should be gone, err = %v", err)
	}
}

func TestAllowHost_BadNQN(t *testing.T) {
	m, _ := newManager(t)
	if err := m.AllowHost(context.Background(), "nqn.ok", "bad-host"); err == nil {
		t.Error("expected error for bad host NQN")
	}
}

func TestEnsureAndRemoveHost(t *testing.T) {
	m, root := newManager(t)
	hostNQN := "nqn.host.delta"
	if err := m.EnsureHost(context.Background(), hostNQN); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "nvmet/hosts", hostNQN)); err != nil {
		t.Fatal(err)
	}
	// Idempotent
	if err := m.EnsureHost(context.Background(), hostNQN); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveHost(context.Background(), hostNQN); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "nvmet/hosts", hostNQN)); !os.IsNotExist(err) {
		t.Errorf("host dir should be gone, err = %v", err)
	}
}

// ----------------------- Ports -----------------------

func TestCreatePort_TCPv4(t *testing.T) {
	m, root := newManager(t)
	kernelStubPortAttrs(t, root, 1)
	p := Port{ID: 1, IP: "10.0.0.5", Port: 4420, Transport: "tcp"}
	if err := m.CreatePort(context.Background(), p); err != nil {
		t.Fatalf("CreatePort: %v", err)
	}
	base := filepath.Join(root, "nvmet/ports/1")
	if got := readFile(t, filepath.Join(base, "addr_traddr")); got != "10.0.0.5" {
		t.Errorf("traddr = %q", got)
	}
	if got := readFile(t, filepath.Join(base, "addr_trtype")); got != "tcp" {
		t.Errorf("trtype = %q", got)
	}
	if got := readFile(t, filepath.Join(base, "addr_adrfam")); got != "ipv4" {
		t.Errorf("adrfam = %q", got)
	}
	if got := readFile(t, filepath.Join(base, "addr_trsvcid")); got != "4420" {
		t.Errorf("trsvcid = %q", got)
	}
}

func TestCreatePort_RDMAv6(t *testing.T) {
	m, root := newManager(t)
	kernelStubPortAttrs(t, root, 2)
	p := Port{ID: 2, IP: "fe80::1", Port: 4420, Transport: "rdma"}
	if err := m.CreatePort(context.Background(), p); err != nil {
		t.Fatalf("CreatePort: %v", err)
	}
	base := filepath.Join(root, "nvmet/ports/2")
	if got := readFile(t, filepath.Join(base, "addr_adrfam")); got != "ipv6" {
		t.Errorf("adrfam = %q", got)
	}
	if got := readFile(t, filepath.Join(base, "addr_trtype")); got != "rdma" {
		t.Errorf("trtype = %q", got)
	}
}

func TestCreatePort_BadInput(t *testing.T) {
	m, _ := newManager(t)
	cases := []Port{
		{ID: 1, IP: "not-an-ip", Port: 4420, Transport: "tcp"},
		{ID: 1, IP: "10.0.0.1", Port: 70000, Transport: "tcp"},
		{ID: 1, IP: "10.0.0.1", Port: 4420, Transport: "udp"},
	}
	for i, p := range cases {
		if err := m.CreatePort(context.Background(), p); err == nil {
			t.Errorf("case %d: expected error, got nil", i)
		}
	}
}

func TestListPorts(t *testing.T) {
	m, root := newManager(t)
	kernelStubPortAttrs(t, root, 1)
	kernelStubPortAttrs(t, root, 2)
	if err := m.CreatePort(context.Background(), Port{ID: 1, IP: "10.0.0.1", Port: 4420, Transport: "tcp"}); err != nil {
		t.Fatal(err)
	}
	if err := m.CreatePort(context.Background(), Port{ID: 2, IP: "10.0.0.2", Port: 4421, Transport: "tcp"}); err != nil {
		t.Fatal(err)
	}
	got, err := m.ListPorts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(got))
	}
	sort.Slice(got, func(i, j int) bool { return got[i].ID < got[j].ID })
	if got[0].ID != 1 || got[0].IP != "10.0.0.1" || got[0].Port != 4420 || got[0].Transport != "tcp" {
		t.Errorf("port 0 wrong: %+v", got[0])
	}
}

func TestLinkUnlinkSubsystemToPort(t *testing.T) {
	m, root := newManager(t)
	nqn := "nqn.link.s"
	kernelStubSubsystemAttrs(t, root, nqn)
	if err := m.CreateSubsystem(context.Background(), Subsystem{NQN: nqn}); err != nil {
		t.Fatal(err)
	}
	kernelStubPortAttrs(t, root, 1)
	if err := m.CreatePort(context.Background(), Port{ID: 1, IP: "10.0.0.1", Port: 4420, Transport: "tcp"}); err != nil {
		t.Fatal(err)
	}
	if err := m.LinkSubsystemToPort(context.Background(), nqn, 1); err != nil {
		t.Fatalf("Link: %v", err)
	}
	link := filepath.Join(root, "nvmet/ports/1/subsystems", nqn)
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	want := filepath.Join(root, "nvmet/subsystems", nqn)
	if target != want {
		t.Errorf("target = %q, want %q", target, want)
	}
	if err := m.UnlinkSubsystemFromPort(context.Background(), nqn, 1); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("link should be gone")
	}
}

func TestDeletePort(t *testing.T) {
	m, root := newManager(t)
	kernelStubPortAttrs(t, root, 5)
	if err := m.CreatePort(context.Background(), Port{ID: 5, IP: "10.0.0.9", Port: 4420, Transport: "tcp"}); err != nil {
		t.Fatal(err)
	}
	nqn := "nqn.delport.s"
	kernelStubSubsystemAttrs(t, root, nqn)
	if err := m.CreateSubsystem(context.Background(), Subsystem{NQN: nqn}); err != nil {
		t.Fatal(err)
	}
	if err := m.LinkSubsystemToPort(context.Background(), nqn, 5); err != nil {
		t.Fatal(err)
	}
	portPath := filepath.Join(root, "nvmet/ports/5")
	err := m.DeletePort(context.Background(), 5)
	if err == nil {
		if _, e := os.Stat(portPath); !os.IsNotExist(e) {
			t.Errorf("port dir should be gone")
		}
		return
	}
	// Expected on tmpfs: subsystem link was removed, but the port's
	// attr files and subsystems/ container still block rmdir.
	if _, e := os.Lstat(filepath.Join(portPath, "subsystems", nqn)); !os.IsNotExist(e) {
		t.Errorf("port→subsystem link should be gone: %v", e)
	}
	if !strings.Contains(err.Error(), "directory not empty") {
		t.Errorf("unexpected error: %v", err)
	}
	simulateKernelCleanup(t, portPath,
		"addr_traddr", "addr_trtype", "addr_adrfam", "addr_trsvcid", "subsystems")
	if err := m.cfs().Rmdir("nvmet/ports/5"); err != nil {
		t.Fatalf("final rmdir: %v", err)
	}
}
