package handlers

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// --- Local fakes (mirror those in dataset/encryption_test.go) -------------

type encFakeSealer struct{}

func (encFakeSealer) Seal(p []byte) ([]byte, error) {
	out := make([]byte, len(p)+1)
	out[0] = 0x42
	for i, b := range p {
		out[i+1] = b ^ 0xA5
	}
	return out, nil
}

func (encFakeSealer) Unseal(s []byte) ([]byte, error) {
	if len(s) < 1 || s[0] != 0x42 {
		return nil, errors.New("bad sealed prefix")
	}
	out := make([]byte, len(s)-1)
	for i, b := range s[1:] {
		out[i] = b ^ 0xA5
	}
	return out, nil
}

type encFakeSecrets struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newEncFakeSecrets() *encFakeSecrets { return &encFakeSecrets{m: map[string][]byte{}} }

func (f *encFakeSecrets) Get(_ context.Context, k string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.m[k]
	if !ok {
		return nil, errors.New("not found")
	}
	c := make([]byte, len(v))
	copy(c, v)
	return c, nil
}

func (f *encFakeSecrets) Set(_ context.Context, k string, v []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := make([]byte, len(v))
	copy(c, v)
	f.m[k] = c
	return nil
}

func (f *encFakeSecrets) Delete(_ context.Context, k string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.m, k)
	return nil
}

func (f *encFakeSecrets) List(_ context.Context, prefix string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for k := range f.m {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}

// fakeAuditor records audit insertions for assertion.
type fakeAuditor struct {
	mu      sync.Mutex
	entries []storedb.InsertAuditParams
}

func (f *fakeAuditor) InsertAudit(_ context.Context, p storedb.InsertAuditParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, p)
	return nil
}

// fakeStdinRunner makes zfs calls inert.
func fakeStdinRunner(_ context.Context, _ string, _ []byte, _ ...string) ([]byte, error) {
	return nil, nil
}

// --- Helpers --------------------------------------------------------------

func newEncHandler() (*EncryptionHandler, *fakeAuditor, *encFakeSecrets) {
	sec := newEncFakeSecrets()
	aud := &fakeAuditor{}
	mgr := &dataset.EncryptionManager{
		ZFSBin:      "/sbin/zfs",
		Sealer:      encFakeSealer{},
		Secrets:     sec,
		StdinRunner: fakeStdinRunner,
		Now:         func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) },
	}
	h := &EncryptionHandler{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Mgr:     mgr,
		Auditor: aud,
	}
	return h, aud, sec
}

func mountEnc(h *EncryptionHandler) http.Handler {
	r := chi.NewRouter()
	r.Post("/api/v1/datasets/{fullname}/encryption", h.Initialize)
	r.Post("/api/v1/datasets/{fullname}/encryption/load-key", h.LoadKey)
	r.Post("/api/v1/datasets/{fullname}/encryption/unload-key", h.UnloadKey)
	r.Post("/api/v1/datasets/{fullname}/encryption/recover", h.Recover)
	return r
}

// --- Tests ----------------------------------------------------------------

func TestEncryption_InitializeWritesEscrow(t *testing.T) {
	h, _, sec := newEncHandler()
	srv := httptest.NewServer(mountEnc(h))
	defer srv.Close()

	body, _ := json.Marshal(EncryptionInitRequest{Type: "filesystem"})
	resp, err := http.Post(srv.URL+"/api/v1/datasets/tank%2Fsecret/encryption",
		"application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	var got EncryptionInitResponse
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.Dataset != "tank/secret" {
		t.Errorf("dataset=%q", got.Dataset)
	}
	if got.Algorithm != dataset.DefaultEncryptionAlgorithm {
		t.Errorf("algorithm=%q", got.Algorithm)
	}

	// Escrow was written.
	skey, _ := dataset.EncodeSecretKey("tank/secret")
	if _, err := sec.Get(context.Background(), skey); err != nil {
		t.Errorf("expected escrow: %v", err)
	}
}

func TestEncryption_InitializeRejectsBadBody(t *testing.T) {
	h, _, _ := newEncHandler()
	srv := httptest.NewServer(mountEnc(h))
	defer srv.Close()

	for name, body := range map[string]string{
		"bad-json": "not json",
		"bad-type": `{"type":"snapshot"}`,
		"missing-volsize": `{"type":"volume"}`,
	} {
		t.Run(name, func(t *testing.T) {
			resp, _ := http.Post(srv.URL+"/api/v1/datasets/tank%2Fx/encryption",
				"application/json", strings.NewReader(body))
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status=%d", resp.StatusCode)
			}
		})
	}
}

