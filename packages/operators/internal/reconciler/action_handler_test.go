package reconciler_test

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

func newClient(t *testing.T, objs ...client.Object) (client.Client, *runtime.Scheme) {
	t.Helper()
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
	return c, s
}

func TestHandleActionAnnotation_ClearsOnSuccess(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      "target",
		Namespace: "default",
		Annotations: map[string]string{
			reconciler.ActionAnnotationName("run-now"): "2026-04-22T00:00:00Z",
		},
	}}
	c, _ := newClient(t, cm)

	called := false
	handled, err := reconciler.HandleActionAnnotation(context.Background(), c, cm, "run-now",
		func(_ context.Context, _ client.Object) error {
			called = true
			return nil
		})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !handled {
		t.Fatalf("handled=false; want true")
	}
	if !called {
		t.Fatalf("handler not invoked")
	}
	var got corev1.ConfigMap
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "target"}, &got)
	if _, still := got.Annotations[reconciler.ActionAnnotationName("run-now")]; still {
		t.Fatalf("trigger annotation not cleared: %v", got.Annotations)
	}
	if _, done := got.Annotations[reconciler.ActionCompletedAnnotationName("run-now")]; !done {
		t.Fatalf("completed annotation not set: %v", got.Annotations)
	}
}

func TestHandleActionAnnotation_StampsFailureOnError(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      "target",
		Namespace: "default",
		Annotations: map[string]string{
			reconciler.ActionAnnotationName("renew"): "2026-04-22T00:00:00Z",
		},
	}}
	c, _ := newClient(t, cm)

	handled, err := reconciler.HandleActionAnnotation(context.Background(), c, cm, "renew",
		func(_ context.Context, _ client.Object) error {
			return errors.New("boom")
		})
	if !handled {
		t.Fatalf("handled=false; want true")
	}
	if err == nil {
		t.Fatalf("expected error surface")
	}
	var got corev1.ConfigMap
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "target"}, &got)
	if _, still := got.Annotations[reconciler.ActionAnnotationName("renew")]; still {
		t.Fatalf("trigger annotation not cleared on failure")
	}
	if _, failed := got.Annotations[reconciler.ActionFailedAnnotationName("renew")]; !failed {
		t.Fatalf("failed annotation not set: %v", got.Annotations)
	}
}

func TestHandleActionAnnotation_NoopWhenAbsent(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "default"}}
	c, _ := newClient(t, cm)
	handled, err := reconciler.HandleActionAnnotation(context.Background(), c, cm, "reset",
		func(_ context.Context, _ client.Object) error { return errors.New("should not run") })
	if handled || err != nil {
		t.Fatalf("expected (false, nil), got (%v, %v)", handled, err)
	}
}
