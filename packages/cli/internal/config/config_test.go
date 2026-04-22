package config

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	orig := &Config{
		CurrentContext: "default",
		Contexts: []Context{
			{Name: "default", Server: "https://nas.local", InsecureSkipTLSVerify: false},
			{Name: "lab", Server: "https://lab.nas.local", InsecureSkipTLSVerify: true},
		},
	}
	if err := Save(path, orig); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.CurrentContext != "default" || len(got.Contexts) != 2 {
		t.Fatalf("unexpected config: %+v", got)
	}
	if got.Contexts[1].Server != "https://lab.nas.local" {
		t.Fatalf("bad server: %q", got.Contexts[1].Server)
	}
	if cur := got.Current(); cur == nil || cur.Name != "default" {
		t.Fatalf("Current() = %+v", cur)
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if c == nil || len(c.Contexts) != 0 {
		t.Fatalf("expected empty config, got %+v", c)
	}
}

func TestUpsert(t *testing.T) {
	c := &Config{}
	c.Upsert(Context{Name: "a", Server: "s1"})
	c.Upsert(Context{Name: "a", Server: "s2"})
	c.Upsert(Context{Name: "b", Server: "s3"})
	if len(c.Contexts) != 2 {
		t.Fatalf("want 2, got %d", len(c.Contexts))
	}
	if c.Contexts[0].Server != "s2" {
		t.Fatalf("upsert did not replace: %q", c.Contexts[0].Server)
	}
}
