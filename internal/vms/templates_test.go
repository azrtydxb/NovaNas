package vms

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalog_RealFile(t *testing.T) {
	// Walk up from the package dir to find deploy/vms/templates.json so
	// tests run from any working directory.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := wd
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(root, "deploy", "vms", "templates.json")); err == nil {
			break
		}
		root = filepath.Dir(root)
	}
	path := filepath.Join(root, "deploy", "vms", "templates.json")
	cat, err := LoadCatalog(path)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	if cat.Count() < 4 {
		t.Fatalf("expected at least 4 templates, got %d", cat.Count())
	}
	for _, id := range []string{"debian-12-cloud", "ubuntu-24.04-cloud", "fedora-40-cloud", "alma-9-cloud"} {
		if _, ok := cat.Get(id); !ok {
			t.Fatalf("missing template %q", id)
		}
	}
	win, ok := cat.Get("windows-11")
	if !ok || !win.RequiresUserSuppliedISO {
		t.Fatalf("windows-11 must be flagged as requiring user-supplied ISO; got %+v", win)
	}
}

func TestLoadCatalog_BadFile(t *testing.T) {
	if _, err := LoadCatalog("/nonexistent/path"); err == nil {
		t.Fatal("expected error")
	}
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "x.json")
	if err := os.WriteFile(bad, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalog(bad); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoadCatalog_RejectsDuplicateIDs(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "t.json")
	if err := os.WriteFile(p, []byte(`{"version":1,"templates":[{"id":"x"},{"id":"x"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalog(p); err == nil {
		t.Fatal("expected duplicate-id error")
	}
}
