// Package vmworker drives KubeVirt VirtualMachines from API-server
// state. Replaces the former VmReconciler / VmEngine / KubeVirtEngine
// stack now that VM specs live in Postgres rather than on a CRD.
package vmworker

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-logr/logr"

	rt "github.com/azrtydxb/novanas/packages/runtime"
	novanas "github.com/azrtydxb/novanas/packages/sdk/go-client"
)

// Worker polls the API server for Vm resources and reconciles each one
// against the runtime adapter. Stateless across cycles; the API server
// is the source of truth.
type Worker struct {
	Client   *novanas.Client
	Adapter  rt.Adapter
	Interval time.Duration
	Log      logr.Logger
}

// Start blocks until ctx is cancelled. Each tick lists all Vms and
// reconciles them; failures are logged per-vm and don't stop the loop.
func (w *Worker) Start(ctx context.Context) error {
	if w.Client == nil || w.Adapter == nil {
		return errors.New("vmworker: Client and Adapter required")
	}
	interval := w.Interval
	if interval == 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	w.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	vms, err := w.Client.ListVms(ctx, "")
	if err != nil {
		w.Log.Error(err, "list vms")
		return
	}
	for _, v := range vms {
		w.reconcile(ctx, v)
	}
}

func (w *Worker) reconcile(ctx context.Context, v novanas.Vm) {
	ref := rt.VMRef{Tenant: rt.Tenant(v.Metadata.Namespace), Name: v.Metadata.Name}
	log := w.Log.WithValues("vm", v.Metadata.Namespace+"/"+v.Metadata.Name)

	if err := w.Adapter.EnsureTenant(ctx, ref.Tenant); err != nil {
		log.Error(err, "ensure tenant")
		return
	}

	if _, err := w.Adapter.EnsureVM(ctx, rt.VMSpec{Ref: ref, Spec: v.Spec.Spec}); err != nil {
		log.Error(err, "ensure vm")
		w.patchStatus(ctx, v, "Failed", err.Error())
		return
	}

	if state := normalizePowerState(v.Spec.PowerState); state != "" {
		if err := w.Adapter.SetVMPowerState(ctx, ref, state); err != nil {
			log.Error(err, "set power state", "state", state)
		}
	}

	observed, err := w.Adapter.ObserveVM(ctx, ref)
	phase := string(rt.VMRunning)
	if err == nil {
		phase = string(observed.Phase)
	}
	w.patchStatus(ctx, v, phase, "")
}

func (w *Worker) patchStatus(ctx context.Context, v novanas.Vm, phase, message string) {
	st := novanas.VmStatus{Phase: phase}
	if message != "" {
		st.Conditions = []novanas.Condition{{
			Type:    "Ready",
			Status:  "False",
			Reason:  "RuntimeError",
			Message: message,
		}}
	}
	if err := w.Client.PatchVmStatus(ctx, v.Metadata.Namespace, v.Metadata.Name, st); err != nil {
		w.Log.Error(err, "patch vm status", "vm", v.Metadata.Name)
	}
}

func normalizePowerState(s string) rt.VMPowerState {
	switch strings.ToLower(s) {
	case "running":
		return rt.VMRunning
	case "stopped", "off":
		return rt.VMStopped
	case "paused":
		return rt.VMPaused
	default:
		return ""
	}
}
