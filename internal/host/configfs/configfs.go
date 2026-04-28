// Package configfs provides safe wrapped reads/writes against the Linux
// configfs pseudo-filesystem (typically mounted at /sys/kernel/config).
//
// The configfs filesystem is used by kernel subsystems (notably nvmet for
// NVMe-oF target configuration) to expose configuration knobs as a
// directory tree. Its semantics differ from a normal filesystem in two
// important ways that this package handles:
//
//   - Files cannot be opened with O_TRUNC; they must be opened O_WRONLY
//     and written in a single Write call.
//   - rmdir is not recursive: callers must remove children before
//     removing a parent directory.
//
// All paths are validated to prevent traversal outside Root.
package configfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultRoot is the conventional configfs mount point on Linux.
const DefaultRoot = "/sys/kernel/config"

// ErrNotExist mirrors os.ErrNotExist so callers can use errors.Is.
var ErrNotExist = os.ErrNotExist

// Manager performs safe read/write/mkdir/rmdir operations under Root.
// If Root is empty, DefaultRoot is used. Tests can override Root to a
// temporary directory.
type Manager struct {
	Root string
}

func (m *Manager) root() string {
	if m.Root == "" {
		return DefaultRoot
	}
	return m.Root
}

// validateRel ensures rel is a safe relative path under Root. It rejects
// empty input, leading slash, embedded "..", embedded NUL, backslashes,
// and ASCII control characters. Trailing slashes are trimmed.
func validateRel(rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("configfs: empty relative path")
	}
	if strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("configfs: leading slash not allowed: %q", rel)
	}
	if strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("configfs: NUL byte in path")
	}
	if strings.Contains(rel, "\\") {
		return "", fmt.Errorf("configfs: backslash not allowed: %q", rel)
	}
	for _, r := range rel {
		// Reject all ASCII control chars (0-31 and DEL=127).
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("configfs: control character in path")
		}
	}
	// Reject any ".." path segment.
	cleaned := strings.TrimRight(rel, "/")
	if cleaned == "" {
		return "", fmt.Errorf("configfs: empty relative path")
	}
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".." {
			return "", fmt.Errorf("configfs: path traversal (..) not allowed: %q", rel)
		}
		if seg == "" {
			return "", fmt.Errorf("configfs: empty path segment: %q", rel)
		}
	}
	return cleaned, nil
}

// resolve validates rel and returns the absolute filesystem path under Root.
func (m *Manager) resolve(rel string) (string, error) {
	cleaned, err := validateRel(rel)
	if err != nil {
		return "", err
	}
	return filepath.Join(m.root(), cleaned), nil
}

// Mkdir creates rel (and any missing parents) under Root with mode 0755.
// Idempotent: calling Mkdir on an existing directory is not an error.
func (m *Manager) Mkdir(rel string) error {
	path, err := m.resolve(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("configfs: mkdir %q: %w", rel, err)
	}
	return nil
}

// Rmdir removes a single directory at rel. configfs does not support
// recursive removal; callers must remove children first.
func (m *Manager) Rmdir(rel string) error {
	path, err := m.resolve(rel)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("configfs: rmdir %q: %w", rel, err)
	}
	return nil
}

// WriteFile writes data to rel. configfs files reject O_TRUNC, so the
// file is opened O_WRONLY and the kernel handles replacing the value
// via a single Write.
func (m *Manager) WriteFile(rel string, data []byte) error {
	path, err := m.resolve(rel)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("configfs: open %q: %w", rel, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("configfs: write %q: %w", rel, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("configfs: close %q: %w", rel, err)
	}
	return nil
}

// ReadFile returns the contents of rel. If the target does not exist,
// the returned error wraps ErrNotExist.
func (m *Manager) ReadFile(rel string) ([]byte, error) {
	path, err := m.resolve(rel)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("configfs: read %q: %w", rel, ErrNotExist)
		}
		return nil, fmt.Errorf("configfs: read %q: %w", rel, err)
	}
	return data, nil
}

// Symlink creates a symlink at linkRel (validated, under Root) that
// points to target. target itself is not validated, since configfs
// symlinks may legitimately point to absolute kernel paths or to
// relative paths within the configfs tree.
func (m *Manager) Symlink(target, linkRel string) error {
	linkPath, err := m.resolve(linkRel)
	if err != nil {
		return err
	}
	if err := os.Symlink(target, linkPath); err != nil {
		return fmt.Errorf("configfs: symlink %q -> %q: %w", linkRel, target, err)
	}
	return nil
}

// RemoveSymlink removes the symlink at linkRel.
func (m *Manager) RemoveSymlink(linkRel string) error {
	path, err := m.resolve(linkRel)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("configfs: remove symlink %q: %w", linkRel, err)
	}
	return nil
}

// ListDir returns the names of entries in rel, sorted lexically.
func (m *Manager) ListDir(rel string) ([]string, error) {
	path, err := m.resolve(rel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("configfs: list %q: %w", rel, ErrNotExist)
		}
		return nil, fmt.Errorf("configfs: list %q: %w", rel, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}
