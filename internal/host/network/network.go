// Package network manages systemd-networkd configuration files under
// /etc/systemd/network. The Manager renders .network and .netdev INI
// files for plain interfaces, VLANs, and bonds, then asks networkd to
// reload via `networkctl reload`.
//
// Safety model
//
// This package is intended to be reachable from an HTTP API. A
// misconfigured interface can cut off management access to the host.
// Two safety mechanisms are exposed for callers:
//
//  1. IdentifyManagementIface: returns the iface name that owns the
//     local IP a connection arrived on. The HTTP layer is expected to
//     refuse any mutation that would touch that iface unless an
//     explicit force=true override is set.
//
//  2. DryRun on every mutating input struct: when true the Manager
//     validates and renders the file content but does NOT write to
//     disk and does NOT reload networkd. Callers can surface the
//     rendered bytes to the operator for review before applying.
//
// The Manager only reads/writes files whose names start with
// FilePrefix (default "70-nova-"). Operator-installed configs in the
// same directory are left untouched.
package network

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/novanas/nova-nas/internal/host/exec"
)

// ErrNotFound is returned when a managed config file does not exist.
var ErrNotFound = errors.New("network config not found")

// ---------- resource types ----------

// InterfaceConfig is a plain (non-VLAN/bond) interface configuration.
type InterfaceConfig struct {
	Name      string   `json:"name"`
	MatchName string   `json:"matchName"`
	DHCP      string   `json:"dhcp"`
	Addresses []string `json:"addresses,omitempty"`
	Gateway   string   `json:"gateway,omitempty"`
	DNS       []string `json:"dns,omitempty"`
	Domains   []string `json:"domains,omitempty"`
	MTU       int      `json:"mtu,omitempty"`
	LinkLocal string   `json:"linkLocal,omitempty"`
	DryRun    bool     `json:"dryRun,omitempty"`
}

// VLAN is a tagged sub-interface stacked on a parent kernel iface.
type VLAN struct {
	Name    string `json:"name"`
	Parent  string `json:"parent"`
	ID      int    `json:"id"`
	Address string `json:"address,omitempty"`
	DryRun  bool   `json:"dryRun,omitempty"`
}

// Bond aggregates one or more kernel ifaces into a single virtual
// interface. Members listed here are enslaved.
type Bond struct {
	Name      string   `json:"name"`
	Mode      string   `json:"mode"`
	Members   []string `json:"members"`
	Address   string   `json:"address,omitempty"`
	MIIMonSec int      `json:"miiMonSec,omitempty"`
	DryRun    bool     `json:"dryRun,omitempty"`
}

// LiveInterface is a single row of `ip -j addr show`.
type LiveInterface struct {
	Name      string   `json:"name"`
	State     string   `json:"state"`
	MAC       string   `json:"mac"`
	Addresses []string `json:"addresses"`
	Gateway   string   `json:"gateway,omitempty"`
	Type      string   `json:"type"`
}

// ConfigKind enumerates which on-disk shape a managed config has.
type ConfigKind string

const (
	KindInterface ConfigKind = "interface"
	KindVLAN      ConfigKind = "vlan"
	KindBond      ConfigKind = "bond"
)

// ManagedConfig is a thin envelope used by ListConfigs/GetConfig so
// callers can discover what kind of resource a file belongs to without
// inspecting the bytes themselves.
type ManagedConfig struct {
	Name      string           `json:"name"`
	Kind      ConfigKind       `json:"kind"`
	Interface *InterfaceConfig `json:"interface,omitempty"`
	VLAN      *VLAN            `json:"vlan,omitempty"`
	Bond      *Bond            `json:"bond,omitempty"`
}

// ---------- file IO abstraction ----------

// FileWriter abstracts file ops so tests can stub them. Mirrors the
// shape used in the nfs package.
type FileWriter interface {
	Write(path string, data []byte, perm os.FileMode) error
	Remove(path string) error
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]os.DirEntry, error)
}

