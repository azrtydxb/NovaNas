package workloads

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// helmClient is the narrow contract the Manager uses against Helm. The
// real implementation in this file shells out to action.NewInstall etc;
// tests substitute a fake.
type helmClient interface {
	List(ctx context.Context) ([]*release.Release, error)
	Get(ctx context.Context, name string) (*release.Release, error)
	Install(ctx context.Context, req helmInstallRequest) (*release.Release, error)
	Upgrade(ctx context.Context, req helmUpgradeRequest) (*release.Release, error)
	Uninstall(ctx context.Context, name string) error
	Rollback(ctx context.Context, name string, revision int) (*release.Release, error)

	// kubeClient returns the typed clientset. It may be nil if the helm
	// client was constructed without a reachable cluster — callers must
	// degrade gracefully (Events/Logs return ErrNoCluster).
	kubeClient() kubernetes.Interface
}

type helmInstallRequest struct {
	ReleaseName string
	Namespace   string
	ChartName   string
	Version     string
	RepoURL     string
	Values      map[string]interface{}
}

type helmUpgradeRequest struct {
	ReleaseName string
	Namespace   string
	ChartName   string
	Version     string
	RepoURL     string
	Values      map[string]interface{}
}

// realHelm is the production helmClient. It builds a fresh
// action.Configuration per call so concurrent installs in different
// namespaces don't share state. Repo cache (chart download) is shared
// via the package-level settings.
type realHelm struct {
	logger     *slog.Logger
	kubeconfig string
	clientset  kubernetes.Interface
	settings   *cli.EnvSettings
}

// NewHelmClient wires up the production helm/k8s clients. kubeconfigPath
// is typically /etc/rancher/k3s/k3s.yaml. When the file is missing or
// the cluster is unreachable, a degraded client is returned that can
// still answer "no releases", and Install/Upgrade/Uninstall return
// ErrNoCluster — letting the API stay up on a NAS where k3s hasn't
// been bootstrapped yet.
func NewHelmClient(logger *slog.Logger, kubeconfigPath string) (helmClient, error) {
	settings := cli.New()
	if kubeconfigPath != "" {
		settings.KubeConfig = kubeconfigPath
	}
	rc, err := loadRESTConfig(kubeconfigPath)
	if err != nil {
		if logger != nil {
			logger.Warn("workloads: kubeconfig unavailable; running in degraded mode",
				"path", kubeconfigPath, "err", err)
		}
		return &realHelm{logger: logger, kubeconfig: kubeconfigPath, settings: settings}, nil
	}
	cs, err := kubernetes.NewForConfig(rc)
	if err != nil {
		return nil, fmt.Errorf("workloads: build clientset: %w", err)
	}
	return &realHelm{
		logger:     logger,
		kubeconfig: kubeconfigPath,
		clientset:  cs,
		settings:   settings,
	}, nil
}

func loadRESTConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		if _, err := os.Stat(kubeconfigPath); err == nil {
			return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		}
	}
	// In-cluster fallback (when nova-api runs as a pod, not on the host).
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	return nil, errors.New("no kubeconfig and not running in-cluster")
}

func (h *realHelm) kubeClient() kubernetes.Interface { return h.clientset }

// actionConfig builds a per-namespace action.Configuration. The Helm
// secret-storage driver is the default for namespace-scoped releases
// on k3s and is what we want here.
func (h *realHelm) actionConfig(namespace string) (*action.Configuration, error) {
	if h.settings == nil {
		h.settings = cli.New()
	}
	cfg := new(action.Configuration)
	flags := genericclioptions.NewConfigFlags(false)
	flags.KubeConfig = stringPtr(h.kubeconfig)
	if namespace != "" {
		flags.Namespace = stringPtr(namespace)
	}
	logFn := func(format string, args ...interface{}) {
		if h.logger != nil {
			h.logger.Debug("helm", "msg", fmt.Sprintf(format, args...))
		}
	}
	if err := cfg.Init(flags, namespace, "secrets", logFn); err != nil {
		return nil, fmt.Errorf("workloads: helm init: %w", err)
	}
	return cfg, nil
}

func stringPtr(s string) *string { return &s }

