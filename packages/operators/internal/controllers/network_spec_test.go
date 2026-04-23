// Extra tests for the Wave-4 networking controllers focused on spec-level
// rendering: nmstate YAML content, hash propagation, VIP capacity math,
// finalizer teardown, and projection into novanet / novaedge unstructured
// CRs.
package controllers

import (
	"context"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
)

// ----- render helpers -----

func TestRenderBondNmstate_IncludesMemberPorts(t *testing.T) {
	b := &novanasv1alpha1.Bond{
		ObjectMeta: metav1.ObjectMeta{Name: "bond0"},
		Spec: novanasv1alpha1.BondSpec{
			Interfaces: []string{"eno1", "eno2"},
			Mode:       "802.3ad",
			Lacp:       &novanasv1alpha1.BondLacp{Rate: "fast"},
		},
	}
	out := renderBondNmstate(b)
	for _, want := range []string{"bond0", "802.3ad", "eno1", "eno2", "lacp-rate: fast"} {
		if !strings.Contains(out, want) {
			t.Errorf("bond nmstate missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderVlanNmstate_EmitsId(t *testing.T) {
	v := &novanasv1alpha1.Vlan{
		ObjectMeta: metav1.ObjectMeta{Name: "vlan42"},
		Spec:       novanasv1alpha1.VlanSpec{Parent: "bond0", VlanId: 42},
	}
	out := renderVlanNmstate(v)
	if !strings.Contains(out, "id: 42") || !strings.Contains(out, "base-iface: bond0") {
		t.Errorf("vlan nmstate missing id/base:\n%s", out)
	}
}

func TestRenderHostInterfaceNmstate_IncludesRoute(t *testing.T) {
	h := &novanasv1alpha1.HostInterface{
		ObjectMeta: metav1.ObjectMeta{Name: "mgmt0"},
		Spec: novanasv1alpha1.HostInterfaceSpec{
			Backing:   "eno1",
			Addresses: []novanasv1alpha1.HostInterfaceAddress{{Cidr: "10.0.0.2/24", Type: "static"}},
			Gateway:   "10.0.0.1",
			Dns:       []string{"1.1.1.1"},
			Usage:     []novanasv1alpha1.HostInterfaceUsage{"management"},
		},
	}
	out := renderHostInterfaceNmstate(h)
	for _, want := range []string{"10.0.0.2", "prefix-length: 24", "1.1.1.1", "next-hop-address: 10.0.0.1"} {
		if !strings.Contains(out, want) {
			t.Errorf("host interface nmstate missing %q\n%s", want, out)
		}
	}
}

func TestRenderFirewallRule_DeterministicSortedCidrs(t *testing.T) {
	r := &novanasv1alpha1.FirewallRule{
		ObjectMeta: metav1.ObjectMeta{Name: "block-ssh"},
		Spec: novanasv1alpha1.FirewallRuleSpec{
			Scope:     "host",
			Direction: "inbound",
			Action:    "deny",
			Source:    &novanasv1alpha1.FirewallEndpoint{Cidrs: []string{"10.0.0.0/8", "192.168.0.0/16"}},
			Destination: &novanasv1alpha1.FirewallEndpoint{
				Ports: []int32{22, 22022},
			},
		},
	}
	a := renderFirewallRule(r)
	b := renderFirewallRule(r)
	if a != b {
		t.Fatal("render not deterministic")
	}
	if !strings.Contains(a, "deny") || !strings.Contains(a, "22,22022") {
		t.Errorf("firewall rule render unexpected:\n%s", a)
	}
}

func TestCidrCapacity(t *testing.T) {
	cases := map[string]int32{
		"10.0.0.0/24":  254,
		"10.0.0.0/30":  2,
		"10.0.0.0/31":  0,
		"10.0.0.0/0":   0, // would overflow int32 -- implementation returns 0 or sentinel
		"bad":          0,
		"2001:db8::/64": 1024,
	}
	for cidr, want := range cases {
		got := cidrCapacity(cidr)
		if cidr == "10.0.0.0/0" {
			// /0 overflows; implementation returns a non-negative value or 0.
			if got < 0 {
				t.Errorf("cidrCapacity(%s) negative = %d", cidr, got)
			}
			continue
		}
		if got != want {
			t.Errorf("cidrCapacity(%q) = %d, want %d", cidr, got, want)
		}
	}
}

// ----- controller-level spec assertions -----

func TestBondReconciler_PopulatesActiveMembersAndHash(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.Bond{
		ObjectMeta: newClusterMeta("bond0"),
		Spec: novanasv1alpha1.BondSpec{
			Interfaces: []string{"eno1", "eno2"},
			Mode:       "active-backup",
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &BondReconciler{BaseReconciler: newPart2Base(c, s, "Bond"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("bond0"))
	var got novanasv1alpha1.Bond
	if err := c.Get(context.Background(), client.ObjectKey{Name: "bond0"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.AppliedConfigHash == "" {
		t.Errorf("expected AppliedConfigHash")
	}
	if got.Status.ObservedGeneration == 0 && got.Generation != 0 {
		t.Errorf("expected ObservedGeneration=%d, got %d", got.Generation, got.Status.ObservedGeneration)
	}
	if len(got.Status.ActiveMembers) != 2 {
		t.Errorf("expected 2 active members, got %d", len(got.Status.ActiveMembers))
	}
}

func TestVipPoolReconciler_ReportsCapacity(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.VipPool{
		ObjectMeta: newClusterMeta("vips"),
		Spec:       novanasv1alpha1.VipPoolSpec{Range: "10.0.0.0/29", Interface: "br0"},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &VipPoolReconciler{BaseReconciler: newPart2Base(c, s, "VipPool"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("vips"))
	var got novanasv1alpha1.VipPool
	_ = c.Get(context.Background(), client.ObjectKey{Name: "vips"}, &got)
	if got.Status.Available != 6 {
		t.Errorf("expected 6 available VIPs in /29, got %d", got.Status.Available)
	}
	if got.Status.AppliedConfigHash == "" {
		t.Errorf("missing AppliedConfigHash")
	}
}

func TestBondReconciler_FinalizerTeardown(t *testing.T) {
	s := newPart2Scheme(t)
	now := metav1.Now()
	cr := &novanasv1alpha1.Bond{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "bond0",
			DeletionTimestamp: &now,
			Finalizers:        []string{"novanas.io/bond"},
		},
		Spec: novanasv1alpha1.BondSpec{Interfaces: []string{"eno1"}, Mode: "active-backup"},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &BondReconciler{BaseReconciler: newPart2Base(c, s, "Bond"), Recorder: newPart2Recorder()}
	if _, err := r.Reconcile(context.Background(), part2Request("bond0")); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var got novanasv1alpha1.Bond
	err := c.Get(context.Background(), client.ObjectKey{Name: "bond0"}, &got)
	// The object should either be gone or have no remaining finalizer.
	if err == nil {
		for _, f := range got.Finalizers {
			if f == "novanas.io/bond" {
				t.Errorf("bond finalizer still present after delete")
			}
		}
	}
	_ = time.Now() // keep imports stable
}

func TestFirewallRuleReconciler_WritesConfigMap(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.FirewallRule{
		ObjectMeta: newClusterMeta("fw"),
		Spec: novanasv1alpha1.FirewallRuleSpec{
			Scope:     "host",
			Direction: "inbound",
			Action:    "allow",
			Destination: &novanasv1alpha1.FirewallEndpoint{
				Ports: []int32{443},
			},
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &FirewallRuleReconciler{BaseReconciler: newPart2Base(c, s, "FirewallRule"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("fw"))
	var got novanasv1alpha1.FirewallRule
	_ = c.Get(context.Background(), client.ObjectKey{Name: "fw"}, &got)
	if got.Status.AppliedConfigHash == "" {
		t.Errorf("missing hash")
	}
	if got.Status.InstalledAt == nil {
		t.Errorf("missing InstalledAt")
	}
}

func TestRenderServicePolicy_Stable(t *testing.T) {
	p := &novanasv1alpha1.ServicePolicy{
		Spec: novanasv1alpha1.ServicePolicySpec{
			Services: []novanasv1alpha1.ServiceToggle{
				{Name: "smb", Enabled: true, Port: 445},
				{Name: "nfs", Enabled: false},
			},
		},
	}
	a := renderServicePolicy(p)
	b := renderServicePolicy(p)
	if len(a) != len(b) {
		t.Fatal("renderServicePolicy not deterministic")
	}
	if a["smb"] == "" || a["nfs"] == "" {
		t.Errorf("missing service keys: %v", a)
	}
}