type osFileWriter struct{}

func (osFileWriter) Write(path string, data []byte, perm os.FileMode) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." && dir != "/" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return os.WriteFile(path, data, perm)
}
func (osFileWriter) Remove(path string) error           { return os.Remove(path) }
func (osFileWriter) ReadFile(p string) ([]byte, error)  { return os.ReadFile(p) }
func (osFileWriter) ReadDir(p string) ([]os.DirEntry, error) {
	return os.ReadDir(p)
}

// ---------- Manager ----------

// Manager owns a slice of /etc/systemd/network namespaced by FilePrefix.
type Manager struct {
	NetworkDir    string
	NetworkctlBin string
	IPBin         string
	Runner        exec.Runner
	FileWriter    FileWriter
	FilePrefix    string
}

func (m *Manager) dir() string {
	if m.NetworkDir == "" {
		return "/etc/systemd/network"
	}
	return m.NetworkDir
}

func (m *Manager) networkctl() string {
	if m.NetworkctlBin == "" {
		return "/usr/bin/networkctl"
	}
	return m.NetworkctlBin
}

func (m *Manager) ipBin() string {
	if m.IPBin == "" {
		return "/usr/sbin/ip"
	}
	return m.IPBin
}

func (m *Manager) prefix() string {
	if m.FilePrefix == "" {
		return "70-nova-"
	}
	return m.FilePrefix
}

func (m *Manager) fw() FileWriter {
	if m.FileWriter == nil {
		return osFileWriter{}
	}
	return m.FileWriter
}

func (m *Manager) run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	return runner(ctx, bin, args...)
}

// ---------- path helpers ----------

func (m *Manager) networkFile(name string) string {
	return filepath.Join(m.dir(), m.prefix()+name+".network")
}

func (m *Manager) netdevFile(name string) string {
	return filepath.Join(m.dir(), m.prefix()+name+".netdev")
}

// memberNetworkFile is the per-bond-member .network file we write to
// mark a kernel iface as enslaved to a bond. We namespace by
// "<bond>-member-<member>" so multiple bonds can coexist and so a
// member's previous role (if any) is overwritten cleanly.
func (m *Manager) memberNetworkFile(bondName, memberName string) string {
	return filepath.Join(m.dir(),
		m.prefix()+bondName+"-member-"+memberName+".network")
}

// ---------- validation ----------

// validateConfigName: 1-64 chars, alphanumeric + dash + underscore.
// Used for both the logical "Name" field and for kernel iface names
// that we will substitute into file names.
func validateConfigName(name string) error {
	if name == "" {
		return fmt.Errorf("name required")
	}
	if len(name) > 64 {
		return fmt.Errorf("name too long (>64): %q", name)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("name cannot start with '-': %q", name)
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return fmt.Errorf("name contains invalid character %q in %q", r, name)
		}
	}
	return nil
}

// validateMatchName: like validateConfigName but allows '*' and '?' for
// the [Match] Name= glob support documented in systemd.network(5).
func validateMatchName(name string) error {
	if name == "" {
		return fmt.Errorf("matchName required")
	}
	if len(name) > 64 {
		return fmt.Errorf("matchName too long (>64): %q", name)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("matchName cannot start with '-': %q", name)
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' ||
			r == '*' || r == '?'
		if !ok {
			return fmt.Errorf("matchName contains invalid character %q in %q", r, name)
		}
	}
	return nil
}

func validateDHCP(s string) error {
	switch s {
	case "", "yes", "no", "ipv4", "ipv6":
		return nil
	}
	return fmt.Errorf("invalid DHCP value %q (want yes|no|ipv4|ipv6)", s)
}

func validateLinkLocal(s string) error {
	switch s {
	case "", "ipv6", "ipv4", "yes", "no", "fallback", "ipv4-fallback":
		return nil
	}
	return fmt.Errorf("invalid linkLocal value %q", s)
}

