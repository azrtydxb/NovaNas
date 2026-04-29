package novanas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInitializeDatasetEncryption(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		// "tank/secret" path-escaped → "tank%2Fsecret".
		if r.URL.EscapedPath() != "/api/v1/datasets/tank%2Fsecret/encryption" {
			t.Errorf("path=%s", r.URL.EscapedPath())
		}
		var body EncryptionInitRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Type != "filesystem" || body.Algorithm != "aes-256-gcm" {
			t.Errorf("body=%+v", body)
		}
		writeJSON(t, w, http.StatusCreated, EncryptionInitResponse{
			Dataset:   "tank/secret",
			Algorithm: "aes-256-gcm",
			Created:   "2026-04-29T12:00:00Z",
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	out, err := c.InitializeDatasetEncryption(context.Background(), "tank/secret",
		EncryptionInitRequest{Type: "filesystem", Algorithm: "aes-256-gcm"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Dataset != "tank/secret" {
		t.Errorf("got %+v", out)
	}
}

func TestLoadUnloadDatasetEncryptionKey(t *testing.T) {
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.EscapedPath()]++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	if err := c.LoadDatasetEncryptionKey(context.Background(), "tank/x"); err != nil {
		t.Fatal(err)
	}
	if err := c.UnloadDatasetEncryptionKey(context.Background(), "tank/x"); err != nil {
		t.Fatal(err)
	}
	if hits["/api/v1/datasets/tank%2Fx/encryption/load-key"] != 1 {
		t.Errorf("missing load-key hit: %v", hits)
	}
	if hits["/api/v1/datasets/tank%2Fx/encryption/unload-key"] != 1 {
		t.Errorf("missing unload-key hit: %v", hits)
	}
}

func TestRecoverDatasetEncryptionKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/v1/datasets/tank%2Fsecret/encryption/recover" {
			t.Errorf("path=%s", r.URL.EscapedPath())
		}
		writeJSON(t, w, http.StatusOK, EncryptionRecoverResponse{
			Dataset: "tank/secret",
			KeyHex:  "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
		})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	out, err := c.RecoverDatasetEncryptionKey(context.Background(), "tank/secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.KeyHex) != 64 {
		t.Errorf("keyHex len=%d", len(out.KeyHex))
	}
}
