// Package conformance is the runtime.Adapter test suite. Every Adapter
// implementation must pass it via:
//
//	conformance.Run(t, func(t *testing.T) (runtime.Adapter, func()) {
//	    return myadapter.New(...), func() { /* cleanup */ }
//	})
package conformance

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	rt "github.com/azrtydxb/novanas/packages/runtime"
)

type Factory func(t *testing.T) (rt.Adapter, func())

func Run(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("Name", func(t *testing.T) {
		a, done := factory(t)
		defer done()
		if a.Name() == "" {
			t.Fatal("Name() must be non-empty")
		}
	})

	t.Run("TenantLifecycle", func(t *testing.T) { testTenantLifecycle(t, factory) })
	t.Run("NetworkLifecycle", func(t *testing.T) { testNetworkLifecycle(t, factory) })
	t.Run("WorkloadLifecycle", func(t *testing.T) { testWorkloadLifecycle(t, factory) })
	t.Run("WorkloadValidation", func(t *testing.T) { testWorkloadValidation(t, factory) })
	t.Run("WorkloadIdempotent", func(t *testing.T) { testWorkloadIdempotent(t, factory) })
	t.Run("JobCompletes", func(t *testing.T) { testJobCompletes(t, factory) })
	t.Run("WatchEvents", func(t *testing.T) { testWatchEvents(t, factory) })
	t.Run("LogsRequiresWorkload", func(t *testing.T) { testLogsRequiresWorkload(t, factory) })
}

func testTenantLifecycle(t *testing.T, factory Factory) {
	a, done := factory(t)
	defer done()
	ctx := context.Background()

	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant idempotent: %v", err)
	}
	if err := a.EnsureTenant(ctx, ""); !errors.Is(err, rt.ErrInvalidSpec) {
		t.Fatalf("EnsureTenant(\"\") = %v, want ErrInvalidSpec", err)
	}
	if err := a.DeleteTenant(ctx, "alpha"); err != nil {
		t.Fatalf("DeleteTenant: %v", err)
	}
	if err := a.DeleteTenant(ctx, "ghost"); err != nil {
		t.Fatalf("DeleteTenant absent: %v", err)
	}
}

func testNetworkLifecycle(t *testing.T, factory Factory) {
	a, done := factory(t)
	defer done()
	ctx := context.Background()

	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	if err := a.EnsureNetwork(ctx, rt.NetworkSpec{Tenant: "alpha", Name: "data"}); err != nil {
		t.Fatalf("EnsureNetwork: %v", err)
	}
	err := a.EnsureNetwork(ctx, rt.NetworkSpec{Tenant: "ghost", Name: "x"})
	if !errors.Is(err, rt.ErrNotFound) {
		t.Fatalf("EnsureNetwork(unknown tenant) = %v, want ErrNotFound", err)
	}
	if err := a.DeleteNetwork(ctx, "alpha", "data"); err != nil {
		t.Fatalf("DeleteNetwork: %v", err)
	}
}

func testWorkloadLifecycle(t *testing.T, factory Factory) {
	a, done := factory(t)
	defer done()
	ctx := context.Background()

	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	spec := serviceSpec("alpha", "echo")

	status, err := a.EnsureWorkload(ctx, spec)
	if err != nil {
		t.Fatalf("EnsureWorkload: %v", err)
	}
	if status.Ref != spec.Ref {
		t.Fatalf("status.Ref = %v, want %v", status.Ref, spec.Ref)
	}

	got, err := a.ObserveWorkload(ctx, spec.Ref)
	if err != nil {
		t.Fatalf("ObserveWorkload: %v", err)
	}
	if got.Ref != spec.Ref {
		t.Fatalf("ObserveWorkload.Ref = %v, want %v", got.Ref, spec.Ref)
	}

	if err := a.DeleteWorkload(ctx, spec.Ref); err != nil {
		t.Fatalf("DeleteWorkload: %v", err)
	}
	if err := a.DeleteWorkload(ctx, spec.Ref); err != nil {
		t.Fatalf("DeleteWorkload idempotent: %v", err)
	}
	if _, err := a.ObserveWorkload(ctx, spec.Ref); !errors.Is(err, rt.ErrNotFound) {
		t.Fatalf("ObserveWorkload after delete = %v, want ErrNotFound", err)
	}
}

