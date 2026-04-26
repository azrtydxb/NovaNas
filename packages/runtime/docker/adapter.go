package docker

import (
	"context"
	"io"

	rt "github.com/azrtydxb/novanas/packages/runtime"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "docker" }

func (a *Adapter) EnsureWorkload(_ context.Context, _ rt.WorkloadSpec) (rt.WorkloadStatus, error) {
	return rt.WorkloadStatus{}, rt.ErrNotImplemented
}

func (a *Adapter) DeleteWorkload(_ context.Context, _ rt.WorkloadRef) error {
	return rt.ErrNotImplemented
}

func (a *Adapter) ObserveWorkload(_ context.Context, _ rt.WorkloadRef) (rt.WorkloadStatus, error) {
	return rt.WorkloadStatus{}, rt.ErrNotImplemented
}

func (a *Adapter) EnsureNetwork(_ context.Context, _ rt.NetworkSpec) error {
	return rt.ErrNotImplemented
}

func (a *Adapter) DeleteNetwork(_ context.Context, _ rt.Tenant, _ string) error {
	return rt.ErrNotImplemented
}

func (a *Adapter) EnsureTenant(_ context.Context, _ rt.Tenant) error {
	return rt.ErrNotImplemented
}

func (a *Adapter) DeleteTenant(_ context.Context, _ rt.Tenant) error {
	return rt.ErrNotImplemented
}

func (a *Adapter) Logs(_ context.Context, _ rt.LogOptions, _ io.Writer) error {
	return rt.ErrNotImplemented
}

func (a *Adapter) Exec(_ context.Context, _ rt.ExecRequest, _, _ io.Writer) (int, error) {
	return -1, rt.ErrNotImplemented
}

func (a *Adapter) WatchEvents(_ context.Context, _ rt.Tenant) (<-chan rt.Event, error) {
	return nil, rt.ErrNotImplemented
}
