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

// stubJournal records args and returns canned output / error.
type stubJournal struct {
	args [][]string
	out  []byte
	err  error
}

func (s *stubJournal) Run(_ context.Context, args ...string) ([]byte, error) {
	s.args = append(s.args, append([]string(nil), args...))
	return s.out, s.err
}

func TestSystemdDeployer_Restart(t *testing.T) {
	r := &recordingRunner{}
	d := &SystemdDeployer{Runner: r}
	if err := d.Restart(context.Background(), "rustfs"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if len(r.args) != 1 {
		t.Fatalf("want 1 systemctl call, got %d (%v)", len(r.args), r.args)
	}
	got := r.args[0]
	if len(got) != 2 || got[0] != "restart" || got[1] != "nova-plugin-rustfs.service" {
		t.Errorf("unexpected args=%v", got)
	}
}

func TestSystemdDeployer_RestartEmptyName(t *testing.T) {
	d := &SystemdDeployer{Runner: &recordingRunner{}}
	if err := d.Restart(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestSystemdDeployer_RestartNoRunner(t *testing.T) {
	d := &SystemdDeployer{}
	if err := d.Restart(context.Background(), "rustfs"); err == nil {
		t.Fatal("expected error when runner is nil")
	}
}

func TestSystemdDeployer_Logs(t *testing.T) {
	j := &stubJournal{out: []byte("2026-04-29T10:00:00 line one\n2026-04-29T10:00:01 line two\n")}
	d := &SystemdDeployer{Journal: j}
	out, err := d.Logs(context.Background(), "rustfs", 50)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 lines, got %d (%v)", len(out), out)
	}
	if out[0] != "2026-04-29T10:00:00 line one" {
		t.Errorf("line[0]=%q", out[0])
	}
	if len(j.args) != 1 {
		t.Fatalf("want 1 journal call, got %d", len(j.args))
	}
	wantArgs := []string{"-u", "nova-plugin-rustfs.service", "-n", "50", "--no-pager", "--output=short-iso"}
	if len(j.args[0]) != len(wantArgs) {
		t.Fatalf("args mismatch: got %v", j.args[0])
	}
	for i, a := range wantArgs {
		if j.args[0][i] != a {
			t.Errorf("arg[%d]=%q want %q", i, j.args[0][i], a)
		}
	}
}

func TestSystemdDeployer_LogsClampsLines(t *testing.T) {
	j := &stubJournal{}
	d := &SystemdDeployer{Journal: j}
	if _, err := d.Logs(context.Background(), "x", 0); err != nil {
		t.Fatal(err)
	}
	if j.args[0][3] != "200" {
		t.Errorf("default lines: got %q", j.args[0][3])
	}
	j.args = nil
	if _, err := d.Logs(context.Background(), "x", 99999); err != nil {
		t.Fatal(err)
	}
	if j.args[0][3] != "5000" {
		t.Errorf("clamped lines: got %q", j.args[0][3])
	}
}

func TestSystemdDeployer_LogsEmpty(t *testing.T) {
	j := &stubJournal{out: nil}
	d := &SystemdDeployer{Journal: j}
	out, err := d.Logs(context.Background(), "x", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Errorf("want empty slice, got %v", out)
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
