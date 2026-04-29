// Package vms — console session minting.
package vms

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Valid console kinds.
const (
	ConsoleVNC    = "vnc"
	ConsoleSPICE  = "spice"
	ConsoleSerial = "serial"
)

// validConsoleKind returns true if k is one of the supported kinds.
func validConsoleKind(k string) bool {
	switch k {
	case ConsoleVNC, ConsoleSPICE, ConsoleSerial:
		return true
	}
	return false
}

// Console mints a short-lived token + WebSocket URL for the browser to
// connect directly to virt-api. nova-api validates the caller's
// permission (HTTP layer) and the VM's existence (here) before minting.
//
// The wire path used by virt-api is documented at
// https://kubevirt.io/api-reference; we synthesize it from the
// configured VirtAPIBase plus the canonical subresource path.
func (m *Manager) Console(ctx context.Context, namespace, name, kind string) (*ConsoleSession, error) {
	if namespace == "" || name == "" {
		return nil, fmt.Errorf("%w: namespace and name are required", ErrInvalidRequest)
	}
	if kind == "" {
		kind = ConsoleVNC
	}
	if !validConsoleKind(kind) {
		return nil, fmt.Errorf("%w: invalid console kind %q", ErrInvalidRequest, kind)
	}

	// Verify the VM exists & is running. Console for a stopped VM is a
	// noop the GUI shouldn't surface.
	vm, err := m.Kube.GetVM(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if !vm.Running {
		return nil, fmt.Errorf("%w: VM is not running", ErrConflict)
	}

	token, expires, err := m.Kube.MintConsoleToken(ctx, namespace, name, kind, m.ttl())
	if err != nil {
		return nil, fmt.Errorf("mint token: %w", err)
	}

	base := strings.TrimRight(m.VirtAPIBase, "/")
	if base == "" {
		// Fall back to a relative WebSocket URL the GUI is expected to
		// resolve against the API origin.
		base = ""
	}

	subpath := ""
	switch kind {
	case ConsoleVNC:
		subpath = fmt.Sprintf("/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/vnc", namespace, name)
	case ConsoleSPICE:
		// KubeVirt does not officially expose /spice on the subresource
		// path; we route through virt-api's /vnc and let the front-end
		// renderer pick its protocol. Operators wanting true SPICE must
		// run a different console proxy.
		subpath = fmt.Sprintf("/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/vnc", namespace, name)
	case ConsoleSerial:
		subpath = fmt.Sprintf("/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/console", namespace, name)
	}

	return &ConsoleSession{
		WSURL:     base + subpath,
		Token:     token,
		ExpiresAt: expires,
		Kind:      kind,
	}, nil
}

// CreateSnapshot creates a VirtualMachineSnapshot.
func (m *Manager) CreateSnapshot(ctx context.Context, req CreateSnapshotRequest) (*Snapshot, error) {
	if req.Namespace == "" || req.Name == "" || req.VMName == "" {
		return nil, fmt.Errorf("%w: namespace, name, and vmName are required", ErrInvalidRequest)
	}
	if err := validName(req.Name); err != nil {
		return nil, err
	}
	if _, err := m.Kube.GetVM(ctx, req.Namespace, req.VMName); err != nil {
		return nil, err
	}
	return m.Kube.CreateSnapshot(ctx, Snapshot{
		Namespace: req.Namespace, Name: req.Name, VMName: req.VMName,
		CreatedAt: time.Now().UTC(),
	})
}

// ListSnapshots lists snapshots in a namespace (or across the prefix
// when namespace == "").
func (m *Manager) ListSnapshots(ctx context.Context, namespace string) ([]Snapshot, error) {
	if namespace != "" {
		return m.Kube.ListSnapshots(ctx, namespace)
	}
	nss, err := m.Kube.ListNamespaces(ctx, m.nsPrefix())
	if err != nil {
		return nil, err
	}
	var out []Snapshot
	for _, ns := range nss {
		s, err := m.Kube.ListSnapshots(ctx, ns)
		if err != nil {
			return nil, err
		}
		out = append(out, s...)
	}
	return out, nil
}

// DeleteSnapshot removes a snapshot.
func (m *Manager) DeleteSnapshot(ctx context.Context, namespace, name string) error {
	return m.Kube.DeleteSnapshot(ctx, namespace, name)
}

// CreateRestore creates a VirtualMachineRestore.
func (m *Manager) CreateRestore(ctx context.Context, req CreateRestoreRequest) (*Restore, error) {
	if req.Namespace == "" || req.Name == "" || req.VMName == "" || req.SnapshotName == "" {
		return nil, fmt.Errorf("%w: namespace, name, vmName, and snapshotName are required", ErrInvalidRequest)
	}
	if err := validName(req.Name); err != nil {
		return nil, err
	}
	return m.Kube.CreateRestore(ctx, Restore{
		Namespace: req.Namespace, Name: req.Name, VMName: req.VMName, SnapshotName: req.SnapshotName,
	})
}

// ListRestores lists restores in a namespace (or across the prefix).
func (m *Manager) ListRestores(ctx context.Context, namespace string) ([]Restore, error) {
	if namespace != "" {
		return m.Kube.ListRestores(ctx, namespace)
	}
	nss, err := m.Kube.ListNamespaces(ctx, m.nsPrefix())
	if err != nil {
		return nil, err
	}
	var out []Restore
	for _, ns := range nss {
		r, err := m.Kube.ListRestores(ctx, ns)
		if err != nil {
			return nil, err
		}
		out = append(out, r...)
	}
	return out, nil
}

// DeleteRestore removes a restore object.
func (m *Manager) DeleteRestore(ctx context.Context, namespace, name string) error {
	return m.Kube.DeleteRestore(ctx, namespace, name)
}
