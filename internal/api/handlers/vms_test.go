package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/vms"
)

// minimalKube is a tiny in-memory KubeClient used to drive the handler
// tests without a real cluster.
type minimalKube struct {
	mu    sync.Mutex
	store map[string]*vms.VM
	ns    map[string]struct{}
}

func newMinimalKube() *minimalKube {
	return &minimalKube{
		store: map[string]*vms.VM{},
		ns:    map[string]struct{}{},
	}
}

func k(ns, n string) string { return ns + "/" + n }

func (m *minimalKube) ListNamespaces(_ context.Context, prefix string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for ns := range m.ns {
		if strings.HasPrefix(ns, prefix) {
			out = append(out, ns)
		}
	}
	return out, nil
}
func (m *minimalKube) CreateNamespace(_ context.Context, name string, _ map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ns[name] = struct{}{}
	return nil
}
func (m *minimalKube) DeleteNamespace(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.ns, name)
	return nil
}
func (m *minimalKube) ListVMs(_ context.Context, namespace string) ([]vms.VM, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []vms.VM
	for kk, v := range m.store {
		if strings.HasPrefix(kk, namespace+"/") {
			out = append(out, *v)
		}
	}
	return out, nil
}
func (m *minimalKube) GetVM(_ context.Context, ns, n string) (*vms.VM, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.store[k(ns, n)]
	if !ok {
		return nil, vms.ErrNotFound
	}
	cp := *v
	return &cp, nil
}
func (m *minimalKube) CreateVM(_ context.Context, vm *vms.VM, _ vms.VMCloudInit, _ string) (*vms.VM, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *vm
	m.store[k(vm.Namespace, vm.Name)] = &cp
	return &cp, nil
}
func (m *minimalKube) PatchVM(_ context.Context, ns, n string, p vms.PatchRequest) (*vms.VM, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.store[k(ns, n)]
	if !ok {
		return nil, vms.ErrNotFound
	}
	if p.CPU != nil {
		v.CPU = *p.CPU
	}
	if p.MemoryMB != nil {
		v.MemoryMB = *p.MemoryMB
	}
	cp := *v
	return &cp, nil
}
func (m *minimalKube) DeleteVM(_ context.Context, ns, n string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, k(ns, n))
	return nil
}
func (m *minimalKube) SetVMRunning(_ context.Context, ns, n string, running bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.store[k(ns, n)]
	if !ok {
		return vms.ErrNotFound
	}
	v.Running = running
	if running {
		v.Phase = vms.PhaseRunning
	} else {
		v.Phase = vms.PhaseStopped
	}
	return nil
}
func (m *minimalKube) RestartVM(_ context.Context, _, _ string) error { return nil }
func (m *minimalKube) PauseVM(_ context.Context, _, _ string) error   { return nil }
func (m *minimalKube) UnpauseVM(_ context.Context, _, _ string) error { return nil }
func (m *minimalKube) MigrateVM(_ context.Context, _, _ string) error { return nil }
func (m *minimalKube) CountReadyNodes(_ context.Context) (int, error) { return 1, nil }
func (m *minimalKube) ListSnapshots(_ context.Context, _ string) ([]vms.Snapshot, error) {
	return nil, nil
}
func (m *minimalKube) CreateSnapshot(_ context.Context, s vms.Snapshot) (*vms.Snapshot, error) {
	cp := s
	cp.ReadyToUse = true
	return &cp, nil
}
func (m *minimalKube) DeleteSnapshot(_ context.Context, _, _ string) error { return nil }
func (m *minimalKube) ListRestores(_ context.Context, _ string) ([]vms.Restore, error) {
	return nil, nil
}
func (m *minimalKube) CreateRestore(_ context.Context, r vms.Restore) (*vms.Restore, error) {
	cp := r
	cp.Complete = true
	return &cp, nil
}
func (m *minimalKube) DeleteRestore(_ context.Context, _, _ string) error { return nil }
func (m *minimalKube) MintConsoleToken(_ context.Context, _, _, _ string, ttl time.Duration) (string, time.Time, error) {
	return "tok", time.Now().Add(ttl), nil
}

func newVMTestHandler() *VMsHandler {
	cat := vms.NewCatalogFromTemplates([]vms.Template{{
		ID: "debian-12-cloud", DisplayName: "Debian 12",
		ImageURL:        "https://example/x.qcow2",
		DefaultCPU:      2,
		DefaultMemoryMB: 2048,
		DefaultDiskGB:   20,
	}})
	return &VMsHandler{
		Mgr: &vms.Manager{
			Kube:            newMinimalKube(),
			Templates:       cat,
			VirtAPIBase:     "wss://virt.example",
			NamespacePrefix: "vm-",
			ConsoleTokenTTL: time.Minute,
		},
	}
}