func TestEncryption_LoadKey_NoEscrowFails(t *testing.T) {
	h, _, _ := newEncHandler()
	srv := httptest.NewServer(mountEnc(h))
	defer srv.Close()
	resp, _ := http.Post(srv.URL+"/api/v1/datasets/tank%2Fsecret/encryption/load-key", "", nil)
	if resp.StatusCode == http.StatusNoContent {
		t.Errorf("expected error when no escrow exists, got 204")
	}
}

func TestEncryption_LoadKey_AfterInitialize(t *testing.T) {
	h, _, _ := newEncHandler()
	srv := httptest.NewServer(mountEnc(h))
	defer srv.Close()

	body, _ := json.Marshal(EncryptionInitRequest{Type: "filesystem"})
	if r, _ := http.Post(srv.URL+"/api/v1/datasets/tank%2Fsecret/encryption",
		"application/json", bytes.NewReader(body)); r.StatusCode != http.StatusCreated {
		t.Fatalf("init status=%d", r.StatusCode)
	}
	resp, _ := http.Post(srv.URL+"/api/v1/datasets/tank%2Fsecret/encryption/load-key", "", nil)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("status=%d body=%s", resp.StatusCode, b)
	}
}

func TestEncryption_Recover_AuditsAndReturnsKey(t *testing.T) {
	h, aud, _ := newEncHandler()
	srv := httptest.NewServer(mountEnc(h))
	defer srv.Close()

	body, _ := json.Marshal(EncryptionInitRequest{Type: "filesystem"})
	if r, _ := http.Post(srv.URL+"/api/v1/datasets/tank%2Fsecret/encryption",
		"application/json", bytes.NewReader(body)); r.StatusCode != http.StatusCreated {
		t.Fatalf("init: %d", r.StatusCode)
	}

	resp, err := http.Post(srv.URL+"/api/v1/datasets/tank%2Fsecret/encryption/recover", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("recover status=%d body=%s", resp.StatusCode, b)
	}
	var rr EncryptionRecoverResponse
	_ = json.NewDecoder(resp.Body).Decode(&rr)
	if rr.Dataset != "tank/secret" {
		t.Errorf("dataset=%q", rr.Dataset)
	}
	keyBytes, err := hex.DecodeString(rr.KeyHex)
	if err != nil {
		t.Fatalf("hex: %v", err)
	}
	if len(keyBytes) != dataset.RawKeyLen {
		t.Errorf("key length=%d", len(keyBytes))
	}

	// An audit row with action=encryption.recover and target=tank/secret
	// must have been written.
	found := false
	for _, e := range aud.entries {
		if e.Action == "encryption.recover" && e.Target == "tank/secret" && e.Result == "accepted" {
			found = true
			// Payload must include the dataset name; the actual key MUST
			// NOT appear in the audit payload.
			if !bytes.Contains(e.Payload, []byte("tank/secret")) {
				t.Errorf("payload missing dataset: %s", e.Payload)
			}
			if bytes.Contains(e.Payload, []byte(rr.KeyHex)) {
				t.Errorf("payload leaked the recovered key")
			}
		}
	}
	if !found {
		t.Errorf("no encryption.recover audit row in %d entries", len(aud.entries))
	}
}

func TestEncryption_Recover_AuditsRejection(t *testing.T) {
	h, aud, _ := newEncHandler()
	srv := httptest.NewServer(mountEnc(h))
	defer srv.Close()

	// No initialize → recover should fail and still produce an audit
	// row with result=rejected.
	resp, _ := http.Post(srv.URL+"/api/v1/datasets/tank%2Fnope/encryption/recover", "", nil)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected non-200 from recover with no escrow")
	}
	found := false
	for _, e := range aud.entries {
		if e.Action == "encryption.recover" && e.Result == "rejected" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected rejected audit row")
	}
}

func TestEncryption_NoMgr_503(t *testing.T) {
	h := &EncryptionHandler{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Mgr:    nil,
	}
	srv := httptest.NewServer(mountEnc(h))
	defer srv.Close()
	body, _ := json.Marshal(EncryptionInitRequest{Type: "filesystem"})
	resp, _ := http.Post(srv.URL+"/api/v1/datasets/tank%2Fx/encryption",
		"application/json", bytes.NewReader(body))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status=%d", resp.StatusCode)
	}
}

