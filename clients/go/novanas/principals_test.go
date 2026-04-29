package novanas

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetKDCStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/krb5/kdc/status" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, http.StatusOK, KDCStatus{Realm: "T.LOCAL", Running: true, PrincipalCount: 5})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	st, err := c.GetKDCStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Realm != "T.LOCAL" || !st.Running || st.PrincipalCount != 5 {
		t.Errorf("got %+v", st)
	}
}

func TestListPrincipals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, []string{"a@R", "b@R"})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	got, err := c.ListPrincipals(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a@R" {
		t.Errorf("got %v", got)
	}
}

func TestCreatePrincipal_SerializesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/krb5/principals" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		var body CreatePrincipalSpec
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Name != "nfs/h" || !body.Randkey {
			t.Errorf("body=%+v", body)
		}
		writeJSON(t, w, http.StatusCreated, Principal{Name: "nfs/h@R", KVNO: 1})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	got, err := c.CreatePrincipal(context.Background(), CreatePrincipalSpec{Name: "nfs/h", Randkey: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "nfs/h@R" {
		t.Errorf("got %+v", got)
	}
}

func TestCreatePrincipal_RejectsBoth(t *testing.T) {
	c, _ := New(Config{BaseURL: "http://x"})
	if _, err := c.CreatePrincipal(context.Background(), CreatePrincipalSpec{Name: "x", Randkey: true, Password: "p"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestDeletePrincipal_204(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method=%s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	if err := c.DeletePrincipal(context.Background(), "alice"); err != nil {
		t.Fatal(err)
	}
}

func TestGetPrincipalKeytab(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/krb5/principals/alice/keytab" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0x05, 0x02, 0x01, 0x02})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	got, err := c.GetPrincipalKeytab(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{0x05, 0x02, 0x01, 0x02}) {
		t.Errorf("got %v", got)
	}
}

func TestGetPrincipalKeytab_RejectsBadMagic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{0x00, 0x00})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	if _, err := c.GetPrincipalKeytab(context.Background(), "alice"); err == nil {
		t.Fatal("expected error for bad magic")
	}
}