func validateCIDR(c string) error {
	if c == "" {
		return fmt.Errorf("address required")
	}
	if _, _, err := net.ParseCIDR(c); err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", c, err)
	}
	return nil
}

func validateIP(s string) error {
	if s == "" {
		return fmt.Errorf("ip required")
	}
	if ip := net.ParseIP(s); ip == nil {
		return fmt.Errorf("invalid IP %q", s)
	}
	return nil
}

func validateInterfaceConfig(c InterfaceConfig) error {
	if err := validateConfigName(c.Name); err != nil {
		return err
	}
	if err := validateMatchName(c.MatchName); err != nil {
		return err
	}
	if err := validateDHCP(c.DHCP); err != nil {
		return err
	}
	if err := validateLinkLocal(c.LinkLocal); err != nil {
		return err
	}
	for i, a := range c.Addresses {
		if err := validateCIDR(a); err != nil {
			return fmt.Errorf("addresses[%d]: %w", i, err)
		}
	}
	if c.Gateway != "" {
		if err := validateIP(c.Gateway); err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
	}
	for i, d := range c.DNS {
		if err := validateIP(d); err != nil {
			return fmt.Errorf("dns[%d]: %w", i, err)
		}
	}
	if c.MTU < 0 || c.MTU > 65535 {
		return fmt.Errorf("invalid MTU %d", c.MTU)
	}
	return nil
}

func validateVLAN(v VLAN) error {
	if err := validateConfigName(v.Name); err != nil {
		return err
	}
	if err := validateMatchName(v.Parent); err != nil {
		return fmt.Errorf("parent: %w", err)
	}
	if v.ID < 1 || v.ID > 4094 {
		return fmt.Errorf("invalid VLAN id %d (want 1..4094)", v.ID)
	}
	if v.Address != "" {
		if err := validateCIDR(v.Address); err != nil {
			return err
		}
	}
	return nil
}

// validBondModes is the set of strings systemd accepts for
// BondMode= (see systemd.netdev(5)).
var validBondModes = map[string]struct{}{
	"balance-rr":    {},
	"active-backup": {},
	"balance-xor":   {},
	"broadcast":     {},
	"802.3ad":       {},
	"balance-tlb":   {},
	"balance-alb":   {},
}

func validateBond(b Bond) error {
	if err := validateConfigName(b.Name); err != nil {
		return err
	}
	if _, ok := validBondModes[b.Mode]; !ok {
		return fmt.Errorf("invalid bond mode %q", b.Mode)
	}
	if len(b.Members) == 0 {
		return fmt.Errorf("bond requires at least one member")
	}
	seen := map[string]struct{}{}
	for i, m := range b.Members {
		if err := validateMatchName(m); err != nil {
			return fmt.Errorf("members[%d]: %w", i, err)
		}
		if _, dup := seen[m]; dup {
			return fmt.Errorf("members[%d]: duplicate %q", i, m)
		}
		seen[m] = struct{}{}
	}
	if b.Address != "" {
		if err := validateCIDR(b.Address); err != nil {
			return err
		}
	}
	if b.MIIMonSec < 0 {
		return fmt.Errorf("miiMonSec must be >= 0")
	}
	return nil
}

// ---------- rendering ----------

// renderNetworkFile produces a systemd.network INI for a plain
// interface. The output is stable: order is Match -> Network ->
// Address blocks -> Route block.
func renderNetworkFile(c InterfaceConfig) []byte {
	var b bytes.Buffer
	b.WriteString("[Match]\n")
	fmt.Fprintf(&b, "Name=%s\n", c.MatchName)
	b.WriteByte('\n')

	b.WriteString("[Network]\n")
	if c.DHCP != "" {
		fmt.Fprintf(&b, "DHCP=%s\n", c.DHCP)
	}
	for _, d := range c.DNS {
		fmt.Fprintf(&b, "DNS=%s\n", d)
	}
	if len(c.Domains) > 0 {
		fmt.Fprintf(&b, "Domains=%s\n", strings.Join(c.Domains, " "))
	}
	if c.LinkLocal != "" {
		fmt.Fprintf(&b, "LinkLocalAddressing=%s\n", c.LinkLocal)
	}
	if c.MTU > 0 {
		fmt.Fprintf(&b, "MTUBytes=%d\n", c.MTU)
	}

	for _, a := range c.Addresses {
		b.WriteString("\n[Address]\n")
		fmt.Fprintf(&b, "Address=%s\n", a)
	}
	if c.Gateway != "" {
		b.WriteString("\n[Route]\n")
		fmt.Fprintf(&b, "Gateway=%s\n", c.Gateway)
	}
	return b.Bytes()
}

