package handlers

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/jobs"
)

func TestKrb5SetConfig_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &Krb5WriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"defaultRealm":"EXAMPLE.COM","realms":{"EXAMPLE.COM":{"kdc":["kdc.example.com"]}},"dnsLookupKdc":false,"dnsLookupRealm":false}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/krb5/config", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.SetConfig(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindKrb5SetConfig {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p := disp.calls[0].Payload.(jobs.Krb5SetConfigPayload)
	if p.Config.DefaultRealm != "EXAMPLE.COM" {
		t.Errorf("payload=%+v", p)
	}
}

func TestKrb5SetConfig_RejectsMissingRealm(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &Krb5WriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"realms":{}}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/krb5/config", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.SetConfig(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch")
	}
}

func TestKrb5SetIdmapd_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &Krb5WriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"domain":"example.com","verbosity":0}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/krb5/idmapd", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.SetIdmapd(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindKrb5SetIdmapd {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestKrb5UploadKeytab_Returns202_AndDecodesBase64(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &Krb5WriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	// Real keytabs start with magic byte 0x05; we don't enforce that at the
	// API boundary (the Manager does), but we do verify the bytes round-trip.
	raw := []byte{0x05, 0x02, 0xde, 0xad, 0xbe, 0xef}
	body := `{"data":"` + base64.StdEncoding.EncodeToString(raw) + `"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/krb5/keytab", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.UploadKeytab(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	p := disp.calls[0].Payload.(jobs.Krb5UploadKeytabPayload)
	if !bytes.Equal(p.Data, raw) {
		t.Errorf("payload data=%x want=%x", p.Data, raw)
	}
}

func TestKrb5UploadKeytab_RejectsBadBase64(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &Krb5WriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"data":"!!!not-base64!!!"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/krb5/keytab", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.UploadKeytab(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch")
	}
}

func TestKrb5DeleteKeytab_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &Krb5WriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/krb5/keytab", nil)
	rr := httptest.NewRecorder()
	h.DeleteKeytab(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	if disp.calls[0].Kind != jobs.KindKrb5DeleteKeytab {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}
