package workloads

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"helm.sh/helm/v3/pkg/cli"
	helmrelease "helm.sh/helm/v3/pkg/release"
)

// ManagerOptions configures Manager.
type ManagerOptions struct {
	Logger     *slog.Logger
	Index      IndexProvider
	Helm       helmClient
	Settings   *cli.EnvSettings // optional, used for chart README fetch
	IndexPath  string           // for the audit/log line in Reload
	MetaStore  MetaStore        // optional NovaNAS-side metadata persistence
	ChartCache time.Duration    // README cache TTL; 0 disables caching
}

// MetaStore is the optional persistence layer for NovaNAS-side install
// metadata (who installed what, when). The helm release secret already
// records most of the runtime state; this is purely for the audit log.
//
// The interface is intentionally a tiny CRUD shape so the store layer
// can implement it as one extra table without dragging the workloads
// package into the sqlc world.
type MetaStore interface {
	UpsertReleaseMeta(ctx context.Context, m ReleaseMeta) error
	DeleteReleaseMeta(ctx context.Context, releaseName string) error
	GetReleaseMeta(ctx context.Context, releaseName string) (*ReleaseMeta, error)
	ListReleaseMeta(ctx context.Context) ([]ReleaseMeta, error)
}

// ReleaseMeta is the NovaNAS-side metadata for an installed app.
type ReleaseMeta struct {
	ReleaseName string    `json:"releaseName"`
	IndexName   string    `json:"indexName"`
	Namespace   string    `json:"namespace"`
	InstalledBy string    `json:"installedBy"`
	InstalledAt time.Time `json:"installedAt"`
}

// Manager is the production Lifecycle implementation.
type Manager struct {
	logger    *slog.Logger
	index     IndexProvider
	helm      helmClient
	settings  *cli.EnvSettings
	indexPath string
	meta      MetaStore

	readmeMu    sync.Mutex
	readmeCache map[string]readmeEntry
	readmeTTL   time.Duration
}

type readmeEntry struct {
	when   time.Time
	readme string
	schema map[string]interface{}
}

// NewManager constructs a Manager. opts.Index and opts.Helm are required.
func NewManager(opts ManagerOptions) (*Manager, error) {
	if opts.Index == nil {
		return nil, errors.New("workloads: ManagerOptions.Index is required")
	}
	if opts.Helm == nil {
		return nil, errors.New("workloads: ManagerOptions.Helm is required")
	}
	settings := opts.Settings
	if settings == nil {
		settings = cli.New()
	}
	ttl := opts.ChartCache
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	return &Manager{
		logger:      opts.Logger,
		index:       opts.Index,
		helm:        opts.Helm,
		settings:    settings,
		indexPath:   opts.IndexPath,
		meta:        opts.MetaStore,
		readmeCache: map[string]readmeEntry{},
		readmeTTL:   ttl,
	}, nil
}

// IndexList delegates to the IndexProvider.
func (m *Manager) IndexList(ctx context.Context) ([]IndexEntry, error) {
	return m.index.List(ctx)
}

// IndexGet returns the catalog entry plus a best-effort README fetch.
func (m *Manager) IndexGet(ctx context.Context, name string) (*IndexEntryDetail, error) {
	d, err := m.index.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	// Cached README/schema.
	key := d.Name + "@" + d.Version
	m.readmeMu.Lock()
	hit, ok := m.readmeCache[key]
	m.readmeMu.Unlock()
	if ok && time.Since(hit.when) < m.readmeTTL {
		d.Readme = hit.readme
		d.ValuesSchema = hit.schema
		return d, nil
	}
	readme, schema, ferr := FetchChartReadme(ctx, m.settings, d.IndexEntry)
	if ferr != nil {
		if m.logger != nil {
			m.logger.Debug("workloads: readme fetch failed", "name", name, "err", ferr)
		}
		// Return the metadata anyway — the catalog should not break
		// because the upstream repo is offline.
		return d, nil
	}
	m.readmeMu.Lock()
	m.readmeCache[key] = readmeEntry{when: time.Now(), readme: readme, schema: schema}
	m.readmeMu.Unlock()
	d.Readme = readme
	d.ValuesSchema = schema
	return d, nil
}

