package k8s_test

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"testing"

	rt "github.com/azrtydxb/novanas/packages/runtime"
	"github.com/azrtydxb/novanas/packages/runtime/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// TestLogs_NoMatchingPod verifies that Logs returns ErrNotFound when
// no pod carries the workload label — the same property exercised by
// the conformance suite, but exercised here against the K8s adapter
// specifically since the fake clientset doesn't auto-spawn Pods from
// Deployments.
func TestLogs_NoMatchingPod(t *testing.T) {
	cs := fake.NewClientset()
	a := k8s.New(cs)
	ctx := context.Background()
	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	var buf bytes.Buffer
	err := a.Logs(ctx, rt.LogOptions{Ref: rt.WorkloadRef{Tenant: "alpha", Name: "ghost"}}, &buf)
	if !errors.Is(err, rt.ErrNotFound) {
		t.Fatalf("Logs(missing pod) = %v, want ErrNotFound", err)
	}
}

// TestLogs_StreamsFromPod creates a labeled Pod by hand (since the
// fake clientset doesn't synthesize Pods from Deployments), then
// asserts Logs streams without error. The fake's GetLogs returns an
// empty stream — that's fine for this test; we only care that the
// pod-resolution + stream wiring runs cleanly.
func TestLogs_StreamsFromPod(t *testing.T) {
	cs := fake.NewClientset()
	a := k8s.New(cs)
	ctx := context.Background()
	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo-abc",
			Namespace: "novanas-alpha",
			Labels: map[string]string{
				"novanas.io/tenant":   "alpha",
				"novanas.io/workload": "echo",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main", Image: "echo:latest"}},
		},
	}
	if _, err := cs.CoreV1().Pods("novanas-alpha").Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed pod: %v", err)
	}
	var buf bytes.Buffer
	err := a.Logs(ctx, rt.LogOptions{Ref: rt.WorkloadRef{Tenant: "alpha", Name: "echo"}}, &buf)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
}

// TestExec_NoConfig verifies Exec rejects with a clear error when no
// rest.Config is wired (the conformance suite path).
func TestExec_NoConfig(t *testing.T) {
	cs := fake.NewClientset()
	a := k8s.New(cs)
	ctx := context.Background()
	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	_, err := a.Exec(ctx, rt.ExecRequest{Ref: rt.WorkloadRef{Tenant: "alpha", Name: "echo"}, Command: []string{"true"}}, nil, nil)
	if err == nil {
		t.Fatal("Exec without rest.Config should error")
	}
}

// TestExec_NoMatchingPod confirms Exec maps a missing pod to
// ErrNotFound rather than a transport-level error.
func TestExec_NoMatchingPod(t *testing.T) {
	cs := fake.NewClientset()
	a := k8s.New(cs).WithRestConfig(&rest.Config{Host: "https://example.invalid"})
	ctx := context.Background()
	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	_, err := a.Exec(ctx, rt.ExecRequest{Ref: rt.WorkloadRef{Tenant: "alpha", Name: "ghost"}, Command: []string{"true"}}, nil, nil)
	if !errors.Is(err, rt.ErrNotFound) {
		t.Fatalf("Exec(missing pod) = %v, want ErrNotFound", err)
	}
}

// TestExec_StreamsViaFakeExecutor exercises the full Exec wiring with
// a fake SPDY executor so we cover the URL build, executor factory
// call, and StreamWithContext error mapping without needing a real
// kube-apiserver.
func TestExec_StreamsViaFakeExecutor(t *testing.T) {
	cs := fake.NewClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo-abc",
			Namespace: "novanas-alpha",
			Labels:    map[string]string{"novanas.io/workload": "echo"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
	}
	ctx := context.Background()
	a := k8s.New(cs).WithRestConfig(&rest.Config{Host: "https://example.invalid"}).
		WithExecutor(func(_ *rest.Config, _ string, _ *url.URL) (remotecommand.Executor, error) {
			return fakeExecutor{stdout: []byte("hello\n")}, nil
		})
	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	if _, err := cs.CoreV1().Pods("novanas-alpha").Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed pod: %v", err)
	}
	var stdout bytes.Buffer
	code, err := a.Exec(ctx, rt.ExecRequest{
		Ref:     rt.WorkloadRef{Tenant: "alpha", Name: "echo"},
		Command: []string{"echo", "hello"},
	}, &stdout, nil)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if code != 0 {
		t.Fatalf("Exec exit code = %d, want 0", code)
	}
	if got := stdout.String(); got != "hello\n" {
		t.Fatalf("stdout = %q, want %q", got, "hello\n")
	}
}

type fakeExecutor struct {
	stdout []byte
}

func (f fakeExecutor) Stream(_ remotecommand.StreamOptions) error { return nil }

func (f fakeExecutor) StreamWithContext(_ context.Context, opts remotecommand.StreamOptions) error {
	if opts.Stdout != nil && len(f.stdout) > 0 {
		_, err := opts.Stdout.Write(f.stdout)
		if err != nil {
			return err
		}
	}
	return nil
}
