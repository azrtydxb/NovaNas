package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/novanas/installer/internal/install"
)

func TestRenderGrubConfigMentionsSlots(t *testing.T) {
	cfg := install.RenderGrubConfig()
	for _, want := range []string{"Slot A", "Slot B", "BOOT_ORDER", "rauc.slot=A", "rauc.slot=B"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("grub.cfg missing %q", want)
		}
	}
}

func TestPersistentSeederDryRun(t *testing.T) {
	seeder := &install.PersistentSeeder{DryRun: true}
	if err := seeder.Seed("/tmp/nonexistent-unused", "hostname: nas\n", "stable", "1.2.3"); err != nil {
		t.Errorf("dry run should never fail: %v", err)
	}
}

func TestPersistentSeederRealWritesLayout(t *testing.T) {
	dir := t.TempDir()
	seeder := &install.PersistentSeeder{DryRun: false}
	if err := seeder.Seed(dir, "hostname: nas\n", "stable", "1.2.3"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for _, want := range []string{
		"etc/novanas/network.yaml",
		"etc/novanas/version",
		"etc/novanas/installer-done",
		"var/log",
		"var/lib/novanas",
		"opt/novanas",
	} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
	body, _ := os.ReadFile(filepath.Join(dir, "etc/novanas/version"))
	if !strings.Contains(string(body), "version=1.2.3") {
		t.Errorf("version file content unexpected: %q", string(body))
	}
}

func TestRAUCExtractorVerifyMissing(t *testing.T) {
	r := &install.RAUCExtractor{DryRun: true}
	if err := r.Verify("/nonexistent/bundle"); err == nil {
		t.Error("expected error for missing bundle")
	}
}