func newRouter(h *VMsHandler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/v1/vms", h.List)
	r.Post("/api/v1/vms", h.Create)
	r.Get("/api/v1/vms/{namespace}/{name}", h.Get)
	r.Patch("/api/v1/vms/{namespace}/{name}", h.Patch)
	r.Delete("/api/v1/vms/{namespace}/{name}", h.Delete)
	r.Post("/api/v1/vms/{namespace}/{name}/start", h.Start)
	r.Post("/api/v1/vms/{namespace}/{name}/stop", h.Stop)
	r.Post("/api/v1/vms/{namespace}/{name}/migrate", h.Migrate)
	r.Get("/api/v1/vms/{namespace}/{name}/console", h.Console)
	r.Get("/api/v1/vm-templates", h.ListTemplates)
	r.Post("/api/v1/vm-snapshots", h.CreateSnapshot)
	r.Post("/api/v1/vm-restores", h.CreateRestore)
	return r
}

func do(t *testing.T, r chi.Router, method, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	return resp, rb
}

func TestVMs_503WhenManagerNil(t *testing.T) {
	h := &VMsHandler{}
	r := newRouter(h)
	resp, _ := do(t, r, http.MethodGet, "/api/v1/vms", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestVMs_CreateGetDelete(t *testing.T) {
	h := newVMTestHandler()
	r := newRouter(h)
	resp, body := do(t, r, http.MethodPost, "/api/v1/vms", vms.CreateRequest{
		Name: "alpha", TemplateID: "debian-12-cloud",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	resp, body = do(t, r, http.MethodGet, "/api/v1/vms/vm-alpha/alpha", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: %d %s", resp.StatusCode, body)
	}
	var vm vms.VM
	_ = json.Unmarshal(body, &vm)
	if vm.Name != "alpha" || vm.CPU != 2 {
		t.Fatalf("vm: %+v", vm)
	}
	resp, _ = do(t, r, http.MethodDelete, "/api/v1/vms/vm-alpha/alpha", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	resp, _ = do(t, r, http.MethodGet, "/api/v1/vms/vm-alpha/alpha", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("post-delete get: %d", resp.StatusCode)
	}
}

func TestVMs_StartStop(t *testing.T) {
	h := newVMTestHandler()
	r := newRouter(h)
	if resp, _ := do(t, r, http.MethodPost, "/api/v1/vms", vms.CreateRequest{Name: "b", TemplateID: "debian-12-cloud"}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	if resp, _ := do(t, r, http.MethodPost, "/api/v1/vms/vm-b/b/start", nil); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("start: %d", resp.StatusCode)
	}
	resp, body := do(t, r, http.MethodGet, "/api/v1/vms/vm-b/b", nil)
	var vm vms.VM
	_ = json.Unmarshal(body, &vm)
	if !vm.Running || vm.Phase != vms.PhaseRunning {
		t.Fatalf("vm not running: %+v (%d)", vm, resp.StatusCode)
	}
}

func TestVMs_MigrateSingleNode_501(t *testing.T) {
	h := newVMTestHandler()
	r := newRouter(h)
	_, _ = do(t, r, http.MethodPost, "/api/v1/vms", vms.CreateRequest{Name: "m", TemplateID: "debian-12-cloud"})
	resp, _ := do(t, r, http.MethodPost, "/api/v1/vms/vm-m/m/migrate", nil)
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("migrate: %d", resp.StatusCode)
	}
}

func TestVMs_ConsoleRequiresRunning(t *testing.T) {
	h := newVMTestHandler()
	r := newRouter(h)
	_, _ = do(t, r, http.MethodPost, "/api/v1/vms", vms.CreateRequest{Name: "c", TemplateID: "debian-12-cloud"})
	resp, _ := do(t, r, http.MethodGet, "/api/v1/vms/vm-c/c/console", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("console stopped vm: %d", resp.StatusCode)
	}
	_, _ = do(t, r, http.MethodPost, "/api/v1/vms/vm-c/c/start", nil)
	resp, body := do(t, r, http.MethodGet, "/api/v1/vms/vm-c/c/console", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("console running: %d %s", resp.StatusCode, body)
	}
	var cs vms.ConsoleSession
	_ = json.Unmarshal(body, &cs)
	if cs.Token == "" || cs.WSURL == "" {
		t.Fatalf("console body: %+v", cs)
	}
}

func TestVMs_Templates(t *testing.T) {
	h := newVMTestHandler()
	r := newRouter(h)
	resp, body := do(t, r, http.MethodGet, "/api/v1/vm-templates", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("templates: %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "debian-12-cloud") {
		t.Fatalf("body: %s", body)
	}
}

func TestVMs_TranslateError(t *testing.T) {
	w := httptest.NewRecorder()
	translateError(w, vms.ErrNotFound)
	if w.Code != http.StatusNotFound {
		t.Fatalf("not found: %d", w.Code)
	}
	w = httptest.NewRecorder()
	translateError(w, errors.New("boom"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("internal: %d", w.Code)
	}
}
