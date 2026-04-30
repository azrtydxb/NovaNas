package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// Sentinel errors. Handlers map these to specific HTTP status codes.
var (
	ErrNotFound      = errors.New("plugins: not found")
	ErrAlreadyExists = errors.New("plugins: already installed")
	ErrInvalid       = errors.New("plugins: invalid argument")
	// ErrHasDependents is returned by Uninstall when other installed
	// plugins still depend on the target. Callers can override with
	// UninstallOptions.Force.
	ErrHasDependents = errors.New("plugins: has dependents")
)

// UninstallOptions controls Uninstall behaviour.
type UninstallOptions struct {
	// Purge unwinds auto-provisioned needs (datasets, oidcClients, …).
	Purge bool
	// Force bypasses the dependents-guard. Audit-logged so operators can
	// review what got broken.
	Force bool
}

// DependentsError carries the list of installed plugins that block an
// uninstall. The handler unwraps it to populate the 409 envelope.
type DependentsError struct {
	Plugin    string   `json:"plugin"`
	BlockedBy []string `json:"blockedBy"`
}

func (e *DependentsError) Error() string {
	return fmt.Sprintf("plugins: %q has dependents: %s", e.Plugin, strings.Join(e.BlockedBy, ", "))
}

func (e *DependentsError) Unwrap() error { return ErrHasDependents }

// Status values used in the plugins.status column.
const (
	StatusInstalled = "installed"
	StatusFailed    = "failed"
	StatusUpgrading = "upgrading"
)

