package plugins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PluginRootMagicToken is replaced inside a plugin's systemd unit at
// install time with the absolute path of the plugin's unpacked tree.
// Plugin authors put this token wherever they need an absolute path
// (ExecStart=, WorkingDirectory=, ReadWritePaths=, etc.).
//
// We use a textual token rather than a relative path because systemd
// itself does not interpolate environment variables in `ExecStart`
// arguments at unit-load time, and absolute paths are required there.
const PluginRootMagicToken = "${PLUGIN_ROOT}"

// systemdRunDir is where the deployer drops the rewritten unit files.
// A var so tests can redirect to a t.TempDir().
var systemdRunDir = "/etc/systemd/system"

// SystemctlRunner abstracts the side-effecting `systemctl` calls so
// tests can record them without touching real init. The default
// implementation shells out via os/exec.
type SystemctlRunner interface {
	Run(ctx context.Context, args ...string) error
}

// SystemctlExec is the production runner.
type SystemctlExec struct {
	Bin string
}

func (s *SystemctlExec) Run(ctx context.Context, args ...string) error {
	bin := s.Bin
	if bin == "" {
		bin = "/bin/systemctl"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// JournalctlRunner abstracts read-only journalctl invocations so tests
// can stub the captured output.
type JournalctlRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// JournalctlExec is the production journalctl runner.
type JournalctlExec struct {
	Bin string
}

// Run executes journalctl with the supplied args and returns stdout.
// On non-zero exit the combined output is included in the error so the
// caller can surface it to operators.
func (j *JournalctlExec) Run(ctx context.Context, args ...string) ([]byte, error) {
	bin := j.Bin
	if bin == "" {
		bin = "/bin/journalctl"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		return nil, fmt.Errorf("journalctl %s: %w (%s)", strings.Join(args, " "), err, stderr)
	}
	return out, nil
}

// SystemdDeployer extracts a plugin's systemd unit, rewrites
// ${PLUGIN_ROOT} to the absolute install path, drops the unit at
// /etc/systemd/system/nova-plugin-<name>.service, and brings it up.
//
// Uninstall reverses the sequence. All operations are idempotent.
type SystemdDeployer struct {
	PluginsRoot string
	Runner      SystemctlRunner
	// Journal handles read-only `journalctl` calls for plugin log
	// retrieval. nil falls back to JournalctlExec at first use.
	Journal JournalctlRunner
	Logger  *slog.Logger
	// UnitDirOverride redirects the install-target directory for
	// /etc/systemd/system. Tests set this to a t.TempDir().
	UnitDirOverride string
}

// NewSystemdDeployer constructs a SystemdDeployer with sensible
// production defaults.
func NewSystemdDeployer(pluginsRoot string, logger *slog.Logger) *SystemdDeployer {
	return &SystemdDeployer{
		PluginsRoot: pluginsRoot,
		Runner:      &SystemctlExec{},
		Logger:      logger,
	}
}

func (d *SystemdDeployer) unitName(plugin string) string {
	return "nova-plugin-" + plugin + ".service"
}

func (d *SystemdDeployer) unitDir() string {
	if d.UnitDirOverride != "" {
		return d.UnitDirOverride
	}
	return systemdRunDir
}

func (d *SystemdDeployer) pluginRoot(plugin string) string {
	root := d.PluginsRoot
	if root == "" {
		root = DefaultPluginsRoot
	}
	return filepath.Join(root, plugin)
}

// Install reads the unit named by manifest.Spec.Deployment.Unit from
// the plugin's deploy/ directory, rewrites the magic token, writes it
// to /etc/systemd/system/nova-plugin-<name>.service, daemon-reloads,
// and `enable --now`s it.
//
// If manifest.Spec.Lifecycle.PostInstall is set we also run it (with
// PLUGIN_ROOT set in the env) AFTER the unit file has landed but
// BEFORE `enable --now` — so the script can stage binaries the unit
// will reference.
func (d *SystemdDeployer) Install(ctx context.Context, manifest *Plugin) error {
	if manifest == nil {
		return errors.New("plugins: deploy: nil manifest")
	}
	if manifest.Spec.Deployment.Type != DeploymentSystemd {
		// Helm and other deployment types are out of this deployer's
		// scope; treat as a successful no-op.
		return nil
	}
	name := manifest.Metadata.Name
	unitFile := manifest.Spec.Deployment.Unit
	if unitFile == "" {
		return errors.New("plugins: deploy: empty unit name")
	}
	srcPath := filepath.Join(d.pluginRoot(name), "deploy", unitFile)
	body, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("plugins: deploy: read unit %q: %w", srcPath, err)
	}
	rewritten := strings.ReplaceAll(string(body), PluginRootMagicToken, d.pluginRoot(name))

	dstPath := filepath.Join(d.unitDir(), d.unitName(name))
	if err := os.MkdirAll(d.unitDir(), 0o755); err != nil {
		return fmt.Errorf("plugins: deploy: mkdir %q: %w", d.unitDir(), err)
	}
	if err := os.WriteFile(dstPath, []byte(rewritten), 0o644); err != nil {
		return fmt.Errorf("plugins: deploy: write unit %q: %w", dstPath, err)
	}
	if d.Logger != nil {
		d.Logger.Info("plugins: systemd unit installed", "plugin", name, "path", dstPath)
	}
	if d.Runner == nil {
		// No runner wired (e.g. CI tests against a deployer with only
		// disk side-effects). Skip systemctl invocations.
		return nil
	}
	if err := d.Runner.Run(ctx, "daemon-reload"); err != nil {
		return err
	}

	// Run deploy/install.sh if present BEFORE `systemctl enable --now`.
	// This is the conventional staging hook: it can download binaries,
	// create users, materialize env files from templates — anything the
	// unit needs in place before it can start. The script runs with
	// PLUGIN_ROOT set in the env.
	if err := d.runStagingHook(ctx, name, "deploy/install.sh"); err != nil {
		return fmt.Errorf("plugins: deploy: install hook: %w", err)
	}

	if err := d.Runner.Run(ctx, "enable", "--now", d.unitName(name)); err != nil {
		return err
	}
	if d.Logger != nil {
		d.Logger.Info("plugins: systemd unit enabled", "plugin", name)
	}

	// PostInstall hook runs AFTER the unit is up — for things that need
	// the running service (e.g. seeding initial state via the plugin's
	// own API).
	if hook := manifest.Spec.Lifecycle.PostInstall; hook != "" {
		if err := d.runStagingHook(ctx, name, hook); err != nil {
			return fmt.Errorf("plugins: deploy: postInstall hook: %w", err)
		}
	}
	return nil
}

// runStagingHook executes a script inside the plugin's unpacked tree
// with PLUGIN_ROOT set in its env. Returns nil if the script doesn't
// exist (the script is conventional, not mandatory). Captures combined
// output and includes it on failure for actionable errors.
func (d *SystemdDeployer) runStagingHook(ctx context.Context, plugin, relPath string) error {
	scriptPath := filepath.Join(d.pluginRoot(plugin), relPath)
	st, err := os.Stat(scriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %q: %w", scriptPath, err)
	}
	if st.IsDir() {
		return fmt.Errorf("hook %q is a directory", scriptPath)
	}
	// Make sure the script is executable; some packagers strip perms.
	if st.Mode()&0o111 == 0 {
		if err := os.Chmod(scriptPath, st.Mode()|0o755); err != nil {
			return fmt.Errorf("chmod %q: %w", scriptPath, err)
		}
	}
	cmd := exec.CommandContext(ctx, scriptPath)
	cmd.Dir = d.pluginRoot(plugin)
	cmd.Env = append(os.Environ(),
		"PLUGIN_ROOT="+d.pluginRoot(plugin),
		"PLUGIN_LIB="+d.pluginRoot(plugin), // alias — PLUGIN_LIB and PLUGIN_ROOT are the same dir
		"PLUGIN_NAME="+plugin,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("script %q: %w (%s)", scriptPath, err, strings.TrimSpace(string(out)))
	}
	if d.Logger != nil {
		d.Logger.Info("plugins: hook ran", "plugin", plugin, "script", relPath, "bytes", len(out))
	}
	return nil
}

// Uninstall stops the unit, removes the file, and daemon-reloads.
// Best-effort: errors are returned so the caller can log, but the
// uninstall continues.
func (d *SystemdDeployer) Uninstall(ctx context.Context, plugin string) error {
	var firstErr error
	captureErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if d.Runner != nil {
		captureErr(d.Runner.Run(ctx, "disable", "--now", d.unitName(plugin)))
	}
	dstPath := filepath.Join(d.unitDir(), d.unitName(plugin))
	if err := os.Remove(dstPath); err != nil && !os.IsNotExist(err) {
		captureErr(err)
	}
	if d.Runner != nil {
		captureErr(d.Runner.Run(ctx, "daemon-reload"))
	}
	if d.Logger != nil {
		d.Logger.Info("plugins: systemd unit removed", "plugin", plugin)
	}
	return firstErr
}

// Restart runs `systemctl restart nova-plugin-<name>.service`. The
// runner is responsible for surfacing stderr in any returned error so
// the HTTP layer can echo it back to operators.
func (d *SystemdDeployer) Restart(ctx context.Context, plugin string) error {
	if plugin == "" {
		return errors.New("plugins: restart: empty plugin name")
	}
	if d.Runner == nil {
		return errors.New("plugins: restart: no systemctl runner configured")
	}
	if err := d.Runner.Run(ctx, "restart", d.unitName(plugin)); err != nil {
		return err
	}
	if d.Logger != nil {
		d.Logger.Info("plugins: systemd unit restarted", "plugin", plugin)
	}
	return nil
}

// Logs returns the most recent N journal lines for the plugin's unit.
// lines is clamped to [1,5000]; values <=0 fall back to 200.
func (d *SystemdDeployer) Logs(ctx context.Context, plugin string, lines int) ([]string, error) {
	if plugin == "" {
		return nil, errors.New("plugins: logs: empty plugin name")
	}
	if lines <= 0 {
		lines = 200
	}
	if lines > 5000 {
		lines = 5000
	}
	runner := d.Journal
	if runner == nil {
		runner = &JournalctlExec{}
	}
	args := []string{
		"-u", d.unitName(plugin),
		"-n", fmt.Sprintf("%d", lines),
		"--no-pager",
		"--output=short-iso",
	}
	out, err := runner.Run(ctx, args...)
	if err != nil {
		return nil, err
	}
	// Split on newlines; drop the trailing empty produced by journalctl.
	raw := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(raw) == 1 && raw[0] == "" {
		return []string{}, nil
	}
	return raw, nil
}
