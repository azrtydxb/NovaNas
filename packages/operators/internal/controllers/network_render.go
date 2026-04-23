// Package-level helpers for the Wave-4 networking controllers. These
// functions render desired state into nmstate YAML / nftables /
// tc / WireGuard config strings, deterministically, so controllers can
// compare hashes for drift detection.
package controllers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
)

// hashBytes returns the sha256 hex digest of b.
func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// renderBondNmstate projects a Bond CR into an nmstate YAML document.
// Mode and member list are required fields so we can always emit a valid
// document; optional tuning knobs appear only when set.
func renderBondNmstate(b *novanasv1alpha1.Bond) string {
	var sb strings.Builder
	sb.WriteString("interfaces:\n")
	fmt.Fprintf(&sb, "  - name: %s\n", b.Name)
	sb.WriteString("    type: bond\n")
	sb.WriteString("    state: up\n")
	if b.Spec.Mtu != nil {
		fmt.Fprintf(&sb, "    mtu: %d\n", *b.Spec.Mtu)
	}
	sb.WriteString("    link-aggregation:\n")
	mode := string(b.Spec.Mode)
	if mode == "" {
		mode = "active-backup"
	}
	fmt.Fprintf(&sb, "      mode: %s\n", mode)
	if len(b.Spec.Interfaces) > 0 {
		sb.WriteString("      port:\n")
		for _, p := range b.Spec.Interfaces {
			fmt.Fprintf(&sb, "        - %s\n", p)
		}
	}
	if b.Spec.XmitHashPolicy != "" {
		sb.WriteString("      options:\n")
		fmt.Fprintf(&sb, "        xmit_hash_policy: %s\n", b.Spec.XmitHashPolicy)
		if b.Spec.Miimon != nil {
			fmt.Fprintf(&sb, "        miimon: %d\n", *b.Spec.Miimon)
		}
	} else if b.Spec.Miimon != nil {
		sb.WriteString("      options:\n")
		fmt.Fprintf(&sb, "        miimon: %d\n", *b.Spec.Miimon)
	}
	if b.Spec.Lacp != nil {
		if b.Spec.Lacp.Rate != "" {
			fmt.Fprintf(&sb, "      lacp-rate: %s\n", b.Spec.Lacp.Rate)
		}
	}
	return sb.String()
}

// renderVlanNmstate projects a Vlan CR into nmstate YAML.
func renderVlanNmstate(v *novanasv1alpha1.Vlan) string {
	var sb strings.Builder
	sb.WriteString("interfaces:\n")
	fmt.Fprintf(&sb, "  - name: %s\n", v.Name)
	sb.WriteString("    type: vlan\n")
	sb.WriteString("    state: up\n")
	if v.Spec.Mtu != nil {
		fmt.Fprintf(&sb, "    mtu: %d\n", *v.Spec.Mtu)
	}
	sb.WriteString("    vlan:\n")
	fmt.Fprintf(&sb, "      base-iface: %s\n", v.Spec.Parent)
	fmt.Fprintf(&sb, "      id: %d\n", v.Spec.VlanId)
	return sb.String()
}

// renderHostInterfaceNmstate projects a HostInterface CR into nmstate YAML
// including IP addressing and routing hints.
func renderHostInterfaceNmstate(h *novanasv1alpha1.HostInterface) string {
	var sb strings.Builder
	sb.WriteString("interfaces:\n")
	fmt.Fprintf(&sb, "  - name: %s\n", h.Name)
	sb.WriteString("    type: ethernet\n")
	sb.WriteString("    state: up\n")
	if h.Spec.Mtu != nil {
		fmt.Fprintf(&sb, "    mtu: %d\n", *h.Spec.Mtu)
	}
	if len(h.Spec.Addresses) > 0 {
		sb.WriteString("    ipv4:\n")
		sb.WriteString("      enabled: true\n")
		sb.WriteString("      address:\n")
		for _, a := range h.Spec.Addresses {
			parts := strings.SplitN(a.Cidr, "/", 2)
			ip := parts[0]
			prefix := "24"
			if len(parts) == 2 {
				prefix = parts[1]
			}
			fmt.Fprintf(&sb, "        - ip: %s\n", ip)
			fmt.Fprintf(&sb, "          prefix-length: %s\n", prefix)
		}
	}
	if len(h.Spec.Dns) > 0 {
		sb.WriteString("dns-resolver:\n")
		sb.WriteString("  config:\n")
		sb.WriteString("    server:\n")
		for _, s := range h.Spec.Dns {
			fmt.Fprintf(&sb, "      - %s\n", s)
		}
	}
	if h.Spec.Gateway != "" {
		sb.WriteString("routes:\n")
		sb.WriteString("  config:\n")
		sb.WriteString("    - destination: 0.0.0.0/0\n")
		fmt.Fprintf(&sb, "      next-hop-address: %s\n", h.Spec.Gateway)
		fmt.Fprintf(&sb, "      next-hop-interface: %s\n", h.Name)
	}
	return sb.String()
}

