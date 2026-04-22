// Package reconciler — KubeVirt-backed VmEngine.
//
// Converts NovaNas Vm spec maps into kubevirt.io/v1 VirtualMachine
// resources and applies them via the controller-runtime client. When
// the KubeVirt CRD is not installed, EnsureVM returns
// ErrKubeVirtUnavailable and the caller can surface a clean condition
// without crashing.
package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ErrKubeVirtUnavailable indicates the KubeVirt CRDs are not installed.
// Controllers should treat this as a transient non-fatal condition.
var ErrKubeVirtUnavailable = errors.New("kubevirt: VirtualMachine CRD not installed")

// KubeVirtEngine is a VmEngine backed by the KubeVirt CRD.
type KubeVirtEngine struct {
	// Client is a sigs.k8s.io/controller-runtime client. It must be built
	// with a scheme that either has kubevirt types registered OR accepts
	// unstructured access (which controller-runtime clients always do).
	Client client.Client
}

// NewKubeVirtEngine builds a VmEngine that applies KubeVirt VMs via the
// supplied client. Using unstructured.Unstructured avoids a hard
// dependency on the kubevirt scheme at the caller site — the CRD is
// optional and detected at runtime.
func NewKubeVirtEngine(c client.Client) *KubeVirtEngine {
	return &KubeVirtEngine{Client: c}
}

var kubevirtVMGVK = schema.GroupVersionKind{
	Group:   "kubevirt.io",
	Version: "v1",
	Kind:    "VirtualMachine",
}

// EnsureVM upserts a kubevirt.io/v1 VirtualMachine resource mirroring the
// supplied spec. Returns the observed phase on success.
func (e *KubeVirtEngine) EnsureVM(ctx context.Context, namespace, name string, spec map[string]interface{}) (string, error) {
	if e == nil || e.Client == nil {
		return "", errors.New("kubevirt engine: client not configured")
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(kubevirtVMGVK)
	u.SetName(name)
	u.SetNamespace(namespace)
	// Normalize spec through JSON so embedded structs (time, ptr fields)
	// serialize cleanly into unstructured.
	if spec != nil {
		buf, err := json.Marshal(spec)
		if err != nil {
			return "", fmt.Errorf("kubevirt: marshal spec: %w", err)
		}
		var specMap map[string]any
		if err := json.Unmarshal(buf, &specMap); err != nil {
			return "", fmt.Errorf("kubevirt: unmarshal spec: %w", err)
		}
		if err := unstructured.SetNestedField(u.Object, specMap, "spec"); err != nil {
			return "", fmt.Errorf("kubevirt: set nested spec: %w", err)
		}
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(kubevirtVMGVK)
	getErr := e.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, existing)
	if apierrors.IsNotFound(getErr) {
		if cerr := e.Client.Create(ctx, u); cerr != nil {
			if isCRDMissing(cerr) {
				return "", ErrKubeVirtUnavailable
			}
			return "", fmt.Errorf("kubevirt: create VM: %w", cerr)
		}
		return "Pending", nil
	}
	if getErr != nil {
		if isCRDMissing(getErr) {
			return "", ErrKubeVirtUnavailable
		}
		return "", fmt.Errorf("kubevirt: get VM: %w", getErr)
	}
	// Update spec in place, preserving resourceVersion.
	u.SetResourceVersion(existing.GetResourceVersion())
	if uerr := e.Client.Update(ctx, u); uerr != nil {
		return "", fmt.Errorf("kubevirt: update VM: %w", uerr)
	}
	phase, _, _ := unstructured.NestedString(existing.Object, "status", "printableStatus")
	if phase == "" {
		phase = "Pending"
	}
	return phase, nil
}

// DeleteVM removes the backing VirtualMachine. Absent VMs succeed.
func (e *KubeVirtEngine) DeleteVM(ctx context.Context, namespace, name string) error {
	if e == nil || e.Client == nil {
		return errors.New("kubevirt engine: client not configured")
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(kubevirtVMGVK)
	u.SetName(name)
	u.SetNamespace(namespace)
	if err := e.Client.Delete(ctx, u); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		if isCRDMissing(err) {
			return nil
		}
		return fmt.Errorf("kubevirt: delete VM: %w", err)
	}
	return nil
}

// isCRDMissing reports whether the error indicates the KubeVirt CRDs are
// not installed (no matches for kind / no kind registered).
func isCRDMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// meta.IsNoMatchError would require an import of k8s.io/apimachinery/pkg/api/meta;
	// string match is sufficient and avoids adding transitive deps.
	return strings.Contains(msg, "no matches for kind") ||
		strings.Contains(msg, "no kind \"VirtualMachine\" is registered") ||
		strings.Contains(msg, "failed to get API group resources")
}

// SetPowerState patches the backing VirtualMachine's spec.running
// (and runStrategy for Paused) to match the requested state. When
// KubeVirt isn't installed this returns nil so callers can surface
// "engine unavailable" via conditions without erroring the
// reconcile.
func (e *KubeVirtEngine) SetPowerState(ctx context.Context, namespace, name, state string) error {
	if e == nil || e.Client == nil {
		return errors.New("kubevirt engine: client not configured")
	}
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(kubevirtVMGVK)
	if err := e.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, existing); err != nil {
		if apierrors.IsNotFound(err) || isCRDMissing(err) {
			return nil
		}
		return fmt.Errorf("kubevirt: get VM for power state: %w", err)
	}
	patched := existing.DeepCopy()
	switch strings.ToLower(state) {
	case "running":
		_ = unstructured.SetNestedField(patched.Object, true, "spec", "running")
		unstructured.RemoveNestedField(patched.Object, "spec", "runStrategy")
	case "stopped", "off":
		_ = unstructured.SetNestedField(patched.Object, false, "spec", "running")
		unstructured.RemoveNestedField(patched.Object, "spec", "runStrategy")
	case "paused":
		// KubeVirt exposes pause via a subresource; mirror the intent
		// on the VM via runStrategy=Manual so the desired state is at
		// least recorded. A dedicated subresource caller would then
		// invoke VMI.Pause(); left as TODO.
		_ = unstructured.SetNestedField(patched.Object, "Manual", "spec", "runStrategy")
		unstructured.RemoveNestedField(patched.Object, "spec", "running")
	default:
		return fmt.Errorf("kubevirt: unknown power state %q", state)
	}
	if err := e.Client.Patch(ctx, patched, client.MergeFrom(existing)); err != nil {
		if isCRDMissing(err) {
			return nil
		}
		return fmt.Errorf("kubevirt: patch VM power state: %w", err)
	}
	return nil
}

// Restart requests a hard reset of the VM's running instance. The
// real KubeVirt restart subresource (POST on
// virtualmachines/{name}/restart) isn't reachable via the
// controller-runtime client, so we fall back to a stop/start cycle by
// flipping spec.running. A dedicated subresource client can replace
// this later.
// TODO(operators): switch to KubeVirt subresource client for a true
// hard-reset (VMI.Restart).
func (e *KubeVirtEngine) Restart(ctx context.Context, namespace, name string) error {
	if err := e.SetPowerState(ctx, namespace, name, "Stopped"); err != nil {
		return err
	}
	return e.SetPowerState(ctx, namespace, name, "Running")
}

var _ VmEngine = (*KubeVirtEngine)(nil)
