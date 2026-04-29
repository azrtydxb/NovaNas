package workloads

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/chart"
	helmrelease "helm.sh/helm/v3/pkg/release"
	"k8s.io/client-go/kubernetes"
)

var _ = time.Now

// fakeHelm is an in-memory helmClient for tests.
type fakeHelm struct {
	mu       sync.Mutex
	releases map[string]*helmrelease.Release
	cs       kubernetes.Interface

	installErr   error
	upgradeErr   error
	uninstallErr error
	listErr      error
}

func newFakeHelm() *fakeHelm {
	return &fakeHelm{releases: map[string]*helmrelease.Release{}}
}

func (f *fakeHelm) kubeClient() kubernetes.Interface { return f.cs }

func (f *fakeHelm) List(_ context.Context) ([]*helmrelease.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*helmrelease.Release, 0, len(f.releases))
	for _, r := range f.releases {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeHelm) Get(_ context.Context, name string) (*helmrelease.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.releases[name]
	if !ok {
		return nil, ErrNotFound
	}
	return r, nil
}

func (f *fakeHelm) Install(_ context.Context, req helmInstallRequest) (*helmrelease.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.installErr != nil {
		return nil, f.installErr
	}
	if _, exists := f.releases[req.ReleaseName]; exists {
		return nil, ErrAlreadyExists
	}
	r := &helmrelease.Release{
		Name:      req.ReleaseName,
		Namespace: req.Namespace,
		Version:   1,
		Chart: &chart.Chart{Metadata: &chart.Metadata{
			Name:       req.ChartName,
			Version:    req.Version,
			AppVersion: "1.0.0",
		}},
		Info:   &helmrelease.Info{Status: helmrelease.StatusDeployed},
		Config: req.Values,
	}
	f.releases[req.ReleaseName] = r
	return r, nil
}

func (f *fakeHelm) Upgrade(_ context.Context, req helmUpgradeRequest) (*helmrelease.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upgradeErr != nil {
		return nil, f.upgradeErr
	}
	r, ok := f.releases[req.ReleaseName]
	if !ok {
		return nil, ErrNotFound
	}
	r.Version++
	if req.Version != "" && r.Chart != nil && r.Chart.Metadata != nil {
		r.Chart.Metadata.Version = req.Version
	}
	if len(req.Values) > 0 {
		r.Config = req.Values
	}
	r.Info = &helmrelease.Info{Status: helmrelease.StatusDeployed}
	return r, nil
}

func (f *fakeHelm) Uninstall(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.uninstallErr != nil {
		return f.uninstallErr
	}
	if _, ok := f.releases[name]; !ok {
		return ErrNotFound
	}
	delete(f.releases, name)
	return nil
}

func (f *fakeHelm) Rollback(_ context.Context, name string, revision int) (*helmrelease.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.releases[name]
	if !ok {
		return nil, ErrNotFound
	}
	r.Version = revision
	return r, nil
}

func newTestEntry() IndexEntry {
	return IndexEntry{
		Name:             "plex",
		Chart:            "plex-media-server",
		Version:          "9.4.7",
		RepoURL:          "https://example.test/charts/",
		DefaultNamespace: "nova-app-plex",
		DefaultValues:    map[string]interface{}{"replicaCount": 1},
	}
}

func newTestManager(t *testing.T) (*Manager, *fakeHelm, *MemoryIndex) {
	t.Helper()
	idx := NewMemoryIndex([]IndexEntry{newTestEntry()})
	helm := newFakeHelm()
	mgr, err := NewManager(ManagerOptions{
		Index:      idx,
		Helm:       helm,
		ChartCache: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr, helm, idx
}

func TestValidateReleaseName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"plex", true},
		{"home-assistant", true},
		{"plex9", true},
		{"", false},
		{"-plex", false},
		{"plex-", false},
		{"Plex", false},
		{"plex.media", false},
		{strings.Repeat("a", 60), false},
	}
	for _, c := range cases {
		err := validateReleaseName(c.name)
		if c.ok && err != nil {
			t.Errorf("%q: want ok, got %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%q: want error", c.name)
		}
		if !c.ok && err != nil && !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%q: error %v should wrap ErrInvalidArgument", c.name, err)
		}
	}
}

func TestParseValuesYAML(t *testing.T) {
	v, err := ParseValuesYAML("foo: bar\nnested:\n  a: 1\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v["foo"] != "bar" {
		t.Errorf("foo=%v", v["foo"])
	}
	if _, err := ParseValuesYAML(""); err != nil {
		t.Errorf("empty parse: %v", err)
	}
	if _, err := ParseValuesYAML(":::not yaml"); err == nil {
		t.Errorf("expected error on bad yaml")
	}
}

