package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/network"
)

type fakeNetworkReader struct {
	live    []network.LiveInterface
	configs []network.ManagedConfig
	get     *network.ManagedConfig
	getErr  error
}

func (f *fakeNetworkReader) ListInterfaces(_ context.Context) ([]network.LiveInterface, error) {
	return f.live, nil
}
func (f *fakeNetworkReader) ListConfigs(_ context.Context) ([]network.ManagedConfig, error) {
	return f.configs, nil
}
func (f *fakeNetworkReader) GetConfig(_ context.Context, _ string) (*network.ManagedConfig, error) {
	return f.get, f.getErr
}

func TestNetworkListInterfaces_Returns200(t *testing.T) {
	mgr := &fakeNetworkReader{live: []network.LiveInterface{{Name: "eth0", State: "UP"}}}
	h := &NetworkHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/interfaces", nil)
	rr := httptest.NewRecorder()
	h.ListInterfaces(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestNetworkGetConfig_NotFound(t *testing.T) {
	mgr := &fakeNetworkReader{getErr: network.ErrNotFound}
	h := &NetworkHandler{Logger: newDiscardLogger(), Mgr: mgr}
	r := chi.NewRouter()
	r.Get("/api/v1/network/configs/{name}", h.GetConfig)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/configs/missing", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestNetworkListConfigs_Returns200(t *testing.T) {
	mgr := &fakeNetworkReader{configs: []network.ManagedConfig{{Name: "eth0", Kind: network.KindInterface}}}
	h := &NetworkHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/configs", nil)
	rr := httptest.NewRecorder()
	h.ListConfigs(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}
