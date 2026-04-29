package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/krb5"
)

type fakeKDC struct {
	status      *krb5.KDCStatus
	statusErr   error
	listOut     []string
	listErr     error
	getOut      *krb5.PrincipalInfo
	getErr      error
	createOut   *krb5.PrincipalInfo
	createErr   error
	createSpec  krb5.CreatePrincipalSpec
	deleteName  string
	deleteErr   error
	keytabBytes []byte
	keytabErr   error
}

func (f *fakeKDC) Status(_ context.Context) (*krb5.KDCStatus, error) {
	return f.status, f.statusErr
}
func (f *fakeKDC) ListPrincipals(_ context.Context) ([]string, error) {
	return f.listOut, f.listErr
}
func (f *fakeKDC) GetPrincipal(_ context.Context, name string) (*krb5.PrincipalInfo, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.getOut != nil && f.getOut.Name == "" {
		f.getOut.Name = name
	}
	return f.getOut, nil
}
func (f *fakeKDC) CreatePrincipal(_ context.Context, spec krb5.CreatePrincipalSpec) (*krb5.PrincipalInfo, error) {
	f.createSpec = spec
	return f.createOut, f.createErr
}
func (f *fakeKDC) DeletePrincipal(_ context.Context, name string) error {
	f.deleteName = name
	return f.deleteErr
}
func (f *fakeKDC) GenerateKeytab(_ context.Context, _ string, _ string) ([]byte, error) {
	return f.keytabBytes, f.keytabErr
}

func newKDCHandler(f *fakeKDC) *Krb5KDCHandler {
	return &Krb5KDCHandler{Logger: newDiscardLogger(), KDC: f}
}

func mountKDC(h *Krb5KDCHandler) http.Handler {
	r := chi.NewRouter()
	r.Get("/krb5/kdc/status", h.GetStatus)
	r.Get("/krb5/principals", h.ListPrincipals)
	r.Post("/krb5/principals", h.CreatePrincipal)
	r.Get("/krb5/principals/{name}", h.GetPrincipal)
	r.Delete("/krb5/principals/{name}", h.DeletePrincipal)
	r.Post("/krb5/principals/{name}/keytab", h.GetPrincipalKeytab)
	return r
}

func TestKDC_GetStatus(t *testing.T) {
	f := &fakeKDC{status: &krb5.KDCStatus{Realm: "T.LOCAL", Running: true, PrincipalCount: 3}}
	srv := mountKDC(newKDCHandler(f))
	req := httptest.NewRequest(http.MethodGet, "/krb5/kdc/status", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	var got krb5.KDCStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Realm != "T.LOCAL" || !got.Running || got.PrincipalCount != 3 {
		t.Errorf("got %+v", got)
	}
}

func TestKDC_ListPrincipals_NeverNullJSON(t *testing.T) {
	f := &fakeKDC{listOut: nil}
	srv := mountKDC(newKDCHandler(f))
	req := httptest.NewRequest(http.MethodGet, "/krb5/principals", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "[]") {
		t.Errorf("expected [] in body, got %q", rr.Body.String())
	}
}

func TestKDC_GetPrincipal_404(t *testing.T) {
	f := &fakeKDC{getErr: fs.ErrNotExist}
	srv := mountKDC(newKDCHandler(f))
	req := httptest.NewRequest(http.MethodGet, "/krb5/principals/nfs%2Fhost", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestKDC_CreatePrincipal_201(t *testing.T) {
	f := &fakeKDC{createOut: &krb5.PrincipalInfo{Name: "nfs/host@R", KVNO: 1}}
	srv := mountKDC(newKDCHandler(f))
	body := []byte(`{"name":"nfs/host","randkey":true}`)
	req := httptest.NewRequest(http.MethodPost, "/krb5/principals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	if f.createSpec.Name != "nfs/host" || !f.createSpec.Randkey {
		t.Errorf("spec=%+v", f.createSpec)
	}
}

func TestKDC_CreatePrincipal_RejectsBoth(t *testing.T) {
	f := &fakeKDC{}
	srv := mountKDC(newKDCHandler(f))
	body := []byte(`{"name":"x","randkey":true,"password":"p"}`)
	req := httptest.NewRequest(http.MethodPost, "/krb5/principals", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestKDC_CreatePrincipal_KDCError(t *testing.T) {
	f := &fakeKDC{createErr: errors.New("kadmin add_principal: exists")}
	srv := mountKDC(newKDCHandler(f))
	body := []byte(`{"name":"x"}`)
	req := httptest.NewRequest(http.MethodPost, "/krb5/principals", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestKDC_DeletePrincipal_204(t *testing.T) {
	f := &fakeKDC{}
	srv := mountKDC(newKDCHandler(f))
	req := httptest.NewRequest(http.MethodDelete, "/krb5/principals/alice", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rr.Code)
	}
	if f.deleteName != "alice" {
		t.Errorf("name=%q", f.deleteName)
	}
}

func TestKDC_GetPrincipalKeytab_OctetStream(t *testing.T) {
	f := &fakeKDC{keytabBytes: []byte{0x05, 0x02, 0x00}}
	srv := mountKDC(newKDCHandler(f))
	req := httptest.NewRequest(http.MethodPost, "/krb5/principals/alice/keytab", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("content-type=%q", got)
	}
	if !bytes.Equal(rr.Body.Bytes(), []byte{0x05, 0x02, 0x00}) {
		t.Errorf("body=%v", rr.Body.Bytes())
	}
}

func TestKDC_GetPrincipalKeytab_404(t *testing.T) {
	f := &fakeKDC{keytabErr: fs.ErrNotExist}
	srv := mountKDC(newKDCHandler(f))
	req := httptest.NewRequest(http.MethodPost, "/krb5/principals/nope/keytab", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}
