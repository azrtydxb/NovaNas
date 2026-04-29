package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
)

// Status values used in the plugins.status column.
const (
	StatusInstalled = "installed"
	StatusFailed    = "failed"
	StatusUpgrading = "upgrading"
)

// Installation is the API value object for an installed plugin.
type Installation struct {
	ID          uuid.UUID    `json:"id"`
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Manifest    *Plugin      `json:"manifest"`
	Status      string       `json:"status"`
	InstalledAt time.Time    `json:"installedAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
	Resources   []ResourceRef `json:"resources,omitempty"`
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
	Verifier    *Verifier
	Provisioner NeedsProvisioner
	Router      *Router
	UI          *UIAssets

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// ManagerOptions wires a Manager.
type ManagerOptions struct {
	Logger      *slog.Logger
	Queries     *storedb.Queries
	Marketplace *MarketplaceClient
	Verifier    *Verifier
	Provisioner NeedsProvisioner
	Router      *Router
	UI          *UIAssets
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
		Verifier:    o.Verifier,
		Provisioner: o.Provisioner,
		Router:      o.Router,
		UI:          o.UI,
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
}

// Install fetches+verifies+unpacks+provisions+mounts the plugin.
//
// Failures roll back already-completed steps. The returned error is
// the original cause; rollback errors are logged.
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

	manifest, destDir, uiDir, resources, err := m.fetchVerifyUnpackProvision(ctx, req.Name, req.Version)
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

	if m.Logger != nil {
		m.Logger.Info("plugins: installed", "name", req.Name, "version", manifest.Metadata.Version)
	}
	return m.toInstallation(row, manifest, resources), nil
}

// Uninstall removes runtime mounts + DB row. purge=true also unwinds
// the auto-provisioned `needs:` resources via the provisioner.
func (m *Manager) Uninstall(ctx context.Context, name string, purge bool) error {
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
	manifest, destDir, uiDir, resources, err := m.fetchVerifyUnpackProvision(ctx, name, version)
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
func (m *Manager) fetchVerifyUnpackProvision(ctx context.Context, name, version string) (*Plugin, string, string, []provisionedResource, error) {
	if m.Marketplace == nil {
		return nil, "", "", nil, errors.New("plugins: marketplace not configured")
	}
	_, ver, err := m.Marketplace.FindVersion(ctx, name, version)
	if err != nil {
		return nil, "", "", nil, err
	}
	tarball, sig, err := m.Marketplace.DownloadArtifacts(ctx, ver)
	if err != nil {
		return nil, "", "", nil, err
	}
	if m.Verifier != nil {
		if err := m.Verifier.Verify(ctx, tarball, sig); err != nil {
			return nil, "", "", nil, fmt.Errorf("plugins: signature: %w", err)
		}
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
