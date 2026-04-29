package secrets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// xorSealer is a stub Sealer for tests. It XORs the input with a fixed
// 32-byte key so Seal/Unseal round-trip without needing a real TPM.
// The output of Seal is intentionally different from the plaintext so
// we can verify the FileBackend stores genuinely encrypted bytes.
type xorSealer struct{ key [32]byte }

func newXORSealer() *xorSealer {
	s := &xorSealer{}
	for i := range s.key {
		s.key[i] = byte(i + 1)
	}
	return s
}

func (s *xorSealer) Seal(p []byte) ([]byte, error) {
	out := make([]byte, len(p))
	for i := range p {
		out[i] = p[i] ^ s.key[i%len(s.key)]
	}
	return out, nil
}
func (s *xorSealer) Unseal(c []byte) ([]byte, error) { return s.Seal(c) }

// --- File backend (no sealer) -------------------------------------------------

func TestFileBackend_Plain_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	b, err := NewFileBackend(dir, nil, nil)
	if err != nil {
		t.Fatalf("NewFileBackend: %v", err)
	}
	if b.Backend() != "file" {
		t.Fatalf("Backend()=%q", b.Backend())
	}
	ctx := context.Background()

	if err := b.Set(ctx, "alpha", []byte("one")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := b.Set(ctx, "nested/key", []byte("two")); err != nil {
		t.Fatalf("Set nested: %v", err)
	}
	got, err := b.Get(ctx, "alpha")
	if err != nil || string(got) != "one" {
		t.Fatalf("Get alpha = %q, %v", got, err)
	}
	got, err = b.Get(ctx, "nested/key")
	if err != nil || string(got) != "two" {
		t.Fatalf("Get nested/key = %q, %v", got, err)
	}

	keys, err := b.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"alpha", "nested/key"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("List = %v, want %v", keys, want)
	}

	keys, err = b.List(ctx, "nested/")
	if err != nil {
		t.Fatalf("List prefix: %v", err)
	}
	if !reflect.DeepEqual(keys, []string{"nested/key"}) {
		t.Fatalf("List prefix = %v", keys)
	}

	// Plain mode: file content should equal the raw bytes.
	raw, err := os.ReadFile(filepath.Join(dir, "alpha"))
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if string(raw) != "one" {
		t.Fatalf("raw file = %q, want %q", raw, "one")
	}

	if err := b.Delete(ctx, "alpha"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := b.Get(ctx, "alpha"); err != ErrNotFound {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
	if err := b.Delete(ctx, "alpha"); err != ErrNotFound {
		t.Fatalf("Delete missing = %v, want ErrNotFound", err)
	}
}

func TestFileBackend_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits differ on Windows")
	}
	dir := t.TempDir()
	b, err := NewFileBackend(dir, nil, nil)
	if err != nil {
		t.Fatalf("NewFileBackend: %v", err)
	}
	if err := b.Set(context.Background(), "perm", []byte("x")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	st, err := os.Stat(filepath.Join(dir, "perm"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", st.Mode().Perm())
	}
}

func TestFileBackend_KeyValidation(t *testing.T) {
	dir := t.TempDir()
	b, _ := NewFileBackend(dir, nil, nil)
	ctx := context.Background()
	bad := []string{
		"",
		"/leading",
		"trailing/",
		"has space",
		"has..dot",
		"with\x00nul",
		"weird@char",
		"a//b",
		"./relative",
	}
	for _, k := range bad {
		if err := b.Set(ctx, k, []byte("v")); err == nil {
			t.Errorf("Set(%q) accepted, want error", k)
		}
	}
}

func TestFileBackend_NotFound(t *testing.T) {
	dir := t.TempDir()
	b, _ := NewFileBackend(dir, nil, nil)
	if _, err := b.Get(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("Get missing = %v, want ErrNotFound", err)
	}
}

// --- File backend (sealed) ----------------------------------------------------

func TestFileBackend_Sealed_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	b, err := NewFileBackend(dir, newXORSealer(), nil)
	if err != nil {
		t.Fatalf("NewFileBackend: %v", err)
	}
	ctx := context.Background()
	plain := []byte("hunter2-the-real-password")

	if err := b.Set(ctx, "creds/admin", plain); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// On-disk bytes must NOT equal plaintext (AES-GCM yields nonce+ct+tag).
	raw, err := os.ReadFile(filepath.Join(dir, "creds", "admin"))
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if string(raw) == string(plain) {
		t.Fatalf("on-disk bytes equal plaintext; expected encrypted")
	}
	if len(raw) < gcmNonceSize+16+len(plain) {
		t.Fatalf("ciphertext suspiciously short: %d", len(raw))
	}

	// .dek.sealed must exist.
	if _, err := os.Stat(filepath.Join(dir, dekFile)); err != nil {
		t.Fatalf("expected sealed DEK file: %v", err)
	}

	// Round-trip via a fresh backend instance to exercise loadDEK path.
	b2, err := NewFileBackend(dir, newXORSealer(), nil)
	if err != nil {
		t.Fatalf("NewFileBackend 2: %v", err)
	}
	got, err := b2.Get(ctx, "creds/admin")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("Get = %q, want %q", got, plain)
	}
}