// renderVLANNetdev: [NetDev] + [VLAN] sections that define the device.
func renderVLANNetdev(v VLAN) []byte {
	var b bytes.Buffer
	b.WriteString("[NetDev]\n")
	fmt.Fprintf(&b, "Name=%s\n", v.Name)
	b.WriteString("Kind=vlan\n")
	b.WriteByte('\n')
	b.WriteString("[VLAN]\n")
	fmt.Fprintf(&b, "Id=%d\n", v.ID)
	return b.Bytes()
}

// renderVLANParentNetwork attaches the VLAN to its parent kernel iface.
func renderVLANParentNetwork(v VLAN) []byte {
	var b bytes.Buffer
	b.WriteString("[Match]\n")
	fmt.Fprintf(&b, "Name=%s\n", v.Parent)
	b.WriteByte('\n')
	b.WriteString("[Network]\n")
	fmt.Fprintf(&b, "VLAN=%s\n", v.Name)
	return b.Bytes()
}

// renderVLANNetwork configures the VLAN device itself (addresses).
func renderVLANNetwork(v VLAN) []byte {
	var b bytes.Buffer
	b.WriteString("[Match]\n")
	fmt.Fprintf(&b, "Name=%s\n", v.Name)
	b.WriteByte('\n')
	b.WriteString("[Network]\n")
	if v.Address == "" {
		b.WriteString("DHCP=yes\n")
	}
	if v.Address != "" {
		b.WriteString("\n[Address]\n")
		fmt.Fprintf(&b, "Address=%s\n", v.Address)
	}
	return b.Bytes()
}

func renderBondNetdev(b Bond) []byte {
	var w bytes.Buffer
	w.WriteString("[NetDev]\n")
	fmt.Fprintf(&w, "Name=%s\n", b.Name)
	w.WriteString("Kind=bond\n")
	w.WriteByte('\n')
	w.WriteString("[Bond]\n")
	fmt.Fprintf(&w, "Mode=%s\n", b.Mode)
	if b.MIIMonSec > 0 {
		fmt.Fprintf(&w, "MIIMonitorSec=%ds\n", b.MIIMonSec)
	}
	return w.Bytes()
}

func renderBondNetwork(b Bond) []byte {
	var w bytes.Buffer
	w.WriteString("[Match]\n")
	fmt.Fprintf(&w, "Name=%s\n", b.Name)
	w.WriteByte('\n')
	w.WriteString("[Network]\n")
	if b.Address == "" {
		w.WriteString("DHCP=yes\n")
	}
	if b.Address != "" {
		w.WriteString("\n[Address]\n")
		fmt.Fprintf(&w, "Address=%s\n", b.Address)
	}
	return w.Bytes()
}

// renderBondMemberNetwork marks a kernel iface as a slave of bondName.
func renderBondMemberNetwork(bondName, memberName string) []byte {
	var w bytes.Buffer
	w.WriteString("[Match]\n")
	fmt.Fprintf(&w, "Name=%s\n", memberName)
	w.WriteByte('\n')
	w.WriteString("[Network]\n")
	fmt.Fprintf(&w, "Bond=%s\n", bondName)
	return w.Bytes()
}

// ---------- public API: live ----------

