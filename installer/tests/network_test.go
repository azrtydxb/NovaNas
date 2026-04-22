package tests

import (
	"strings"
	"testing"

	"github.com/azrtydxb/novanas/installer/internal/network"
)

func TestStaticConfigValidate(t *testing.T) {
	good := network.StaticConfig{
		Interface: "eno1", Hostname: "nas",
		Address: "192.168.1.50/24", Gateway: "192.168.1.1",
		DNS: []string{"1.1.1.1"},
	}
	if err := good.Validate(); err != nil {
		t.Errorf("good config: %v", err)
	}
	bad := []network.StaticConfig{
		{Interface: "", Hostname: "nas", Address: "192.168.1.50/24", Gateway: "192.168.1.1"},
		{Interface: "eno1", Hostname: "", Address: "192.168.1.50/24", Gateway: "192.168.1.1"},
		{Interface: "eno1", Hostname: "nas", Address: "notacidr", Gateway: "192.168.1.1"},
		{Interface: "eno1", Hostname: "nas", Address: "192.168.1.50/24", Gateway: "nope"},
		{Interface: "eno1", Hostname: "nas", Address: "192.168.1.50/24", Gateway: "192.168.1.1", DNS: []string{"not-an-ip"}},
	}
	for i, b := range bad {
		if err := b.Validate(); err == nil {
			t.Errorf("case %d: expected error, got nil: %+v", i, b)
		}
	}
}

func TestRenderNmstateDHCP(t *testing.T) {
	y := network.RenderNmstate("eno1", "nas", nil)
	mustContain(t, y, "name: eno1")
	mustContain(t, y, "dhcp: true")
	mustContain(t, y, "hostname:")
}

func TestRenderNmstateStatic(t *testing.T) {
	cfg := &network.StaticConfig{
		Interface: "eno1", Hostname: "nas",
		Address: "10.0.0.50/24", Gateway: "10.0.0.1",
		DNS: []string{"1.1.1.1", "8.8.8.8"},
	}
	y := network.RenderNmstate("eno1", "nas", cfg)
	mustContain(t, y, "ip: 10.0.0.50")
	mustContain(t, y, "prefix-length: 24")
	mustContain(t, y, "next-hop-address: 10.0.0.1")
	mustContain(t, y, "1.1.1.1")
	mustContain(t, y, "8.8.8.8")
}

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("missing %q in:\n%s", sub, s)
	}
}
