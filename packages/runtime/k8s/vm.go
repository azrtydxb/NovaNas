package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	rt "github.com/azrtydxb/novanas/packages/runtime"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// kubevirtVMGVR is the GroupVersionResource for KubeVirt VirtualMachine.
// Pinned at v1; KubeVirt has been v1 since 0.36.
var kubevirtVMGVR = schema.GroupVersionResource{
	Group:    "kubevirt.io",
	Version:  "v1",
	Resource: "virtualmachines",
}

const kubevirtVMKind = "VirtualMachine"

func (a *Adapter) ensureNamespace(ctx context.Context, t rt.Tenant) (string, error) {
	ns := a.namespace(t)
	if _, err := a.cs.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("%w: tenant %q not found", rt.ErrNotFound, t)
		}
		return "", err
	}
	return ns, nil
}

func (a *Adapter) vmClient(ns string) (dynamic.ResourceInterface, error) {
	if a.dyn == nil {
		return nil, errors.New("k8s adapter: dynamic client not configured")
	}
	return a.dyn.Resource(kubevirtVMGVR).Namespace(ns), nil
}

func (a *Adapter) EnsureVM(ctx context.Context, spec rt.VMSpec) (rt.VMStatus, error) {
	if spec.Ref.Name == "" || spec.Ref.Tenant == "" {
		return rt.VMStatus{}, fmt.Errorf("%w: vm name and tenant required", rt.ErrInvalidSpec)
	}
	ns, err := a.ensureNamespace(ctx, spec.Ref.Tenant)
	if err != nil {
		return rt.VMStatus{}, err
	}
	vc, err := a.vmClient(ns)
	if err != nil {
		return rt.VMStatus{}, err
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   kubevirtVMGVR.Group,
		Version: kubevirtVMGVR.Version,
		Kind:    kubevirtVMKind,
	})
	u.SetName(spec.Ref.Name)
	u.SetNamespace(ns)
	u.SetLabels(map[string]string{
		labelTenant: string(spec.Ref.Tenant),
		labelVM:     spec.Ref.Name,
	})
	if spec.Spec != nil {
		buf, err := json.Marshal(spec.Spec)
		if err != nil {
			return rt.VMStatus{}, fmt.Errorf("kubevirt: marshal spec: %w", err)
		}
		var specMap map[string]any
		if err := json.Unmarshal(buf, &specMap); err != nil {
			return rt.VMStatus{}, fmt.Errorf("kubevirt: unmarshal spec: %w", err)
		}
		if err := unstructured.SetNestedField(u.Object, specMap, "spec"); err != nil {
			return rt.VMStatus{}, fmt.Errorf("kubevirt: set nested spec: %w", err)
		}
	}

	existing, getErr := vc.Get(ctx, spec.Ref.Name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(getErr):
		if _, err := vc.Create(ctx, u, metav1.CreateOptions{}); err != nil {
			return rt.VMStatus{}, fmt.Errorf("kubevirt: create: %w", err)
		}
	case getErr != nil:
		return rt.VMStatus{}, fmt.Errorf("kubevirt: get: %w", getErr)
	default:
		u.SetResourceVersion(existing.GetResourceVersion())
		if _, err := vc.Update(ctx, u, metav1.UpdateOptions{}); err != nil {
			return rt.VMStatus{}, fmt.Errorf("kubevirt: update: %w", err)
		}
	}

	return rt.VMStatus{Ref: spec.Ref, Phase: phaseFromVM(u)}, nil
}

func (a *Adapter) DeleteVM(ctx context.Context, ref rt.VMRef) error {
	ns := a.namespace(ref.Tenant)
	vc, err := a.vmClient(ns)
	if err != nil {
		return err
	}
	if err := vc.Delete(ctx, ref.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (a *Adapter) ObserveVM(ctx context.Context, ref rt.VMRef) (rt.VMStatus, error) {
	ns := a.namespace(ref.Tenant)
	vc, err := a.vmClient(ns)
	if err != nil {
		return rt.VMStatus{}, err
	}
	u, err := vc.Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return rt.VMStatus{}, rt.ErrNotFound
		}
		return rt.VMStatus{}, err
	}
	return rt.VMStatus{Ref: ref, Phase: phaseFromVM(u)}, nil
}

func (a *Adapter) SetVMPowerState(ctx context.Context, ref rt.VMRef, state rt.VMPowerState) error {
	ns := a.namespace(ref.Tenant)
	vc, err := a.vmClient(ns)
	if err != nil {
		return err
	}
	running := state == rt.VMRunning
	patch := fmt.Appendf(nil, `{"spec":{"running":%t}}`, running)
	if _, err := vc.Patch(ctx, ref.Name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return rt.ErrNotFound
		}
		return err
	}
	return nil
}

// RestartVM triggers a hard restart by toggling spec.running off then
// on. Equivalent to KubeVirt's restart subresource without needing the
// typed client.
func (a *Adapter) RestartVM(ctx context.Context, ref rt.VMRef) error {
	if err := a.SetVMPowerState(ctx, ref, rt.VMStopped); err != nil {
		return err
	}
	return a.SetVMPowerState(ctx, ref, rt.VMRunning)
}

func phaseFromVM(u *unstructured.Unstructured) rt.VMPowerState {
	if u == nil {
		return rt.VMStopped
	}
	if running, found, _ := unstructured.NestedBool(u.Object, "spec", "running"); found && running {
		if printable, ok, _ := unstructured.NestedString(u.Object, "status", "printableStatus"); ok && printable != "" {
			switch printable {
			case "Running":
				return rt.VMRunning
			case "Paused":
				return rt.VMPaused
			default:
				return rt.VMStopped
			}
		}
		return rt.VMRunning
	}
	return rt.VMStopped
}

const labelVM = "novanas.io/vm"