// IndexReload reloads the file index (no-op for memory).
func (m *Manager) IndexReload(ctx context.Context) error {
	if err := m.index.Reload(ctx); err != nil {
		return err
	}
	if m.logger != nil {
		m.logger.Info("workloads: index reloaded", "path", m.indexPath)
	}
	m.readmeMu.Lock()
	m.readmeCache = map[string]readmeEntry{}
	m.readmeMu.Unlock()
	return nil
}

// List returns all NovaNAS-managed releases.
func (m *Manager) List(ctx context.Context) ([]Release, error) {
	rels, err := m.helm.List(ctx)
	if err != nil {
		return nil, err
	}
	metaByName := map[string]ReleaseMeta{}
	if m.meta != nil {
		ms, err := m.meta.ListReleaseMeta(ctx)
		if err != nil && m.logger != nil {
			m.logger.Warn("workloads: list meta", "err", err)
		}
		for _, x := range ms {
			metaByName[x.ReleaseName] = x
		}
	}
	out := make([]Release, 0, len(rels))
	for _, r := range rels {
		out = append(out, releaseFromHelm(r, metaByName[r.Name]))
	}
	return out, nil
}

// Get returns an installed release with extended detail.
func (m *Manager) Get(ctx context.Context, releaseName string) (*ReleaseDetail, error) {
	if err := validateReleaseName(releaseName); err != nil {
		return nil, err
	}
	rel, err := m.helm.Get(ctx, releaseName)
	if err != nil {
		return nil, err
	}
	var meta ReleaseMeta
	if m.meta != nil {
		got, gerr := m.meta.GetReleaseMeta(ctx, releaseName)
		if gerr == nil && got != nil {
			meta = *got
		}
	}
	d := &ReleaseDetail{Release: releaseFromHelm(rel, meta)}
	if rel.Config != nil {
		d.Values = rel.Config
	}
	d.Pods, _ = m.podSummary(ctx, rel.Namespace)
	d.Resources = manifestResources(rel)
	return d, nil
}

// Install installs a chart from the curated index.
func (m *Manager) Install(ctx context.Context, req InstallRequest) (*Release, error) {
	if req.IndexName == "" {
		return nil, errInvalid("indexName is required")
	}
	if err := validateReleaseName(req.ReleaseName); err != nil {
		return nil, err
	}
	entry, err := m.index.Get(ctx, req.IndexName)
	if err != nil {
		return nil, err
	}
	values, err := ParseValuesYAML(req.ValuesYAML)
	if err != nil {
		return nil, errInvalid(err.Error())
	}
	if len(values) == 0 && len(entry.DefaultValues) > 0 {
		values = cloneValues(entry.DefaultValues)
	}
	ns := strings.TrimSpace(req.Namespace)
	if ns == "" {
		ns = NamespacePrefix + req.ReleaseName
	}
	if !strings.HasPrefix(ns, NamespacePrefix) {
		return nil, errInvalid(fmt.Sprintf("namespace must start with %q", NamespacePrefix))
	}
	rel, err := m.helm.Install(ctx, helmInstallRequest{
		ReleaseName: req.ReleaseName,
		Namespace:   ns,
		ChartName:   entry.Chart,
		Version:     entry.Version,
		RepoURL:     entry.RepoURL,
		Values:      values,
	})
	if err != nil {
		return nil, err
	}
	if m.meta != nil {
		_ = m.meta.UpsertReleaseMeta(ctx, ReleaseMeta{
			ReleaseName: req.ReleaseName,
			IndexName:   req.IndexName,
			Namespace:   ns,
			InstalledBy: req.InstalledBy,
			InstalledAt: time.Now().UTC(),
		})
	}
	out := releaseFromHelm(rel, ReleaseMeta{IndexName: req.IndexName, InstalledBy: req.InstalledBy})
	return &out, nil
}