func TestFileBackend_Sealed_TamperDetected(t *testing.T) {
	dir := t.TempDir()
	b, _ := NewFileBackend(dir, newXORSealer(), nil)
	ctx := context.Background()
	if err := b.Set(ctx, "k", []byte("payload")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	p := filepath.Join(dir, "k")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Flip a byte inside the ciphertext (after the 12-byte nonce).
	raw[gcmNonceSize] ^= 0x01
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := b.Get(ctx, "k"); err == nil {
		t.Fatalf("Get after tamper succeeded, want auth-fail")
	}
}

func TestFileBackend_Sealed_GetWithNoDEK(t *testing.T) {
	// Reading a key when the DEK file doesn't exist yet should surface
	// ErrNotFound (no encrypted secrets can exist), not a hard error.
	dir := t.TempDir()
	b, _ := NewFileBackend(dir, newXORSealer(), nil)
	if _, err := b.Get(context.Background(), "anything"); err != ErrNotFound {
		t.Fatalf("Get pre-init = %v, want ErrNotFound", err)
	}
}

func TestFileBackend_DEKHiddenFromList(t *testing.T) {
	dir := t.TempDir()
	b, _ := NewFileBackend(dir, newXORSealer(), nil)
	ctx := context.Background()
	if err := b.Set(ctx, "a", []byte("x")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	keys, err := b.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, k := range keys {
		if strings.HasPrefix(k, ".") || k == dekFile {
			t.Fatalf("List returned hidden key %q", k)
		}
	}
	if !reflect.DeepEqual(keys, []string{"a"}) {
		t.Fatalf("List = %v, want [a]", keys)
	}
}

// --- Bao backend (httptest) ---------------------------------------------------

// fakeBao is a tiny in-memory KV v2 server good enough for our tests.
type fakeBao struct {
	t        *testing.T
	mu       map[string]string // key -> base64 value
	mount    string
	wantTok  string
	gotToken string
}

func newFakeBao(t *testing.T) *fakeBao {
	return &fakeBao{t: t, mu: map[string]string{}, mount: "secret", wantTok: "test-token"}
}

func (f *fakeBao) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.gotToken = r.Header.Get("X-Vault-Token")
		if f.gotToken != f.wantTok {
			http.Error(w, "bad token", http.StatusForbidden)
			return
		}
		path := r.URL.Path
		dataPrefix := "/v1/" + f.mount + "/data/"
		metaPrefix := "/v1/" + f.mount + "/metadata/"
		switch {
		case strings.HasPrefix(path, dataPrefix):
			key := strings.TrimPrefix(path, dataPrefix)
			f.handleData(w, r, key)
		case strings.HasPrefix(path, metaPrefix):
			key := strings.TrimPrefix(path, metaPrefix)
			key = strings.TrimSuffix(key, "/")
			f.handleMetadata(w, r, key)
		default:
			http.NotFound(w, r)
		}
	})
}

