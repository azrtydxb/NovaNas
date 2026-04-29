package plugins

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// makeTarball constructs an in-memory plugin tarball with the given
// manifest at the root. Useful for unit tests of ExtractTarball.
func makeTarball(t *testing.T, manifest string, extra map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	write := func(name string, data string) {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	write("manifest.yaml", manifest)
	for k, v := range extra {
		write(k, v)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractTarball_ManifestAndUI(t *testing.T) {
	tar := makeTarball(t, goodManifest, map[string]string{
		"ui/main.js":    "console.log('hi');",
		"ui/style.css":  "body{}",
	})
	dir := t.TempDir()
	manifest, uiDir, err := ExtractTarball(tar, dir)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(string(manifest), "rustfs") {
		t.Errorf("manifest not extracted")
	}
	if uiDir == "" {
		t.Errorf("ui dir not detected")
	}
}

func TestExtractTarball_MissingManifest(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "ui/main.js", Mode: 0o644, Size: 5, Typeflag: tar.TypeReg})
	tw.Write([]byte("hello"))
	tw.Close()
	gz.Close()
	if _, _, err := ExtractTarball(buf.Bytes(), t.TempDir()); err == nil {
		t.Fatal("expected missing-manifest error")
	}
}

func TestExtractTarball_PathTraversal(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "../etc/passwd", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	tw.Write([]byte("x"))
	tw.Close()
	gz.Close()
	if _, _, err := ExtractTarball(buf.Bytes(), t.TempDir()); err == nil {
		t.Fatal("expected path-traversal rejection")
	}
}

func TestRouter_MountUnmount_ServeProxy(t *testing.T) {
	// Stand up a fake upstream the router will proxy to.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-Path", r.URL.Path)
		w.WriteHeader(200)
		w.Write([]byte("upstream-ok"))
	}))
	defer upstream.Close()

	rt := NewRouter(nil, nil)
	if err := rt.Mount("rustfs", []APIRoute{
		{Path: "/buckets", Upstream: upstream.URL, Auth: AuthBearerPassthrough},
	}); err != nil {
		t.Fatalf("mount: %v", err)
	}
	if !rt.IsMounted("rustfs") {
		t.Fatal("not mounted")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://x/api/v1/plugins/rustfs/api/buckets/abc", nil)
	rt.ServeProxy(rec, req, "rustfs", "/buckets/abc")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "upstream-ok") {
		t.Fatalf("proxy failed: %d %s", rec.Code, rec.Body.String())
	}
	rt.Unmount("rustfs")
	if rt.IsMounted("rustfs") {
		t.Fatal("still mounted")
	}
}

func TestNeedsRollback(t *testing.T) {
	// First two needs succeed, third fails — rollback must run for the
	// first two.
	prov := &countingProvisioner{failOn: 3}
	_, err := runNeeds(context.Background(), prov, "p", []Need{
		{Kind: NeedDataset, Dataset: &DatasetNeed{Pool: "t", Name: "a"}},
		{Kind: NeedOIDCClient, OIDCClient: &OIDCClientNeed{ClientID: "x"}},
		{Kind: NeedPermission, Permission: &PermissionNeed{Role: "r"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if prov.dsRollback != 1 || prov.oidcRollback != 1 {
		t.Errorf("rollback counts: ds=%d oidc=%d", prov.dsRollback, prov.oidcRollback)
	}
}

type countingProvisioner struct {
	NopProvisioner
	failOn       int
	calls        int
	dsRollback   int
	oidcRollback int
}

func (c *countingProvisioner) ProvisionDataset(ctx context.Context, p string, n DatasetNeed) (string, error) {
	c.calls++
	if c.calls == c.failOn {
		return "", &fakeErr{"forced"}
	}
	return c.NopProvisioner.ProvisionDataset(ctx, p, n)
}
func (c *countingProvisioner) ProvisionOIDCClient(ctx context.Context, p string, n OIDCClientNeed) (string, error) {
	c.calls++
	if c.calls == c.failOn {
		return "", &fakeErr{"forced"}
	}
	return c.NopProvisioner.ProvisionOIDCClient(ctx, p, n)
}
func (c *countingProvisioner) ProvisionPermission(ctx context.Context, p string, n PermissionNeed) (string, error) {
	c.calls++
	if c.calls == c.failOn {
		return "", &fakeErr{"forced"}
	}
	return c.NopProvisioner.ProvisionPermission(ctx, p, n)
}
func (c *countingProvisioner) UnprovisionDataset(ctx context.Context, p, id string) error {
	c.dsRollback++
	return nil
}
func (c *countingProvisioner) UnprovisionOIDCClient(ctx context.Context, p, id string) error {
	c.oidcRollback++
	return nil
}

type fakeErr struct{ s string }

func (e *fakeErr) Error() string { return e.s }