func testWorkloadValidation(t *testing.T, factory Factory) {
	a, done := factory(t)
	defer done()
	ctx := context.Background()

	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}

	cases := []struct {
		name string
		spec rt.WorkloadSpec
	}{
		{"missing-name", rt.WorkloadSpec{Ref: rt.WorkloadRef{Tenant: "alpha"}, Kind: rt.WorkloadService, Containers: []rt.ContainerSpec{{Name: "c", Image: "img"}}}},
		{"missing-tenant", rt.WorkloadSpec{Ref: rt.WorkloadRef{Name: "x"}, Kind: rt.WorkloadService, Containers: []rt.ContainerSpec{{Name: "c", Image: "img"}}}},
		{"missing-kind", rt.WorkloadSpec{Ref: rt.WorkloadRef{Tenant: "alpha", Name: "x"}, Containers: []rt.ContainerSpec{{Name: "c", Image: "img"}}}},
		{"no-containers", rt.WorkloadSpec{Ref: rt.WorkloadRef{Tenant: "alpha", Name: "x"}, Kind: rt.WorkloadService}},
		{"hostpath-without-privilege", rt.WorkloadSpec{
			Ref:        rt.WorkloadRef{Tenant: "alpha", Name: "x"},
			Kind:       rt.WorkloadService,
			Privilege:  rt.PrivilegeRestricted,
			Containers: []rt.ContainerSpec{{Name: "c", Image: "img"}},
			Volumes:    []rt.VolumeSpec{{Name: "v", Source: rt.VolumeSource{HostPath: &rt.HostPathSource{Path: "/etc"}}}},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := a.EnsureWorkload(ctx, tc.spec)
			if !errors.Is(err, rt.ErrInvalidSpec) {
				t.Fatalf("EnsureWorkload(%s) = %v, want ErrInvalidSpec", tc.name, err)
			}
		})
	}
}

func testWorkloadIdempotent(t *testing.T, factory Factory) {
	a, done := factory(t)
	defer done()
	ctx := context.Background()

	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	spec := serviceSpec("alpha", "echo")

	if _, err := a.EnsureWorkload(ctx, spec); err != nil {
		t.Fatalf("first EnsureWorkload: %v", err)
	}
	if _, err := a.EnsureWorkload(ctx, spec); err != nil {
		t.Fatalf("second EnsureWorkload: %v", err)
	}
}

func testJobCompletes(t *testing.T, factory Factory) {
	a, done := factory(t)
	defer done()
	ctx := context.Background()

	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	spec := rt.WorkloadSpec{
		Ref:        rt.WorkloadRef{Tenant: "alpha", Name: "migrate"},
		Kind:       rt.WorkloadJob,
		Containers: []rt.ContainerSpec{{Name: "main", Image: "migrator:1"}},
	}
	status, err := a.EnsureWorkload(ctx, spec)
	if err != nil {
		t.Fatalf("EnsureWorkload: %v", err)
	}
	// Real adapters may return Progressing on initial submit; the
	// memory adapter shortcuts to Completed. Either is acceptable.
	switch status.Phase {
	case rt.PhaseProgressing, rt.PhaseReady, rt.PhaseCompleted:
	default:
		t.Fatalf("Job status.Phase = %q, want progressing|ready|completed", status.Phase)
	}
}

func testWatchEvents(t *testing.T, factory Factory) {
	a, done := factory(t)
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	events, err := a.WatchEvents(ctx, "alpha")
	if err != nil {
		t.Fatalf("WatchEvents: %v", err)
	}

	spec := serviceSpec("alpha", "echo")
	if _, err := a.EnsureWorkload(ctx, spec); err != nil {
		t.Fatalf("EnsureWorkload: %v", err)
	}

	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("event channel closed before delivery")
		}
		if ev.Ref != spec.Ref {
			t.Fatalf("event.Ref = %v, want %v", ev.Ref, spec.Ref)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for event")
	}
}

func testLogsRequiresWorkload(t *testing.T, factory Factory) {
	a, done := factory(t)
	defer done()
	ctx := context.Background()

	if err := a.EnsureTenant(ctx, "alpha"); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	var buf bytes.Buffer
	err := a.Logs(ctx, rt.LogOptions{Ref: rt.WorkloadRef{Tenant: "alpha", Name: "ghost"}}, &buf)
	if !errors.Is(err, rt.ErrNotFound) && !errors.Is(err, rt.ErrNotImplemented) {
		t.Fatalf("Logs(missing) = %v, want ErrNotFound or ErrNotImplemented", err)
	}
}

func serviceSpec(tenant rt.Tenant, name string) rt.WorkloadSpec {
	return rt.WorkloadSpec{
		Ref:       rt.WorkloadRef{Tenant: tenant, Name: name},
		Kind:      rt.WorkloadService,
		Privilege: rt.PrivilegeRestricted,
		Replicas:  2,
		Containers: []rt.ContainerSpec{{
			Name:  "main",
			Image: "echo:latest",
			Ports: []rt.PortSpec{{Name: "http", ContainerPort: 8080, Protocol: "TCP"}},
		}},
	}
}