func (f *fakeBao) handleData(w http.ResponseWriter, r *http.Request, key string) {
	switch r.Method {
	case http.MethodGet:
		v, ok := f.mu[key]
		if !ok {
			http.Error(w, `{"errors":["not found"]}`, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"data":     map[string]string{"value": v},
				"metadata": map[string]any{"version": 1},
			},
		})
	case http.MethodPost, http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		var p kvWriteRequest
		if err := json.Unmarshal(body, &p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		v, ok := p.Data["value"]
		if !ok {
			http.Error(w, "missing value", http.StatusBadRequest)
			return
		}
		f.mu[key] = v
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (f *fakeBao) handleMetadata(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method == http.MethodDelete {
		if _, ok := f.mu[key]; !ok {
			http.Error(w, `{"errors":["not found"]}`, http.StatusNotFound)
			return
		}
		delete(f.mu, key)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method == http.MethodGet && r.URL.Query().Get("list") == "true" {
		// Return immediate children of `key` (treated as directory).
		dir := key
		seen := map[string]bool{}
		var keys []string
		for k := range f.mu {
			rel := k
			if dir != "" {
				if !strings.HasPrefix(k, dir+"/") {
					continue
				}
				rel = strings.TrimPrefix(k, dir+"/")
			}
			if i := strings.Index(rel, "/"); i >= 0 {
				name := rel[:i+1]
				if !seen[name] {
					seen[name] = true
					keys = append(keys, name)
				}
			} else {
				if !seen[rel] {
					seen[rel] = true
					keys = append(keys, rel)
				}
			}
		}
		if len(keys) == 0 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"keys": keys},
		})
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func TestBaoBackend_RoundTrip(t *testing.T) {
	fb := newFakeBao(t)
	srv := httptest.NewServer(fb.handler())
	defer srv.Close()

	b, err := NewBaoBackend(BaoOpts{
		Address: srv.URL,
		Token:   "test-token",
		KVMount: "secret",
	}, nil)
	if err != nil {
		t.Fatalf("NewBaoBackend: %v", err)
	}
	if b.Backend() != "bao" {
		t.Fatalf("Backend() = %q", b.Backend())
	}
	ctx := context.Background()

	if err := b.Set(ctx, "app/db_password", []byte("s3cret")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if fb.gotToken != "test-token" {
		t.Fatalf("token header = %q, want test-token", fb.gotToken)
	}

	// Verify on-server stored value is base64 of the bytes.
	stored := fb.mu["app/db_password"]
	if want := base64.StdEncoding.EncodeToString([]byte("s3cret")); stored != want {
		t.Fatalf("stored = %q, want %q (base64)", stored, want)
	}

	got, err := b.Get(ctx, "app/db_password")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "s3cret" {
		t.Fatalf("Get = %q, want s3cret", got)
	}

	// Add another key; list should sort.
	if err := b.Set(ctx, "app/api_key", []byte("k")); err != nil {
		t.Fatalf("Set 2: %v", err)
	}
	keys, err := b.List(ctx, "app/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"app/api_key", "app/db_password"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("List = %v, want %v", keys, want)
	}

	if err := b.Delete(ctx, "app/db_password"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := b.Get(ctx, "app/db_password"); err != ErrNotFound {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
}

func TestBaoBackend_NotFoundAndError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/secret/data/missing", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"errors":["not found"]}`, http.StatusNotFound)
	})
	mux.HandleFunc("/v1/secret/data/boom", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "kaboom", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b, err := NewBaoBackend(BaoOpts{Address: srv.URL, Token: "t", KVMount: "secret"}, nil)
	if err != nil {
		t.Fatalf("NewBaoBackend: %v", err)
	}
	if _, err := b.Get(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("Get missing = %v, want ErrNotFound", err)
	}
	if _, err := b.Get(context.Background(), "boom"); err == nil || err == ErrNotFound {
		t.Fatalf("Get 5xx = %v, want a non-ErrNotFound error", err)
	}
}

func TestBaoBackend_NamespaceHeader(t *testing.T) {
	var gotNS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotNS = r.Header.Get("X-Vault-Namespace")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	b, err := NewBaoBackend(BaoOpts{
		Address:   srv.URL,
		Token:     "t",
		KVMount:   "secret",
		Namespace: "team-a",
	}, nil)
	if err != nil {
		t.Fatalf("NewBaoBackend: %v", err)
	}
	if err := b.Set(context.Background(), "k", []byte("v")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if gotNS != "team-a" {
		t.Fatalf("namespace header = %q, want team-a", gotNS)
	}
}

func TestBaoBackend_RequiredOpts(t *testing.T) {
	cases := []BaoOpts{
		{Token: "t", KVMount: "secret"},
		{Address: "https://x", KVMount: "secret"},
		{Address: "https://x", Token: "t"},
		{Address: "not a url", Token: "t", KVMount: "secret"},
	}
	for i, c := range cases {
		if _, err := NewBaoBackend(c, nil); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

// --- FromEnv -----------------------------------------------------------------

func TestFromEnv_DefaultsToFile(t *testing.T) {
	t.Setenv(envBackend, "")
	t.Setenv(envFileRoot, t.TempDir())
	t.Setenv(envFileTPMSeal, "")
	m, err := FromEnv(nil)
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if m.Backend() != "file" {
		t.Fatalf("Backend()=%q", m.Backend())
	}
}

func TestFromEnv_File_TPMSealRequiresFactory(t *testing.T) {
	t.Setenv(envBackend, "file")
	t.Setenv(envFileRoot, t.TempDir())
	t.Setenv(envFileTPMSeal, "true")
	prev := tpmSealerFactory
	RegisterTPMSealerFactory(nil)
	defer RegisterTPMSealerFactory(prev)
	if _, err := FromEnv(nil); err == nil {
		t.Fatalf("expected error when TPM_SEAL=true and no factory")
	}
}

func TestFromEnv_File_TPMSealUsesFactory(t *testing.T) {
	t.Setenv(envBackend, "file")
	t.Setenv(envFileRoot, t.TempDir())
	t.Setenv(envFileTPMSeal, "true")
	prev := tpmSealerFactory
	RegisterTPMSealerFactory(func() (Sealer, error) { return newXORSealer(), nil })
	defer RegisterTPMSealerFactory(prev)
	m, err := FromEnv(nil)
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	fb, ok := m.(*FileBackend)
	if !ok {
		t.Fatalf("Manager type = %T", m)
	}
	if fb.Sealer == nil {
		t.Fatalf("expected sealer to be installed")
	}
}

func TestFromEnv_Bao(t *testing.T) {
	t.Setenv(envBackend, "bao")
	t.Setenv(envBaoAddr, "https://example.invalid:8200")
	t.Setenv(envBaoToken, "tok")
	t.Setenv(envBaoKVMount, "kv")
	t.Setenv(envBaoNamespace, "ns")
	m, err := FromEnv(nil)
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	bb, ok := m.(*BaoBackend)
	if !ok {
		t.Fatalf("Manager type = %T", m)
	}
	if bb.KVMount != "kv" || bb.Namespace != "ns" || bb.Token != "tok" {
		t.Fatalf("opts not parsed: %+v", bb)
	}
}

func TestFromEnv_Bao_TokenFile(t *testing.T) {
	dir := t.TempDir()
	tf := filepath.Join(dir, "tok")
	if err := os.WriteFile(tf, []byte("file-token\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv(envBackend, "bao")
	t.Setenv(envBaoAddr, "https://example.invalid:8200")
	t.Setenv(envBaoToken, "")
	t.Setenv(envVaultToken, "")
	t.Setenv(envBaoTokenFile, tf)
	m, err := FromEnv(nil)
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	bb := m.(*BaoBackend)
	if bb.Token != "file-token" {
		t.Fatalf("token = %q, want file-token", bb.Token)
	}
}

func TestFromEnv_UnknownBackend(t *testing.T) {
	t.Setenv(envBackend, "weird")
	if _, err := FromEnv(nil); err == nil {
		t.Fatalf("expected error")
	}
}
