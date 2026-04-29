package plugins

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// previewFixtures spins up a fake marketplace (index + tarball + sig
// endpoints) and a Verifier wired against the test key.
type previewFixtures struct {
	srv      *httptest.Server
	client   *MarketplaceClient
	verifier *Verifier
	tarball  []byte
}

func setupPreviewServer(t *testing.T, manifest string) *previewFixtures {
	t.Helper()
	priv, pubPath := writeTestKey(t)
	tarball := makeTarball(t, manifest, nil)
	digest := sha256.Sum256(tarball)
	sigBytes, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatal(err)
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
		idx := Index{
			Version: 1,
			Plugins: []IndexPlugin{
				{
					Name:     "rustfs",
					Vendor:   "NovaNAS Project",
					Category: "storage",
					Versions: []IndexVersion{
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

	mc := NewMarketplaceClient(srv.URL+"/index.json", nil)
	return &previewFixtures{
		srv:      srv,
		client:   mc,
		verifier: NewVerifier(pubPath),
		tarball:  tarball,
	}
}

func TestPreviewPlugin_Happy(t *testing.T) {
	fx := setupPreviewServer(t, goodManifest)
	defer fx.srv.Close()

	res, err := PreviewPlugin(context.Background(), fx.client, fx.verifier, "rustfs", "1.2.3")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if res.Manifest == nil || res.Manifest.Metadata.Name != "rustfs" {
		t.Fatalf("manifest=%+v", res.Manifest)
	}
	if res.TarballSHA256 == "" || len(res.TarballSHA256) != 64 {
		t.Errorf("sha256=%q", res.TarballSHA256)
	}
	if res.Permissions.Category != "storage" {
		t.Errorf("category=%q", res.Permissions.Category)
	}
	if len(res.Permissions.WillCreate) != 4 {
		t.Errorf("willCreate len=%d", len(res.Permissions.WillCreate))
	}
	if len(res.Permissions.WillMount) != 1 || !strings.Contains(res.Permissions.WillMount[0], "rustfs/buckets") {
		t.Errorf("willMount=%v", res.Permissions.WillMount)
	}
	// Verify the SHA matches the tarball we served.
	want := sha256.Sum256(fx.tarball)
	if fmt.Sprintf("%x", want[:]) != res.TarballSHA256 {
		t.Errorf("sha mismatch")
	}
}

func TestPreviewPlugin_DefaultsToLatest(t *testing.T) {
	fx := setupPreviewServer(t, goodManifest)
	defer fx.srv.Close()
	res, err := PreviewPlugin(context.Background(), fx.client, fx.verifier, "rustfs", "")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if res.Manifest.Metadata.Version != "1.2.3" {
		t.Errorf("version=%q", res.Manifest.Metadata.Version)
	}
}

func TestPreviewPlugin_UnknownName(t *testing.T) {
	fx := setupPreviewServer(t, goodManifest)
	defer fx.srv.Close()
	_, err := PreviewPlugin(context.Background(), fx.client, fx.verifier, "nope", "1.0.0")
	var pe *PreviewError
	if !errors.As(err, &pe) || pe.Code != PreviewErrNotFound {
		t.Fatalf("want PreviewErrNotFound, got %v", err)
	}
}

func TestPreviewPlugin_UnknownVersion(t *testing.T) {
	fx := setupPreviewServer(t, goodManifest)
	defer fx.srv.Close()
	_, err := PreviewPlugin(context.Background(), fx.client, fx.verifier, "rustfs", "9.9.9")
	var pe *PreviewError
	if !errors.As(err, &pe) || pe.Code != PreviewErrNotFound {
		t.Fatalf("want PreviewErrNotFound, got %v", err)
	}
}

func TestPreviewPlugin_MarketplaceDown(t *testing.T) {
	mc := NewMarketplaceClient("http://127.0.0.1:1/index.json", nil)
	_, err := PreviewPlugin(context.Background(), mc, nil, "rustfs", "1.2.3")
	var pe *PreviewError
	if !errors.As(err, &pe) || pe.Code != PreviewErrMarketplaceUnreach {
		t.Fatalf("want PreviewErrMarketplaceUnreach, got %v", err)
	}
}

func TestPreviewPlugin_BadSignature(t *testing.T) {
	fx := setupPreviewServer(t, goodManifest)
	defer fx.srv.Close()
	// Swap in a verifier that uses a DIFFERENT key — the served sig
	// won't validate against it.
	_, otherKey := writeTestKey(t)
	other := NewVerifier(otherKey)
	_, err := PreviewPlugin(context.Background(), fx.client, other, "rustfs", "1.2.3")
	var pe *PreviewError
	if !errors.As(err, &pe) || pe.Code != PreviewErrSignatureInvalid {
		t.Fatalf("want PreviewErrSignatureInvalid, got %v", err)
	}
}

func TestPreviewPlugin_ManifestNameMismatch(t *testing.T) {
	bad := strings.Replace(goodManifest, "name: rustfs", "name: somethingelse", 1)
	fx := setupPreviewServer(t, bad)
	defer fx.srv.Close()
	_, err := PreviewPlugin(context.Background(), fx.client, fx.verifier, "rustfs", "1.2.3")
	var pe *PreviewError
	if !errors.As(err, &pe) || pe.Code != PreviewErrManifestInvalid {
		t.Fatalf("want PreviewErrManifestInvalid, got %v", err)
	}
}

func TestReadManifestFromTarball_Missing(t *testing.T) {
	// Tarball with only a UI file — no manifest.
	tar := makeTarballWithoutManifest(t)
	_, err := readManifestFromTarball(tar)
	if err == nil {
		t.Fatal("expected missing-manifest error")
	}
}

// makeTarballWithoutManifest builds a tarball that has only a ui/ entry
// — no manifest.yaml. Constructed inline (makeTarball always writes a
// manifest.yaml, which is the opposite of what this test needs).
func makeTarballWithoutManifest(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("console.log('x');")
	if err := tw.WriteHeader(&tar.Header{Name: "ui/main.js", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
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
