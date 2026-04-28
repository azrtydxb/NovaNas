// Package rdma enumerates RDMA-capable network interfaces by reading
// /sys/class/infiniband.
package rdma

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const defaultSysPath = "/sys/class/infiniband"

// Lister discovers RDMA-capable network interfaces.
type Lister struct {
	SysPath string // default "/sys/class/infiniband"
}

// Adapter describes one InfiniBand/RoCE host channel adapter and its
// active ports.
type Adapter struct {
	Name    string        `json:"name"` // e.g. "mlx5_0"
	BoardID string        `json:"boardId,omitempty"`
	HCAType string        `json:"hcaType,omitempty"`
	Ports   []AdapterPort `json:"ports"`
}

// AdapterPort describes a single port of an Adapter.
type AdapterPort struct {
	Number    int      `json:"number"`            // 1-indexed
	State     string   `json:"state"`             // "ACTIVE", "DOWN", "INIT", "ARMED"
	LinkLayer string   `json:"linkLayer"`         // "InfiniBand" or "Ethernet" (RoCE)
	GIDs      []string `json:"gids,omitempty"`    // optional, port/<n>/gids/0...
}

func (l *Lister) sysPath() string {
	if l.SysPath != "" {
		return l.SysPath
	}
	return defaultSysPath
}

// List returns all adapters present. Returns empty (no error) when
// /sys/class/infiniband doesn't exist (no RDMA hardware).
func (l *Lister) List(ctx context.Context) ([]Adapter, error) {
	root := l.sysPath()
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var adapters []Adapter
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Adapter directories are typically symlinks; accept both dirs and symlinks.
		if !e.IsDir() && e.Type()&fs.ModeSymlink == 0 {
			continue
		}
		a := readAdapter(filepath.Join(root, e.Name()), e.Name())
		adapters = append(adapters, a)
	}
	sort.Slice(adapters, func(i, j int) bool { return adapters[i].Name < adapters[j].Name })
	return adapters, nil
}

// HasActiveRDMA returns true if at least one adapter has an ACTIVE port.
// Convenience for "should the API offer iSER/NVMe-oF-RDMA?"
func (l *Lister) HasActiveRDMA(ctx context.Context) (bool, error) {
	adapters, err := l.List(ctx)
	if err != nil {
		return false, err
	}
	for _, a := range adapters {
		for _, p := range a.Ports {
			if p.State == "ACTIVE" {
				return true, nil
			}
		}
	}
	return false, nil
}

func readAdapter(dir, name string) Adapter {
	a := Adapter{Name: name}
	if v, ok := readTrim(filepath.Join(dir, "board_id")); ok {
		a.BoardID = v
	}
	if v, ok := readTrim(filepath.Join(dir, "hca_type")); ok {
		a.HCAType = v
	}

	portsDir := filepath.Join(dir, "ports")
	portEntries, err := os.ReadDir(portsDir)
	if err != nil {
		return a
	}
	for _, pe := range portEntries {
		num, err := strconv.Atoi(pe.Name())
		if err != nil {
			continue
		}
		a.Ports = append(a.Ports, readPort(filepath.Join(portsDir, pe.Name()), num))
	}
	sort.Slice(a.Ports, func(i, j int) bool { return a.Ports[i].Number < a.Ports[j].Number })
	return a
}

func readPort(dir string, num int) AdapterPort {
	p := AdapterPort{Number: num}
	// state file format is "4: ACTIVE\n" — take the token after ": ".
	if raw, ok := readTrim(filepath.Join(dir, "state")); ok {
		if i := strings.Index(raw, ":"); i >= 0 {
			p.State = strings.TrimSpace(raw[i+1:])
		}
		// If no colon, leave state empty (don't crash on malformed file).
	}
	if v, ok := readTrim(filepath.Join(dir, "link_layer")); ok {
		p.LinkLayer = v
	}
	p.GIDs = readGIDs(filepath.Join(dir, "gids"))
	return p
}

// readGIDs is best-effort: returns the first non-zero GID, if any.
// Errors are silently ignored.
func readGIDs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	// Sort numerically by filename so index 0 is read first.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Slice(names, func(i, j int) bool {
		ai, errA := strconv.Atoi(names[i])
		bi, errB := strconv.Atoi(names[j])
		if errA == nil && errB == nil {
			return ai < bi
		}
		return names[i] < names[j]
	})

	var out []string
	for _, n := range names {
		v, ok := readTrim(filepath.Join(dir, n))
		if !ok || v == "" {
			continue
		}
		if isZeroGID(v) {
			continue
		}
		out = append(out, v)
		break // first non-zero only, per spec
	}
	return out
}

func isZeroGID(s string) bool {
	for _, r := range s {
		if r == ':' || r == '0' {
			continue
		}
		return false
	}
	return true
}

func readTrim(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}