func TestManagerInstallListGet(t *testing.T) {
	ctx := context.Background()
	mgr, _, _ := newTestManager(t)

	rel, err := mgr.Install(ctx, InstallRequest{
		IndexName:   "plex",
		ReleaseName: "plex",
		InstalledBy: "alice",
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if rel.Namespace != "nova-app-plex" {
		t.Errorf("ns=%q", rel.Namespace)
	}
	if rel.IndexName != "plex" {
		t.Errorf("indexName=%q", rel.IndexName)
	}

	// Re-install should fail with AlreadyExists.
	if _, err := mgr.Install(ctx, InstallRequest{IndexName: "plex", ReleaseName: "plex"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("re-install: want AlreadyExists, got %v", err)
	}

	list, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "plex" {
		t.Errorf("list=%+v", list)
	}

	// Get without meta store still returns the release.
	d, err := mgr.Get(ctx, "plex")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if d.Chart != "plex-media-server" {
		t.Errorf("chart=%q", d.Chart)
	}
}

func TestManagerInstallValidation(t *testing.T) {
	ctx := context.Background()
	mgr, _, _ := newTestManager(t)
	if _, err := mgr.Install(ctx, InstallRequest{}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("missing indexName: %v", err)
	}
	if _, err := mgr.Install(ctx, InstallRequest{IndexName: "plex", ReleaseName: "Bad-Name"}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("bad release name: %v", err)
	}
	if _, err := mgr.Install(ctx, InstallRequest{IndexName: "missing", ReleaseName: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown index: %v", err)
	}
	if _, err := mgr.Install(ctx, InstallRequest{IndexName: "plex", ReleaseName: "plex", Namespace: "default"}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("non-prefixed namespace: %v", err)
	}
}

func TestManagerUpgrade(t *testing.T) {
	ctx := context.Background()
	mgr, _, _ := newTestManager(t)
	mgr.meta = newMemMeta() // wire metastore so upgrade can recover repoURL

	if _, err := mgr.Install(ctx, InstallRequest{IndexName: "plex", ReleaseName: "plex"}); err != nil {
		t.Fatalf("install: %v", err)
	}
	rel, err := mgr.Upgrade(ctx, "plex", UpgradeRequest{ValuesYAML: "replicaCount: 3\n"})
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if rel.Revision != 2 {
		t.Errorf("revision=%d", rel.Revision)
	}
	// Upgrade with neither version nor values should reject.
	if _, err := mgr.Upgrade(ctx, "plex", UpgradeRequest{}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("empty upgrade: %v", err)
	}
}

func TestManagerUninstall(t *testing.T) {
	ctx := context.Background()
	mgr, _, _ := newTestManager(t)
	if _, err := mgr.Install(ctx, InstallRequest{IndexName: "plex", ReleaseName: "plex"}); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := mgr.Uninstall(ctx, "plex"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := mgr.Get(ctx, "plex"); !errors.Is(err, ErrNotFound) {
		t.Errorf("post-uninstall get: want NotFound got %v", err)
	}
}

func TestManagerRollback(t *testing.T) {
	ctx := context.Background()
	mgr, _, _ := newTestManager(t)
	if _, err := mgr.Install(ctx, InstallRequest{IndexName: "plex", ReleaseName: "plex"}); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := mgr.Rollback(ctx, "plex", 0); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("zero revision: %v", err)
	}
	if _, err := mgr.Rollback(ctx, "plex", 1); err != nil {
		t.Errorf("rollback: %v", err)
	}
}

func TestManagerEventsLogsNoCluster(t *testing.T) {
	ctx := context.Background()
	mgr, _, _ := newTestManager(t)
	if _, err := mgr.Events(ctx, "plex"); !errors.Is(err, ErrNoCluster) {
		t.Errorf("events: %v", err)
	}
	if _, err := mgr.Logs(ctx, "plex", LogRequest{}); !errors.Is(err, ErrNoCluster) {
		t.Errorf("logs: %v", err)
	}
}

func TestIndexFileRoundTrip(t *testing.T) {
	idx := NewMemoryIndex([]IndexEntry{newTestEntry()})
	got, err := idx.List(context.Background())
	if err != nil || len(got) != 1 {
		t.Fatalf("list: %v %+v", err, got)
	}
	d, err := idx.Get(context.Background(), "plex")
	if err != nil || d.Name != "plex" {
		t.Fatalf("get: %v %+v", err, d)
	}
	if _, err := idx.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
}

// memMeta is a minimal in-memory MetaStore for tests.
type memMeta struct {
	mu sync.Mutex
	m  map[string]ReleaseMeta
}

func newMemMeta() *memMeta { return &memMeta{m: map[string]ReleaseMeta{}} }

func (s *memMeta) UpsertReleaseMeta(_ context.Context, m ReleaseMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[m.ReleaseName] = m
	return nil
}
func (s *memMeta) DeleteReleaseMeta(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, name)
	return nil
}
func (s *memMeta) GetReleaseMeta(_ context.Context, name string) (*ReleaseMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[name]
	if !ok {
		return nil, ErrNotFound
	}
	return &v, nil
}
func (s *memMeta) ListReleaseMeta(_ context.Context) ([]ReleaseMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ReleaseMeta, 0, len(s.m))
	for _, v := range s.m {
		out = append(out, v)
	}
	return out, nil
}

// satisfy unused imports
var _ = io.EOF
