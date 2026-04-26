package memory

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	rt "github.com/azrtydxb/novanas/packages/runtime"
)

type Adapter struct {
	mu        sync.Mutex
	tenants   map[rt.Tenant]struct{}
	networks  map[networkKey]rt.NetworkSpec
	workloads map[workloadKey]workloadEntry
	vms       map[vmKey]vmEntry
	watchers  map[rt.Tenant][]chan rt.Event
}

type networkKey struct {
	tenant rt.Tenant
	name   string
}

type workloadKey struct {
	tenant rt.Tenant
	name   string
}

type workloadEntry struct {
	spec   rt.WorkloadSpec
	status rt.WorkloadStatus
}

type vmKey struct {
	tenant rt.Tenant
	name   string
}

type vmEntry struct {
	spec   rt.VMSpec
	status rt.VMStatus
}

func New() *Adapter {
	return &Adapter{
		tenants:   make(map[rt.Tenant]struct{}),
		networks:  make(map[networkKey]rt.NetworkSpec),
		workloads: make(map[workloadKey]workloadEntry),
		vms:       make(map[vmKey]vmEntry),
		watchers:  make(map[rt.Tenant][]chan rt.Event),
	}
}

func (a *Adapter) Name() string { return "memory" }

func (a *Adapter) EnsureTenant(_ context.Context, tenant rt.Tenant) error {
	if tenant == "" {
		return fmt.Errorf("%w: empty tenant", rt.ErrInvalidSpec)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tenants[tenant] = struct{}{}
	return nil
}

func (a *Adapter) DeleteTenant(_ context.Context, tenant rt.Tenant) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k := range a.workloads {
		if k.tenant == tenant {
			return fmt.Errorf("%w: tenant %q has workloads", rt.ErrInvalidSpec, tenant)
		}
	}
	delete(a.tenants, tenant)
	for k := range a.networks {
		if k.tenant == tenant {
			delete(a.networks, k)
		}
	}
	return nil
}

func (a *Adapter) EnsureNetwork(_ context.Context, spec rt.NetworkSpec) error {
	if spec.Name == "" || spec.Tenant == "" {
		return fmt.Errorf("%w: network name and tenant required", rt.ErrInvalidSpec)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.tenants[spec.Tenant]; !ok {
		return fmt.Errorf("%w: tenant %q not found", rt.ErrNotFound, spec.Tenant)
	}
	a.networks[networkKey{spec.Tenant, spec.Name}] = spec
	return nil
}

func (a *Adapter) DeleteNetwork(_ context.Context, tenant rt.Tenant, name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, w := range a.workloads {
		if w.spec.Ref.Tenant == tenant && w.spec.Network.Network == name {
			return fmt.Errorf("%w: network %q still attached", rt.ErrInvalidSpec, name)
		}
	}
	delete(a.networks, networkKey{tenant, name})
	return nil
}

func (a *Adapter) EnsureWorkload(_ context.Context, spec rt.WorkloadSpec) (rt.WorkloadStatus, error) {
	if err := validate(spec); err != nil {
		return rt.WorkloadStatus{}, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.tenants[spec.Ref.Tenant]; !ok {
		return rt.WorkloadStatus{}, fmt.Errorf("%w: tenant %q not found", rt.ErrNotFound, spec.Ref.Tenant)
	}

	desired := spec.Replicas
	if desired == 0 {
		desired = 1
	}

	phase := rt.PhaseReady
	if spec.Kind == rt.WorkloadJob {
		phase = rt.PhaseCompleted
	}

	status := rt.WorkloadStatus{
		Ref:      spec.Ref,
		Phase:    phase,
		Replicas: rt.ReplicaCounts{Desired: desired, Ready: desired},
		Updated:  time.Now(),
	}

	a.workloads[workloadKey{spec.Ref.Tenant, spec.Ref.Name}] = workloadEntry{spec: spec, status: status}
	a.fanout(spec.Ref.Tenant, rt.Event{Ref: spec.Ref, Status: status})
	return status, nil
}

func (a *Adapter) DeleteWorkload(_ context.Context, ref rt.WorkloadRef) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.workloads, workloadKey{ref.Tenant, ref.Name})
	a.fanout(ref.Tenant, rt.Event{Ref: ref, Status: rt.WorkloadStatus{Ref: ref, Phase: rt.PhaseFailed, Message: "deleted"}})
	return nil
}

func (a *Adapter) ObserveWorkload(_ context.Context, ref rt.WorkloadRef) (rt.WorkloadStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.workloads[workloadKey{ref.Tenant, ref.Name}]
	if !ok {
		return rt.WorkloadStatus{}, rt.ErrNotFound
	}
	return entry.status, nil
}

