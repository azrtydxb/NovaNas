// Package vms — Manager: high-level VM operations on top of KubeClient.
package vms

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Default knobs.
const (
	DefaultNamespacePrefix = "vm-"
	DefaultPageSize        = 50
	MaxPageSize            = 200
	DefaultBootDiskGB      = 20
	DefaultDiskBus         = "virtio"
)

// vmNameRE constrains VM names to a Kubernetes-compatible label. Per-VM
// namespaces use the same restriction (prefix + name).
var vmNameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?$`)

// Manager is the orchestration entrypoint the HTTP layer talks to.
type Manager struct {
	Logger    *slog.Logger
	Kube      KubeClient
	Templates *TemplateCatalog
	// VirtAPIBase is the externally-reachable virt-api WebSocket base
	// (e.g. wss://nas.example.com/k8s). The console URL returned to the
	// browser is built from this plus the canonical KubeVirt subresource
	// path. Empty in tests / nil-Kube setups.
	VirtAPIBase string
	// ConsoleTokenTTL caps the lifetime of console session tokens.
	ConsoleTokenTTL time.Duration

	// NamespacePrefix overrides DefaultNamespacePrefix for per-VM
	// namespace scoping. Tests use "test-vm-" so they don't collide
	// with real "vm-*" namespaces.
	NamespacePrefix string

	// MultiNodeOverride lets tests force the migrate path. When zero, the
	// real CountReadyNodes from KubeClient is used.
	MultiNodeOverride int
}

// nsPrefix returns the configured namespace prefix or the default.
func (m *Manager) nsPrefix() string {
	if m.NamespacePrefix != "" {
		return m.NamespacePrefix
	}
	return DefaultNamespacePrefix
}

// ttl returns the configured ConsoleTokenTTL or 5 minutes.
func (m *Manager) ttl() time.Duration {
	if m.ConsoleTokenTTL > 0 {
		return m.ConsoleTokenTTL
	}
	return 5 * time.Minute
}

// validName returns ErrInvalidRequest if name doesn't match vmNameRE.
func validName(name string) error {
	if !vmNameRE.MatchString(name) {
		return fmt.Errorf("%w: name %q must match %s", ErrInvalidRequest, name, vmNameRE.String())
	}
	return nil
}

// resolveNamespace returns the effective namespace for a request. If
// the operator explicitly set Namespace it is used (and validated to
// start with the prefix). Otherwise we synthesize <prefix><name>.
func (m *Manager) resolveNamespace(req *CreateRequest) (string, error) {
	if req.Namespace != "" {
		if !strings.HasPrefix(req.Namespace, m.nsPrefix()) {
			return "", fmt.Errorf("%w: namespace must begin with %q", ErrInvalidRequest, m.nsPrefix())
		}
		return req.Namespace, nil
	}
	return m.nsPrefix() + req.Name, nil
}

// List returns VMs across all "<prefix>*" namespaces, paginated.
//
// Pagination is opaque-cursor: the cursor is the last namespace+name we
// returned. The implementation is O(n) over visible namespaces and is
// fine for the per-VM-namespace pattern (one VM per namespace) at the
// scales we expect (tens to low hundreds).
func (m *Manager) List(ctx context.Context, opts ListOptions) (Page[VM], error) {
	if opts.NamespacePrefix == "" {
		opts.NamespacePrefix = m.nsPrefix()
	}
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	nss, err := m.Kube.ListNamespaces(ctx, opts.NamespacePrefix)
	if err != nil {
		return Page[VM]{}, fmt.Errorf("list namespaces: %w", err)
	}
	sort.Strings(nss)

	var all []VM
	for _, ns := range nss {
		vms, err := m.Kube.ListVMs(ctx, ns)
		if err != nil {
			return Page[VM]{}, fmt.Errorf("list vms in %s: %w", ns, err)
		}
		all = append(all, vms...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Namespace != all[j].Namespace {
			return all[i].Namespace < all[j].Namespace
		}
		return all[i].Name < all[j].Name
	})

	// Apply cursor.
	start := 0
	if opts.Cursor != "" {
		for i, v := range all {
			key := v.Namespace + "/" + v.Name
			if key > opts.Cursor {
				start = i
				break
			}
			start = i + 1
		}
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	page := Page[VM]{Items: append([]VM(nil), all[start:end]...)}
	if end < len(all) {
		last := page.Items[len(page.Items)-1]
		page.NextCursor = last.Namespace + "/" + last.Name
	}
	return page, nil
}

// Get returns a single VM by namespace+name.
func (m *Manager) Get(ctx context.Context, namespace, name string) (*VM, error) {
	if namespace == "" || name == "" {
		return nil, fmt.Errorf("%w: namespace and name are required", ErrInvalidRequest)
	}
	return m.Kube.GetVM(ctx, namespace, name)
}

// Create validates the request, resolves a template (when provided),
// ensures the per-VM namespace exists, and creates a VirtualMachine.
//
// Defaults applied:
//   - CPU/memory from template when zero
//   - First disk is the boot disk sourced from the template
//   - Network: pod-default unless caller specified
func (m *Manager) Create(ctx context.Context, req CreateRequest) (*VM, error) {
	if err := validName(req.Name); err != nil {
		return nil, err
	}
	ns, err := m.resolveNamespace(&req)
	if err != nil {
		return nil, err
	}

	// Resolve template if any. A template may pre-populate cpu / memory /
	// boot disk; explicit fields on the request always win.
	var tmpl *Template
	if req.TemplateID != "" {
		t, ok := m.Templates.Get(req.TemplateID)
		if !ok {
			return nil, fmt.Errorf("%w: unknown templateID %q", ErrInvalidRequest, req.TemplateID)
		}
		if t.RequiresUserSuppliedISO {
			// We accept the request — the operator must have supplied a
			// disk with source "url:..." pointing at their ISO. Verify.
			haveBoot := false
			for _, d := range req.Disks {
				if d.Boot && strings.HasPrefix(d.Source, "url:") {
					haveBoot = true
					break
				}
			}
			if !haveBoot {
				return nil, fmt.Errorf("%w: template %q requires a user-supplied ISO; provide a boot disk with source \"url:<iso>\"", ErrInvalidRequest, t.ID)
			}
		}
		tmpl = &t
	}

	if req.CPU <= 0 {
		if tmpl != nil {
			req.CPU = tmpl.DefaultCPU
		} else {
			req.CPU = 1
		}
	}
	if req.MemoryMB <= 0 {
		if tmpl != nil {
			req.MemoryMB = tmpl.DefaultMemoryMB
		} else {
			req.MemoryMB = 1024
		}
	}
	if req.CPU > 64 {
		return nil, fmt.Errorf("%w: cpu must be <= 64", ErrInvalidRequest)
	}
	if req.MemoryMB > 256*1024 {
		return nil, fmt.Errorf("%w: memoryMB must be <= 262144", ErrInvalidRequest)
	}

	// Synthesize a boot disk if none provided AND a cloud-image template
	// is available.
	if len(req.Disks) == 0 && tmpl != nil && tmpl.ImageURL != "" {
		req.Disks = []VMDisk{{
			Name:   "rootdisk",
			SizeGB: tmpl.DefaultDiskGB,
			Source: "template:" + tmpl.ID,
			Boot:   true,
			Bus:    DefaultDiskBus,
		}}
	}

	if len(req.Disks) == 0 {
		return nil, fmt.Errorf("%w: at least one disk is required", ErrInvalidRequest)
	}
	hasBoot := false
	for i := range req.Disks {
		d := &req.Disks[i]
		if d.Name == "" {
			d.Name = fmt.Sprintf("disk%d", i)
		}
		if d.SizeGB <= 0 {
			d.SizeGB = DefaultBootDiskGB
		}
		if d.Bus == "" {
			d.Bus = DefaultDiskBus
		}
		if d.Boot {
			hasBoot = true
		}
	}
	if !hasBoot {
		req.Disks[0].Boot = true
	}

	if len(req.Networks) == 0 {
		req.Networks = []VMNetwork{{Name: "default", Type: "pod"}}
	}

	// Default cloud-init hostname.
	if req.CloudInit.Hostname == "" {
		req.CloudInit.Hostname = req.Name
	}

	// Ensure namespace exists. Idempotent.
	if err := m.Kube.CreateNamespace(ctx, ns, map[string]string{
		"novanas.io/managed":    "true",
		"novanas.io/component":  "vm",
		"novanas.io/vm":         req.Name,
	}); err != nil {
		return nil, fmt.Errorf("create namespace: %w", err)
	}

	vm := &VM{
		Namespace:  ns,
		Name:       req.Name,
		CPU:        req.CPU,
		MemoryMB:   req.MemoryMB,
		Running:    req.StartOnCreate,
		Disks:      req.Disks,
		Networks:   req.Networks,
		Labels:     mergeLabels(req.Labels, map[string]string{"novanas.io/vm": req.Name}),
		Phase:      PhaseStopped,
		TemplateID: req.TemplateID,
		CreatedAt:  time.Now().UTC(),
	}
	out, err := m.Kube.CreateVM(ctx, vm, req.CloudInit, req.TemplateID)
	if err != nil {
		return nil, err
	}
	if req.StartOnCreate {
		if err := m.Kube.SetVMRunning(ctx, ns, req.Name, true); err != nil {
			return nil, fmt.Errorf("start: %w", err)
		}
	}
	return out, nil
}

// Patch applies partial updates to a VM.
func (m *Manager) Patch(ctx context.Context, namespace, name string, p PatchRequest) (*VM, error) {
	if p.CPU != nil && (*p.CPU <= 0 || *p.CPU > 64) {
		return nil, fmt.Errorf("%w: cpu must be in [1,64]", ErrInvalidRequest)
	}
	if p.MemoryMB != nil && (*p.MemoryMB <= 0 || *p.MemoryMB > 256*1024) {
		return nil, fmt.Errorf("%w: memoryMB must be in [1,262144]", ErrInvalidRequest)
	}
	return m.Kube.PatchVM(ctx, namespace, name, p)
}

// Delete removes the VM and its per-VM namespace.
func (m *Manager) Delete(ctx context.Context, namespace, name string) error {
	if namespace == "" || name == "" {
		return fmt.Errorf("%w: namespace and name are required", ErrInvalidRequest)
	}
	if err := m.Kube.DeleteVM(ctx, namespace, name); err != nil {
		return err
	}
	// If the namespace was created by us (matches "<prefix><name>"),
	// delete it too. This is the cascade-delete pattern.
	if namespace == m.nsPrefix()+name {
		if err := m.Kube.DeleteNamespace(ctx, namespace); err != nil {
			return fmt.Errorf("delete namespace: %w", err)
		}
	}
	return nil
}

// Start sets running=true; idempotent.
func (m *Manager) Start(ctx context.Context, namespace, name string) error {
	return m.Kube.SetVMRunning(ctx, namespace, name, true)
}

// Stop sets running=false (graceful: kubevirt issues ACPI shutdown).
func (m *Manager) Stop(ctx context.Context, namespace, name string) error {
	return m.Kube.SetVMRunning(ctx, namespace, name, false)
}

// Restart asks kubevirt to stop+start.
func (m *Manager) Restart(ctx context.Context, namespace, name string) error {
	return m.Kube.RestartVM(ctx, namespace, name)
}

// Pause issues qemu pause.
func (m *Manager) Pause(ctx context.Context, namespace, name string) error {
	return m.Kube.PauseVM(ctx, namespace, name)
}

// Unpause resumes a paused VM.
func (m *Manager) Unpause(ctx context.Context, namespace, name string) error {
	return m.Kube.UnpauseVM(ctx, namespace, name)
}

// Migrate live-migrates a VM. Returns ErrNotImplemented when the cluster
// only has a single ready node, since there's nowhere to migrate to.
func (m *Manager) Migrate(ctx context.Context, namespace, name string) error {
	count := m.MultiNodeOverride
	if count == 0 {
		c, err := m.Kube.CountReadyNodes(ctx)
		if err != nil {
			return fmt.Errorf("count nodes: %w", err)
		}
		count = c
	}
	if count < 2 {
		return ErrNotImplemented
	}
	return m.Kube.MigrateVM(ctx, namespace, name)
}

func mergeLabels(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
