package hostagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRevision_Deterministic(t *testing.T) {
	a := Revision([]byte("hello"))
	b := Revision([]byte("hello"))
	if a != b {
		t.Fatalf("revision not stable: %s vs %s", a, b)
	}
	if a == Revision([]byte("world")) {
		t.Fatalf("expected different revision for different input")
	}
}

func TestClient_Unconfigured(t *testing.T) {
	var c *Client // nil
	if _, err := c.ApplyNmstate(context.Background(), []byte("foo")); err == nil {
		t.Fatal("expected error from nil client")
	}
	zero := &Client{}
	rev, err := zero.ApplyNmstate(context.Background(), []byte("foo"))
	if err != ErrNotConfigured {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
	if rev == "" {
		t.Fatal("expected local revision fallback")
	}
}

func TestClient_ApplyNmstate_ServerOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/network/nmstate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	rev, err := c.ApplyNmstate(context.Background(), []byte("interfaces: []\n"))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if rev == "" {
		t.Fatal("empty revision")
	}
}

func TestClient_InstallFirewallRules_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer srv.Close()
	c := New(srv.URL)
	if _, err := c.InstallFirewallRules(context.Background(), []byte("rule")); err == nil {
		t.Fatal("expected error on 400")
	}
}