// renderFirewallRule projects the FirewallRule into a deterministic
// nftables-style rule string. The rule is prefix-sortable so repeated
// reconciles produce identical bytes.
func renderFirewallRule(r *novanasv1alpha1.FirewallRule) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s scope=%s dir=%s action=%s prio=%d\n",
		r.Name, r.Spec.Scope, r.Spec.Direction, r.Spec.Action, r.Spec.Priority)
	if r.Spec.Interface != "" {
		fmt.Fprintf(&sb, "iif %s ", r.Spec.Interface)
	}
	if r.Spec.Source != nil {
		if len(r.Spec.Source.Cidrs) > 0 {
			cidrs := append([]string(nil), r.Spec.Source.Cidrs...)
			sort.Strings(cidrs)
			fmt.Fprintf(&sb, "ip saddr {%s} ", strings.Join(cidrs, ","))
		}
		if r.Spec.Source.Protocol != "" {
			fmt.Fprintf(&sb, "%s ", r.Spec.Source.Protocol)
		}
	}
	if r.Spec.Destination != nil {
		if len(r.Spec.Destination.Cidrs) > 0 {
			cidrs := append([]string(nil), r.Spec.Destination.Cidrs...)
			sort.Strings(cidrs)
			fmt.Fprintf(&sb, "ip daddr {%s} ", strings.Join(cidrs, ","))
		}
		if len(r.Spec.Destination.Ports) > 0 {
			ports := append([]int32(nil), r.Spec.Destination.Ports...)
			sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })
			var ps []string
			for _, p := range ports {
				ps = append(ps, fmt.Sprintf("%d", p))
			}
			fmt.Fprintf(&sb, "dport {%s} ", strings.Join(ps, ","))
		}
	}
	sb.WriteString(string(r.Spec.Action))
	sb.WriteString("\n")
	return sb.String()
}

// renderTrafficLimits projects a TrafficPolicy into a tc-friendly config
// blob. The actual installer on the host turns this into `tc qdisc`
// invocations; we only need deterministic bytes for hashing.
func renderTrafficLimits(t *novanasv1alpha1.TrafficPolicy) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "scope: %s/%s\n", t.Spec.Scope.Kind, t.Spec.Scope.Name)
	if t.Spec.Scope.Namespace != "" {
		fmt.Fprintf(&sb, "namespace: %s\n", t.Spec.Scope.Namespace)
	}
	if t.Spec.Limits != nil {
		if t.Spec.Limits.Egress != nil {
			fmt.Fprintf(&sb, "egress: max=%s burst=%s\n", t.Spec.Limits.Egress.Max, t.Spec.Limits.Egress.Burst)
		}
		if t.Spec.Limits.Ingress != nil {
			fmt.Fprintf(&sb, "ingress: max=%s burst=%s\n", t.Spec.Limits.Ingress.Max, t.Spec.Limits.Ingress.Burst)
		}
	}
	if len(t.Spec.Scheduling) > 0 {
		keys := make([]string, 0, len(t.Spec.Scheduling))
		for k := range t.Spec.Scheduling {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			w := t.Spec.Scheduling[k]
			fmt.Fprintf(&sb, "window %s: cron=%q dur=%d\n", k, w.Cron, w.DurationMinutes)
		}
	}
	fmt.Fprintf(&sb, "priority: %d\n", t.Spec.Priority)
	return sb.String()
}

// renderServicePolicy projects a ServicePolicy into deterministic
// ConfigMap payload data.
func renderServicePolicy(p *novanasv1alpha1.ServicePolicy) map[string]string {
	out := map[string]string{}
	items := append([]novanasv1alpha1.ServiceToggle(nil), p.Spec.Services...)
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	for _, s := range items {
		val := fmt.Sprintf("enabled=%t port=%d iface=%s", s.Enabled, s.Port, s.BindInterface)
		out[string(s.Name)] = val
	}
	return out
}

// cidrCapacity returns the number of usable addresses in a CIDR. Returns 0
// when the input is malformed so the caller can surface Failed status.
// This is a deliberately conservative calculator: we only support /0..32
// for IPv4 style strings and a small-CIDR IPv6 approximation. Enough for
// MetalLB-style "250 VIPs in a /24" reporting.
func cidrCapacity(cidr string) int32 {
	parts := strings.SplitN(cidr, "/", 2)
	if len(parts) != 2 {
		return 0
	}
	// Detect IPv6 by colon; fall back to a fixed quota — exact IPv6 sizes
	// overflow int32 anyway.
	if strings.Contains(parts[0], ":") {
		return 1024
	}
	var prefix int
	if _, err := fmt.Sscanf(parts[1], "%d", &prefix); err != nil || prefix < 0 || prefix > 32 {
		return 0
	}
	host := 32 - prefix
	if host >= 31 {
		return 0
	}
	return int32(1<<host) - 2
}
