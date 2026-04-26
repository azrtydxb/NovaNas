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
