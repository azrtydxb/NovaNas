package configfs

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func newManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{Root: t.TempDir()}
}

func TestValidateRel_Rejections(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"leading slash":    "/foo",
		"path traversal":   "foo/../bar",
		"single dotdot":    "..",
		"NUL byte":         "foo\x00bar",
		"backslash":        "foo\\bar",
		"control char":     "foo\x01bar",
		"empty segment":    "foo//bar",
		"only slash":       "/",
		"trailing dotdot":  "foo/..",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := validateRel(in); err == nil {
				t.Fatalf("expected error for %q", in)
			}
		})
	}
}

func TestValidateRel_Accepts(t *testing.T) {
	cases := []string{
		"foo",
		"foo/bar",
		"a/b/c",
		"with-dash_and.dot/leaf",
		"trailing/",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if _, err := validateRel(in); err != nil {
				t.Fatalf("unexpected error for %q: %v", in, err)
			}
		})
	}
}

func TestMkdirAndListDir(t *testing.T) {
	m := newManager(t)
	if err := m.Mkdir("a/b/c"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	// Idempotent.
	if err := m.Mkdir("a/b/c"); err != nil {
		t.Fatalf("Mkdir idempotent: %v", err)
	}
	if err := m.Mkdir("a/b/d"); err != nil {
		t.Fatalf("Mkdir sibling: %v", err)
	}
	names, err := m.ListDir("a/b")
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	want := []string{"c", "d"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("ListDir = %v, want %v", names, want)
	}
}

func TestRmdir(t *testing.T) {
	m := newManager(t)
	if err := m.Mkdir("dir"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := m.Rmdir("dir"); err != nil {
		t.Fatalf("Rmdir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(m.Root, "dir")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dir still exists: %v", err)
	}
}

func TestWriteFileIdempotent(t *testing.T) {
	m := newManager(t)
	if err := m.Mkdir("d"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	// Pre-create the file. The real configfs creates files automatically
	// inside group directories; in tests we touch it manually since our
	// WriteFile uses O_WRONLY without O_CREATE (matching configfs).
	path := filepath.Join(m.Root, "d/attr")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := m.WriteFile("d/attr", []byte("first")); err != nil {
		t.Fatalf("WriteFile first: %v", err)
	}
	if err := m.WriteFile("d/attr", []byte("second-longer")); err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}
	got, err := m.ReadFile("d/attr")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Without O_TRUNC, writing "second-longer" over "first" leaves the
	// underlying file at "second-longer" only because the new payload is
	// strictly longer. This mirrors configfs semantics (the kernel handles
	// truncation internally on each write).
	if string(got) != "second-longer" {
		t.Fatalf("ReadFile = %q, want %q", got, "second-longer")
	}
}

func TestReadFileNotExist(t *testing.T) {
	m := newManager(t)
	_, err := m.ReadFile("missing")
	if !errors.Is(err, ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}

func TestSymlinkAndRemove(t *testing.T) {
	m := newManager(t)
	if err := m.Mkdir("targets/x"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := m.Mkdir("links"); err != nil {
		t.Fatalf("Mkdir links: %v", err)
	}
	target := filepath.Join(m.Root, "targets/x")
	if err := m.Symlink(target, "links/x"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	got, err := os.Readlink(filepath.Join(m.Root, "links/x"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != target {
		t.Fatalf("Readlink = %q, want %q", got, target)
	}
	if err := m.RemoveSymlink("links/x"); err != nil {
		t.Fatalf("RemoveSymlink: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(m.Root, "links/x")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink still exists: %v", err)
	}
}

func TestListDirNotExist(t *testing.T) {
	m := newManager(t)
	_, err := m.ListDir("nope")
	if !errors.Is(err, ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}

func TestMethodsRejectBadInput(t *testing.T) {
	m := newManager(t)
	bad := "../escape"
	if err := m.Mkdir(bad); err == nil {
		t.Fatal("Mkdir accepted bad input")
	}
	if err := m.Rmdir(bad); err == nil {
		t.Fatal("Rmdir accepted bad input")
	}
	if err := m.WriteFile(bad, nil); err == nil {
		t.Fatal("WriteFile accepted bad input")
	}
	if _, err := m.ReadFile(bad); err == nil {
		t.Fatal("ReadFile accepted bad input")
	}
	if err := m.Symlink("/tmp", bad); err == nil {
		t.Fatal("Symlink accepted bad input")
	}
	if err := m.RemoveSymlink(bad); err == nil {
		t.Fatal("RemoveSymlink accepted bad input")
	}
	if _, err := m.ListDir(bad); err == nil {
		t.Fatal("ListDir accepted bad input")
	}
}

func TestDefaultRoot(t *testing.T) {
	m := &Manager{}
	if m.root() != DefaultRoot {
		t.Fatalf("root() = %q, want %q", m.root(), DefaultRoot)
	}
}
