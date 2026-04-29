package novanas

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newMarketplacesTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}
	return c, srv
}

func TestClient_ListMarketplaces(t *testing.T) {
	c, srv := newMarketplacesTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/marketplaces" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]Marketplace{{Name: "novanas-official", Locked: true, Enabled: true}})
	})
	defer srv.Close()
	out, err := c.ListMarketplaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || !out[0].Locked {
		t.Errorf("got=%+v", out)
	}
}

func TestClient_AddMarketplace(t *testing.T) {
	c, srv := newMarketplacesTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		var got MarketplaceCreateRequest
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		if got.Name != "truecharts" {
			t.Errorf("name=%s", got.Name)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Marketplace{Name: got.Name, IndexURL: got.IndexURL, Enabled: true})
	})
	defer srv.Close()
	out, err := c.AddMarketplace(context.Background(), MarketplaceCreateRequest{
		Name: "truecharts", IndexURL: "https://example/i.json", TrustKeyURL: "https://example/k.pub",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "truecharts" {
		t.Errorf("got=%+v", out)
	}
}

func TestClient_DeleteMarketplaceLocked(t *testing.T) {
	c, srv := newMarketplacesTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"locked_marketplace"}`))
	})
	defer srv.Close()
	if err := c.DeleteMarketplace(context.Background(), "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Fatal("expected error for 409")
	}
}

func TestClient_RefreshTrustKey(t *testing.T) {
	c, srv := newMarketplacesTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(Marketplace{Name: "x", TrustKeyPEM: "fresh"})
	})
	defer srv.Close()
	out, err := c.RefreshMarketplaceTrustKey(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if out.TrustKeyPEM != "fresh" {
		t.Errorf("got=%+v", out)
	}
}
