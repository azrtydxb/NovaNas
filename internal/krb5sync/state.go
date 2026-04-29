package krb5sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// State is the on-disk state of the sync daemon. It records the last
// successful sync timestamp and a per-user mapping from Keycloak user UUID
// to the set of principal names we have provisioned for that user. The
// mapping lets us detect deletes without requiring a full server-side diff.
//
// File format is JSON; written atomically (write-temp + rename).
type State struct {
	Version        int                 `json:"version"`
	LastSyncUnix   int64               `json:"lastSyncUnix"`
	LastEventUnix  int64               `json:"lastEventUnix,omitempty"`
	UserPrincipals map[string][]string `json:"userPrincipals"`
}

// CurrentVersion is the schema version written by Save. Bump only on
// breaking changes; loaders should accept older versions where feasible.
const CurrentVersion = 1

// NewState returns an empty State with the current schema version.
func NewState() *State {
	return &State{Version: CurrentVersion, UserPrincipals: map[string][]string{}}
}

// Load reads a state file from disk. A missing file returns a fresh empty
// State with no error — first-run startup is a normal case.
func Load(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NewState(), nil
		}
		return nil, fmt.Errorf("krb5sync: load state %s: %w", path, err)
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("krb5sync: parse state %s: %w", path, err)
	}
	if s.UserPrincipals == nil {
		s.UserPrincipals = map[string][]string{}
	}
	if s.Version == 0 {
		s.Version = CurrentVersion
	}
	return &s, nil
}

// Save writes the state to disk atomically. The parent directory must
// exist (the daemon ensures this at startup).
func Save(path string, s *State) error {
	if s == nil {
		return errors.New("krb5sync: nil state")
	}
	if s.UserPrincipals == nil {
		s.UserPrincipals = map[string][]string{}
	}
	// Normalise: sort principal name slices for deterministic output.
	for k, v := range s.UserPrincipals {
		sort.Strings(v)
		s.UserPrincipals[k] = v
	}
	if s.Version == 0 {
		s.Version = CurrentVersion
	}
	buf, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("krb5sync: marshal state: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".state.json.tmp.*")
	if err != nil {
		return fmt.Errorf("krb5sync: create temp state: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName) // best-effort cleanup if rename succeeds it's already gone
	}()
	if _, err := tmp.Write(buf); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("krb5sync: write temp state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("krb5sync: fsync temp state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("krb5sync: close temp state: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("krb5sync: rename temp state: %w", err)
	}
	return nil
}

// MemState is an in-memory wrapper providing concurrent-safe access to
// State fields used by the sync loop. The on-disk State struct is kept
// plain JSON-serialisable so MemState owns the locking.
type MemState struct {
	mu sync.Mutex
	s  *State
}

// NewMemState wraps a State for concurrent use.
func NewMemState(s *State) *MemState {
	if s == nil {
		s = NewState()
	}
	return &MemState{s: s}
}

// Snapshot returns a deep copy of the underlying State.
func (m *MemState) Snapshot() *State {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := &State{
		Version:        m.s.Version,
		LastSyncUnix:   m.s.LastSyncUnix,
		LastEventUnix:  m.s.LastEventUnix,
		UserPrincipals: make(map[string][]string, len(m.s.UserPrincipals)),
	}
	for k, v := range m.s.UserPrincipals {
		cp := make([]string, len(v))
		copy(cp, v)
		out.UserPrincipals[k] = cp
	}
	return out
}

// SetUserPrincipals records that user `uuid` currently has the given
// principal names (replaces any previous record for that user).
func (m *MemState) SetUserPrincipals(uuid string, principals []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if uuid == "" {
		return
	}
	if len(principals) == 0 {
		delete(m.s.UserPrincipals, uuid)
		return
	}
	cp := make([]string, len(principals))
	copy(cp, principals)
	sort.Strings(cp)
	m.s.UserPrincipals[uuid] = cp
}

// MarkSynced stamps the last-sync timestamp.
func (m *MemState) MarkSynced(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.LastSyncUnix = t.Unix()
}

// MarkEvent stamps the last admin-event timestamp consumed.
func (m *MemState) MarkEvent(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.LastEventUnix = t.Unix()
}

// LastEvent returns the last consumed admin-event timestamp (zero if none).
func (m *MemState) LastEvent() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.s.LastEventUnix == 0 {
		return time.Time{}
	}
	return time.Unix(m.s.LastEventUnix, 0)
}

// AllPrincipals returns the union of every principal currently recorded
// in state. Order is sorted and deduplicated.
func (m *MemState) AllPrincipals() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]struct{}{}
	for _, ps := range m.s.UserPrincipals {
		for _, p := range ps {
			seen[p] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
