package plugins

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DefaultPluginsRoot is the on-disk root for unpacked plugin trees.
// Each plugin has /var/lib/nova-nas/plugins/<name>/{ui,manifest.yaml,data,...}.
const DefaultPluginsRoot = "/var/lib/nova-nas/plugins"

// UIAssets serves /api/v1/plugins/{name}/ui/* from the unpacked
// per-plugin tree. The set of registered plugins is mutated by the
// lifecycle manager; reads are concurrent.
type UIAssets struct {
	Root string

	mu       sync.RWMutex
	plugins  map[string]string // name -> ui dir
}

// NewUIAssets constructs a UIAssets rooted at root. Empty falls back
// to DefaultPluginsRoot.
func NewUIAssets(root string) *UIAssets {
	if root == "" {
		root = DefaultPluginsRoot
	}
	return &UIAssets{Root: root, plugins: map[string]string{}}
}

// Register marks plugin's UI directory as servable. Called by the
// lifecycle manager after unpacking the package and finding ui/ inside.
// uiDir "" deregisters.
func (u *UIAssets) Register(plugin, uiDir string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if uiDir == "" {
		delete(u.plugins, plugin)
		return
	}
	u.plugins[plugin] = uiDir
}

// Deregister removes plugin from the served set.
func (u *UIAssets) Deregister(plugin string) { u.Register(plugin, "") }

// Lookup returns the on-disk directory for plugin, "" if not registered.
func (u *UIAssets) Lookup(plugin string) string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.plugins[plugin]
}

// Serve handles /api/v1/plugins/{name}/ui/{rest}. It refuses
// directory traversal (.. segments) and returns 404 on any
// not-registered plugin.
func (u *UIAssets) Serve(w http.ResponseWriter, _ *http.Request, plugin, rest string) {
	dir := u.Lookup(plugin)
	if dir == "" {
		http.Error(w, `{"error":"ui_not_registered"}`, http.StatusNotFound)
		return
	}
	clean := filepath.Clean("/" + rest)
	if strings.Contains(clean, "..") {
		http.Error(w, `{"error":"bad_path"}`, http.StatusBadRequest)
		return
	}
	full := filepath.Join(dir, clean)
	// Ensure full is still inside dir after symlink-free clean.
	rel, err := filepath.Rel(dir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.Error(w, `{"error":"bad_path"}`, http.StatusBadRequest)
		return
	}
	info, err := os.Stat(full)
	if err != nil {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	if info.IsDir() {
		full = filepath.Join(full, "main.js")
	}
	http.ServeFile(w, &http.Request{URL: nil}, full)
}

// PluginRootFor returns the on-disk path for plugin under root. Used
// by the lifecycle manager.
func (u *UIAssets) PluginRootFor(plugin string) string {
	return filepath.Join(u.Root, plugin)
}

// ExtractTarball unpacks a gzipped tarball into destDir. The tarball
// is assumed to be a NovaNAS plugin package — the validator ensures
// no entry escapes destDir. Returns the manifest bytes (manifest.yaml
// at the root) and the path of the ui/ subdirectory if present.
func ExtractTarball(tarball []byte, destDir string) (manifestBytes []byte, uiDir string, err error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, "", err
	}
	gz, err := gzip.NewReader(bytesReader(tarball))
	if err != nil {
		return nil, "", fmt.Errorf("plugins: gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var hasUI bool
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("plugins: tar: %w", err)
		}
		clean := filepath.Clean("/" + hdr.Name)
		if strings.Contains(clean, "..") {
			return nil, "", fmt.Errorf("plugins: tarball entry escapes root: %q", hdr.Name)
		}
		full := filepath.Join(destDir, clean)
		rel, err := filepath.Rel(destDir, full)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, "", fmt.Errorf("plugins: tarball entry escapes root: %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(full, 0o755); err != nil {
				return nil, "", err
			}
			if rel == "ui" {
				hasUI = true
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return nil, "", err
			}
			f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return nil, "", err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return nil, "", err
			}
			if err := f.Close(); err != nil {
				return nil, "", err
			}
			if rel == "manifest.yaml" {
				manifestBytes, err = os.ReadFile(full)
				if err != nil {
					return nil, "", err
				}
			}
			if strings.HasPrefix(rel, "ui/") || strings.HasPrefix(rel, "ui"+string(filepath.Separator)) {
				hasUI = true
			}
		default:
			// Skip symlinks and other types deliberately.
		}
	}
	if manifestBytes == nil {
		return nil, "", errors.New("plugins: tarball: missing manifest.yaml at root")
	}
	if hasUI {
		uiDir = filepath.Join(destDir, "ui")
	}
	return manifestBytes, uiDir, nil
}

// bytesReader is a tiny io.Reader over a []byte. Avoids importing
// bytes for one tiny use site (and to keep this file self-contained).
type byteSliceReader struct {
	b []byte
	i int
}

func (r *byteSliceReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
func bytesReader(b []byte) io.Reader { return &byteSliceReader{b: b} }