func (h *realHelm) List(ctx context.Context) ([]*release.Release, error) {
	if h.clientset == nil {
		return nil, nil
	}
	// List across all namespaces matching nova-app-* by listing every
	// namespace's helm storage. Helm's action.List with AllNamespaces=true
	// requires cluster-scoped read; on k3s nova-api typically has it.
	cfg, err := h.actionConfig("")
	if err != nil {
		return nil, err
	}
	lister := action.NewList(cfg)
	lister.AllNamespaces = true
	lister.All = true
	releases, err := lister.Run()
	if err != nil {
		return nil, fmt.Errorf("workloads: helm list: %w", err)
	}
	out := releases[:0]
	for _, r := range releases {
		if r != nil && strings.HasPrefix(r.Namespace, NamespacePrefix) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (h *realHelm) Get(_ context.Context, name string) (*release.Release, error) {
	if h.clientset == nil {
		return nil, ErrNoCluster
	}
	ns := NamespacePrefix + name
	cfg, err := h.actionConfig(ns)
	if err != nil {
		return nil, err
	}
	get := action.NewGet(cfg)
	rel, err := get.Run(name)
	if err != nil {
		if errors.Is(err, driverNotFound) || strings.Contains(err.Error(), "not found") {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("workloads: helm get %q: %w", name, err)
	}
	return rel, nil
}

// driverNotFound is helm's storage-driver "release not found" sentinel,
// which is unfortunately defined in an internal driver package. We
// match by error string elsewhere; this var exists so future helm
// versions that export the sentinel can be wired in by replacing this
// declaration.
var driverNotFound = errors.New("release: not found")

func (h *realHelm) ensureNamespace(ctx context.Context, ns string) error {
	if h.clientset == nil {
		return ErrNoCluster
	}
	_, err := h.clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("workloads: get namespace %q: %w", ns, err)
	}
	_, err = h.clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "novanas",
				"novanas.io/workload":          strings.TrimPrefix(ns, NamespacePrefix),
			},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("workloads: create namespace %q: %w", ns, err)
	}
	return nil
}

func (h *realHelm) Install(ctx context.Context, req helmInstallRequest) (*release.Release, error) {
	if h.clientset == nil {
		return nil, ErrNoCluster
	}
	if err := h.ensureNamespace(ctx, req.Namespace); err != nil {
		return nil, err
	}
	cfg, err := h.actionConfig(req.Namespace)
	if err != nil {
		return nil, err
	}
	inst := action.NewInstall(cfg)
	inst.ReleaseName = req.ReleaseName
	inst.Namespace = req.Namespace
	inst.CreateNamespace = false // we already created it
	inst.Version = req.Version
	inst.Wait = false
	inst.Timeout = 5 * time.Minute
	inst.RepoURL = req.RepoURL

	chartPath, err := inst.LocateChart(req.ChartName, h.settings)
	if err != nil {
		return nil, fmt.Errorf("workloads: locate chart %q: %w", req.ChartName, err)
	}
	ch, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("workloads: load chart: %w", err)
	}
	rel, err := inst.RunWithContext(ctx, ch, req.Values)
	if err != nil {
		if strings.Contains(err.Error(), "cannot re-use a name") {
			return nil, ErrAlreadyExists
		}
		return nil, fmt.Errorf("workloads: install: %w", err)
	}
	return rel, nil
}

func (h *realHelm) Upgrade(ctx context.Context, req helmUpgradeRequest) (*release.Release, error) {
	if h.clientset == nil {
		return nil, ErrNoCluster
	}
	cfg, err := h.actionConfig(req.Namespace)
	if err != nil {
		return nil, err
	}
	up := action.NewUpgrade(cfg)
	up.Namespace = req.Namespace
	up.Version = req.Version
	up.RepoURL = req.RepoURL
	up.Wait = false
	up.Timeout = 5 * time.Minute
	up.ReuseValues = len(req.Values) == 0 && req.Version != ""

	chartPath, err := up.LocateChart(req.ChartName, h.settings)
	if err != nil {
		return nil, fmt.Errorf("workloads: locate chart: %w", err)
	}
	ch, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("workloads: load chart: %w", err)
	}
	rel, err := up.RunWithContext(ctx, req.ReleaseName, ch, req.Values)
	if err != nil {
		return nil, fmt.Errorf("workloads: upgrade: %w", err)
	}
	return rel, nil
}

func (h *realHelm) Uninstall(_ context.Context, name string) error {
	if h.clientset == nil {
		return ErrNoCluster
	}
	ns := NamespacePrefix + name
	cfg, err := h.actionConfig(ns)
	if err != nil {
		return err
	}
	un := action.NewUninstall(cfg)
	un.Wait = false
	un.Timeout = 5 * time.Minute
	if _, err := un.Run(name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ErrNotFound
		}
		return fmt.Errorf("workloads: uninstall: %w", err)
	}
	// Drop the namespace too — that's the "delete the app" semantic.
	bg := metav1.DeletePropagationBackground
	_ = h.clientset.CoreV1().Namespaces().Delete(context.Background(), ns, metav1.DeleteOptions{PropagationPolicy: &bg})
	return nil
}

func (h *realHelm) Rollback(_ context.Context, name string, revision int) (*release.Release, error) {
	if h.clientset == nil {
		return nil, ErrNoCluster
	}
	ns := NamespacePrefix + name
	cfg, err := h.actionConfig(ns)
	if err != nil {
		return nil, err
	}
	rb := action.NewRollback(cfg)
	rb.Version = revision
	rb.Wait = false
	rb.Timeout = 5 * time.Minute
	if err := rb.Run(name); err != nil {
		return nil, fmt.Errorf("workloads: rollback: %w", err)
	}
	get := action.NewGet(cfg)
	rel, err := get.Run(name)
	if err != nil {
		return nil, fmt.Errorf("workloads: get after rollback: %w", err)
	}
	return rel, nil
}

