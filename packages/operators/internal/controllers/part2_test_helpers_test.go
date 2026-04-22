package controllers

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// newPart2Scheme builds a scheme that knows about NovaNas CRDs plus the core
// Kubernetes types the A7-Operators-Part2 controllers create (ConfigMap,
// DaemonSet, Deployment, CronJob).
func newPart2Scheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("core scheme: %v", err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("corev1 scheme: %v", err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatalf("appsv1 scheme: %v", err)
	}
	if err := batchv1.AddToScheme(s); err != nil {
		t.Fatalf("batchv1 scheme: %v", err)
	}
	if err := novanasv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("novanas scheme: %v", err)
	}
	return s
}

// newPart2Client builds a fake client with the NovaNas status subresource
// enabled for the given object kinds. The caller passes the status-objs to
// be tracked via WithStatusSubresource so Status().Update works.
func newPart2Client(s *runtime.Scheme, init []client.Object, withStatus []client.Object) client.Client {
	b := fake.NewClientBuilder().WithScheme(s).WithObjects(init...)
	if len(withStatus) > 0 {
		b = b.WithStatusSubresource(withStatus...)
	}
	return b.Build()
}

// newPart2Base returns a BaseReconciler wired to the given client + scheme.
func newPart2Base(c client.Client, s *runtime.Scheme, name string) reconciler.BaseReconciler {
	return reconciler.BaseReconciler{Client: c, Scheme: s, ControllerName: name}
}

// newPart2Recorder returns a fake event recorder suitable for tests.
func newPart2Recorder() record.EventRecorder {
	return record.NewFakeRecorder(32)
}

// part2Request builds a Reconcile request for the named cluster-scoped CR.
func part2Request(name string) ctrl.Request {
	return ctrl.Request{NamespacedName: client.ObjectKey{Name: name}}
}

// part2NsRequest builds a Reconcile request for a namespaced CR.
func part2NsRequest(ns, name string) ctrl.Request {
	return ctrl.Request{NamespacedName: client.ObjectKey{Namespace: ns, Name: name}}
}

// mustReconcileOK runs reconcile twice (once to install finalizer, once to do
// the work) and fails the test on either error.
func mustReconcileOK(t *testing.T, ctx context.Context, rec interface {
	Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
}, req ctrl.Request) {
	t.Helper()
	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
}

// newClusterMeta returns an ObjectMeta for a cluster-scoped CR.
func newClusterMeta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name}
}

// newNsMeta returns an ObjectMeta for a namespaced CR.
func newNsMeta(ns, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Namespace: ns, Name: name}
}
