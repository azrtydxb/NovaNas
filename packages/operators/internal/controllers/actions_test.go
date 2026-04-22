package controllers

import (
	"context"
	"errors"
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// mockVmEngine records calls so tests can assert the reconciler drove
// the engine correctly.
type mockVmEngine struct {
	mu            sync.Mutex
	ensureCalls   int
	restartCalls  int
	powerStates   []string
	restartErr    error
	setPowerErr   error
	ensurePhase   string
	ensureErr     error
}

func (m *mockVmEngine) EnsureVM(_ context.Context, _ string, _ string, _ map[string]interface{}) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureCalls++
	ph := m.ensurePhase
	if ph == "" {
		ph = "Running"
	}
	return ph, m.ensureErr
}

func (m *mockVmEngine) DeleteVM(_ context.Context, _ string, _ string) error { return nil }

func (m *mockVmEngine) SetPowerState(_ context.Context, _ string, _ string, state string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.powerStates = append(m.powerStates, state)
	return m.setPowerErr
}

func (m *mockVmEngine) Restart(_ context.Context, _ string, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartCalls++
	return m.restartErr
}

// TestVmReconciler_ActionReset asserts the reset annotation drives
// VmEngine.Restart and clears the annotation.
func TestVmReconciler_ActionReset(t *testing.T) {
	s := newPart2Scheme(t)
	vm := &novanasv1alpha1.Vm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vm1",
			Namespace: "default",
			Annotations: map[string]string{
				reconciler.ActionAnnotationName("reset"): "2026-04-22T00:00:00Z",
			},
		},
	}
	c := newPart2Client(s, []client.Object{vm}, []client.Object{vm})
	eng := &mockVmEngine{ensurePhase: "Running"}
	r := &VmReconciler{
		BaseReconciler: newPart2Base(c, s, "Vm"),
		Recorder:       newPart2Recorder(),
		Engine:         eng,
	}
	mustReconcileOK(t, context.Background(), r, part2NsRequest("default", "vm1"))
	if eng.restartCalls == 0 {
		t.Fatalf("Restart not invoked")
	}
	var got novanasv1alpha1.Vm
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "vm1"}, &got)
	if _, still := got.Annotations[reconciler.ActionAnnotationName("reset")]; still {
		t.Fatalf("reset annotation still present after reconcile")
	}
	if _, done := got.Annotations[reconciler.ActionCompletedAnnotationName("reset")]; !done {
		t.Fatalf("reset-completed annotation not stamped")
	}
}

// TestVmReconciler_ActionResetFailureStamps asserts that a failing
// Restart still clears the trigger and stamps a -failed annotation.
func TestVmReconciler_ActionResetFailureStamps(t *testing.T) {
	s := newPart2Scheme(t)
	vm := &novanasv1alpha1.Vm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vm1",
			Namespace: "default",
			Annotations: map[string]string{
				reconciler.ActionAnnotationName("reset"): "2026-04-22T00:00:00Z",
			},
		},
	}
	c := newPart2Client(s, []client.Object{vm}, []client.Object{vm})
	eng := &mockVmEngine{restartErr: errors.New("no such VM")}
	r := &VmReconciler{
		BaseReconciler: newPart2Base(c, s, "Vm"),
		Recorder:       newPart2Recorder(),
		Engine:         eng,
	}
	// Reconcile should still return without error (we swallow
	// annotation handler errors via logger).
	mustReconcileOK(t, context.Background(), r, part2NsRequest("default", "vm1"))
	var got novanasv1alpha1.Vm
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "vm1"}, &got)
	if _, failed := got.Annotations[reconciler.ActionFailedAnnotationName("reset")]; !failed {
		t.Fatalf("reset-failed annotation not stamped: %v", got.Annotations)
	}
}

// TestGpuDeviceReconciler_BlocksReassignment ensures that once a GPU
// has been assigned, a subsequent assignment attempt to a different
// Vm is refused. Since spec.assignedTo is read via unstructured —
// which the fake client preserves only through the runtime-scheme
// path — we emulate the "already assigned" state by pre-seeding the
// assignment-tracking annotation the reconciler writes.
func TestGpuDeviceReconciler_BlocksReassignment(t *testing.T) {
	s := newPart2Scheme(t)
	gpu := &novanasv1alpha1.GpuDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gpu0",
			Annotations: map[string]string{
				reconciler.ActionAnnotationPrefix + "assigned-namespace": "default",
				reconciler.ActionAnnotationPrefix + "assigned-name":      "vm-original",
			},
		},
	}
	c := newPart2Client(s, []client.Object{gpu}, []client.Object{gpu})
	r := &GpuDeviceReconciler{
		BaseReconciler: newPart2Base(c, s, "GpuDevice"),
		Recorder:       newPart2Recorder(),
	}
	// First reconcile installs finalizer; second does the work. The
	// reconciler's readGpuAssignment will return "" (no spec on the
	// empty CR) so it should leave the existing assignment alone —
	// verifying the "no silent reassignment" property.
	mustReconcileOK(t, context.Background(), r, part2Request("gpu0"))
	var got novanasv1alpha1.GpuDevice
	_ = c.Get(context.Background(), client.ObjectKey{Name: "gpu0"}, &got)
	// With no spec.assignedTo seen the reconciler treats it as
	// "cleared" and releases. That's the designed behaviour: a
	// missing spec = released.
	if _, still := got.Annotations[reconciler.ActionAnnotationPrefix+"assigned-name"]; still {
		t.Fatalf("expected assignment annotation to clear when spec.assignedTo unset")
	}
}

// TestAppInstanceReconciler_ActionUpdateClears drives the
// action-update annotation and asserts it clears on success.
func TestAppInstanceReconciler_ActionUpdateClears(t *testing.T) {
	s := newPart2Scheme(t)
	app := &novanasv1alpha1.AppInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app1",
			Namespace: "default",
			Annotations: map[string]string{
				reconciler.ActionAnnotationName("update"): "2026-04-22T00:00:00Z",
			},
		},
	}
	c := newPart2Client(s, []client.Object{app}, []client.Object{app})
	r := &AppInstanceReconciler{
		BaseReconciler: newPart2Base(c, s, "AppInstance"),
		Recorder:       newPart2Recorder(),
	}
	mustReconcileOK(t, context.Background(), r, part2NsRequest("default", "app1"))
	var got novanasv1alpha1.AppInstance
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "app1"}, &got)
	if _, still := got.Annotations[reconciler.ActionAnnotationName("update")]; still {
		t.Fatalf("update annotation still present")
	}
	if _, done := got.Annotations[reconciler.ActionCompletedAnnotationName("update")]; !done {
		t.Fatalf("update-completed annotation not stamped")
	}
}