// Installation is the API value object for an installed plugin.
type Installation struct {
	ID          uuid.UUID     `json:"id"`
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Manifest    *Plugin       `json:"manifest"`
	Status      string        `json:"status"`
	InstalledAt time.Time     `json:"installedAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
	Resources   []ResourceRef `json:"resources,omitempty"`
	// InstalledDeps lists tier-2 dependencies that were installed (or
	// skipped) as part of this Install call. Aurora uses it to surface
	// what was pulled in. Empty when the install had no deps.
	InstalledDeps []PlanStep `json:"installedDeps,omitempty"`
}

// ResourceRef is one auto-provisioned `needs:` resource recorded for
// cleanup at uninstall time.
type ResourceRef struct {
	Type NeedKind `json:"type"`
	ID   string   `json:"id"`
}

// Manager orchestrates install/uninstall/upgrade and owns the live
// state (router mounts + UI bundle registrations). It is safe for
// concurrent use; individual install/upgrade/uninstall operations on
// the same plugin are serialized by an internal name-keyed mutex.
type Manager struct {
	Logger      *slog.Logger
	Queries     *storedb.Queries
	Marketplace *MarketplaceClient
	// Multi, when non-nil, takes precedence over Marketplace for
	// install/upgrade routing. The single-source Marketplace remains
	// supported for backward-compat (tests, dev installs without a DB
	// registry). When Multi is set, per-marketplace pinned trust keys
	// are used and the top-level Verifier is ignored.
	Multi       *MultiMarketplaceClient
	Verifier    *Verifier
	Provisioner NeedsProvisioner
	Router      *Router
	UI          *UIAssets
	// Deployer runs the plugin's runtime — currently only SystemdDeployer
	// for spec.deployment.type=systemd. nil means deployment is skipped
	// (the engine still records the install; the operator may bring up
	// the plugin out-of-band).
	Deployer Deployer

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// Deployer runs the plugin's runtime substrate after needs are
// fulfilled. Implementations are expected to be idempotent on Install
// and best-effort on Uninstall.
type Deployer interface {
	Install(ctx context.Context, manifest *Plugin) error
	Uninstall(ctx context.Context, plugin string) error
	Restart(ctx context.Context, plugin string) error
	Logs(ctx context.Context, plugin string, lines int) ([]string, error)
}

// ManagerOptions wires a Manager.
type ManagerOptions struct {
	Logger      *slog.Logger
	Queries     *storedb.Queries
	Marketplace *MarketplaceClient
	// Multi is the multi-source marketplace client. When non-nil it
	// takes precedence over Marketplace; install/upgrade routes through
	// the registry instead of the single hardcoded source.
	Multi       *MultiMarketplaceClient
	Verifier    *Verifier
	Provisioner NeedsProvisioner
	Router      *Router
	UI          *UIAssets
	Deployer    Deployer
}

// NewManager constructs a Manager. Provisioner nil falls back to
// NopProvisioner. Router/UI nil disables those subsystems (operations
// still record DB rows so a future restart can pick up the missing
// runtime registrations).
func NewManager(o ManagerOptions) *Manager {
	if o.Provisioner == nil {
		o.Provisioner = NopProvisioner{}
	}
	return &Manager{
		Logger:      o.Logger,
		Queries:     o.Queries,
		Marketplace: o.Marketplace,
		Multi:       o.Multi,
		Verifier:    o.Verifier,
		Provisioner: o.Provisioner,
		Router:      o.Router,
		UI:          o.UI,
		Deployer:    o.Deployer,
		locks:       map[string]*sync.Mutex{},
	}
}

func (m *Manager) lockFor(name string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.locks[name]
	if !ok {
		l = &sync.Mutex{}
		m.locks[name] = l
	}
	return l
}

// InstallRequest is the payload to Install.
type InstallRequest struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	ValuesYAML string `json:"valuesYAML,omitempty"`
	// MarketplaceID, when set, pins the install to a specific
	// registered marketplace. Empty means the engine searches all
	// enabled marketplaces in registration order (locked first). Use
	// this to disambiguate when two marketplaces publish a plugin with
	// the same name.
	MarketplaceID string `json:"marketplaceId,omitempty"`
}

// installedLookup is the InstalledLookup adapter that asks the
// Manager's DB for the currently-installed version of a plugin.
type installedLookup struct{ m *Manager }

func (l installedLookup) InstalledVersion(ctx context.Context, name string) (string, bool, error) {
	if l.m == nil || l.m.Queries == nil {
		return "", false, nil
	}
	row, err := l.m.Queries.GetPluginByName(ctx, name)
	if err != nil {
		// Not found vs. real errors are indistinguishable from the
		// generated querier; treat as "not installed". The caller will
		// recover any real DB error from the next query.
		return "", false, nil
	}
	return row.Version, true, nil
}

// Install fetches+verifies+unpacks+provisions+mounts the plugin.
//
// Failures roll back already-completed steps. The returned error is
// the original cause; rollback errors are logged.
//
// Dependency handling: before the existing fetch + verify + provision
// path, the engine fetches the root manifest, resolves its
// `spec.dependencies` graph, and recursively installs any tier-2 deps
// that are missing or at a satisfying version. Bundled deps are noted
// for audit but never installed. If a dep install fails the root
// install fails too; deps that DID install are NOT auto-rolled-back —
// they may be useful on their own. The partial-state outcome is
// audit-logged.
func (m *Manager) Install(ctx context.Context, req InstallRequest) (*Installation, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if m.Queries == nil {
		return nil, errors.New("plugins: store not configured")
	}
	l := m.lockFor(req.Name)
	l.Lock()
	defer l.Unlock()

	// Conflict check.
	if _, err := m.Queries.GetPluginByName(ctx, req.Name); err == nil {
		return nil, ErrAlreadyExists
	}

	// Resolve + install dependencies first. We fetch the root manifest
	// up front so the resolver can see its spec.dependencies. This is a
	// cheap operation (an index lookup + a tarball download) and the
	// downloaded artefacts are reused by fetchVerifyUnpackProvision.
	plan, _, err := m.planInstall(ctx, req.Name, req.Version, req.MarketplaceID)
	if err != nil {
		return nil, err
	}
	for _, step := range plan {
		if step.Name == req.Name {
			continue // root is installed last by the existing path
		}
		if step.Action != PlanActionInstall {
			continue
		}
		if _, err := m.Queries.GetPluginByName(ctx, step.Name); err == nil {
			continue // raced with a parallel install — accept the dep
		}
		if _, derr := m.Install(ctx, InstallRequest{Name: step.Name, Version: step.Version, MarketplaceID: req.MarketplaceID}); derr != nil {
			if m.Logger != nil {
				m.Logger.Error("plugins: dependency install failed",
					"root", req.Name, "dep", step.Name, "err", derr,
					"note", "previously-installed deps were left in place")
			}
			return nil, fmt.Errorf("plugins: install dependency %q for %q: %w", step.Name, req.Name, derr)
		}
		if m.Logger != nil {
			m.Logger.Info("plugins: dependency installed", "root", req.Name, "dep", step.Name, "version", step.Version)
		}
	}

	manifest, destDir, uiDir, resources, err := m.fetchVerifyUnpackProvision(ctx, req.Name, req.Version, req.MarketplaceID)
	if err != nil {
		return nil, err
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		_ = rollbackNeeds(ctx, m.Provisioner, req.Name, resources)
		_ = os.RemoveAll(destDir)
		return nil, err
	}
	id := uuid.New()
	row, err := m.Queries.CreatePlugin(ctx, storedb.CreatePluginParams{
		ID:       pgtype.UUID{Bytes: id, Valid: true},
		Name:     req.Name,
		Version:  manifest.Metadata.Version,
		Manifest: manifestJSON,
		Status:   StatusInstalled,
	})
	if err != nil {
		_ = rollbackNeeds(ctx, m.Provisioner, req.Name, resources)
		_ = os.RemoveAll(destDir)
		return nil, fmt.Errorf("plugins: persist: %w", err)
	}
	for _, r := range resources {
		_ = m.Queries.AddPluginResource(ctx, storedb.AddPluginResourceParams{
			PluginID:     row.ID,
			ResourceType: string(r.Kind),
			ResourceID:   r.ID,
		})
	}

	if m.Router != nil && len(manifest.Spec.API.Routes) > 0 {
		if err := m.Router.Mount(req.Name, manifest.Spec.API.Routes); err != nil {
			m.Logger.Error("plugins: route mount failed", "name", req.Name, "err", err)
		}
	}
	if m.UI != nil && uiDir != "" {
		m.UI.Register(req.Name, uiDir)
	}

	// Runtime deployment. A failure rolls everything back so the engine
	// never advertises a half-installed plugin. The Deployer is
	// responsible for being idempotent — see SystemdDeployer.
	if m.Deployer != nil {
		if err := m.Deployer.Install(ctx, manifest); err != nil {
			if m.Logger != nil {
				m.Logger.Error("plugins: deploy failed; rolling back", "name", req.Name, "err", err)
			}
			if m.Router != nil {
				m.Router.Unmount(req.Name)
			}
			if m.UI != nil {
				m.UI.Deregister(req.Name)
			}
			_ = m.Queries.DeletePlugin(ctx, req.Name)
			_ = rollbackNeeds(ctx, m.Provisioner, req.Name, resources)
			_ = os.RemoveAll(destDir)
			return nil, fmt.Errorf("plugins: deploy: %w", err)
		}
	}

	if m.Logger != nil {
		m.Logger.Info("plugins: installed", "name", req.Name, "version", manifest.Metadata.Version)
	}
	inst := m.toInstallation(row, manifest, resources)
	for _, step := range plan {
		if step.Name == req.Name {
			continue
		}
		inst.InstalledDeps = append(inst.InstalledDeps, step)
	}
	return inst, nil
}

// Uninstall removes runtime mounts + DB row. opts.Purge unwinds the
// auto-provisioned `needs:` resources via the provisioner. opts.Force
// bypasses the dependents-guard; otherwise the call returns
// *DependentsError when other installed plugins still depend on this
// plugin.
func (m *Manager) Uninstall(ctx context.Context, name string, opts UninstallOptions) error {
	if m.Queries == nil {
		return errors.New("plugins: store not configured")
	}
	l := m.lockFor(name)
	l.Lock()
	defer l.Unlock()

	row, err := m.Queries.GetPluginByName(ctx, name)
	if err != nil {
		return ErrNotFound
	}
	if dependents, derr := m.dependentsOf(ctx, name); derr == nil && len(dependents) > 0 {
		if !opts.Force {
			return &DependentsError{Plugin: name, BlockedBy: dependents}
		}
		if m.Logger != nil {
			m.Logger.Warn("plugins: forced uninstall — breaking dependents",
				"name", name, "dependents", dependents)
		}
	}
	purge := opts.Purge

	if m.Deployer != nil {
		if err := m.Deployer.Uninstall(ctx, name); err != nil && m.Logger != nil {
			m.Logger.Warn("plugins: deployer uninstall partial", "name", name, "err", err)
		}
	}
	if m.Router != nil {
		m.Router.Unmount(name)
	}
	if m.UI != nil {
		m.UI.Deregister(name)
	}
	if purge {
		resources, err := m.Queries.ListPluginResources(ctx, row.ID)
		if err == nil {
			done := make([]provisionedResource, 0, len(resources))
			for _, r := range resources {
				done = append(done, provisionedResource{Kind: NeedKind(r.ResourceType), ID: r.ResourceID})
			}
			if rbErr := rollbackNeeds(ctx, m.Provisioner, name, done); rbErr != nil && m.Logger != nil {
				m.Logger.Warn("plugins: purge: provisioner cleanup partial", "name", name, "err", rbErr)
			}
		}
	}
	// Delete on-disk tree (UI bundle, manifest, etc.). Best-effort.
	if m.UI != nil {
		_ = os.RemoveAll(m.UI.PluginRootFor(name))
	}
	if err := m.Queries.DeletePlugin(ctx, name); err != nil {
		return fmt.Errorf("plugins: db delete: %w", err)
	}
	if m.Logger != nil {
		m.Logger.Info("plugins: uninstalled", "name", name, "purge", purge)
	}
	return nil
}

// Upgrade swaps to a newer version side-by-side. The new version's
// API/UI are mounted atomically, then the old runtime is undeployed.
// `needs:` resources are preserved across upgrades; only Mount/UI are
// replaced.
func (m *Manager) Upgrade(ctx context.Context, name, version string) (*Installation, error) {
	if m.Queries == nil {
		return nil, errors.New("plugins: store not configured")
	}
	l := m.lockFor(name)
	l.Lock()
	defer l.Unlock()

	if _, err := m.Queries.GetPluginByName(ctx, name); err != nil {
		return nil, ErrNotFound
	}
	// Upgrade does not pin to a marketplace — the engine searches the
	// same set of enabled marketplaces it would for a fresh install. If
	// operators need to switch upgrade source, uninstall + reinstall.
	manifest, destDir, uiDir, resources, err := m.fetchVerifyUnpackProvision(ctx, name, version, "")
	if err != nil {
		return nil, err
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		_ = os.RemoveAll(destDir)
		return nil, err
	}
	row, err := m.Queries.UpdatePlugin(ctx, storedb.UpdatePluginParams{
		Name:     name,
		Version:  manifest.Metadata.Version,
		Manifest: manifestJSON,
		Status:   StatusInstalled,
	})
	if err != nil {
		_ = os.RemoveAll(destDir)
		return nil, fmt.Errorf("plugins: persist: %w", err)
	}
	// Newly-provisioned resources (rare on upgrade — usually idempotent)
	// are appended to the resource ledger.
	for _, r := range resources {
		_ = m.Queries.AddPluginResource(ctx, storedb.AddPluginResourceParams{
			PluginID:     row.ID,
			ResourceType: string(r.Kind),
			ResourceID:   r.ID,
		})
	}

	if m.Router != nil && len(manifest.Spec.API.Routes) > 0 {
		_ = m.Router.Mount(name, manifest.Spec.API.Routes)
	}
	if m.UI != nil && uiDir != "" {
		m.UI.Register(name, uiDir)
	}
	if m.Deployer != nil {
		if err := m.Deployer.Install(ctx, manifest); err != nil {
			if m.Logger != nil {
				m.Logger.Error("plugins: upgrade deploy failed", "name", name, "err", err)
			}
		}
	}
	if m.Logger != nil {
		m.Logger.Info("plugins: upgraded", "name", name, "version", manifest.Metadata.Version)
	}
	return m.toInstallation(row, manifest, nil), nil
}

// List returns all installed plugins.
func (m *Manager) List(ctx context.Context) ([]Installation, error) {
	if m.Queries == nil {
		return nil, nil
	}
	rows, err := m.Queries.ListPlugins(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Installation, 0, len(rows))
	for _, row := range rows {
		var manifest Plugin
		_ = json.Unmarshal(row.Manifest, &manifest)
		inst := m.toInstallation(row, &manifest, nil)
		out = append(out, *inst)
	}
	return out, nil
}

// Get returns a single installed plugin with its resource ledger.
func (m *Manager) Get(ctx context.Context, name string) (*Installation, error) {
	if m.Queries == nil {
		return nil, ErrNotFound
	}
	row, err := m.Queries.GetPluginByName(ctx, name)
	if err != nil {
		return nil, ErrNotFound
	}
	var manifest Plugin
	_ = json.Unmarshal(row.Manifest, &manifest)
	resources, _ := m.Queries.ListPluginResources(ctx, row.ID)
	inst := m.toInstallation(row, &manifest, nil)
	for _, r := range resources {
		inst.Resources = append(inst.Resources, ResourceRef{Type: NeedKind(r.ResourceType), ID: r.ResourceID})
	}
	return inst, nil
}

// Restart bounces the plugin's runtime via the wired Deployer. The
// plugin must be installed (a DB row must exist) — restarting an
// unknown plugin returns ErrNotFound rather than asking systemd to
// chase a unit that is not on disk. Without a Deployer, restart is
// not available (503 at the HTTP layer).
func (m *Manager) Restart(ctx context.Context, name string) error {
	if m.Queries == nil {
		return ErrNotFound
	}
	if _, err := m.Queries.GetPluginByName(ctx, name); err != nil {
		return ErrNotFound
	}
	if m.Deployer == nil {
		return errors.New("plugins: restart: no deployer configured")
	}
	lock := m.lockFor(name)
	lock.Lock()
	defer lock.Unlock()
	return m.Deployer.Restart(ctx, name)
}

// Logs fetches the most recent journal lines for the plugin's runtime
// unit. Same not-installed contract as Restart; lines is forwarded to
// the Deployer (which clamps to a sane range).
func (m *Manager) Logs(ctx context.Context, name string, lines int) ([]string, error) {
	if m.Queries == nil {
		return nil, ErrNotFound
	}
	if _, err := m.Queries.GetPluginByName(ctx, name); err != nil {
		return nil, ErrNotFound
	}
	if m.Deployer == nil {
		return nil, errors.New("plugins: logs: no deployer configured")
	}
	return m.Deployer.Logs(ctx, name, lines)
}

// RestoreAtStartup re-mounts API routes and re-registers UI bundles
// for every installed plugin. Called from cmd/nova-api/main.go on
// boot. Errors are logged, never returned, so a single broken plugin
// does not block startup.
func (m *Manager) RestoreAtStartup(ctx context.Context) {
	if m.Queries == nil {
		return
	}
	rows, err := m.Queries.ListPlugins(ctx)
	if err != nil {
		if m.Logger != nil {
			m.Logger.Warn("plugins: restore: list", "err", err)
		}
		return
	}
	for _, row := range rows {
		var manifest Plugin
		if err := json.Unmarshal(row.Manifest, &manifest); err != nil {
			if m.Logger != nil {
				m.Logger.Warn("plugins: restore: bad manifest", "name", row.Name, "err", err)
			}
			continue
		}
		if m.Router != nil && len(manifest.Spec.API.Routes) > 0 {
			_ = m.Router.Mount(row.Name, manifest.Spec.API.Routes)
		}
		if m.UI != nil {
			uiDir := filepath.Join(m.UI.PluginRootFor(row.Name), "ui")
			if _, err := os.Stat(uiDir); err == nil {
				m.UI.Register(row.Name, uiDir)
			}
		}
	}
	if m.Logger != nil {
		m.Logger.Info("plugins: restore complete", "count", len(rows))
	}
}

// fetchVerifyUnpackProvision is the shared install/upgrade flow. It
// stops at any failure with the partial state cleaned up.
//
// When the Manager has a MultiMarketplaceClient configured (the
// production path), the install routes through the registry: the
// requested marketplaceID (or empty for "search all enabled") picks
// which marketplace to download from, and verification uses THAT
// marketplace's pinned trust key. The single-source Marketplace +
// Verifier path remains for backward-compat with existing tests and
// dev installs.
func (m *Manager) fetchVerifyUnpackProvision(ctx context.Context, name, version, marketplaceID string) (*Plugin, string, string, []provisionedResource, error) {
	var tarball []byte
	switch {
	case m.Multi != nil:
		_, ver, mp, err := m.Multi.FindVersion(ctx, name, version, marketplaceID)
		if err != nil {
			return nil, "", "", nil, err
		}
		tb, err := m.Multi.DownloadAndVerify(ctx, mp.ID, ver)
		if err != nil {
			return nil, "", "", nil, err
		}
		tarball = tb
	case m.Marketplace != nil:
		_, ver, err := m.Marketplace.FindVersion(ctx, name, version)
		if err != nil {
			return nil, "", "", nil, err
		}
		tb, sig, err := m.Marketplace.DownloadArtifacts(ctx, ver)
		if err != nil {
			return nil, "", "", nil, err
		}
		if m.Verifier != nil {
			if err := m.Verifier.Verify(ctx, tb, sig); err != nil {
				return nil, "", "", nil, fmt.Errorf("plugins: signature: %w", err)
			}
		}
		tarball = tb
	default:
		return nil, "", "", nil, errors.New("plugins: marketplace not configured")
	}
	root := DefaultPluginsRoot
	if m.UI != nil && m.UI.Root != "" {
		root = m.UI.Root
	}
	destDir := filepath.Join(root, name)
	// Wipe any half-installed previous attempt.
	_ = os.RemoveAll(destDir)
	manifestBytes, uiDir, err := ExtractTarball(tarball, destDir)
	if err != nil {
		return nil, "", "", nil, err
	}
	manifest, err := ParseManifest(manifestBytes)
	if err != nil {
		_ = os.RemoveAll(destDir)
		return nil, "", "", nil, err
	}
	if manifest.Metadata.Name != name {
		_ = os.RemoveAll(destDir)
		return nil, "", "", nil, fmt.Errorf("plugins: manifest name %q != requested %q", manifest.Metadata.Name, name)
	}
	resources, err := runNeeds(ctx, m.Provisioner, name, manifest.Spec.Needs)
	if err != nil {
		_ = os.RemoveAll(destDir)
		return nil, "", "", nil, fmt.Errorf("plugins: needs: %w", err)
	}
	return manifest, destDir, uiDir, resources, nil
}

func (m *Manager) toInstallation(row storedb.Plugin, manifest *Plugin, resources []provisionedResource) *Installation {
	id, _ := uuid.FromBytes(row.ID.Bytes[:])
	out := &Installation{
		ID:       id,
		Name:     row.Name,
		Version:  row.Version,
		Manifest: manifest,
		Status:   row.Status,
	}
	if row.InstalledAt.Valid {
		out.InstalledAt = row.InstalledAt.Time
	}
	if row.UpdatedAt.Valid {
		out.UpdatedAt = row.UpdatedAt.Time
	}
	for _, r := range resources {
		out.Resources = append(out.Resources, ResourceRef{Type: r.Kind, ID: r.ID})
	}
	return out
}

// planInstall fetches the root plugin's manifest and runs the
// resolver against it. Returns the install plan (deps in order, root
// last) and the parsed root manifest.
//
// This duplicates a small amount of fetchVerifyUnpackProvision work —
// we download the tarball once here to read its manifest, then again
// inside fetchVerifyUnpackProvision for the actual install. The
// duplication is intentional: keeping plan-time fetches separate from
// install-time fetches means a planned install that fails partway
// (e.g. mid-dependency) doesn't leave half-extracted artefacts at the
// real install root.
func (m *Manager) planInstall(ctx context.Context, name, version, marketplaceID string) ([]PlanStep, *Plugin, error) {
	tarball, err := m.fetchVerifiedTarball(ctx, name, version, marketplaceID)
	if err != nil {
		return nil, nil, err
	}
	tmp, err := os.MkdirTemp("", "nova-plugin-plan-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(tmp)
	manifestBytes, _, err := ExtractTarball(tarball, tmp)
	if err != nil {
		return nil, nil, err
	}
	manifest, err := ParseManifest(manifestBytes)
	if err != nil {
		return nil, nil, err
	}
	resolver := NewResolver(m.dependencyLookup(), installedLookup{m: m})
	plan, err := resolver.Plan(ctx, manifest)
	if err != nil {
		return nil, nil, err
	}
	return plan, manifest, nil
}

// fetchVerifiedTarball downloads the (verified) tarball bytes for the
// requested plugin/version. It is a smaller-surface analog of
// fetchVerifyUnpackProvision used at plan time — no extraction, no
// needs-provisioning. Mirrors the multi/single marketplace switch.
func (m *Manager) fetchVerifiedTarball(ctx context.Context, name, version, marketplaceID string) ([]byte, error) {
	switch {
	case m.Multi != nil:
		_, ver, mp, err := m.Multi.FindVersion(ctx, name, version, marketplaceID)
		if err != nil {
			return nil, err
		}
		return m.Multi.DownloadAndVerify(ctx, mp.ID, ver)
	case m.Marketplace != nil:
		_, ver, err := m.Marketplace.FindVersion(ctx, name, version)
		if err != nil {
			return nil, err
		}
		tb, sig, err := m.Marketplace.DownloadArtifacts(ctx, ver)
		if err != nil {
			return nil, err
		}
		if m.Verifier != nil {
			if err := m.Verifier.Verify(ctx, tb, sig); err != nil {
				return nil, fmt.Errorf("plugins: signature: %w", err)
			}
		}
		return tb, nil
	default:
		return nil, errors.New("plugins: marketplace not configured")
	}
}

// dependencyLookup returns the resolver-friendly lookup adapter. When
// Multi is configured it consults all enabled marketplaces; otherwise
// it falls back to the single MarketplaceClient. Returns nil when no
// marketplace is configured (the resolver tolerates nil for tests
// that pass a fake lookup directly).
func (m *Manager) dependencyLookup() DependencyLookup {
	if m.Multi != nil {
		return multiLookup{m: m.Multi}
	}
	if m.Marketplace != nil {
		return m.Marketplace
	}
	return nil
}

// multiLookup adapts MultiMarketplaceClient.FindVersion to the
// DependencyLookup three-arg signature the resolver expects. It hands
// "" as marketplaceID — the multi client searches all enabled sources
// in registration order, which is the right behaviour for transitive
// deps (we don't pin transitive deps to a specific marketplace).
type multiLookup struct{ m *MultiMarketplaceClient }

func (l multiLookup) FindVersion(ctx context.Context, name, version string) (*IndexPlugin, *IndexVersion, error) {
	idxPlugin, idxVer, _, err := l.m.FindVersion(ctx, name, version, "")
	return idxPlugin, idxVer, err
}

// dependentsOf returns the names of installed plugins whose
// spec.dependencies list `name` as a tier-2 dep.
func (m *Manager) dependentsOf(ctx context.Context, name string) ([]string, error) {
	if m.Queries == nil {
		return nil, nil
	}
	rows, err := m.Queries.ListPlugins(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, row := range rows {
		if row.Name == name {
			continue
		}
		var p Plugin
		if err := json.Unmarshal(row.Manifest, &p); err != nil {
			continue
		}
		for _, d := range p.Spec.Dependencies {
			if d.Source == DependencySourceTier2 && d.Name == name {
				out = append(out, row.Name)
				break
			}
		}
	}
	return out, nil
}

// DependentsOf is the exported form of dependentsOf for the
// /plugins/{name}/dependents handler.
func (m *Manager) DependentsOf(ctx context.Context, name string) ([]string, error) {
	return m.dependentsOf(ctx, name)
}

// Resolver returns a fresh resolver wired to this Manager. Useful for
// the /plugins/{name}/dependencies endpoint.
func (m *Manager) Resolver() *Resolver {
	return NewResolver(m.dependencyLookup(), installedLookup{m: m})
}

// ManifestForPlanning fetches and parses a plugin's manifest from the
// configured marketplace WITHOUT installing anything. Used by
// /plugins/{name}/dependencies to render a tree for an uninstalled
// plugin.
func (m *Manager) ManifestForPlanning(ctx context.Context, name, version string) (*Plugin, error) {
	tarball, err := m.fetchVerifiedTarball(ctx, name, version, "")
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "nova-plugin-plan-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	manifestBytes, _, err := ExtractTarball(tarball, tmp)
	if err != nil {
		return nil, err
	}
	return ParseManifest(manifestBytes)
}
