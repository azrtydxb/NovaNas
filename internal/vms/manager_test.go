package vms

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeKube is an in-memory KubeClient used by every test in this
// package. It mimics the behaviour KubeVirt would expose for the small
// surface area Manager talks to. It is deliberately not a fixture you'd
// reuse outside these tests.
type fakeKube struct {
	mu sync.Mutex

	namespaces map[string]map[string]string // ns -> labels
	vms        map[string]*VM                // ns/name -> vm
	snapshots  map[string]*Snapshot          // ns/name
	restores   map[string]*Restore           // ns/name

	nodes int

	// errInjectors lets tests force a specific operation to fail.
	failCreateVM bool
}

func newFakeKube() *fakeKube {
	return &fakeKube{
		namespaces: map[string]map[string]string{},
		vms:        map[string]*VM{},
		snapshots:  map[string]*Snapshot{},
		restores:   map[string]*Restore{},
		nodes:      1,
	}
}

func (f *fakeKube) ListNamespaces(_ context.Context, prefix string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for ns := range f.namespaces {
		if strings.HasPrefix(ns, prefix) {
			out = append(out, ns)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (f *fakeKube) CreateNamespace(_ context.Context, name string, labels map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.namespaces[name]; ok {
		return nil // idempotent
	}
	f.namespaces[name] = labels
	return nil
}

func (f *fakeKube) DeleteNamespace(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.namespaces, name)
	for k := range f.vms {
		if strings.HasPrefix(k, name+"/") {
			delete(f.vms, k)
		}
	}
	return nil
}

func key(ns, name string) string { return ns + "/" + name }

func (f *fakeKube) ListVMs(_ context.Context, namespace string) ([]VM, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []VM
	for k, v := range f.vms {
		if strings.HasPrefix(k, namespace+"/") {
			out = append(out, *v)
		}
	}
	return out, nil
}

func (f *fakeKube) GetVM(_ context.Context, namespace, name string) (*VM, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.vms[key(namespace, name)]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}

func (f *fakeKube) CreateVM(_ context.Context, vm *VM, _ VMCloudInit, _ string) (*VM, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failCreateVM {
		return nil, errors.New("injected failure")
	}
	if _, ok := f.vms[key(vm.Namespace, vm.Name)]; ok {
		return nil, ErrAlreadyExists
	}
	cp := *vm
	cp.UID = "fake-uid-" + vm.Name
	f.vms[key(vm.Namespace, vm.Name)] = &cp
	return &cp, nil
}

func (f *fakeKube) PatchVM(_ context.Context, namespace, name string, p PatchRequest) (*VM, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.vms[key(namespace, name)]
	if !ok {
		return nil, ErrNotFound
	}
	if p.CPU != nil {
		v.CPU = *p.CPU
	}
	if p.MemoryMB != nil {
		v.MemoryMB = *p.MemoryMB
	}
	if p.Disks != nil {
		v.Disks = p.Disks
	}
	if p.Labels != nil {
		v.Labels = p.Labels
	}
	cp := *v
	return &cp, nil
}

func (f *fakeKube) DeleteVM(_ context.Context, namespace, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key(namespace, name)
	if _, ok := f.vms[k]; !ok {
		return ErrNotFound
	}
	delete(f.vms, k)
	return nil
}

func (f *fakeKube) SetVMRunning(_ context.Context, namespace, name string, running bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.vms[key(namespace, name)]
	if !ok {
		return ErrNotFound
	}
	v.Running = running
	if running {
		v.Phase = PhaseRunning
	} else {
		v.Phase = PhaseStopped
	}
	return nil
}

func (f *fakeKube) RestartVM(_ context.Context, namespace, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.vms[key(namespace, name)]; !ok {
		return ErrNotFound
	}
	return nil
}

func (f *fakeKube) PauseVM(_ context.Context, namespace, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.vms[key(namespace, name)]
	if !ok {
		return ErrNotFound
	}
	v.Phase = PhasePaused
	return nil
}

func (f *fakeKube) UnpauseVM(_ context.Context, namespace, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.vms[key(namespace, name)]
	if !ok {
		return ErrNotFound
	}
	v.Phase = PhaseRunning
	return nil
}

func (f *fakeKube) MigrateVM(_ context.Context, namespace, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.vms[key(namespace, name)]; !ok {
		return ErrNotFound
	}
	return nil
}

func (f *fakeKube) CountReadyNodes(_ context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nodes, nil
}

func (f *fakeKube) ListSnapshots(_ context.Context, namespace string) ([]Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Snapshot
	for k, s := range f.snapshots {
		if strings.HasPrefix(k, namespace+"/") {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (f *fakeKube) CreateSnapshot(_ context.Context, s Snapshot) (*Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key(s.Namespace, s.Name)
	if _, ok := f.snapshots[k]; ok {
		return nil, ErrAlreadyExists
	}
	cp := s
	cp.ReadyToUse = true
	cp.Phase = "Succeeded"
	f.snapshots[k] = &cp
	return &cp, nil
}

func (f *fakeKube) DeleteSnapshot(_ context.Context, namespace, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key(namespace, name)
	if _, ok := f.snapshots[k]; !ok {
		return ErrNotFound
	}
	delete(f.snapshots, k)
	return nil
}

func (f *fakeKube) ListRestores(_ context.Context, namespace string) ([]Restore, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Restore
	for k, r := range f.restores {
		if strings.HasPrefix(k, namespace+"/") {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (f *fakeKube) CreateRestore(_ context.Context, r Restore) (*Restore, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key(r.Namespace, r.Name)
	if _, ok := f.restores[k]; ok {
		return nil, ErrAlreadyExists
	}
	cp := r
	cp.Complete = true
	f.restores[k] = &cp
	return &cp, nil
}

func (f *fakeKube) DeleteRestore(_ context.Context, namespace, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key(namespace, name)
	if _, ok := f.restores[k]; !ok {
		return ErrNotFound
	}
	delete(f.restores, k)
	return nil
}

func (f *fakeKube) MintConsoleToken(_ context.Context, _, _, _ string, ttl time.Duration) (string, time.Time, error) {
	return "fake-console-token", time.Now().Add(ttl), nil
}

// helpers ---------------------------------------------------------------

func newTestManager() (*Manager, *fakeKube) {
	fk := newFakeKube()
	cat := NewCatalogFromTemplates([]Template{{
		ID:                "debian-12-cloud",
		DisplayName:       "Debian 12",
		ImageURL:          "https://example/debian.qcow2",
		DefaultCPU:        2,
		DefaultMemoryMB:   2048,
		DefaultDiskGB:     20,
		CloudInitFriendly: true,
	}, {
		ID:                      "windows-11",
		DisplayName:             "Windows 11",
		RequiresUserSuppliedISO: true,
	}})
	m := &Manager{
		Kube:            fk,
		Templates:       cat,
		VirtAPIBase:     "wss://virt.example/k8s",
		ConsoleTokenTTL: time.Minute,
		NamespacePrefix: "vm-",
	}
	return m, fk
}

// tests -----------------------------------------------------------------

func TestCreate_DefaultsAndStartFlag(t *testing.T) {
	m, fk := newTestManager()
	ctx := context.Background()

	vm, err := m.Create(ctx, CreateRequest{
		Name:          "alpha",
		TemplateID:    "debian-12-cloud",
		StartOnCreate: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if vm.Namespace != "vm-alpha" {
		t.Fatalf("ns: got %q", vm.Namespace)
	}
	if vm.CPU != 2 || vm.MemoryMB != 2048 {
		t.Fatalf("template defaults not applied: %+v", vm)
	}
	if len(vm.Disks) != 1 || !vm.Disks[0].Boot || vm.Disks[0].Source != "template:debian-12-cloud" {
		t.Fatalf("boot disk: %+v", vm.Disks)
	}
	got := fk.vms[key("vm-alpha", "alpha")]
	if !got.Running {
		t.Fatal("StartOnCreate should have set running=true")
	}
}

func TestCreate_RequiresKnownTemplate(t *testing.T) {
	m, _ := newTestManager()
	_, err := m.Create(context.Background(), CreateRequest{Name: "beta", TemplateID: "no-such"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
	}
}

func TestCreate_WindowsNeedsUserISO(t *testing.T) {
	m, _ := newTestManager()
	_, err := m.Create(context.Background(), CreateRequest{Name: "win", TemplateID: "windows-11"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
	}
	// Now provide a user-supplied ISO disk.
	vm, err := m.Create(context.Background(), CreateRequest{
		Name:       "win",
		TemplateID: "windows-11",
		Disks:      []VMDisk{{Name: "boot", SizeGB: 64, Source: "url:https://example/win.iso", Boot: true}},
	})
	if err != nil {
		t.Fatalf("with iso: %v", err)
	}
	if vm.Name != "win" {
		t.Fatalf("vm: %+v", vm)
	}
}

func TestCreate_RejectsBadName(t *testing.T) {
	m, _ := newTestManager()
	_, err := m.Create(context.Background(), CreateRequest{Name: "BAD_NAME"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want invalid, got %v", err)
	}
}

func TestList_PaginatesAcrossNamespaces(t *testing.T) {
	m, fk := newTestManager()
	ctx := context.Background()
	for i := 0; i < 7; i++ {
		name := fmt.Sprintf("v%02d", i)
		ns := "vm-" + name
		fk.namespaces[ns] = nil
		fk.vms[key(ns, name)] = &VM{Namespace: ns, Name: name}
	}
	page, err := m.List(ctx, ListOptions{PageSize: 3})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Items) != 3 || page.NextCursor == "" {
		t.Fatalf("page1: %+v", page)
	}
	page2, err := m.List(ctx, ListOptions{PageSize: 3, Cursor: page.NextCursor})
	if err != nil {
		t.Fatalf("list2: %v", err)
	}
	if len(page2.Items) != 3 || page2.NextCursor == "" {
		t.Fatalf("page2: %+v", page2)
	}
	page3, err := m.List(ctx, ListOptions{PageSize: 3, Cursor: page2.NextCursor})
	if err != nil {
		t.Fatalf("list3: %v", err)
	}
	if len(page3.Items) != 1 || page3.NextCursor != "" {
		t.Fatalf("page3: %+v", page3)
	}
}

func TestStartStopRestartLifecycle(t *testing.T) {
	m, _ := newTestManager()
	ctx := context.Background()
	if _, err := m.Create(ctx, CreateRequest{Name: "ll", TemplateID: "debian-12-cloud"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx, "vm-ll", "ll"); err != nil {
		t.Fatal(err)
	}
	v, _ := m.Get(ctx, "vm-ll", "ll")
	if !v.Running || v.Phase != PhaseRunning {
		t.Fatalf("start: %+v", v)
	}
	if err := m.Pause(ctx, "vm-ll", "ll"); err != nil {
		t.Fatal(err)
	}
	v, _ = m.Get(ctx, "vm-ll", "ll")
	if v.Phase != PhasePaused {
		t.Fatalf("pause: %+v", v)
	}
	if err := m.Stop(ctx, "vm-ll", "ll"); err != nil {
		t.Fatal(err)
	}
	v, _ = m.Get(ctx, "vm-ll", "ll")
	if v.Running {
		t.Fatalf("stop: %+v", v)
	}
}

func TestMigrate_SingleNodeReturns501(t *testing.T) {
	m, fk := newTestManager()
	ctx := context.Background()
	if _, err := m.Create(ctx, CreateRequest{Name: "mig", TemplateID: "debian-12-cloud"}); err != nil {
		t.Fatal(err)
	}
	fk.nodes = 1
	if err := m.Migrate(ctx, "vm-mig", "mig"); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("want ErrNotImplemented, got %v", err)
	}
	fk.nodes = 3
	if err := m.Migrate(ctx, "vm-mig", "mig"); err != nil {
		t.Fatalf("multinode migrate: %v", err)
	}
}

func TestDeleteCascadesNamespace(t *testing.T) {
	m, fk := newTestManager()
	ctx := context.Background()
	if _, err := m.Create(ctx, CreateRequest{Name: "x", TemplateID: "debian-12-cloud"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(ctx, "vm-x", "x"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := fk.namespaces["vm-x"]; ok {
		t.Fatal("namespace should have been deleted")
	}
}

func TestConsole_RequiresRunningVM(t *testing.T) {
	m, _ := newTestManager()
	ctx := context.Background()
	if _, err := m.Create(ctx, CreateRequest{Name: "c", TemplateID: "debian-12-cloud"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Console(ctx, "vm-c", "c", ConsoleVNC); !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	if err := m.Start(ctx, "vm-c", "c"); err != nil {
		t.Fatal(err)
	}
	cs, err := m.Console(ctx, "vm-c", "c", ConsoleVNC)
	if err != nil {
		t.Fatalf("console: %v", err)
	}
	if cs.Token == "" || cs.WSURL == "" {
		t.Fatalf("console: %+v", cs)
	}
	if !strings.Contains(cs.WSURL, "/virtualmachineinstances/c/vnc") {
		t.Fatalf("ws url: %s", cs.WSURL)
	}
	if cs.ExpiresAt.Before(time.Now()) {
		t.Fatalf("expires in the past: %v", cs.ExpiresAt)
	}
}

func TestSnapshotRestoreFlow(t *testing.T) {
	m, _ := newTestManager()
	ctx := context.Background()
	if _, err := m.Create(ctx, CreateRequest{Name: "s", TemplateID: "debian-12-cloud"}); err != nil {
		t.Fatal(err)
	}
	snap, err := m.CreateSnapshot(ctx, CreateSnapshotRequest{Namespace: "vm-s", Name: "snap1", VMName: "s"})
	if err != nil {
		t.Fatalf("snap: %v", err)
	}
	if !snap.ReadyToUse {
		t.Fatalf("snap not ready: %+v", snap)
	}
	rest, err := m.CreateRestore(ctx, CreateRestoreRequest{Namespace: "vm-s", Name: "r1", VMName: "s", SnapshotName: "snap1"})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !rest.Complete {
		t.Fatalf("restore not complete: %+v", rest)
	}
	if err := m.DeleteSnapshot(ctx, "vm-s", "snap1"); err != nil {
		t.Fatal(err)
	}
}

func TestPatch_BoundsCheck(t *testing.T) {
	m, _ := newTestManager()
	ctx := context.Background()
	if _, err := m.Create(ctx, CreateRequest{Name: "p", TemplateID: "debian-12-cloud"}); err != nil {
		t.Fatal(err)
	}
	tooBig := 999999
	if _, err := m.Patch(ctx, "vm-p", "p", PatchRequest{MemoryMB: &tooBig}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want invalid, got %v", err)
	}
	cpu := 4
	mem := 4096
	v, err := m.Patch(ctx, "vm-p", "p", PatchRequest{CPU: &cpu, MemoryMB: &mem})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if v.CPU != 4 || v.MemoryMB != 4096 {
		t.Fatalf("patch result: %+v", v)
	}
}
