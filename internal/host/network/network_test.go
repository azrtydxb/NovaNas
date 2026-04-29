package network

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---------- shared fakes ----------

type captureRunner struct {
	calls [][]string
	err   error
	out   []byte
}

func (c *captureRunner) run(_ context.Context, bin string, args ...string) ([]byte, error) {
	cp := append([]string{bin}, args...)
	c.calls = append(c.calls, cp)
	return c.out, c.err
}

type captureFW struct {
	files      map[string][]byte
	writes     []string
	removes    []string
	dirEntries []os.DirEntry
}

func newFW() *captureFW { return &captureFW{files: map[string][]byte{}} }

func (c *captureFW) Write(path string, data []byte, _ os.FileMode) error {
	c.files[path] = append([]byte(nil), data...)
	c.writes = append(c.writes, path)
	return nil
}
func (c *captureFW) Remove(path string) error {
	if _, ok := c.files[path]; !ok {
		return &os.PathError{Op: "remove", Path: path, Err: fs.ErrNotExist}
	}
	delete(c.files, path)
	c.removes = append(c.removes, path)
	return nil
}
func (c *captureFW) ReadFile(path string) ([]byte, error) {
	if d, ok := c.files[path]; ok {
		return append([]byte(nil), d...), nil
	}
	return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
}
func (c *captureFW) ReadDir(_ string) ([]os.DirEntry, error) {
	if c.dirEntries != nil {
		return c.dirEntries, nil
	}
	out := make([]os.DirEntry, 0, len(c.files))
	for p := range c.files {
		out = append(out, fakeEntry{name: p[strings.LastIndex(p, "/")+1:]})
	}
	return out, nil
}

type fakeEntry struct{ name string }

func (f fakeEntry) Name() string               { return f.name }
func (f fakeEntry) IsDir() bool                { return false }
func (f fakeEntry) Type() os.FileMode          { return 0 }
func (f fakeEntry) Info() (os.FileInfo, error) { return nil, nil }

func newMgr() (*Manager, *captureRunner, *captureFW) {
	r := &captureRunner{}
	fw := newFW()
	return &Manager{
		NetworkDir: "/etc/systemd/network",
		Runner:     r.run,
		FileWriter: fw,
	}, r, fw
}

// ---------- render -> parse round-trip ----------

func TestInterfaceConfig_RoundTrip(t *testing.T) {
	in := InterfaceConfig{
		Name:      "wan",
		MatchName: "eno1",
		DHCP:      "no",
		Addresses: []string{"10.0.0.5/24", "192.168.1.5/24"},
		Gateway:   "10.0.0.1",
		DNS:       []string{"1.1.1.1", "8.8.8.8"},
		Domains:   []string{"example.com"},
		MTU:       1500,
		LinkLocal: "ipv6",
	}
	body := renderNetworkFile(in)
	got, err := parseInterfaceConfig("wan", body)
	if err != nil {
		t.Fatal(err)
	}
	// Re-render to confirm bytes are stable.
	body2 := renderNetworkFile(*got)
	if string(body) != string(body2) {
		t.Fatalf("not stable:\n--- a ---\n%s\n--- b ---\n%s", body, body2)
	}
	if !reflect.DeepEqual(got.Addresses, in.Addresses) {
		t.Errorf("addresses: %v vs %v", got.Addresses, in.Addresses)
	}
	if got.Gateway != in.Gateway {
		t.Errorf("gateway: %q vs %q", got.Gateway, in.Gateway)
	}
	if !reflect.DeepEqual(got.DNS, in.DNS) {
		t.Errorf("dns: %v vs %v", got.DNS, in.DNS)
	}
}

func TestVLAN_RoundTrip(t *testing.T) {
	v := VLAN{Name: "vlan10", Parent: "eno1", ID: 10, Address: "10.10.0.1/24"}
	netdev := renderVLANNetdev(v)
	netw := renderVLANNetwork(v)
	got, err := parseVLAN("vlan10", netdev, netw)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != v.ID || got.Address != v.Address || got.Name != v.Name {
		t.Errorf("vlan mismatch: %+v vs %+v", got, v)
	}
}

