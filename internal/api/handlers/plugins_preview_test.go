package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/plugins"
)

const previewManifest = `apiVersion: novanas.io/v1
kind: Plugin
metadata:
  name: rustfs
  version: 1.2.3
  vendor: NovaNAS Project
spec:
  description: S3-compatible object storage
  category: storage
  deployment:
    type: helm
    chart: chart/
    namespace: rustfs
  needs:
    - kind: dataset
      dataset:
        pool: tank
        name: objects
    - kind: tlsCert
      tlsCert:
        commonName: rustfs.novanas.local
    - kind: oidcClient
      oidcClient:
        clientId: rustfs
    - kind: permission
      permission:
        role: nova-operator
  api:
    routes:
      - path: /admin
        upstream: http://127.0.0.1:9000
        auth: bearer-passthrough
  ui:
    window:
      name: RustFS
      route: /apps/rustfs
      bundle: main.js
`

// previewServerFixture returns the handler + cleanup. The fixture
// owns a fake marketplace with one signed plugin.
type previewServerFixture struct {
	handler *PluginsPreviewHandler
	close   func()
}

func setupPreview(t *testing.T, manifestYAML string, signWithRealKey bool) *previewServerFixture {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	pubPath := filepath.Join(t.TempDir(), "trust.pub")
	if err := os.WriteFile(pubPath, pubPEM, 0o644); err != nil {
		t.Fatal(err)
	}

	tarball := buildPreviewTarball(t, manifestYAML)
	digest := sha256.Sum256(tarball)
	var sigBytes []byte
	if signWithRealKey {
		sigBytes, err = ecdsa.SignASN1(rand.Reader, priv, digest[:])
		if err != nil {
			t.Fatal(err)
		}
	} else {
		// Sign with a different key — verify will fail.
		other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		sigBytes, _ = ecdsa.SignASN1(rand.Reader, other, digest[:])
	}
	sig := []byte(base64.StdEncoding.EncodeToString(sigBytes))

	mux := http.NewServeMux()
	mux.HandleFunc("/tarball", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarball)
	})
	mux.HandleFunc("/signature", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(sig)
	})
	srv := httptest.NewServer(mux)

	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		idx := plugins.Index{
			Version: 1,
			Plugins: []plugins.IndexPlugin{
				{
					Name:     "rustfs",
					Vendor:   "NovaNAS Project",
					Category: "storage",
					Versions: []plugins.IndexVersion{
						{
							Version:      "1.2.3",
							TarballURL:   srv.URL + "/tarball",
							SignatureURL: srv.URL + "/signature",
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(idx)
	})

	mc := plugins.NewMarketplaceClient(srv.URL+"/index.json", nil)
	ver := plugins.NewVerifier(pubPath)
	h := &PluginsPreviewHandler{Marketplace: mc, Verifier: ver}
	return &previewServerFixture{handler: h, close: srv.Close}
}

func buildPreviewTarball(t *testing.T, manifest string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.yaml", Mode: 0o644, Size: int64(len(manifest)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func mountPreview(h *PluginsPreviewHandler) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/plugins/index/{name}/manifest", h.Preview)
	return r
}

func TestPluginsPreview_Happy(t *testing.T) {
	fx := setupPreview(t, previewManifest, true)
	defer fx.close()
	srv := httptest.NewServer(mountPreview(fx.handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/plugins/index/rustfs/manifest?version=1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got plugins.PreviewResult
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Manifest == nil || got.Manifest.Metadata.Name != "rustfs" {
		t.Errorf("manifest=%+v", got.Manifest)
	}
	if got.TarballSHA256 == "" {
		t.Errorf("missing sha")
	}
	if got.Permissions.Category != "storage" {
		t.Errorf("category=%q", got.Permissions.Category)
	}
	if len(got.Permissions.WillCreate) != 4 {
		t.Errorf("willCreate=%v", got.Permissions.WillCreate)
	}
	if len(got.Permissions.WillMount) != 1 {
		t.Errorf("willMount=%v", got.Permissions.WillMount)
	}
}

func TestPluginsPreview_MissingVersion(t *testing.T) {
	fx := setupPreview(t, previewManifest, true)
	defer fx.close()
	srv := httptest.NewServer(mountPreview(fx.handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/plugins/index/rustfs/manifest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestPluginsPreview_UnknownName(t *testing.T) {
	fx := setupPreview(t, previewManifest, true)
	defer fx.close()
	srv := httptest.NewServer(mountPreview(fx.handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/plugins/index/nope/manifest?version=1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestPluginsPreview_MarketplaceUnreachable(t *testing.T) {
	mc := plugins.NewMarketplaceClient("http://127.0.0.1:1/index.json", nil)
	h := &PluginsPreviewHandler{Marketplace: mc}
	srv := httptest.NewServer(mountPreview(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/plugins/index/anything/manifest?version=1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestPluginsPreview_SignatureFails(t *testing.T) {
	fx := setupPreview(t, previewManifest, false) // sign with wrong key
	defer fx.close()
	srv := httptest.NewServer(mountPreview(fx.handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/plugins/index/rustfs/manifest?version=1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&env)
	if env.Error == "" {
		t.Errorf("expected error envelope")
	}
}

func TestPluginsPreview_NotConfigured(t *testing.T) {
	h := &PluginsPreviewHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plugins/index/x/manifest?version=1", nil)
	h.Preview(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rec.Code)
	}
}