// Upgrade upgrades an existing release. Either Version, ValuesYAML, or
// both must be set.
func (m *Manager) Upgrade(ctx context.Context, releaseName string, req UpgradeRequest) (*Release, error) {
	if err := validateReleaseName(releaseName); err != nil {
		return nil, err
	}
	if req.Version == "" && req.ValuesYAML == "" {
		return nil, errInvalid("either version or valuesYAML must be set")
	}
	current, err := m.helm.Get(ctx, releaseName)
	if err != nil {
		return nil, err
	}
	values, err := ParseValuesYAML(req.ValuesYAML)
	if err != nil {
		return nil, errInvalid(err.Error())
	}
	chartName := ""
	repoURL := ""
	version := req.Version
	if current.Chart != nil && current.Chart.Metadata != nil {
		chartName = current.Chart.Metadata.Name
		if version == "" {
			version = current.Chart.Metadata.Version
		}
	}
	// Try to recover repo URL from the index. Without it we cannot
	// re-locate the chart on a version bump.
	if m.meta != nil {
		meta, _ := m.meta.GetReleaseMeta(ctx, releaseName)
		if meta != nil && meta.IndexName != "" {
			if e, gerr := m.index.Get(ctx, meta.IndexName); gerr == nil {
				repoURL = e.RepoURL
				if chartName == "" {
					chartName = e.Chart
				}
			}
		}
	}
	if chartName == "" {
		return nil, errInvalid("could not determine chart name for upgrade")
	}
	rel, err := m.helm.Upgrade(ctx, helmUpgradeRequest{
		ReleaseName: releaseName,
		Namespace:   current.Namespace,
		ChartName:   chartName,
		Version:     version,
		RepoURL:     repoURL,
		Values:      values,
	})
	if err != nil {
		return nil, err
	}
	out := releaseFromHelm(rel, ReleaseMeta{})
	return &out, nil
}

// Uninstall removes a release and its namespace.
func (m *Manager) Uninstall(ctx context.Context, releaseName string) error {
	if err := validateReleaseName(releaseName); err != nil {
		return err
	}
	if err := m.helm.Uninstall(ctx, releaseName); err != nil {
		return err
	}
	if m.meta != nil {
		_ = m.meta.DeleteReleaseMeta(ctx, releaseName)
	}
	return nil
}

// Rollback rolls a release back to revision.
func (m *Manager) Rollback(ctx context.Context, releaseName string, revision int) (*Release, error) {
	if err := validateReleaseName(releaseName); err != nil {
		return nil, err
	}
	if revision < 1 {
		return nil, errInvalid("revision must be >= 1")
	}
	rel, err := m.helm.Rollback(ctx, releaseName, revision)
	if err != nil {
		return nil, err
	}
	out := releaseFromHelm(rel, ReleaseMeta{})
	return &out, nil
}

// Events returns recent k8s events from the release namespace.
func (m *Manager) Events(ctx context.Context, releaseName string) ([]Event, error) {
	if err := validateReleaseName(releaseName); err != nil {
		return nil, err
	}
	cs := m.helm.kubeClient()
	if cs == nil {
		return nil, ErrNoCluster
	}
	ns := NamespacePrefix + releaseName
	evs, err := cs.CoreV1().Events(ns).List(ctx, metav1.ListOptions{Limit: 200})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("workloads: list events: %w", err)
	}
	out := make([]Event, 0, len(evs.Items))
	for _, e := range evs.Items {
		out = append(out, Event{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			Object:    e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name,
			Count:     e.Count,
			FirstSeen: e.FirstTimestamp.Time,
			LastSeen:  e.LastTimestamp.Time,
		})
	}
	return out, nil
}