func TestBond_RoundTrip(t *testing.T) {
	b := Bond{
		Name: "bond0", Mode: "active-backup",
		Members: []string{"eno1", "eno2"}, Address: "10.0.0.5/24",
		MIIMonSec: 1,
	}
	netdev := renderBondNetdev(b)
	netw := renderBondNetwork(b)
	got, err := parseBond("bond0", netdev, netw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != b.Mode || got.Address != b.Address || got.MIIMonSec != b.MIIMonSec {
		t.Errorf("bond mismatch: %+v vs %+v", got, b)
	}
}

// ---------- IdentifyManagementIface ----------

func TestMatchInterfaceByIP(t *testing.T) {
	live := []LiveInterface{
		{Name: "lo", Addresses: []string{"127.0.0.1/8"}},
		{Name: "eno1", Addresses: []string{"10.0.0.5/24", "fe80::1/64"}},
		{Name: "eno2", Addresses: []string{"192.168.1.5/24"}},
	}
	cases := []struct {
		ip   string
		want string
		err  bool
	}{
		{"10.0.0.5", "eno1", false},
		{"192.168.1.5", "eno2", false},
		{"127.0.0.1", "lo", false},
		{"8.8.8.8", "", true},
	}
	for _, tc := range cases {
		got, err := matchInterfaceByIP(live, net.ParseIP(tc.ip))
		if (err != nil) != tc.err {
			t.Errorf("ip=%s err=%v want_err=%v", tc.ip, err, tc.err)
			continue
		}
		if got != tc.want {
			t.Errorf("ip=%s got=%q want=%q", tc.ip, got, tc.want)
		}
	}
}

// ---------- validation ----------

func TestValidate_BadCIDR(t *testing.T) {
	c := InterfaceConfig{Name: "x", MatchName: "eno1", DHCP: "no",
		Addresses: []string{"not-a-cidr"}}
	if err := validateInterfaceConfig(c); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidate_VLANRange(t *testing.T) {
	for _, id := range []int{0, 4095, -1, 99999} {
		err := validateVLAN(VLAN{Name: "v", Parent: "eno1", ID: id})
		if err == nil {
			t.Errorf("vlan id=%d: expected error", id)
		}
	}
	if err := validateVLAN(VLAN{Name: "v", Parent: "eno1", ID: 100}); err != nil {
		t.Errorf("vlan id=100: unexpected error %v", err)
	}
}

func TestValidate_BondNoMembers(t *testing.T) {
	err := validateBond(Bond{Name: "b", Mode: "active-backup"})
	if err == nil {
		t.Fatal("expected error for empty members")
	}
}

func TestValidate_BondBadMode(t *testing.T) {
	err := validateBond(Bond{Name: "b", Mode: "magic", Members: []string{"a"}})
	if err == nil {
		t.Fatal("expected error for bad mode")
	}
}

// ---------- DryRun: no FileWriter, no Runner ----------

func TestDryRun_Interface(t *testing.T) {
	m, r, fw := newMgr()
	cfg := InterfaceConfig{
		Name: "wan", MatchName: "eno1", DHCP: "yes", DryRun: true,
	}
	if err := m.ApplyInterfaceConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if len(fw.writes) != 0 {
		t.Errorf("expected zero writes, got %v", fw.writes)
	}
	if len(r.calls) != 0 {
		t.Errorf("expected zero runner calls, got %v", r.calls)
	}
}

func TestDryRun_VLAN(t *testing.T) {
	m, r, fw := newMgr()
	v := VLAN{Name: "vlan10", Parent: "eno1", ID: 10, DryRun: true}
	if err := m.ApplyVLAN(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	if len(fw.writes) != 0 || len(r.calls) != 0 {
		t.Errorf("vlan dry-run leaked: writes=%v calls=%v", fw.writes, r.calls)
	}
}

func TestDryRun_Bond(t *testing.T) {
	m, r, fw := newMgr()
	b := Bond{Name: "bond0", Mode: "active-backup",
		Members: []string{"eno1"}, DryRun: true}
	if err := m.ApplyBond(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if len(fw.writes) != 0 || len(r.calls) != 0 {
		t.Errorf("bond dry-run leaked: writes=%v calls=%v", fw.writes, r.calls)
	}
}

func TestDryRun_Delete(t *testing.T) {
	m, r, fw := newMgr()
	if err := m.DeleteInterfaceConfig(context.Background(), "x", true); err != nil {
		t.Fatal(err)
	}
	if len(fw.writes)+len(fw.removes)+len(r.calls) != 0 {
		t.Errorf("delete dry-run leaked")
	}
}

// ---------- Apply happy paths ----------

func TestApplyInterfaceConfig_WritesAndReloads(t *testing.T) {
	m, r, fw := newMgr()
	cfg := InterfaceConfig{
		Name: "wan", MatchName: "eno1", DHCP: "no",
		Addresses: []string{"10.0.0.5/24"}, Gateway: "10.0.0.1",
	}
	if err := m.ApplyInterfaceConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	wantPath := "/etc/systemd/network/70-nova-wan.network"
	if _, ok := fw.files[wantPath]; !ok {
		t.Fatalf("expected file %s, files=%v", wantPath, keys(fw.files))
	}
	if len(r.calls) != 1 || r.calls[0][1] != "reload" {
		t.Errorf("expected one networkctl reload, got %v", r.calls)
	}
}

func TestApplyBond_WritesMemberFiles(t *testing.T) {
	m, _, fw := newMgr()
	b := Bond{Name: "bond0", Mode: "active-backup",
		Members: []string{"eno1", "eno2"}}
	if err := m.ApplyBond(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/etc/systemd/network/70-nova-bond0.netdev",
		"/etc/systemd/network/70-nova-bond0.network",
		"/etc/systemd/network/70-nova-bond0-member-eno1.network",
		"/etc/systemd/network/70-nova-bond0-member-eno2.network",
	}
	for _, p := range want {
		if _, ok := fw.files[p]; !ok {
			t.Errorf("missing file %s", p)
		}
	}
}

func TestApplyVLAN_WritesThreeFiles(t *testing.T) {
	m, _, fw := newMgr()
	v := VLAN{Name: "vlan10", Parent: "eno1", ID: 10, Address: "10.10.0.1/24"}
	if err := m.ApplyVLAN(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/etc/systemd/network/70-nova-vlan10.netdev",
		"/etc/systemd/network/70-nova-vlan10.network",
		"/etc/systemd/network/70-nova-vlan10-parent.network",
	}
	for _, p := range want {
		if _, ok := fw.files[p]; !ok {
			t.Errorf("missing file %s, have %v", p, keys(fw.files))
		}
	}
}

// ---------- Delete missing ----------

func TestDeleteInterfaceConfig_NotFound(t *testing.T) {
	m, _, _ := newMgr()
	err := m.DeleteInterfaceConfig(context.Background(), "missing", false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound got %v", err)
	}
}

// ---------- ip -j parsing ----------

func TestParseIPAddrJSON(t *testing.T) {
	data := []byte(`[
	  {"ifname":"lo","operstate":"UNKNOWN","link_type":"loopback","address":"00:00:00:00:00:00",
	   "addr_info":[{"family":"inet","local":"127.0.0.1","prefixlen":8}]},
	  {"ifname":"eno1","operstate":"UP","link_type":"ether","address":"aa:bb:cc:dd:ee:ff",
	   "addr_info":[{"family":"inet","local":"10.0.0.5","prefixlen":24}]}
	]`)
	got, err := parseIPAddrJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Name != "eno1" || got[1].Addresses[0] != "10.0.0.5/24" {
		t.Fatalf("bad parse: %+v", got)
	}
}

// ---------- helpers ----------

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// keep imports referenced even if some tests are dropped.
var _ = time.Second
