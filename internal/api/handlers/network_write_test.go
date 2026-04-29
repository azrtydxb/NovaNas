package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/network"
	"github.com/novanas/nova-nas/internal/jobs"
)

// stubIPRunner returns a fixed `ip -j addr show` JSON. The default
// fixture has eth0 owning 10.0.0.5/24 — making 10.0.0.5 the
// "management iface" source for guard tests.
func stubIPRunner(json string) func(ctx context.Context, bin string, args ...string) ([]byte, error) {
	return func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte(json), nil
	}
}

const ipFixtureEth0 = `[
  {"ifname":"eth0","link_type":"ether","address":"aa:bb:cc:dd:ee:ff","operstate":"UP","addr_info":[{"family":"inet","local":"10.0.0.5","prefixlen":24}]},
  {"ifname":"eth1","link_type":"ether","address":"aa:bb:cc:dd:ee:00","operstate":"UP","addr_info":[{"family":"inet","local":"192.168.1.5","prefixlen":24}]}
]`

func newStubNetworkMgr(fixture string) *network.Manager {
	return &network.Manager{Runner: stubIPRunner(fixture)}
}

func TestNetworkApplyInterface_GuardRefusesManagementIface(t *testing.T) {
	disp := &fakeDispatcher{}
	mgr := newStubNetworkMgr(ipFixtureEth0)
	h := &NetworkWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp, Mgr: mgr}

	// Source IP 10.0.0.5 → eth0 is "management". A request that
	// touches eth0 without force=true must be refused with 400 and
	// code "management_interface_protected".
	body := `{"name":"eth0","matchName":"eth0","dhcp":"yes"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/network/configs", bytes.NewBufferString(body))
	req.RemoteAddr = "10.0.0.5:54321"
	rr := httptest.NewRecorder()
	h.ApplyInterface(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("management_interface_protected")) {
		t.Errorf("expected management_interface_protected error, got %s", rr.Body.String())
	}
	if len(disp.calls) != 0 {
		t.Errorf("must not dispatch when guard fails")
	}
}

func TestNetworkApplyInterface_ForceBypassesGuard(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	mgr := newStubNetworkMgr(ipFixtureEth0)
	h := &NetworkWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp, Mgr: mgr}

	body := `{"name":"eth0","matchName":"eth0","dhcp":"yes"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/network/configs?force=true", bytes.NewBufferString(body))
	req.RemoteAddr = "10.0.0.5:54321"
	rr := httptest.NewRecorder()
	h.ApplyInterface(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindNetworkInterfaceApply {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestNetworkApplyInterface_NonManagementIfaceProceeds(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	mgr := newStubNetworkMgr(ipFixtureEth0)
	h := &NetworkWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp, Mgr: mgr}

	// Source 10.0.0.5 → mgmt = eth0. Touching eth1 is fine without force.
	body := `{"name":"eth1","matchName":"eth1","dhcp":"yes"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/network/configs", bytes.NewBufferString(body))
	req.RemoteAddr = "10.0.0.5:54321"
	rr := httptest.NewRecorder()
	h.ApplyInterface(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNetworkApplyInterface_LoopbackSourceRequiresForce(t *testing.T) {
	disp := &fakeDispatcher{}
	mgr := newStubNetworkMgr(ipFixtureEth0)
	h := &NetworkWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp, Mgr: mgr}

	body := `{"name":"eth1","matchName":"eth1","dhcp":"yes"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/network/configs", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:54321"
	rr := httptest.NewRecorder()
	h.ApplyInterface(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNetworkDeleteInterface_GuardRefusesManagementIface(t *testing.T) {
	disp := &fakeDispatcher{}
	mgr := newStubNetworkMgr(ipFixtureEth0)
	h := &NetworkWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp, Mgr: mgr}
	r := chi.NewRouter()
	r.Delete("/api/v1/network/configs/{name}", h.DeleteInterface)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/network/configs/eth0", nil)
	req.RemoteAddr = "10.0.0.5:54321"
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("management_interface_protected")) {
		t.Errorf("expected guard error, got %s", rr.Body.String())
	}
}

func TestNetworkReload_RequiresForce(t *testing.T) {
	disp := &fakeDispatcher{}
	mgr := newStubNetworkMgr(ipFixtureEth0)
	h := &NetworkWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp, Mgr: mgr}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/network/reload", nil)
	req.RemoteAddr = "10.0.0.5:54321"
	rr := httptest.NewRecorder()
	h.Reload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
	// With force=true the dispatch must succeed.
	disp = &fakeDispatcher{out: uuid.New()}
	h.Dispatcher = disp
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/network/reload?force=true", nil)
	req2.RemoteAddr = "10.0.0.5:54321"
	rr2 := httptest.NewRecorder()
	h.Reload(rr2, req2)
	if rr2.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr2.Code)
	}
}

func TestNetworkApplyVLAN_BadInput(t *testing.T) {
	disp := &fakeDispatcher{}
	mgr := newStubNetworkMgr(ipFixtureEth0)
	h := &NetworkWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp, Mgr: mgr}

	body := `{"name":"vlan10"}` // missing parent
	req := httptest.NewRequest(http.MethodPost, "/api/v1/network/vlans?force=true", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.ApplyVLAN(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}
