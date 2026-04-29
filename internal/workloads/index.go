package workloads

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
)

// FileIndex is the default IndexProvider — backed by a JSON file on disk
// (deploy/workloads/index.json) that is loaded once at startup and
// reloadable via Reload (operator can call POST
// /api/v1/workloads/index/reload, or send SIGHUP — both end up here).
type FileIndex struct {
	path string

	mu      sync.RWMutex
	entries []IndexEntry
	byName  map[string]IndexEntry
}

// NewFileIndex constructs a FileIndex bound to path. The file is NOT
// read at construction time; call Reload (or rely on the manager's
// startup hook). This split is so a missing/unreadable file at boot is
// recoverable without refusing to start the server.
func NewFileIndex(path string) *FileIndex {
	return &FileIndex{path: path, byName: map[string]IndexEntry{}}
}

// MemoryIndex is a tiny IndexProvider used by tests. It bypasses the JSON
// file and serves a fixed set of entries.
type MemoryIndex struct {
	mu      sync.RWMutex
	entries []IndexEntry
	byName  map[string]IndexEntry
}

// NewMemoryIndex constructs a MemoryIndex from entries (copied).
func NewMemoryIndex(entries []IndexEntry) *MemoryIndex {
	m := &MemoryIndex{byName: map[string]IndexEntry{}}
	m.set(entries)
	return m
}

func (m *MemoryIndex) set(entries []IndexEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append([]IndexEntry(nil), entries...)
	m.byName = make(map[string]IndexEntry, len(entries))
	for _, e := range entries {
		m.byName[e.Name] = e
	}
}

// List returns entries sorted by Name.
func (m *MemoryIndex) List(_ context.Context) ([]IndexEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := append([]IndexEntry(nil), m.entries...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get returns one entry; readme/values-schema are not populated by the
// memory index (chart fetch is out of scope for tests).
func (m *MemoryIndex) Get(_ context.Context, name string) (*IndexEntryDetail, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.byName[name]
	if !ok {
		return nil, ErrNotFound
	}
	return &IndexEntryDetail{IndexEntry: e}, nil
}

// Reload is a no-op for the in-memory provider.
func (m *MemoryIndex) Reload(_ context.Context) error { return nil }

// Reload re-reads the JSON file. Safe to call concurrently with List/Get.
func (f *FileIndex) Reload(_ context.Context) error {
	if f.path == "" {
		return fmt.Errorf("workloads: index path is empty")
	}
	b, err := os.ReadFile(f.path)
	if err != nil {
		return fmt.Errorf("workloads: read index %q: %w", f.path, err)
	}
	var doc IndexFile
	if err := json.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("workloads: parse index %q: %w", f.path, err)
	}
	byName := make(map[string]IndexEntry, len(doc.Entries))
	for _, e := range doc.Entries {
		if e.Name == "" || e.Chart == "" || e.Version == "" || e.RepoURL == "" {
			return fmt.Errorf("workloads: index entry missing required fields: %+v", e)
		}
		if _, dup := byName[e.Name]; dup {
			return fmt.Errorf("workloads: duplicate index entry %q", e.Name)
		}
		byName[e.Name] = e
	}
	f.mu.Lock()
	f.entries = doc.Entries
	f.byName = byName
	f.mu.Unlock()
	return nil
}

// List returns the cached entries.
func (f *FileIndex) List(_ context.Context) ([]IndexEntry, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := append([]IndexEntry(nil), f.entries...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get returns the catalog entry. README + values-schema are best-effort:
// the file index does NOT fetch the chart from upstream — that's the
// Helm client's job, and is layered on in the Manager.
func (f *FileIndex) Get(_ context.Context, name string) (*IndexEntryDetail, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	e, ok := f.byName[name]
	if !ok {
		return nil, ErrNotFound
	}
	return &IndexEntryDetail{IndexEntry: e}, nil
}
