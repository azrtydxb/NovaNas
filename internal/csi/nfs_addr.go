package csi

import (
	"fmt"
	"net"
	"strings"
)

// validatePrivateNFSEndpoints checks that NFSServer and DefaultNFSClients
// do not contain public IPs. Tenant data should never be exposed to a
// public-routable address by accident — operators who really want to
// override this for a non-NovaNAS scenario can set NOVA_CSI_ALLOW_PUBLIC_NFS=1
// at the wrapper level (the driver itself does not honor that env var; it's
// the operator's responsibility to gate it before NewDriver is called).
//
// Allowed forms:
//   - empty string (NFS-mode disabled)
//   - hostname (we cannot validate; trust the operator)
//   - IPv4 address inside RFC1918 (10/8, 172.16/12, 192.168/16) or 127/8
//   - IPv6 ULA (fc00::/7) or loopback (::1/128) or link-local (fe80::/10)
//   - CIDR likewise restricted to private ranges
//
// Returns a descriptive error if any input is public.
func validatePrivateNFSEndpoints(server, clientsCSV string) error {
	if err := validatePrivateEndpoint(server); err != nil {
		return fmt.Errorf("nfs-server %q: %w", server, err)
	}
	for _, raw := range strings.Split(clientsCSV, ",") {
		c := strings.TrimSpace(raw)
		if c == "" {
			continue
		}
		if err := validatePrivateEndpoint(c); err != nil {
			return fmt.Errorf("default-nfs-clients entry %q: %w", c, err)
		}
	}
	return nil
}

func validatePrivateEndpoint(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// Strip a port if present (host:port form).
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	// CIDR?
	if strings.Contains(s, "/") {
		_, ipnet, err := net.ParseCIDR(s)
		if err != nil {
			return fmt.Errorf("invalid CIDR")
		}
		if !isPrivateIP(ipnet.IP) {
			return fmt.Errorf("CIDR is not RFC1918 / ULA / loopback / link-local — refusing to expose NFS shares to public networks")
		}
		return nil
	}
	// Bare IP?
	if ip := net.ParseIP(s); ip != nil {
		if !isPrivateIP(ip) {
			return fmt.Errorf("IP is not RFC1918 / ULA / loopback / link-local — refusing to expose NFS shares to public networks")
		}
		return nil
	}
	// Hostname or wildcard ("*", "*.example.org") — trust the operator. The
	// downstream NFS export tooling will reject malformed entries.
	return nil
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
		return true
	}
	// net.IP.IsPrivate() covers RFC1918 + ULA. Belt-and-braces for IPv6 ULA.
	if ip16 := ip.To16(); ip16 != nil && ip16[0]&0xfe == 0xfc {
		return true
	}
	return false
}
