package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/samba"
)

type fakeSambaReader struct {
	shares []samba.Share
	get    *samba.Share
	getErr error
	users  []samba.User
}

func (f *fakeSambaReader) ListShares(_ context.Context) ([]samba.Share, error) {
	return f.shares, nil
}
func (f *fakeSambaReader) GetShare(_ context.Context, _ string) (*samba.Share, error) {
	return f.get, f.getErr
}
func (f *fakeSambaReader) ListUsers(_ context.Context) ([]samba.User, error) {
	return f.users, nil
}

func TestSambaListShares_Returns200(t *testing.T) {
	mgr := &fakeSambaReader{shares: []samba.Share{{Name: "data", Path: "/tank/data"}}}
	h := &SambaHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/samba/shares", nil)
	rr := httptest.NewRecorder()
	h.ListShares(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []samba.Share
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "data" {
		t.Errorf("got %+v", got)
	}
}

func TestSambaGetShare_NotFound(t *testing.T) {
	mgr := &fakeSambaReader{getErr: samba.ErrNotFound}
	h := &SambaHandler{Logger: newDiscardLogger(), Mgr: mgr}
	r := chi.NewRouter()
	r.Get("/api/v1/samba/shares/{name}", h.GetShare)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/samba/shares/missing", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestSambaListUsers_Returns200(t *testing.T) {
	mgr := &fakeSambaReader{users: []samba.User{{Username: "alice"}}}
	h := &SambaHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/samba/users", nil)
	rr := httptest.NewRecorder()
	h.ListUsers(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}
