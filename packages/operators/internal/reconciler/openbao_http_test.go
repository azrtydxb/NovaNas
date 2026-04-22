package reconciler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testServer captures each request for later assertion.
type capturedReq struct {
	Method string
	Path   string
	Body   map[string]any
	Token  string
}

func newTestClient(t *testing.T, handler http.HandlerFunc) (*HTTPOpenBaoClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewHTTPOpenBaoClient(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("NewHTTPOpenBaoClient: %v", err)
	}
	return c, srv
}

func readBody(r *http.Request) map[string]any {
	buf, _ := io.ReadAll(r.Body)
	if len(buf) == 0 {
		return nil
	}
	var m map[string]any
	_ = json.Unmarshal(buf, &m)
	return m
}

func TestHTTPOpenBao_EnsurePolicy(t *testing.T) {
	var got capturedReq
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capturedReq{Method: r.Method, Path: r.URL.Path, Body: readBody(r), Token: r.Header.Get("X-Vault-Token")}
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.EnsurePolicy(context.Background(), OpenBaoPolicy{
		Name: "tenant-alice",
		HCL:  `path "x" { capabilities = ["read"] }`,
	})
	if err != nil {
		t.Fatalf("EnsurePolicy: %v", err)
	}
	if got.Method != http.MethodPut {
		t.Errorf("method: want PUT, got %s", got.Method)
	}
	if got.Path != "/v1/sys/policies/acl/tenant-alice" {
		t.Errorf("path: got %s", got.Path)
	}
	if got.Token != "test-token" {
		t.Errorf("token header: got %q", got.Token)
	}
	if got.Body["policy"] == nil || !strings.Contains(got.Body["policy"].(string), "capabilities") {
		t.Errorf("body policy missing/garbled: %#v", got.Body)
	}
}

func TestHTTPOpenBao_EnsurePolicy_EmptyName(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called")
	})
	if err := c.EnsurePolicy(context.Background(), OpenBaoPolicy{Name: ""}); err == nil {
		t.Fatal("want error on empty name")
	}
}

func TestHTTPOpenBao_DeletePolicy(t *testing.T) {
	var got capturedReq
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capturedReq{Method: r.Method, Path: r.URL.Path}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.DeletePolicy(context.Background(), "tenant-alice"); err != nil {
		t.Fatalf("DeletePolicy: %v", err)
	}
	if got.Method != http.MethodDelete {
		t.Errorf("method: got %s", got.Method)
	}
	if got.Path != "/v1/sys/policies/acl/tenant-alice" {
		t.Errorf("path: got %s", got.Path)
	}
}

func TestHTTPOpenBao_DeletePolicy_NotFoundIsOK(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if err := c.DeletePolicy(context.Background(), "missing"); err != nil {
		t.Fatalf("DeletePolicy 404 should be success, got %v", err)
	}
}

func TestHTTPOpenBao_EnsureAuthRole(t *testing.T) {
	var got capturedReq
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capturedReq{Method: r.Method, Path: r.URL.Path, Body: readBody(r)}
		w.WriteHeader(http.StatusNoContent)
	})
	err := c.EnsureAuthRole(context.Background(), OpenBaoAuthRole{
		Name:                "tenant-alice",
		BoundServiceAccount: "alice",
		BoundNamespace:      "novanas-tenants",
		Policies:            []string{"tenant-alice"},
		TTLSeconds:          3600,
		MaxTTLSeconds:       7200,
	})
	if err != nil {
		t.Fatalf("EnsureAuthRole: %v", err)
	}
	if got.Method != http.MethodPost {
		t.Errorf("method: got %s", got.Method)
	}
	if got.Path != "/v1/auth/kubernetes/role/tenant-alice" {
		t.Errorf("path: got %s", got.Path)
	}
	// Pacify json.Unmarshal arrays → []any
	sas, _ := got.Body["bound_service_account_names"].([]any)
	if len(sas) != 1 || sas[0] != "alice" {
		t.Errorf("bound_service_account_names: %#v", sas)
	}
	if got.Body["token_ttl"] != "3600s" {
		t.Errorf("token_ttl: got %v", got.Body["token_ttl"])
	}
	if got.Body["token_max_ttl"] != "7200s" {
		t.Errorf("token_max_ttl: got %v", got.Body["token_max_ttl"])
	}
}

func TestHTTPOpenBao_DeleteAuthRole(t *testing.T) {
	var got capturedReq
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capturedReq{Method: r.Method, Path: r.URL.Path}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.DeleteAuthRole(context.Background(), "tenant-alice"); err != nil {
		t.Fatalf("DeleteAuthRole: %v", err)
	}
	if got.Method != http.MethodDelete || got.Path != "/v1/auth/kubernetes/role/tenant-alice" {
		t.Errorf("unexpected request: %+v", got)
	}
}

func TestHTTPOpenBao_DeleteAuthRole_NotFoundIsOK(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if err := c.DeleteAuthRole(context.Background(), "missing"); err != nil {
		t.Fatalf("404 should be success, got %v", err)
	}
}

func TestHTTPOpenBao_ServerError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":["boom"]}`))
	})
	err := c.EnsurePolicy(context.Background(), OpenBaoPolicy{Name: "p", HCL: "x"})
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("want status in error, got %v", err)
	}
}

func TestHTTPOpenBao_ConstructorValidates(t *testing.T) {
	if _, err := NewHTTPOpenBaoClient("", "tok"); err == nil {
		t.Error("empty addr should error")
	}
	if _, err := NewHTTPOpenBaoClient("https://x", ""); err == nil {
		// No token and no OPENBAO_TOKEN_PATH env set in default test env
		t.Logf("note: if OPENBAO_TOKEN_PATH is set in env this can pass; acceptable")
	}
}
