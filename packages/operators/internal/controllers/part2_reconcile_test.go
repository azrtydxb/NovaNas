package controllers

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
)

// The tests in this file exercise the happy path of each A7-Operators-Part2
// controller: create CR, reconcile twice (finalizer + work), assert status
// reaches Ready / Observed / Pending. We use the fake controller-runtime
// client, the Noop* interface defaults, and a deterministic fake event
// recorder.

func TestBondReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.Bond{ObjectMeta: newClusterMeta("bond0")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &BondReconciler{BaseReconciler: newPart2Base(c, s, "Bond"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("bond0"))
	var got novanasv1alpha1.Bond
	if err := c.Get(context.Background(), client.ObjectKey{Name: "bond0"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q, want Ready", got.Status.Phase)
	}
}

func TestVlanReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.Vlan{ObjectMeta: newClusterMeta("vlan100")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &VlanReconciler{BaseReconciler: newPart2Base(c, s, "Vlan"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("vlan100"))
	var got novanasv1alpha1.Vlan
	_ = c.Get(context.Background(), client.ObjectKey{Name: "vlan100"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestPhysicalInterfaceReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.PhysicalInterface{ObjectMeta: newClusterMeta("eth0")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &PhysicalInterfaceReconciler{BaseReconciler: newPart2Base(c, s, "PhysicalInterface"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("eth0"))
	var got novanasv1alpha1.PhysicalInterface
	_ = c.Get(context.Background(), client.ObjectKey{Name: "eth0"}, &got)
	if got.Status.Phase != "Observed" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestHostInterfaceReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.HostInterface{ObjectMeta: newClusterMeta("hif0")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &HostInterfaceReconciler{BaseReconciler: newPart2Base(c, s, "HostInterface"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("hif0"))
	var got novanasv1alpha1.HostInterface
	_ = c.Get(context.Background(), client.ObjectKey{Name: "hif0"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestClusterNetworkReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.ClusterNetwork{ObjectMeta: newClusterMeta("primary")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ClusterNetworkReconciler{BaseReconciler: newPart2Base(c, s, "ClusterNetwork"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("primary"))
	var got novanasv1alpha1.ClusterNetwork
	_ = c.Get(context.Background(), client.ObjectKey{Name: "primary"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestVipPoolReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.VipPool{ObjectMeta: newClusterMeta("vips")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &VipPoolReconciler{BaseReconciler: newPart2Base(c, s, "VipPool"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("vips"))
	var got novanasv1alpha1.VipPool
	_ = c.Get(context.Background(), client.ObjectKey{Name: "vips"}, &got)
	if got.Status.Phase != "Ready" && got.Status.Phase != "Pending" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestIngressReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.Ingress{ObjectMeta: newNsMeta("default", "ing1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &IngressReconciler{BaseReconciler: newPart2Base(c, s, "Ingress"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2NsRequest("default", "ing1"))
	var got novanasv1alpha1.Ingress
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "ing1"}, &got)
	if got.Status.Phase != "Ready" && got.Status.Phase != "Pending" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestRemoteAccessTunnelReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.RemoteAccessTunnel{ObjectMeta: newClusterMeta("tunnel")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &RemoteAccessTunnelReconciler{BaseReconciler: newPart2Base(c, s, "RemoteAccessTunnel"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("tunnel"))
	var got novanasv1alpha1.RemoteAccessTunnel
	_ = c.Get(context.Background(), client.ObjectKey{Name: "tunnel"}, &got)
	if got.Status.Phase != "Ready" && got.Status.Phase != "Pending" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestCustomDomainReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.CustomDomain{ObjectMeta: newClusterMeta("example")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &CustomDomainReconciler{BaseReconciler: newPart2Base(c, s, "CustomDomain"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("example"))
	var got novanasv1alpha1.CustomDomain
	_ = c.Get(context.Background(), client.ObjectKey{Name: "example"}, &got)
	if got.Status.Phase != "Ready" && got.Status.Phase != "Pending" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestFirewallRuleReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.FirewallRule{ObjectMeta: newClusterMeta("fw")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &FirewallRuleReconciler{BaseReconciler: newPart2Base(c, s, "FirewallRule"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("fw"))
	var got novanasv1alpha1.FirewallRule
	_ = c.Get(context.Background(), client.ObjectKey{Name: "fw"}, &got)
	if got.Status.Phase != "Ready" && got.Status.Phase != "Pending" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestTrafficPolicyReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.TrafficPolicy{ObjectMeta: newClusterMeta("tp")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &TrafficPolicyReconciler{BaseReconciler: newPart2Base(c, s, "TrafficPolicy"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("tp"))
	var got novanasv1alpha1.TrafficPolicy
	_ = c.Get(context.Background(), client.ObjectKey{Name: "tp"}, &got)
	if got.Status.Phase != "Ready" && got.Status.Phase != "Pending" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestAppCatalogReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.AppCatalog{ObjectMeta: newClusterMeta("core")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &AppCatalogReconciler{BaseReconciler: newPart2Base(c, s, "AppCatalog"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("core"))
	var got novanasv1alpha1.AppCatalog
	_ = c.Get(context.Background(), client.ObjectKey{Name: "core"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestAppReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.App{ObjectMeta: newClusterMeta("hello")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &AppReconciler{BaseReconciler: newPart2Base(c, s, "App"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("hello"))
	var got novanasv1alpha1.App
	_ = c.Get(context.Background(), client.ObjectKey{Name: "hello"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestAppInstanceReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.AppInstance{ObjectMeta: newNsMeta("default", "myapp")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &AppInstanceReconciler{BaseReconciler: newPart2Base(c, s, "AppInstance"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2NsRequest("default", "myapp"))
	var got novanasv1alpha1.AppInstance
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "myapp"}, &got)
	if got.Status.Phase != "Pending" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestVmReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.Vm{ObjectMeta: newNsMeta("default", "vm1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &VmReconciler{BaseReconciler: newPart2Base(c, s, "Vm"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2NsRequest("default", "vm1"))
	var got novanasv1alpha1.Vm
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "vm1"}, &got)
	if got.Status.Phase != "Pending" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestIsoLibraryReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.IsoLibrary{ObjectMeta: newClusterMeta("isos")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &IsoLibraryReconciler{BaseReconciler: newPart2Base(c, s, "IsoLibrary"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("isos"))
	var got novanasv1alpha1.IsoLibrary
	_ = c.Get(context.Background(), client.ObjectKey{Name: "isos"}, &got)
	if got.Status.Phase != "Pending" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestGpuDeviceReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.GpuDevice{ObjectMeta: newClusterMeta("gpu0")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &GpuDeviceReconciler{BaseReconciler: newPart2Base(c, s, "GpuDevice"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("gpu0"))
	var got novanasv1alpha1.GpuDevice
	_ = c.Get(context.Background(), client.ObjectKey{Name: "gpu0"}, &got)
	if got.Status.Phase != "Observed" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestSmartPolicyReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.SmartPolicy{ObjectMeta: newClusterMeta("smart")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &SmartPolicyReconciler{BaseReconciler: newPart2Base(c, s, "SmartPolicy"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("smart"))
	var got novanasv1alpha1.SmartPolicy
	_ = c.Get(context.Background(), client.ObjectKey{Name: "smart"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestAlertChannelReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.AlertChannel{
		ObjectMeta: newClusterMeta("email"),
		Spec: novanasv1alpha1.AlertChannelSpec{
			Type: "email",
			Email: &novanasv1alpha1.EmailChannelConfig{
				To:   []string{"ops@example.com"},
				From: "novanas@example.com",
			},
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &AlertChannelReconciler{BaseReconciler: newPart2Base(c, s, "AlertChannel"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("email"))
	var got novanasv1alpha1.AlertChannel
	_ = c.Get(context.Background(), client.ObjectKey{Name: "email"}, &got)
	if got.Status.Phase != "Active" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestAlertPolicyReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.AlertPolicy{
		ObjectMeta: newClusterMeta("disk-full"),
		Spec: novanasv1alpha1.AlertPolicySpec{
			Severity: "warning",
			Condition: novanasv1alpha1.AlertCondition{
				Query: "disk_usage_ratio", Operator: ">", Threshold: "0.9", For: "5m",
			},
			Channels: []string{"ops-email"},
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &AlertPolicyReconciler{BaseReconciler: newPart2Base(c, s, "AlertPolicy"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("disk-full"))
	var got novanasv1alpha1.AlertPolicy
	_ = c.Get(context.Background(), client.ObjectKey{Name: "disk-full"}, &got)
	if got.Status.Phase != "Active" && got.Status.Phase != "Pending" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestAuditPolicyReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.AuditPolicy{ObjectMeta: newClusterMeta("default-audit")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &AuditPolicyReconciler{BaseReconciler: newPart2Base(c, s, "AuditPolicy"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("default-audit"))
	var got novanasv1alpha1.AuditPolicy
	_ = c.Get(context.Background(), client.ObjectKey{Name: "default-audit"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestUpsPolicyReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.UpsPolicy{ObjectMeta: newClusterMeta("ups")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &UpsPolicyReconciler{BaseReconciler: newPart2Base(c, s, "UpsPolicy"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("ups"))
	var got novanasv1alpha1.UpsPolicy
	_ = c.Get(context.Background(), client.ObjectKey{Name: "ups"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestServiceLevelObjectiveReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.ServiceLevelObjective{
		ObjectMeta: newClusterMeta("api-slo"),
		Spec: novanasv1alpha1.ServiceLevelObjectiveSpec{
			Target: "99.9",
			Window: "30d",
			Indicator: novanasv1alpha1.SLOIndicator{
				GoodQuery:  "sum(rate(http_requests_total{status!~\"5..\"}[5m]))",
				TotalQuery: "sum(rate(http_requests_total[5m]))",
			},
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ServiceLevelObjectiveReconciler{
		BaseReconciler: newPart2Base(c, s, "ServiceLevelObjective"),
		Recorder:       newPart2Recorder(),
		Prom:           stubPromClient{good: 995, total: 1000},
	}
	mustReconcileOK(t, context.Background(), r, part2Request("api-slo"))
	var got novanasv1alpha1.ServiceLevelObjective
	_ = c.Get(context.Background(), client.ObjectKey{Name: "api-slo"}, &got)
	if got.Status.Phase != "Active" && got.Status.Phase != "Pending" && got.Status.Phase != "AtRisk" && got.Status.Phase != "Breached" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestConfigBackupPolicyReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.ConfigBackupPolicy{ObjectMeta: newClusterMeta("cfg")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ConfigBackupPolicyReconciler{BaseReconciler: newPart2Base(c, s, "ConfigBackupPolicy"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("cfg"))
	var got novanasv1alpha1.ConfigBackupPolicy
	_ = c.Get(context.Background(), client.ObjectKey{Name: "cfg"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestSystemSettingsReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.SystemSettings{ObjectMeta: newClusterMeta("defaults")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &SystemSettingsReconciler{BaseReconciler: newPart2Base(c, s, "SystemSettings"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("defaults"))
	var got novanasv1alpha1.SystemSettings
	_ = c.Get(context.Background(), client.ObjectKey{Name: "defaults"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestUpdatePolicyReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.UpdatePolicy{ObjectMeta: newClusterMeta("stable")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &UpdatePolicyReconciler{BaseReconciler: newPart2Base(c, s, "UpdatePolicy"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("stable"))
	var got novanasv1alpha1.UpdatePolicy
	_ = c.Get(context.Background(), client.ObjectKey{Name: "stable"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestServicePolicyReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.ServicePolicy{ObjectMeta: newClusterMeta("smb")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ServicePolicyReconciler{BaseReconciler: newPart2Base(c, s, "ServicePolicy"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("smb"))
	var got novanasv1alpha1.ServicePolicy
	_ = c.Get(context.Background(), client.ObjectKey{Name: "smb"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}