// FetchChartReadme retrieves the README and (when present)
// values.schema.json for an index entry. Best-effort — failures are
// returned to the caller but should not block the GUI from displaying
// the catalog metadata.
func FetchChartReadme(_ context.Context, settings *cli.EnvSettings, entry IndexEntry) (string, map[string]interface{}, error) {
	if settings == nil {
		settings = cli.New()
	}
	tmp, err := os.MkdirTemp("", "nova-helm-fetch-*")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(tmp)

	chartRef := entry.Chart
	g, err := getter.NewHTTPGetter()
	if err != nil {
		return "", nil, err
	}
	// Pull the index file so we can resolve the chart's tar URL.
	indexURL := strings.TrimRight(entry.RepoURL, "/") + "/index.yaml"
	idxBuf, err := g.Get(indexURL)
	if err != nil {
		return "", nil, fmt.Errorf("fetch index: %w", err)
	}
	idxFile, err := loadIndexBytes(idxBuf.Bytes())
	if err != nil {
		return "", nil, err
	}
	cv, err := idxFile.Get(chartRef, entry.Version)
	if err != nil {
		return "", nil, fmt.Errorf("chart %s@%s not in index: %w", chartRef, entry.Version, err)
	}
	if len(cv.URLs) == 0 {
		return "", nil, fmt.Errorf("chart %s@%s has no download URLs", chartRef, entry.Version)
	}
	chartURL := cv.URLs[0]
	if !strings.Contains(chartURL, "://") {
		chartURL = strings.TrimRight(entry.RepoURL, "/") + "/" + chartURL
	}
	tgz, err := g.Get(chartURL)
	if err != nil {
		return "", nil, fmt.Errorf("fetch chart: %w", err)
	}
	out := filepath.Join(tmp, chartRef+".tgz")
	if err := os.WriteFile(out, tgz.Bytes(), 0o644); err != nil {
		return "", nil, err
	}
	ch, err := loader.Load(out)
	if err != nil {
		return "", nil, fmt.Errorf("load chart: %w", err)
	}
	readme := chartReadme(ch)
	schema := chartValuesSchema(ch)
	return readme, schema, nil
}

func chartReadme(ch *chart.Chart) string {
	for _, f := range ch.Files {
		if f == nil {
			continue
		}
		name := strings.ToLower(f.Name)
		if name == "readme.md" || strings.HasSuffix(name, "/readme.md") {
			return string(f.Data)
		}
	}
	return ""
}

func chartValuesSchema(ch *chart.Chart) map[string]interface{} {
	if len(ch.Schema) == 0 {
		return nil
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal(ch.Schema, &m); err != nil {
		return nil
	}
	return m
}

func loadIndexBytes(b []byte) (*repo.IndexFile, error) {
	tmp, err := os.CreateTemp("", "nova-helm-index-*.yaml")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	_ = tmp.Close()
	return repo.LoadIndexFile(tmp.Name())
}

// PodLogs streams pod logs from the apiserver. Returns ErrNoCluster
// if the helm client was constructed in degraded mode.
func PodLogs(ctx context.Context, h helmClient, namespace, pod, container string, follow, previous, timestamps bool, tail int64, since time.Duration) (io.ReadCloser, error) {
	cs := h.kubeClient()
	if cs == nil {
		return nil, ErrNoCluster
	}
	opts := &corev1.PodLogOptions{
		Container:  container,
		Follow:     follow,
		Previous:   previous,
		Timestamps: timestamps,
	}
	if tail > 0 {
		opts.TailLines = &tail
	}
	if since > 0 {
		secs := int64(since.Seconds())
		opts.SinceSeconds = &secs
	}
	req := cs.CoreV1().Pods(namespace).GetLogs(pod, opts)
	return req.Stream(ctx)
}

// MarshalValuesYAML converts a values map to YAML. Used by handlers
// when rendering ReleaseDetail responses.
func MarshalValuesYAML(values map[string]interface{}) ([]byte, error) {
	if len(values) == 0 {
		return []byte{}, nil
	}
	return yaml.Marshal(values)
}

// ParseValuesYAML parses a YAML document into a values map.
func ParseValuesYAML(s string) (map[string]interface{}, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return map[string]interface{}{}, nil
	}
	var out map[string]interface{}
	if err := yaml.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("workloads: parse values: %w", err)
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	return out, nil
}

// CopyBytes is a tiny io.Reader helper used by tests.
func CopyBytes(b []byte) io.ReadCloser { return io.NopCloser(bytes.NewReader(b)) }
