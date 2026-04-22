package install

import (
	"os"
	"path/filepath"
)

// PersistentSeeder creates the expected directory structure on the persistent
// partition and writes the initial network + version files.
type PersistentSeeder struct {
	DryRun bool
	Log    func(format string, args ...any)
}

// Seed initializes `mount` (the mounted persistent partition) with the
// directory tree described in docs/06, writes the nmstate YAML, and drops a
// version manifest from the bundle.
func (p *PersistentSeeder) Seed(mount string, nmstateYAML string, channel string, version string) error {
	dirs := []string{
		"etc/overlay",
		"var/log",
		"var/lib/novanas",
		"var/lib/rancher/k3s",
		"var/lib/postgresql",
		"var/lib/openbao",
		"home/novanas-admin",
		"opt/novanas",
	}
	for _, d := range dirs {
		full := filepath.Join(mount, d)
		if p.Log != nil {
			p.Log("mkdir -p %s", full)
		}
		if p.DryRun {
			continue
		}
		if err := os.MkdirAll(full, 0o755); err != nil {
			return err
		}
	}

	// Write nmstate yaml.
	nmPath := filepath.Join(mount, "etc", "novanas", "network.yaml")
	if p.Log != nil {
		p.Log("write %s (%d bytes)", nmPath, len(nmstateYAML))
	}
	if !p.DryRun {
		if err := os.MkdirAll(filepath.Dir(nmPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(nmPath, []byte(nmstateYAML), 0o644); err != nil {
			return err
		}
	}

	// Version manifest.
	verPath := filepath.Join(mount, "etc", "novanas", "version")
	body := "channel=" + channel + "\nversion=" + version + "\n"
	if p.Log != nil {
		p.Log("write %s", verPath)
	}
	if !p.DryRun {
		if err := os.WriteFile(verPath, []byte(body), 0o644); err != nil {
			return err
		}
	}

	// Installer-done marker.
	donePath := filepath.Join(mount, "etc", "novanas", "installer-done")
	if !p.DryRun {
		if err := os.WriteFile(donePath, []byte("ok\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}
