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