// Logs streams pod logs. If req.Pod is empty, the first running pod in
// the namespace is selected. Caller MUST close the returned reader.
func (m *Manager) Logs(ctx context.Context, releaseName string, req LogRequest) (io.ReadCloser, error) {
	if err := validateReleaseName(releaseName); err != nil {
		return nil, err
	}
	cs := m.helm.kubeClient()
	if cs == nil {
		return nil, ErrNoCluster
	}
	ns := NamespacePrefix + releaseName
	pod := req.Pod
	if pod == "" {
		p, err := pickPod(ctx, cs, ns)
		if err != nil {
			return nil, err
		}
		pod = p
	}
	return PodLogs(ctx, m.helm, ns, pod, req.Container, req.Follow, req.Previous, req.Timestamps, req.TailLines, req.Since)
}

// podSummary builds a small projection of pods for ReleaseDetail.
func (m *Manager) podSummary(ctx context.Context, ns string) ([]PodInfo, error) {
	cs := m.helm.kubeClient()
	if cs == nil {
		return nil, nil
	}
	pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil //nolint:nilerr // non-fatal for the summary
	}
	out := make([]PodInfo, 0, len(pods.Items))
	for _, p := range pods.Items {
		ready := true
		var restarts int32
		var containers []string
		for _, c := range p.Status.ContainerStatuses {
			if !c.Ready {
				ready = false
			}
			restarts += c.RestartCount
		}
		for _, c := range p.Spec.Containers {
			containers = append(containers, c.Name)
		}
		out = append(out, PodInfo{
			Name:       p.Name,
			Phase:      string(p.Status.Phase),
			Ready:      ready,
			Restarts:   restarts,
			Containers: containers,
			NodeName:   p.Spec.NodeName,
		})
	}
	return out, nil
}

func pickPod(ctx context.Context, cs kubernetes.Interface, ns string) (string, error) {
	pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("workloads: list pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return "", ErrNotFound
	}
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			return p.Name, nil
		}
	}
	return pods.Items[0].Name, nil
}

func releaseFromHelm(r *helmrelease.Release, meta ReleaseMeta) Release {
	if r == nil {
		return Release{}
	}
	rel := Release{
		Name:        r.Name,
		Namespace:   r.Namespace,
		IndexName:   meta.IndexName,
		InstalledBy: meta.InstalledBy,
	}
	if r.Chart != nil && r.Chart.Metadata != nil {
		rel.Chart = r.Chart.Metadata.Name
		rel.Version = r.Chart.Metadata.Version
		rel.AppVersion = r.Chart.Metadata.AppVersion
	}
	if r.Info != nil {
		rel.Status = r.Info.Status.String()
		rel.Notes = r.Info.Notes
		rel.Updated = r.Info.LastDeployed.Time
	}
	rel.Revision = r.Version
	return rel
}

// manifestResources is intentionally minimal: parse the rendered manifest
// only enough to display "this chart shipped these top-level objects".
// We avoid pulling kubectl/yaml-list parsers in the hot path; instead
// the GUI is told to fetch detailed resource state via existing k8s
// APIs (live pods are surfaced via Pods).
func manifestResources(r *helmrelease.Release) []ResourceRef {
	if r == nil || r.Manifest == "" {
		return nil
	}
	var out []ResourceRef
	for _, doc := range strings.Split(r.Manifest, "\n---") {
		kind, name := scanKindName(doc)
		if kind == "" || name == "" {
			continue
		}
		out = append(out, ResourceRef{Kind: kind, Name: name, Namespace: r.Namespace})
	}
	return out
}

func scanKindName(doc string) (string, string) {
	var kind, name string
	for _, line := range strings.Split(doc, "\n") {
		l := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(l, "kind:"):
			kind = strings.TrimSpace(strings.TrimPrefix(l, "kind:"))
		case strings.HasPrefix(l, "name:") && name == "":
			name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(l, "name:")), `"'`)
		}
		if kind != "" && name != "" {
			return kind, name
		}
	}
	return kind, name
}

func cloneValues(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		switch vv := v.(type) {
		case map[string]interface{}:
			out[k] = cloneValues(vv)
		case []interface{}:
			arr := make([]interface{}, len(vv))
			copy(arr, vv)
			out[k] = arr
		default:
			out[k] = vv
		}
	}
	return out
}
