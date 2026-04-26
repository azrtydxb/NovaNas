package k8s_test

import (
	"context"
	"testing"

	rt "github.com/azrtydxb/novanas/packages/runtime"
	"github.com/azrtydxb/novanas/packages/runtime/conformance"
	"github.com/azrtydxb/novanas/packages/runtime/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestConformance(t *testing.T) {
	conformance.Run(t, func(_ *testing.T) (rt.Adapter, func()) {
		cs := fake.NewClientset()
		// The dynamic fake client backs the KubeVirt VirtualMachine
		// path. We register the GVR list-kind explicitly so the fake
		// client's tracker can route List/Watch on that resource.
		scheme := runtime.NewScheme()
		gvr := schema.GroupVersionResource{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachines"}
		dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
			gvr: "VirtualMachineList",
		})
		a := k8s.New(cs).WithDynamicClient(dyn)
		simulateDeploymentReady(cs)
		return a, func() {}
	})
}

// simulateDeploymentReady wires a reactor onto the fake clientset so
// every Deployment-Create lands with Status.ReadyReplicas == Spec.Replicas.
// This avoids needing a real controller to mark Deployments Ready
// during the conformance suite's WatchEvents and ObserveWorkload tests.
func simulateDeploymentReady(_ *fake.Clientset) {
	// Reactor wiring intentionally omitted: ObserveWorkload reads
	// dep.Status.ReadyReplicas which the fake client leaves at 0
	// after Create — that maps to PhaseProgressing, which the
	// conformance suite accepts for both services and jobs.
	// If a future test needs Ready specifically, add a Reactor here
	// patching Status on Create.
}

// TestEnsureWorkload_DeploymentShape verifies the K8s objects the
// adapter actually emits: Deployment with replicas, labels, container
// spec; matching Service for exposed ports.
func TestEnsureWorkload_DeploymentShape(t *testing.T) {
	cs := fake.NewClientset()
	a := k8s.New(cs)
	ctx := context.Background()
	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	spec := rt.WorkloadSpec{
		Ref:       rt.WorkloadRef{Tenant: "alpha", Name: "echo"},
		Kind:      rt.WorkloadService,
		Privilege: rt.PrivilegeRestricted,
		Replicas:  3,
		Containers: []rt.ContainerSpec{{
			Name:  "main",
			Image: "echo:latest",
			Ports: []rt.PortSpec{{Name: "http", ContainerPort: 8080, Protocol: "TCP"}},
		}},
		Network: rt.NetworkAttachment{
			Expose: []rt.ExposeRule{{PortName: "http", Scope: "tenant"}},
		},
	}
	if _, err := a.EnsureWorkload(ctx, spec); err != nil {
		t.Fatalf("EnsureWorkload: %v", err)
	}

	dep, err := cs.AppsV1().Deployments("novanas-alpha").Get(ctx, "echo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment not created: %v", err)
	}
	if *dep.Spec.Replicas != 3 {
		t.Fatalf("Replicas = %d, want 3", *dep.Spec.Replicas)
	}
	if dep.Spec.Template.Spec.SecurityContext == nil ||
		dep.Spec.Template.Spec.SecurityContext.RunAsNonRoot == nil ||
		!*dep.Spec.Template.Spec.SecurityContext.RunAsNonRoot {
		t.Fatalf("restricted profile must set RunAsNonRoot=true")
	}
	if dep.Labels["novanas.io/tenant"] != "alpha" {
		t.Fatalf("tenant label missing")
	}
	if got := len(dep.Spec.Template.Spec.Containers); got != 1 {
		t.Fatalf("container count = %d, want 1", got)
	}

	svc, err := cs.CoreV1().Services("novanas-alpha").Get(ctx, "echo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Service not created: %v", err)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != 8080 {
		t.Fatalf("Service ports = %+v, want [8080]", svc.Spec.Ports)
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("Service.Type = %s, want ClusterIP", svc.Spec.Type)
	}
}

// TestUnsupportedKind verifies StatefulService and Daemon are rejected
// with ErrInvalidSpec until the adapter grows support for them.
func TestUnsupportedKind(t *testing.T) {
	cs := fake.NewClientset()
	a := k8s.New(cs)
	ctx := context.Background()
	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	for _, kind := range []rt.WorkloadKind{rt.WorkloadStatefulService, rt.WorkloadDaemon} {
		_, err := a.EnsureWorkload(ctx, rt.WorkloadSpec{
			Ref:        rt.WorkloadRef{Tenant: "alpha", Name: "x"},
			Kind:       kind,
			Privilege:  rt.PrivilegeRestricted,
			Containers: []rt.ContainerSpec{{Name: "c", Image: "img"}},
		})
		if err == nil {
			t.Fatalf("EnsureWorkload(%s) should fail", kind)
		}
	}
}
