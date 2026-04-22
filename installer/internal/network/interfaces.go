// Package network probes NICs and renders nmstate-compatible YAML.
package network

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Interface is a candidate management NIC.
type Interface struct {
	Name    string // e.g. "eno1", "enp0s25"
	MAC     string
	LinkUp  bool
	Carrier bool
}

// SysClassNet is the Linux sysfs path where NICs live.
const SysClassNet = "/sys/class/net"

// List enumerates ethernet-class interfaces from sysfs. Loopback, tunnels,
// bridges, and wireless are excluded — the installer configures a single
// management NIC; more complex topologies are set later from the UI.
func List() ([]Interface, error) {
	return listFrom(SysClassNet)
}

func listFrom(root string) ([]Interface, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []Interface
	for _, e := range entries {
		name := e.Name()
		if name == "lo" {
			continue
		}
		typePath := filepath.Join(root, name, "type")
		b, err := os.ReadFile(typePath)
		if err != nil {
			continue
		}
		// 1 == ether; see include/uapi/linux/if_arp.h
		if strings.TrimSpace(string(b)) != "1" {
			continue
		}
		iface := Interface{Name: name}
		if mac, err := os.ReadFile(filepath.Join(root, name, "address")); err == nil {
			iface.MAC = strings.TrimSpace(string(mac))
		}
		if op, err := os.ReadFile(filepath.Join(root, name, "operstate")); err == nil {
			iface.LinkUp = strings.TrimSpace(string(op)) == "up"
		}
		if c, err := os.ReadFile(filepath.Join(root, name, "carrier")); err == nil {
			iface.Carrier = strings.TrimSpace(string(c)) == "1"
		}
		out = append(out, iface)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
