package runtime

import (
	"context"
	"io"
)

// Adapter is the boundary between NovaNas controllers and the
// container runtime.
//
// Implementations MUST:
//   - be safe for concurrent use,
//   - make Ensure* idempotent (re-applying the same spec is a no-op;
//     a changed spec converges to the new state),
//   - make Delete* idempotent (deleting an absent resource is a success),
//   - return synchronously without waiting for runtime convergence
//     (callers stream updates via WatchEvents).
//
// Every implementation is exercised by packages/runtime/conformance.
type Adapter interface {
	Name() string

	EnsureWorkload(ctx context.Context, spec WorkloadSpec) (WorkloadStatus, error)
	DeleteWorkload(ctx context.Context, ref WorkloadRef) error
	ObserveWorkload(ctx context.Context, ref WorkloadRef) (WorkloadStatus, error)

	EnsureNetwork(ctx context.Context, spec NetworkSpec) error
	DeleteNetwork(ctx context.Context, tenant Tenant, name string) error

	EnsureTenant(ctx context.Context, tenant Tenant) error
	DeleteTenant(ctx context.Context, tenant Tenant) error

	Logs(ctx context.Context, opts LogOptions, out io.Writer) error
	Exec(ctx context.Context, req ExecRequest, stdout, stderr io.Writer) (int, error)

	WatchEvents(ctx context.Context, tenant Tenant) (<-chan Event, error)

	// VM lifecycle. Implementations that don't support VMs return
	// ErrNotImplemented from each method. The k8s adapter uses
	// KubeVirt; a future docker adapter would use libvirt/qemu.
	EnsureVM(ctx context.Context, spec VMSpec) (VMStatus, error)
	DeleteVM(ctx context.Context, ref VMRef) error
	ObserveVM(ctx context.Context, ref VMRef) (VMStatus, error)
	SetVMPowerState(ctx context.Context, ref VMRef, state VMPowerState) error
	RestartVM(ctx context.Context, ref VMRef) error
}

type Event struct {
	Ref    WorkloadRef
	Status WorkloadStatus
}
