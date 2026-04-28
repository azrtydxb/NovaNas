package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/jobs"
)

// fakeImportableLister is a test double for ImportableLister.
type fakeImportableLister struct {
	pools []pool.ImportablePool
	err   error
}

func (f *fakeImportableLister) Importable(_ context.Context) ([]pool.ImportablePool, error) {
	return f.pools, f.err
}

// routedHandler wires a single POST /pools/{name}/<sub> into chi so that
// chi.URLParam resolves correctly.
func routedHandler(method, pattern string, handlerFn http.HandlerFunc) http.Handler {
	r := chi.NewRouter()
	r.Method(method, pattern, handlerFn)
	return r
}

func newLifecycleHandler(disp *fakeDispatcher) *PoolsWriteHandler {
	return &PoolsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
}

// ---------------------------------------------------------------------------
// Replace
// ---------------------------------------------------------------------------

func TestPoolsReplace_Returns202(t *testing.T) {
	id := uuid.New()
	disp := &fakeDispatcher{out: id}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/replace", h.Replace)
	body := `{"oldDisk":"/dev/sda","newDisk":"/dev/sdb"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/replace", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(disp.calls) != 1 || disp.calls[0].Kind != jobs.KindPoolReplace {
		t.Errorf("dispatch=%+v", disp.calls)
	}
	p, ok := disp.calls[0].Payload.(jobs.PoolReplacePayload)
	if !ok || p.Name != "tank" || p.OldDisk != "/dev/sda" || p.NewDisk != "/dev/sdb" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
	if disp.calls[0].UniqueKey != "pool:tank" {
		t.Errorf("UniqueKey=%q", disp.calls[0].UniqueKey)
	}
}

// ---------------------------------------------------------------------------
// Offline
// ---------------------------------------------------------------------------

func TestPoolsOffline_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/offline", h.Offline)
	body := `{"disk":"/dev/sda","temporary":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/offline", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindPoolOffline {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.PoolOfflinePayload)
	if !ok || p.Disk != "/dev/sda" || !p.Temporary {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

// ---------------------------------------------------------------------------
// Online
// ---------------------------------------------------------------------------

func TestPoolsOnline_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/online", h.Online)
	body := `{"disk":"/dev/sda"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/online", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindPoolOnline {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.PoolOnlinePayload)
	if !ok || p.Name != "tank" || p.Disk != "/dev/sda" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

// ---------------------------------------------------------------------------
// Clear
// ---------------------------------------------------------------------------

func TestPoolsClear_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/clear", h.Clear)
	body := `{"disk":"/dev/sda"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/clear", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindPoolClear {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.PoolClearPayload)
	if !ok || p.Name != "tank" || p.Disk != "/dev/sda" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestPoolsClear_DiskOptional(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/clear", h.Clear)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/clear", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	p := disp.calls[0].Payload.(jobs.PoolClearPayload)
	if p.Disk != "" {
		t.Errorf("disk should be empty, got %q", p.Disk)
	}
}

// ---------------------------------------------------------------------------
// Attach
// ---------------------------------------------------------------------------

func TestPoolsAttach_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/attach", h.Attach)
	body := `{"existing":"/dev/sda","newDisk":"/dev/sdb"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/attach", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindPoolAttach {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.PoolAttachPayload)
	if !ok || p.Existing != "/dev/sda" || p.NewDisk != "/dev/sdb" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

// ---------------------------------------------------------------------------
// Detach
// ---------------------------------------------------------------------------

func TestPoolsDetach_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/detach", h.Detach)
	body := `{"disk":"/dev/sda"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/detach", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindPoolDetach {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.PoolDetachPayload)
	if !ok || p.Name != "tank" || p.Disk != "/dev/sda" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

// ---------------------------------------------------------------------------
// Add
// ---------------------------------------------------------------------------

func TestPoolsAdd_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/add", h.Add)
	body := `{"vdevs":[{"type":"mirror","disks":["/dev/sdc","/dev/sdd"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/add", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindPoolAdd {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.PoolAddPayload)
	if !ok || p.Name != "tank" || len(p.Spec.Vdevs) != 1 {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

// ---------------------------------------------------------------------------
// Export
// ---------------------------------------------------------------------------

func TestPoolsExport_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/export", h.Export)
	body := `{"force":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/export", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindPoolExport {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.PoolExportPayload)
	if !ok || p.Name != "tank" || !p.Force {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

// ---------------------------------------------------------------------------
// Import (POST)
// ---------------------------------------------------------------------------

func TestPoolsImport_Returns202(t *testing.T) {
	id := uuid.New()
	disp := &fakeDispatcher{out: id}
	h := newLifecycleHandler(disp)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/import", bytes.NewBufferString(`{"name":"tank"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(disp.calls) != 1 || disp.calls[0].Kind != jobs.KindPoolImport {
		t.Errorf("dispatch=%+v", disp.calls)
	}
	p, ok := disp.calls[0].Payload.(jobs.PoolImportPayload)
	if !ok || p.Name != "tank" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
	if disp.calls[0].UniqueKey != "pool:tank" {
		t.Errorf("UniqueKey=%q", disp.calls[0].UniqueKey)
	}
	if rr.Header().Get("Location") != "/api/v1/jobs/"+id.String() {
		t.Errorf("Location=%q", rr.Header().Get("Location"))
	}
}

func TestPoolsImport_BadName400(t *testing.T) {
	disp := &fakeDispatcher{}
	h := newLifecycleHandler(disp)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/import", bytes.NewBufferString(`{"name":"bad/name"}`))
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch on bad name")
	}
}

func TestPoolsImport_BadJSON400(t *testing.T) {
	disp := &fakeDispatcher{}
	h := newLifecycleHandler(disp)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/import", bytes.NewBufferString("not json"))
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Importable (GET) — uses stub, no dispatcher
// ---------------------------------------------------------------------------

func TestPoolsImportable_ReturnsList(t *testing.T) {
	lister := &fakeImportableLister{
		pools: []pool.ImportablePool{
			{Name: "backup", State: "ONLINE"},
			{Name: "archive", State: "DEGRADED", Status: "some issue"},
		},
	}
	h := &PoolsWriteHandler{Logger: newDiscardLogger(), Pools: lister}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools/import", nil)
	rr := httptest.NewRecorder()
	h.Importable(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got []pool.ImportablePool
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "backup" || got[1].State != "DEGRADED" {
		t.Errorf("body=%+v", got)
	}
}

func TestPoolsImportable_EmptyReturnsArray(t *testing.T) {
	h := &PoolsWriteHandler{Logger: newDiscardLogger(), Pools: &fakeImportableLister{pools: nil}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools/import", nil)
	rr := httptest.NewRecorder()
	h.Importable(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	// Must return [] not null
	body := rr.Body.String()
	if body != "[]\n" {
		t.Errorf("want [] got %q", body)
	}
}

func TestPoolsImportable_HostError500(t *testing.T) {
	h := &PoolsWriteHandler{Logger: newDiscardLogger(), Pools: &fakeImportableLister{err: errors.New("zpool gone")}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools/import", nil)
	rr := httptest.NewRecorder()
	h.Importable(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rr.Code)
	}
	var env struct{ Error string `json:"error"` }
	_ = json.NewDecoder(rr.Body).Decode(&env)
	if env.Error != "list_error" {
		t.Errorf("error=%q", env.Error)
	}
}

// ---------------------------------------------------------------------------
// Bad-name 400 (shared validation — tested via Replace as representative)
// ---------------------------------------------------------------------------

func TestPoolsReplace_BadName400(t *testing.T) {
	disp := &fakeDispatcher{}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/replace", h.Replace)
	body := `{"oldDisk":"/dev/sda","newDisk":"/dev/sdb"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/123invalid/replace", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch on bad name")
	}
}