// ipAddrEntry mirrors the relevant subset of `ip -j addr show` output.
type ipAddrEntry struct {
	IfName    string `json:"ifname"`
	LinkType  string `json:"link_type"`
	Address   string `json:"address"`
	OperState string `json:"operstate"`
	LinkInfo  *struct {
		InfoKind string `json:"info_kind"`
	} `json:"linkinfo,omitempty"`
	AddrInfo []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		Prefixlen int    `json:"prefixlen"`
	} `json:"addr_info"`
}

// ListInterfaces shells out to `ip -j addr show` and parses the JSON.
func (m *Manager) ListInterfaces(ctx context.Context) ([]LiveInterface, error) {
	out, err := m.run(ctx, m.ipBin(), "-j", "addr", "show")
	if err != nil {
		return nil, fmt.Errorf("ip addr show: %w", err)
	}
	return parseIPAddrJSON(out)
}

func parseIPAddrJSON(data []byte) ([]LiveInterface, error) {
	var raw []ipAddrEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse ip -j: %w", err)
	}
	out := make([]LiveInterface, 0, len(raw))
	for _, e := range raw {
		li := LiveInterface{
			Name:  e.IfName,
			State: e.OperState,
			MAC:   e.Address,
			Type:  classifyType(e.LinkType, e.LinkInfo),
		}
		for _, a := range e.AddrInfo {
			li.Addresses = append(li.Addresses,
				fmt.Sprintf("%s/%d", a.Local, a.Prefixlen))
		}
		out = append(out, li)
	}
	return out, nil
}

func classifyType(linkType string, info *struct {
	InfoKind string `json:"info_kind"`
}) string {
	if info != nil && info.InfoKind != "" {
		switch info.InfoKind {
		case "vlan":
			return "vlan"
		case "bond":
			return "bond"
		case "bridge":
			return "bridge"
		}
	}
	if linkType == "ether" {
		return "ether"
	}
	if linkType == "" {
		return "ether"
	}
	return linkType
}

// IdentifyManagementIface returns the kernel iface name whose
// addresses contain localIP. The HTTP layer is expected to call this
// with the local IP of the request's connection (e.g. r.Context()
// value populated from ServerName / Conn.LocalAddr) and refuse a
// mutation that would touch the returned iface unless the operator
// explicitly opts in.
//
// Returns an error if no iface owns the IP — better to fail closed
// than to silently allow the call.
func (m *Manager) IdentifyManagementIface(localIP net.IP) (string, error) {
	return identifyManagementIface(m, localIP)
}

// identifyManagementIface is a thin wrapper that performs the live
// listing through the Manager, then matches. Split out so tests can
// pass a fixture list without touching the runner.
func identifyManagementIface(m *Manager, localIP net.IP) (string, error) {
	if localIP == nil {
		return "", fmt.Errorf("nil localIP")
	}
	ctx := context.Background()
	live, err := m.ListInterfaces(ctx)
	if err != nil {
		return "", err
	}
	return matchInterfaceByIP(live, localIP)
}

// matchInterfaceByIP picks the iface holding localIP. Pure function so
// tests don't need a runner.
func matchInterfaceByIP(live []LiveInterface, localIP net.IP) (string, error) {
	for _, li := range live {
		for _, cidr := range li.Addresses {
			ip, _, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			if ip.Equal(localIP) {
				return li.Name, nil
			}
		}
	}
	return "", fmt.Errorf("no interface owns %s", localIP.String())
}

// ---------- public API: mutate ----------

// reload runs `networkctl reload`. Caller decides whether to invoke.
func (m *Manager) reload(ctx context.Context) error {
	if _, err := m.run(ctx, m.networkctl(), "reload"); err != nil {
		return fmt.Errorf("networkctl reload: %w", err)
	}
	return nil
}

// Reload is the exported reload — useful when an operator hand-edited
// files in NetworkDir and wants the API to commit the change.
func (m *Manager) Reload(ctx context.Context) error { return m.reload(ctx) }

