package krb5sync

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLoadMissingFileReturnsEmptyState(t *testing.T) {
	dir := t.TempDir()
	st, err := Load(filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if st.Version != CurrentVersion {
		t.Errorf("expected version %d, got %d", CurrentVersion, st.Version)
	}
	if len(st.UserPrincipals) != 0 {
		t.Errorf("expected empty UserPrincipals, got %v", st.UserPrincipals)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	in := NewState()
	in.LastSyncUnix = 42
	in.UserPrincipals["u1"] = []string{"alice/acme@NOVANAS.LOCAL", "alice/foo@NOVANAS.LOCAL"}
	in.UserPrincipals["u2"] = []string{"bob/acme@NOVANAS.LOCAL"}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(in.UserPrincipals, out.UserPrincipals) {
		t.Errorf("UserPrincipals mismatch: in=%v out=%v", in.UserPrincipals, out.UserPrincipals)
	}
	if out.LastSyncUnix != 42 {
		t.Errorf("LastSyncUnix=%d, want 42", out.LastSyncUnix)
	}
}

func TestMemStateSetAndDelete(t *testing.T) {
	m := NewMemState(NewState())
	m.SetUserPrincipals("u1", []string{"a/x@R", "a/y@R"})
	m.SetUserPrincipals("u2", []string{"b/x@R"})
	snap := m.Snapshot()
	if len(snap.UserPrincipals) != 2 {
		t.Errorf("expected 2 users, got %d", len(snap.UserPrincipals))
	}
	m.SetUserPrincipals("u1", nil) // delete
	if _, ok := m.Snapshot().UserPrincipals["u1"]; ok {
		t.Errorf("u1 should have been deleted")
	}
	all := m.AllPrincipals()
	if len(all) != 1 || all[0] != "b/x@R" {
		t.Errorf("AllPrincipals=%v, want [b/x@R]", all)
	}
}

func TestMemStateMarkEvent(t *testing.T) {
	m := NewMemState(NewState())
	if !m.LastEvent().IsZero() {
		t.Errorf("zero state should have zero LastEvent")
	}
	tt := time.Unix(1700000000, 0)
	m.MarkEvent(tt)
	if got := m.LastEvent(); !got.Equal(tt) {
		t.Errorf("LastEvent=%v, want %v", got, tt)
	}
}

func TestSavePrincipalsAreSorted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	in := NewState()
	in.UserPrincipals["u1"] = []string{"z@R", "a@R", "m@R"}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, _ := Load(path)
	want := []string{"a@R", "m@R", "z@R"}
	if !reflect.DeepEqual(out.UserPrincipals["u1"], want) {
		t.Errorf("got %v, want sorted %v", out.UserPrincipals["u1"], want)
	}
}
