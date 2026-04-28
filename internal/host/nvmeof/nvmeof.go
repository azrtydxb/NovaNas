// Package nvmeof manages Linux NVMe-oF (NVMe over Fabrics) target
// configuration via the kernel's nvmet configfs interface, mounted at
// /sys/kernel/config/nvmet/.
//
// The package wraps the underlying configfs primitive (internal/host/configfs)
// to provide a higher-level Manager API that understands the lifecycle
// rules of nvmet objects: subsystems, namespaces, ports, and hosts.
//
// Lifecycle ordering matters in nvmet:
//
//   - Ports must have addr_traddr/addr_trtype/addr_adrfam/addr_trsvcid
//     written before they are useful, and any subsystem symlinks must
//     be removed before rmdir of the port directory.
//   - Namespaces must have device_path written, then enable=1; deletion
//     requires enable=0 first, then rmdir.
//   - Subsystems must have all namespaces and allowed_hosts symlinks
//     removed before rmdir.
//
// All methods are validated for safe input (NQN format, IP/port,
// transport type, NSID) and are intended to be called by higher-level
// orchestration code, not directly from API handlers.
package nvmeof

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/novanas/nova-nas/internal/host/configfs"
)

// Manager controls nvmet configuration via configfs.
//
// CFS may be nil; in that case a default Manager{Root: "/sys/kernel/config"}
// is used. Tests construct a Manager with a temporary root.
type Manager struct {
	CFS *configfs.Manager
}

func (m *Manager) cfs() *configfs.Manager {
	if m.CFS == nil {
		return &configfs.Manager{Root: configfs.DefaultRoot}
	}
	return m.CFS
}

// configfs root used to build absolute symlink targets. When CFS.Root is
// set to a temp dir (in tests), absolute targets still resolve under that
// temp tree; the kernel cares about absolute paths, but tests just check
// the link target string.
func (m *Manager) absRoot() string {
	r := configfs.DefaultRoot
	if m.CFS != nil && m.CFS.Root != "" {
		r = m.CFS.Root
	}
	return r
}

// Subsystem describes an NVMe-oF subsystem (target namespace container).
type Subsystem struct {
	NQN          string `json:"nqn"`
	AllowAnyHost bool   `json:"allowAnyHost"`
	Serial       string `json:"serial,omitempty"`
}

// Namespace describes a single NVMe namespace exposed by a subsystem.
type Namespace struct {
	NSID       int    `json:"nsid"`
	DevicePath string `json:"devicePath"` // /dev/zvol/...
	Enabled    bool   `json:"enabled"`
}

// Port describes a transport port that subsystems can be linked to.
type Port struct {
	ID        int    `json:"id"`
	IP        string `json:"ip"`
	Port      int    `json:"port"`
	Transport string `json:"transport"` // "tcp" | "rdma"
}

// Host represents an allowed host NQN identity.
type Host struct {
	NQN string `json:"nqn"`
}

// SubsystemDetail bundles a Subsystem with its namespaces and allowed
// hosts (as observed from configfs).
type SubsystemDetail struct {
	Subsystem    Subsystem   `json:"subsystem"`
	Namespaces   []Namespace `json:"namespaces"`
	AllowedHosts []string    `json:"allowedHosts"`
}

// nqnRegexp validates NQN strings strictly: must start with "nqn." and
// contain only alphanumerics, dot, dash, colon, underscore. The leading
// character of segments is constrained at parse time to forbid "-".
//
// The NVMe spec is somewhat lenient about NQN content (UTF-8, up to 223
// bytes); we are intentionally stricter for security: this string is
// embedded in configfs path components and we never want shell/path
// metacharacters or whitespace sneaking through.
var nqnRegexp = regexp.MustCompile(`^nqn\.[A-Za-z0-9._:][A-Za-z0-9._:\-]*$`)

func validateNQN(nqn string) error {
	if nqn == "" {
		return fmt.Errorf("nqn: empty")
	}
	if !strings.HasPrefix(nqn, "nqn.") {
		return fmt.Errorf("nqn: must start with %q (got %q)", "nqn.", nqn)
	}
	if strings.HasPrefix(nqn, "nqn.-") {
		return fmt.Errorf("nqn: leading dash after prefix not allowed")
	}
	if !nqnRegexp.MatchString(nqn) {
		return fmt.Errorf("nqn: invalid characters in %q", nqn)
	}
	if len(nqn) > 223 {
		return fmt.Errorf("nqn: too long (%d > 223)", len(nqn))
	}
	return nil
}

func validateTransport(t string) error {
	switch t {
	case "tcp", "rdma":
		return nil
	default:
		return fmt.Errorf("transport: must be tcp or rdma, got %q", t)
	}
}