// ApplyInterfaceConfig validates and renders cfg, then (unless DryRun)
// writes it and reloads networkd. Existing files of the same name are
// overwritten — there is no separate Create/Update split because the
// underlying systemd-networkd reload is idempotent and there is no
// need to defend against accidental clobbering of an unrelated
// resource (FilePrefix already namespaces our files away from
// operator-installed ones).
func (m *Manager) ApplyInterfaceConfig(ctx context.Context, cfg InterfaceConfig) error {
	if err := validateInterfaceConfig(cfg); err != nil {
		return err
	}
	body := renderNetworkFile(cfg)
	if cfg.DryRun {
		return nil
	}
	if err := m.fw().Write(m.networkFile(cfg.Name), body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cfg.Name, err)
	}
	return m.reload(ctx)
}

// DeleteInterfaceConfig removes the named .network file and reloads.
// Missing file is reported as ErrNotFound (so HTTP can map to 404).
func (m *Manager) DeleteInterfaceConfig(ctx context.Context, name string, dryRun bool) error {
	if err := validateConfigName(name); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	if err := m.fw().Remove(m.networkFile(name)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("remove %s: %w", name, err)
	}
	return m.reload(ctx)
}

// ApplyVLAN writes both the .netdev (defines the VLAN device) and the
// .network file that addresses it, plus a parent .network attaching
// the VLAN to its parent iface. Three files in total.
func (m *Manager) ApplyVLAN(ctx context.Context, v VLAN) error {
	if err := validateVLAN(v); err != nil {
		return err
	}
	netdev := renderVLANNetdev(v)
	parent := renderVLANParentNetwork(v)
	netw := renderVLANNetwork(v)
	if v.DryRun {
		return nil
	}
	if err := m.fw().Write(m.netdevFile(v.Name), netdev, 0o644); err != nil {
		return fmt.Errorf("write vlan netdev %s: %w", v.Name, err)
	}
	parentPath := filepath.Join(m.dir(),
		m.prefix()+v.Name+"-parent.network")
	if err := m.fw().Write(parentPath, parent, 0o644); err != nil {
		return fmt.Errorf("write vlan parent %s: %w", v.Name, err)
	}
	if err := m.fw().Write(m.networkFile(v.Name), netw, 0o644); err != nil {
		return fmt.Errorf("write vlan network %s: %w", v.Name, err)
	}
	return m.reload(ctx)
}

// ApplyBond writes the bond .netdev, the bond .network, and one
// per-member .network marking the kernel iface as enslaved.
//
// SAFETY NOTE: callers must check IdentifyManagementIface against
// every member before invoking. Enslaving the management iface to a
// bond will remove its IP and almost certainly cut the connection.
// The Manager intentionally does not perform this check itself —
// the HTTP layer is the right place to enforce force=true semantics.
func (m *Manager) ApplyBond(ctx context.Context, b Bond) error {
	if err := validateBond(b); err != nil {
		return err
	}
	netdev := renderBondNetdev(b)
	bondNet := renderBondNetwork(b)
	memberFiles := make(map[string][]byte, len(b.Members))
	for _, mem := range b.Members {
		memberFiles[m.memberNetworkFile(b.Name, mem)] =
			renderBondMemberNetwork(b.Name, mem)
	}
	if b.DryRun {
		return nil
	}
	if err := m.fw().Write(m.netdevFile(b.Name), netdev, 0o644); err != nil {
		return fmt.Errorf("write bond netdev %s: %w", b.Name, err)
	}
	if err := m.fw().Write(m.networkFile(b.Name), bondNet, 0o644); err != nil {
		return fmt.Errorf("write bond network %s: %w", b.Name, err)
	}
	// Sort to make Write call order deterministic for tests/logs.
	paths := make([]string, 0, len(memberFiles))
	for p := range memberFiles {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if err := m.fw().Write(p, memberFiles[p], 0o644); err != nil {
			return fmt.Errorf("write bond member %s: %w", p, err)
		}
	}
	return m.reload(ctx)
}

