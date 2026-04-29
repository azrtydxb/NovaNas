package plugins

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type recordingRunner struct {
	mu   sync.Mutex
	args [][]string
	fail map[string]error
}

func (r *recordingRunner) Run(_ context.Context, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.args = append(r.args, append([]string(nil), args...))
	if r.fail != nil {
		if err, ok := r.fail[args[0]]; ok {
			return err
		}
	}
	return nil
}

func TestSystemdDeployer_InstallUninstall(t *testing.T) {
	pluginsRoot := t.TempDir()
	unitDir := t.TempDir()
	pluginDir := filepath.Join(pluginsRoot, "rustfs", "deploy")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	unitBody := "[Service]\nExecStart=" + PluginRootMagicToken + "/bin/rustfs serve\nWorkingDirectory=" + PluginRootMagicToken + "\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "rustfs.service"), []byte(unitBody), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &recordingRunner{}
	d := &SystemdDeployer{
		PluginsRoot:     pluginsRoot,
		Runner:          r,
		UnitDirOverride: unitDir,
	}
	manifest := &Plugin{
		Metadata: PluginMetadata{Name: "rustfs"},
		Spec: PluginSpec{Deployment: Deployment{
			Type: DeploymentSystemd, Unit: "rustfs.service",
		}},
	}
	if err := d.Install(context.Background(), manifest); err != nil {
		t.Fatalf("install: %v", err)
	}
	dst := filepath.Join(unitDir, "nova-plugin-rustfs.service")
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read installed unit: %v", err)
	}
	wantPath := filepath.Join(pluginsRoot, "rustfs")
	if !contains2(string(got), wantPath) {
		t.Errorf("unit body did not get path rewritten:\n%s", got)
	}
	if contains2(string(got), PluginRootMagicToken) {
		t.Errorf("magic token not replaced: %s", got)
	}
	if len(r.args) != 2 {
		t.Fatalf("systemctl calls=%d (%v)", len(r.args), r.args)
	}
	if r.args[0][0] != "daemon-reload" {
		t.Errorf("first call=%v", r.args[0])
	}
	if r.args[1][0] != "enable" || r.args[1][1] != "--now" {
		t.Errorf("second call=%v", r.args[1])
	}

	// Uninstall.
	r.args = nil
	if err := d.Uninstall(context.Background(), "rustfs"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("unit file still present")
	}
	if len(r.args) != 2 {
		t.Fatalf("uninstall systemctl calls=%d", len(r.args))
	}
	if r.args[0][0] != "disable" {
		t.Errorf("first call=%v", r.args[0])
	}
	if r.args[1][0] != "daemon-reload" {
		t.Errorf("second call=%v", r.args[1])
	}
}

func TestSystemdDeployer_HelmDeploymentIsNoop(t *testing.T) {
	d := &SystemdDeployer{}
	if err := d.Install(context.Background(), &Plugin{
		Metadata: PluginMetadata{Name: "x"},
		Spec:     PluginSpec{Deployment: Deployment{Type: DeploymentHelm}},
	}); err != nil {
		t.Fatalf("helm should be noop: %v", err)
	}
}

func TestSystemdDeployer_MissingUnit(t *testing.T) {
	d := &SystemdDeployer{
		PluginsRoot:     t.TempDir(),
		UnitDirOverride: t.TempDir(),
	}
	err := d.Install(context.Background(), &Plugin{
		Metadata: PluginMetadata{Name: "rustfs"},
		Spec: PluginSpec{Deployment: Deployment{
			Type: DeploymentSystemd, Unit: "rustfs.service",
		}},
	})
	if err == nil {
		t.Fatal("expected error reading missing unit")
	}
}

func contains2(haystack, needle string) bool {
	return len(needle) <= len(haystack) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	}())
}