func validatePort(p int) error {
	if p <= 0 || p > 65535 {
		return fmt.Errorf("port: out of range: %d", p)
	}
	return nil
}

func adrfamFor(ip net.IP) (string, error) {
	if ip == nil {
		return "", fmt.Errorf("ip: invalid")
	}
	if ip.To4() != nil {
		return "ipv4", nil
	}
	return "ipv6", nil
}

// ----------------------- Subsystems -----------------------

func subsysDir(nqn string) string                  { return path.Join("nvmet/subsystems", nqn) }
func subsysAttr(nqn, attr string) string           { return path.Join(subsysDir(nqn), attr) }
func subsysNS(nqn string, nsid int) string         { return path.Join(subsysDir(nqn), "namespaces", strconv.Itoa(nsid)) }
func subsysNSAttr(nqn string, nsid int, a string) string {
	return path.Join(subsysNS(nqn, nsid), a)
}
func subsysAllowedHostLink(nqn, hostNQN string) string {
	return path.Join(subsysDir(nqn), "allowed_hosts", hostNQN)
}
func hostDir(hostNQN string) string  { return path.Join("nvmet/hosts", hostNQN) }
func portDir(id int) string          { return path.Join("nvmet/ports", strconv.Itoa(id)) }
func portAttr(id int, a string) string {
	return path.Join(portDir(id), a)
}
func portSubsysLink(id int, nqn string) string {
	return path.Join(portDir(id), "subsystems", nqn)
}

// CreateSubsystem creates a new subsystem directory and applies its
// attributes (allow_any_host, serial). Idempotent on Mkdir, but writes
// always overwrite existing attribute values.
func (m *Manager) CreateSubsystem(_ context.Context, sub Subsystem) error {
	if err := validateNQN(sub.NQN); err != nil {
		return err
	}
	c := m.cfs()
	if err := c.Mkdir(subsysDir(sub.NQN)); err != nil {
		return err
	}
	allow := []byte("0")
	if sub.AllowAnyHost {
		allow = []byte("1")
	}
	if err := c.WriteFile(subsysAttr(sub.NQN, "attr_allow_any_host"), allow); err != nil {
		return err
	}
	if sub.Serial != "" {
		if err := c.WriteFile(subsysAttr(sub.NQN, "attr_serial"), []byte(sub.Serial)); err != nil {
			return err
		}
	}
	return nil
}

// DeleteSubsystem tears down a subsystem: disables and removes all its
// namespaces, unlinks all allowed_hosts symlinks, and rmdirs the subsystem.
func (m *Manager) DeleteSubsystem(ctx context.Context, nqn string) error {
	if err := validateNQN(nqn); err != nil {
		return err
	}
	c := m.cfs()
	nsDir := path.Join(subsysDir(nqn), "namespaces")
	nsids, err := c.ListDir(nsDir)
	if err != nil && !errors.Is(err, configfs.ErrNotExist) {
		return err
	}
	for _, n := range nsids {
		nsid, convErr := strconv.Atoi(n)
		if convErr != nil {
			continue
		}
		if err := m.RemoveNamespace(ctx, nqn, nsid); err != nil {
			return err
		}
	}
	ahDir := path.Join(subsysDir(nqn), "allowed_hosts")
	hosts, err := c.ListDir(ahDir)
	if err != nil && !errors.Is(err, configfs.ErrNotExist) {
		return err
	}
	for _, h := range hosts {
		if err := c.RemoveSymlink(path.Join(ahDir, h)); err != nil {
			return err
		}
	}
	return c.Rmdir(subsysDir(nqn))
}