func (a *Adapter) Logs(_ context.Context, opts rt.LogOptions, out io.Writer) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.workloads[workloadKey{opts.Ref.Tenant, opts.Ref.Name}]; !ok {
		return rt.ErrNotFound
	}
	_, err := fmt.Fprintf(out, "[memory adapter] no logs for workload %s/%s\n", opts.Ref.Tenant, opts.Ref.Name)
	return err
}

func (a *Adapter) Exec(_ context.Context, _ rt.ExecRequest, _, _ io.Writer) (int, error) {
	return -1, rt.ErrNotImplemented
}

func (a *Adapter) WatchEvents(ctx context.Context, tenant rt.Tenant) (<-chan rt.Event, error) {
	ch := make(chan rt.Event, 16)
	a.mu.Lock()
	a.watchers[tenant] = append(a.watchers[tenant], ch)
	a.mu.Unlock()

	go func() {
		<-ctx.Done()
		a.mu.Lock()
		defer a.mu.Unlock()
		watchers := a.watchers[tenant]
		for i, w := range watchers {
			if w == ch {
				a.watchers[tenant] = append(watchers[:i], watchers[i+1:]...)
				break
			}
		}
		close(ch)
	}()

	return ch, nil
}

// fanout: caller must hold a.mu. Drops events on full receivers; the
// memory adapter pins delivery semantics to best-effort.
func (a *Adapter) fanout(tenant rt.Tenant, ev rt.Event) {
	for _, ch := range a.watchers[tenant] {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (a *Adapter) EnsureVM(_ context.Context, spec rt.VMSpec) (rt.VMStatus, error) {
	if spec.Ref.Name == "" || spec.Ref.Tenant == "" {
		return rt.VMStatus{}, fmt.Errorf("%w: vm name and tenant required", rt.ErrInvalidSpec)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.tenants[spec.Ref.Tenant]; !ok {
		return rt.VMStatus{}, fmt.Errorf("%w: tenant %q not found", rt.ErrNotFound, spec.Ref.Tenant)
	}
	prev, existed := a.vms[vmKey{spec.Ref.Tenant, spec.Ref.Name}]
	phase := rt.VMRunning
	if existed {
		phase = prev.status.Phase
	}
	status := rt.VMStatus{Ref: spec.Ref, Phase: phase}
	a.vms[vmKey{spec.Ref.Tenant, spec.Ref.Name}] = vmEntry{spec: spec, status: status}
	return status, nil
}

func (a *Adapter) DeleteVM(_ context.Context, ref rt.VMRef) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.vms, vmKey{ref.Tenant, ref.Name})
	return nil
}

func (a *Adapter) ObserveVM(_ context.Context, ref rt.VMRef) (rt.VMStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.vms[vmKey{ref.Tenant, ref.Name}]
	if !ok {
		return rt.VMStatus{}, rt.ErrNotFound
	}
	return entry.status, nil
}

func (a *Adapter) SetVMPowerState(_ context.Context, ref rt.VMRef, state rt.VMPowerState) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.vms[vmKey{ref.Tenant, ref.Name}]
	if !ok {
		return rt.ErrNotFound
	}
	entry.status.Phase = state
	a.vms[vmKey{ref.Tenant, ref.Name}] = entry
	return nil
}

func (a *Adapter) RestartVM(_ context.Context, ref rt.VMRef) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.vms[vmKey{ref.Tenant, ref.Name}]
	if !ok {
		return rt.ErrNotFound
	}
	entry.status.Phase = rt.VMRunning
	entry.status.Message = "restarted"
	a.vms[vmKey{ref.Tenant, ref.Name}] = entry
	return nil
}

func validate(spec rt.WorkloadSpec) error {
	if spec.Ref.Name == "" || spec.Ref.Tenant == "" {
		return fmt.Errorf("%w: workload name and tenant required", rt.ErrInvalidSpec)
	}
	if spec.Kind == "" {
		return fmt.Errorf("%w: workload kind required", rt.ErrInvalidSpec)
	}
	if len(spec.Containers) == 0 {
		return fmt.Errorf("%w: at least one container required", rt.ErrInvalidSpec)
	}
	for _, v := range spec.Volumes {
		if err := validateVolumeSource(v.Source, spec.Privilege); err != nil {
			return fmt.Errorf("volume %q: %w", v.Name, err)
		}
	}
	return nil
}

func validateVolumeSource(src rt.VolumeSource, profile rt.PrivilegeProfile) error {
	count := 0
	if src.EmptyDir != nil {
		count++
	}
	if src.Dataset != nil {
		count++
	}
	if src.BlockVolume != nil {
		count++
	}
	if src.HostPath != nil {
		count++
	}
	if src.Secret != nil {
		count++
	}
	if count != 1 {
		return errors.New("exactly one volume source must be set")
	}
	if src.HostPath != nil && profile != rt.PrivilegePrivileged {
		return fmt.Errorf("%w: hostPath requires privileged profile", rt.ErrInvalidSpec)
	}
	return nil
}