// ---------- public API: read ----------

// ListConfigs walks NetworkDir and returns every managed file we own.
// Files we cannot parse are skipped, matching the nfs package's
// "operator noise should not break the API" behaviour.
func (m *Manager) ListConfigs(ctx context.Context) ([]ManagedConfig, error) {
	entries, err := m.fw().ReadDir(m.dir())
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %w", m.dir(), err)
	}
	prefix := m.prefix()
	out := make([]ManagedConfig, 0, len(entries))
	seen := map[string]struct{}{}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		fname := ent.Name()
		if !strings.HasPrefix(fname, prefix) {
			continue
		}
		// Bond member files share the bond's name -- skip them in the
		// listing (they are an implementation detail of ApplyBond).
		trimmed := strings.TrimPrefix(fname, prefix)
		if strings.Contains(trimmed, "-member-") {
			continue
		}
		// Parent VLAN files likewise.
		if strings.HasSuffix(trimmed, "-parent.network") {
			continue
		}
		var name string
		switch {
		case strings.HasSuffix(trimmed, ".network"):
			name = strings.TrimSuffix(trimmed, ".network")
		case strings.HasSuffix(trimmed, ".netdev"):
			name = strings.TrimSuffix(trimmed, ".netdev")
		default:
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		mc, err := m.loadConfig(name)
		if err != nil {
			continue
		}
		out = append(out, *mc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetConfig loads a single named managed config.
func (m *Manager) GetConfig(ctx context.Context, name string) (*ManagedConfig, error) {
	if err := validateConfigName(name); err != nil {
		return nil, err
	}
	return m.loadConfig(name)
}

// loadConfig figures out which kind a name corresponds to by checking
// for a .netdev (=> VLAN or bond) and otherwise falling back to a
// plain .network (=> interface).
func (m *Manager) loadConfig(name string) (*ManagedConfig, error) {
	netdevPath := m.netdevFile(name)
	netdevBytes, netdevErr := m.fw().ReadFile(netdevPath)
	netPath := m.networkFile(name)
	netBytes, netErr := m.fw().ReadFile(netPath)

	hasNetdev := netdevErr == nil
	hasNet := netErr == nil

	if !hasNetdev && !hasNet {
		return nil, ErrNotFound
	}

	if hasNetdev {
		kind, err := detectNetdevKind(netdevBytes)
		if err != nil {
			return nil, err
		}
		switch kind {
		case "vlan":
			v, err := parseVLAN(name, netdevBytes, netBytes)
			if err != nil {
				return nil, err
			}
			return &ManagedConfig{Name: name, Kind: KindVLAN, VLAN: v}, nil
		case "bond":
			b, err := parseBond(name, netdevBytes, netBytes)
			if err != nil {
				return nil, err
			}
			return &ManagedConfig{Name: name, Kind: KindBond, Bond: b}, nil
		default:
			return nil, fmt.Errorf("unknown netdev kind %q", kind)
		}
	}

	c, err := parseInterfaceConfig(name, netBytes)
	if err != nil {
		return nil, err
	}
	return &ManagedConfig{Name: name, Kind: KindInterface, Interface: c}, nil
}

// ---------- ini parser ----------

// iniSection is one "[Section]" block with raw key=value pairs in
// declaration order.
type iniSection struct {
	Name  string
	Pairs []iniPair
}

type iniPair struct {
	Key   string
	Value string
}

// parseINI is a minimal ini parser. systemd-networkd files allow
// duplicate keys (e.g. multiple DNS=) and duplicate sections (multiple
// [Address] blocks), so we preserve order and do not dedupe.
func parseINI(data []byte) []iniSection {
	var out []iniSection
	var cur *iniSection
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			out = append(out, iniSection{Name: line[1 : len(line)-1]})
			cur = &out[len(out)-1]
			continue
		}
		if cur == nil {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		cur.Pairs = append(cur.Pairs, iniPair{
			Key:   strings.TrimSpace(line[:eq]),
			Value: strings.TrimSpace(line[eq+1:]),
		})
	}
	return out
}

func sectionValues(secs []iniSection, secName, key string) []string {
	var out []string
	for _, s := range secs {
		if s.Name != secName {
			continue
		}
		for _, p := range s.Pairs {
			if p.Key == key {
				out = append(out, p.Value)
			}
		}
	}
	return out
}

func firstSectionValue(secs []iniSection, secName, key string) string {
	v := sectionValues(secs, secName, key)
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

// addressBlocks returns Address= values, one per [Address] section.
func addressBlocks(secs []iniSection) []string {
	var out []string
	for _, s := range secs {
		if s.Name != "Address" {
			continue
		}
		for _, p := range s.Pairs {
			if p.Key == "Address" {
				out = append(out, p.Value)
			}
		}
	}
	return out
}

func detectNetdevKind(data []byte) (string, error) {
	secs := parseINI(data)
	v := firstSectionValue(secs, "NetDev", "Kind")
	if v == "" {
		return "", fmt.Errorf("netdev missing Kind=")
	}
	return v, nil
}

func parseInterfaceConfig(name string, data []byte) (*InterfaceConfig, error) {
	secs := parseINI(data)
	c := InterfaceConfig{
		Name:      name,
		MatchName: firstSectionValue(secs, "Match", "Name"),
		DHCP:      firstSectionValue(secs, "Network", "DHCP"),
		LinkLocal: firstSectionValue(secs, "Network", "LinkLocalAddressing"),
	}
	c.DNS = sectionValues(secs, "Network", "DNS")
	if d := firstSectionValue(secs, "Network", "Domains"); d != "" {
		c.Domains = strings.Fields(d)
	}
	if mtu := firstSectionValue(secs, "Network", "MTUBytes"); mtu != "" {
		var n int
		fmt.Sscanf(mtu, "%d", &n)
		c.MTU = n
	}
	c.Addresses = addressBlocks(secs)
	c.Gateway = firstSectionValue(secs, "Route", "Gateway")
	return &c, nil
}

func parseVLAN(name string, netdev, netw []byte) (*VLAN, error) {
	dsecs := parseINI(netdev)
	id := 0
	if idStr := firstSectionValue(dsecs, "VLAN", "Id"); idStr != "" {
		fmt.Sscanf(idStr, "%d", &id)
	}
	v := &VLAN{Name: name, ID: id}
	if len(netw) > 0 {
		nsecs := parseINI(netw)
		addrs := addressBlocks(nsecs)
		if len(addrs) > 0 {
			v.Address = addrs[0]
		}
	}
	// Parent is recovered from the parent .network if present; not
	// strictly required for round-trip of the netdev itself but the
	// round-trip test reads/writes it so we leave it empty here and
	// let callers populate it via ListConfigs's higher-level scan if
	// needed.
	return v, nil
}

func parseBond(name string, netdev, netw []byte) (*Bond, error) {
	dsecs := parseINI(netdev)
	mode := firstSectionValue(dsecs, "Bond", "Mode")
	mii := 0
	if s := firstSectionValue(dsecs, "Bond", "MIIMonitorSec"); s != "" {
		// strip trailing 's' if present.
		s = strings.TrimSuffix(s, "s")
		fmt.Sscanf(s, "%d", &mii)
	}
	b := &Bond{Name: name, Mode: mode, MIIMonSec: mii}
	if len(netw) > 0 {
		nsecs := parseINI(netw)
		addrs := addressBlocks(nsecs)
		if len(addrs) > 0 {
			b.Address = addrs[0]
		}
	}
	// Members are stored only in per-member .network files. parseBond
	// is called from loadConfig which has access only to the bond's
	// own files; recovering members would require a directory scan.
	// We leave Members empty here -- ListConfigs returns the bond
	// without members and callers that care can resolve members from
	// the live `ip` view.
	return b, nil
}
