package network

// DHCPConfig represents a DHCP-only network configuration for one NIC.
type DHCPConfig struct {
	Interface string
	Hostname  string
}

// Probe is a placeholder for a future DHCP probe. The installer cannot
// reliably test a live DHCP lease before partitioning, so we defer actual
// probing to first boot. Returning nil means "assume DHCP will work".
func Probe(iface string) error {
	return nil
}