// ListSubsystems returns all subsystems under nvmet/subsystems with
// their attr_allow_any_host and attr_serial values.
func (m *Manager) ListSubsystems(_ context.Context) ([]Subsystem, error) {
	c := m.cfs()
	names, err := c.ListDir("nvmet/subsystems")
	if err != nil {
		if errors.Is(err, configfs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Subsystem, 0, len(names))
	for _, n := range names {
		s := Subsystem{NQN: n}
		if data, err := c.ReadFile(subsysAttr(n, "attr_allow_any_host")); err == nil {
			s.AllowAnyHost = strings.TrimSpace(string(data)) == "1"
		}
		if data, err := c.ReadFile(subsysAttr(n, "attr_serial")); err == nil {
			s.Serial = strings.TrimSpace(string(data))
		}
		out = append(out, s)
	}
	return out, nil
}

// GetSubsystem returns a subsystem and its namespaces and allowed-hosts
// list (by NQN) as observed from configfs.
func (m *Manager) GetSubsystem(_ context.Context, nqn string) (*SubsystemDetail, error) {
	if err := validateNQN(nqn); err != nil {
		return nil, err
	}
	c := m.cfs()
	if _, err := c.ListDir(subsysDir(nqn)); err != nil {
		return nil, err
	}
	d := &SubsystemDetail{Subsystem: Subsystem{NQN: nqn}}
	if data, err := c.ReadFile(subsysAttr(nqn, "attr_allow_any_host")); err == nil {
		d.Subsystem.AllowAnyHost = strings.TrimSpace(string(data)) == "1"
	}
	if data, err := c.ReadFile(subsysAttr(nqn, "attr_serial")); err == nil {
		d.Subsystem.Serial = strings.TrimSpace(string(data))
	}

	if names, err := c.ListDir(path.Join(subsysDir(nqn), "namespaces")); err == nil {
		for _, n := range names {
			nsid, convErr := strconv.Atoi(n)
			if convErr != nil {
				continue
			}
			ns := Namespace{NSID: nsid}
			if data, err := c.ReadFile(subsysNSAttr(nqn, nsid, "device_path")); err == nil {
				ns.DevicePath = strings.TrimSpace(string(data))
			}
			if data, err := c.ReadFile(subsysNSAttr(nqn, nsid, "enable")); err == nil {
				ns.Enabled = strings.TrimSpace(string(data)) == "1"
			}
			d.Namespaces = append(d.Namespaces, ns)
		}
	} else if !errors.Is(err, configfs.ErrNotExist) {
		return nil, err
	}

	if hosts, err := c.ListDir(path.Join(subsysDir(nqn), "allowed_hosts")); err == nil {
		d.AllowedHosts = hosts
	} else if !errors.Is(err, configfs.ErrNotExist) {
		return nil, err
	}
	return d, nil
}

// ----------------------- Namespaces -----------------------

// AddNamespace creates a namespace under a subsystem, sets device_path,
// and optionally enables it. NSID must be > 0; device_path must start
// with /dev/.
func (m *Manager) AddNamespace(_ context.Context, nqn string, ns Namespace) error {
	if err := validateNQN(nqn); err != nil {
		return err
	}
	if ns.NSID <= 0 {
		return fmt.Errorf("nsid: must be > 0")
	}
	if !strings.HasPrefix(ns.DevicePath, "/dev/") {
		return fmt.Errorf("device_path: must start with /dev/")
	}
	c := m.cfs()
	if err := c.Mkdir(subsysNS(nqn, ns.NSID)); err != nil {
		return err
	}
	if err := c.WriteFile(subsysNSAttr(nqn, ns.NSID, "device_path"), []byte(ns.DevicePath)); err != nil {
		return err
	}
	if ns.Enabled {
		if err := c.WriteFile(subsysNSAttr(nqn, ns.NSID, "enable"), []byte("1")); err != nil {
			return err
		}
	}
	return nil
}

// RemoveNamespace disables and rmdirs the given namespace.
func (m *Manager) RemoveNamespace(_ context.Context, nqn string, nsid int) error {
	if err := validateNQN(nqn); err != nil {
		return err
	}
	if nsid <= 0 {
		return fmt.Errorf("nsid: must be > 0")
	}
	c := m.cfs()
	if err := c.WriteFile(subsysNSAttr(nqn, nsid, "enable"), []byte("0")); err != nil {
		// Best-effort: if enable file is missing, fall through to rmdir
		// which will surface the real error.
		if !errors.Is(err, configfs.ErrNotExist) {
			return err
		}
	}
	return c.Rmdir(subsysNS(nqn, nsid))
}

// ----------------------- Hosts -----------------------

// EnsureHost creates a host directory if missing. Idempotent.
func (m *Manager) EnsureHost(_ context.Context, hostNQN string) error {
	if err := validateNQN(hostNQN); err != nil {
		return err
	}
	return m.cfs().Mkdir(hostDir(hostNQN))
}

// RemoveHost rmdirs a host directory. Fails if any subsystem still
// references this host (kernel enforces this on real configfs).
func (m *Manager) RemoveHost(_ context.Context, hostNQN string) error {
	if err := validateNQN(hostNQN); err != nil {
		return err
	}
	return m.cfs().Rmdir(hostDir(hostNQN))
}

// AllowHost ensures the host directory exists then symlinks it into
// the subsystem's allowed_hosts.
func (m *Manager) AllowHost(ctx context.Context, nqn, hostNQN string) error {
	if err := validateNQN(nqn); err != nil {
		return err
	}
	if err := validateNQN(hostNQN); err != nil {
		return err
	}
	if err := m.EnsureHost(ctx, hostNQN); err != nil {
		return err
	}
	target := path.Join(m.absRoot(), hostDir(hostNQN))
	return m.cfs().Symlink(target, subsysAllowedHostLink(nqn, hostNQN))
}

// DisallowHost removes the symlink that authorizes hostNQN on a subsystem.
func (m *Manager) DisallowHost(_ context.Context, nqn, hostNQN string) error {
	if err := validateNQN(nqn); err != nil {
		return err
	}
	if err := validateNQN(hostNQN); err != nil {
		return err
	}
	return m.cfs().RemoveSymlink(subsysAllowedHostLink(nqn, hostNQN))
}

// ----------------------- Ports -----------------------

// CreatePort creates a port directory and writes traddr/trtype/adrfam/trsvcid
// in the order required by nvmet.
func (m *Manager) CreatePort(_ context.Context, p Port) error {
	if p.ID < 0 {
		return fmt.Errorf("port id: must be >= 0")
	}
	if err := validateTransport(p.Transport); err != nil {
		return err
	}
	if err := validatePort(p.Port); err != nil {
		return err
	}
	ip := net.ParseIP(p.IP)
	if ip == nil {
		return fmt.Errorf("ip: invalid: %q", p.IP)
	}
	adrfam, err := adrfamFor(ip)
	if err != nil {
		return err
	}
	c := m.cfs()
	if err := c.Mkdir(portDir(p.ID)); err != nil {
		return err
	}
	if err := c.WriteFile(portAttr(p.ID, "addr_traddr"), []byte(p.IP)); err != nil {
		return err
	}
	if err := c.WriteFile(portAttr(p.ID, "addr_trtype"), []byte(p.Transport)); err != nil {
		return err
	}
	if err := c.WriteFile(portAttr(p.ID, "addr_adrfam"), []byte(adrfam)); err != nil {
		return err
	}
	if err := c.WriteFile(portAttr(p.ID, "addr_trsvcid"), []byte(strconv.Itoa(p.Port))); err != nil {
		return err
	}
	return nil
}

// DeletePort unlinks any subsystem symlinks under the port and rmdirs it.
func (m *Manager) DeletePort(_ context.Context, id int) error {
	if id < 0 {
		return fmt.Errorf("port id: must be >= 0")
	}
	c := m.cfs()
	subDir := path.Join(portDir(id), "subsystems")
	links, err := c.ListDir(subDir)
	if err != nil && !errors.Is(err, configfs.ErrNotExist) {
		return err
	}
	for _, l := range links {
		if err := c.RemoveSymlink(path.Join(subDir, l)); err != nil {
			return err
		}
	}
	return c.Rmdir(portDir(id))
}

// ListPorts returns all ports with their addr_* attributes.
func (m *Manager) ListPorts(_ context.Context) ([]Port, error) {
	c := m.cfs()
	names, err := c.ListDir("nvmet/ports")
	if err != nil {
		if errors.Is(err, configfs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Port, 0, len(names))
	for _, n := range names {
		id, convErr := strconv.Atoi(n)
		if convErr != nil {
			continue
		}
		p := Port{ID: id}
		if data, err := c.ReadFile(portAttr(id, "addr_traddr")); err == nil {
			p.IP = strings.TrimSpace(string(data))
		}
		if data, err := c.ReadFile(portAttr(id, "addr_trtype")); err == nil {
			p.Transport = strings.TrimSpace(string(data))
		}
		if data, err := c.ReadFile(portAttr(id, "addr_trsvcid")); err == nil {
			if v, e := strconv.Atoi(strings.TrimSpace(string(data))); e == nil {
				p.Port = v
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// LinkSubsystemToPort creates a symlink from the port to the subsystem,
// activating the subsystem on that transport.
func (m *Manager) LinkSubsystemToPort(_ context.Context, nqn string, portID int) error {
	if err := validateNQN(nqn); err != nil {
		return err
	}
	if portID < 0 {
		return fmt.Errorf("port id: must be >= 0")
	}
	target := path.Join(m.absRoot(), subsysDir(nqn))
	return m.cfs().Symlink(target, portSubsysLink(portID, nqn))
}

// UnlinkSubsystemFromPort removes the port→subsystem symlink.
func (m *Manager) UnlinkSubsystemFromPort(_ context.Context, nqn string, portID int) error {
	if err := validateNQN(nqn); err != nil {
		return err
	}
	if portID < 0 {
		return fmt.Errorf("port id: must be >= 0")
	}
	return m.cfs().RemoveSymlink(portSubsysLink(portID, nqn))
}
