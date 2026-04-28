package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/novanas/nova-nas/internal/host/krb5"
)

type fakeKrb5Reader struct {
	cfg    *krb5.Config
	idmapd *krb5.IdmapdConfig
	keytab []krb5.KeytabEntry
}

func (f *fakeKrb5Reader) GetConfig(_ context.Context) (*krb5.Config, error) {
	return f.cfg, nil
}
func (f *fakeKrb5Reader) GetIdmapdConfig(_ context.Context) (*krb5.IdmapdConfig, error) {
	return f.idmapd, nil
}
func (f *fakeKrb5Reader) ListKeytab(_ context.Context) ([]krb5.KeytabEntry, error) {
	return f.keytab, nil
}

func TestKrb5GetConfig_Returns200(t *testing.T) {
	mgr := &fakeKrb5Reader{cfg: &krb5.Config{DefaultRealm: "EXAMPLE.COM", Realms: map[string]krb5.Realm{}}}
	h := &Krb5Handler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/krb5/config", nil)
	rr := httptest.NewRecorder()
	h.GetConfig(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got krb5.Config
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DefaultRealm != "EXAMPLE.COM" {
		t.Errorf("got %+v", got)
	}
}

func TestKrb5GetIdmapd_Returns200(t *testing.T) {
	mgr := &fakeKrb5Reader{idmapd: &krb5.IdmapdConfig{Domain: "example.com"}}
	h := &Krb5Handler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/krb5/idmapd", nil)
	rr := httptest.NewRecorder()
	h.GetIdmapd(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestKrb5ListKeytab_Returns200_NoSecrets(t *testing.T) {
	mgr := &fakeKrb5Reader{keytab: []krb5.KeytabEntry{
		{KVNO: 2, Principal: "nfs/host.example.com@EXAMPLE.COM", Encryption: "aes256-cts-hmac-sha1-96"},
	}}
	h := &Krb5Handler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/krb5/keytab", nil)
	rr := httptest.NewRecorder()
	h.ListKeytab(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []krb5.KeytabEntry
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	if len(got) != 1 || got[0].Principal != "nfs/host.example.com@EXAMPLE.COM" {
		t.Errorf("got %+v", got)
	}
}
