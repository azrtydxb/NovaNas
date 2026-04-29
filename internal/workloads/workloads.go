// Package workloads is the NovaNAS Apps subsystem: a Helm-driven lifecycle
// manager that installs/upgrades/uninstalls operator-curated applications
// (Plex, Jellyfin, Nextcloud, …) on the embedded k3s cluster.
//
// The public types in this file are the value objects exchanged with HTTP
// handlers and the SDK. The Helm and Kubernetes wiring lives in helm.go,
// the curated chart catalog in index.go, and the lifecycle orchestration
// (which composes them) in manager.go.
package workloads

import (
	"context"
	"errors"
	"io"
	"time"
)

// Common sentinel errors. Handlers map these to specific HTTP status codes.
var (
	// ErrNotFound is returned when a release/index entry does not exist.
	ErrNotFound = errors.New("workloads: not found")
	// ErrAlreadyExists is returned when a release name is already in use.
	ErrAlreadyExists = errors.New("workloads: release already exists")
	// ErrInvalidArgument is returned for malformed values, names, etc.
	ErrInvalidArgument = errors.New("workloads: invalid argument")
	// ErrNoCluster is returned when the manager is constructed without a
	// reachable k3s kubeconfig (common in dev VMs without k3s).
	ErrNoCluster = errors.New("workloads: kubernetes cluster not available")
)

// NamespacePrefix is prepended to every release's namespace. Deleting an
// app == deleting the namespace == reclaiming the chart's resources.
const NamespacePrefix = "nova-app-"

// IndexEntry is one curated chart in the operator-facing index. The fields
// mirror deploy/workloads/index.json.
type IndexEntry struct {
	Name             string                 `json:"name"`
	DisplayName      string                 `json:"displayName,omitempty"`
	Category         string                 `json:"category,omitempty"`
	Description      string                 `json:"description,omitempty"`
	Chart            string                 `json:"chart"`
	Version          string                 `json:"version"`
	RepoURL          string                 `json:"repoURL"`
	AppVersion       string                 `json:"appVersion,omitempty"`
	Icon             string                 `json:"icon,omitempty"`
	Homepage         string                 `json:"homepage,omitempty"`
	ReadmeURL        string                 `json:"readmeURL,omitempty"`
	DefaultNamespace string                 `json:"defaultNamespace,omitempty"`
	Permissions      []string               `json:"permissions,omitempty"`
	DefaultValues    map[string]interface{} `json:"defaultValues,omitempty"`
}

// IndexFile is the on-disk schema. Only `entries` is load-bearing.
type IndexFile struct {
	Version     int          `json:"version"`
	UpdatedAt   string       `json:"updatedAt,omitempty"`
	Description string       `json:"description,omitempty"`
	Entries     []IndexEntry `json:"entries"`
}

// IndexEntryDetail is what GET /workloads/index/{name} returns. README
// and ValuesSchema are best-effort: when the chart cannot be fetched
// (offline operator, repo down) the catalog metadata is still returned
// with empty README/Schema so the GUI degrades gracefully.
type IndexEntryDetail struct {
	IndexEntry
	Readme       string                 `json:"readme,omitempty"`
	ValuesSchema map[string]interface{} `json:"valuesSchema,omitempty"`
}

// Release is the runtime state of an installed app. It mirrors the subset
// of fields the GUI needs from a Helm release, plus NovaNAS metadata
// (origin index entry, install-time, who installed it).
type Release struct {
	Name        string    `json:"name"`
	Namespace   string    `json:"namespace"`
	IndexName   string    `json:"indexName,omitempty"`
	Chart       string    `json:"chart"`
	Version     string    `json:"version"`
	AppVersion  string    `json:"appVersion,omitempty"`
	Status      string    `json:"status"`
	Revision    int       `json:"revision"`
	Updated     time.Time `json:"updated"`
	InstalledBy string    `json:"installedBy,omitempty"`
	Notes       string    `json:"notes,omitempty"`
}

// ReleaseDetail extends Release with values + k8s resource summary.
type ReleaseDetail struct {
	Release
	Values    map[string]interface{} `json:"values,omitempty"`
	Resources []ResourceRef          `json:"resources,omitempty"`
	Pods      []PodInfo              `json:"pods,omitempty"`
}

// ResourceRef is a thin description of a k8s object created by the chart.
type ResourceRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
}

// PodInfo is a minimal pod summary for the Package Center GUI.
type PodInfo struct {
	Name       string   `json:"name"`
	Phase      string   `json:"phase"`
	Ready      bool     `json:"ready"`
	Restarts   int32    `json:"restarts"`
	Containers []string `json:"containers,omitempty"`
	NodeName   string   `json:"nodeName,omitempty"`
}

// Event is one Kubernetes event projected into the workloads namespace.
type Event struct {
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Object    string    `json:"object"`
	Count     int32     `json:"count"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
}

// InstallRequest is the payload to POST /workloads.
type InstallRequest struct {
	IndexName   string `json:"indexName"`
	ReleaseName string `json:"releaseName"`
	ValuesYAML  string `json:"valuesYAML,omitempty"`
	Namespace   string `json:"namespace,omitempty"` // optional override; default nova-app-<release>
	InstalledBy string `json:"-"`                   // populated from auth identity
}

// UpgradeRequest is the payload to PATCH /workloads/{releaseName}.
type UpgradeRequest struct {
	Version    string `json:"version,omitempty"`
	ValuesYAML string `json:"valuesYAML,omitempty"`
}

// LogRequest controls GET /workloads/{releaseName}/logs.
type LogRequest struct {
	Pod        string
	Container  string
	Follow     bool
	TailLines  int64
	Since      time.Duration
	Timestamps bool
	Previous   bool
}

// IndexProvider abstracts the curated catalog. Used so handlers and tests
// can substitute a static fixture without touching disk.
type IndexProvider interface {
	List(ctx context.Context) ([]IndexEntry, error)
	Get(ctx context.Context, name string) (*IndexEntryDetail, error)
	Reload(ctx context.Context) error
}

// Lifecycle is the contract the HTTP handlers depend on. The concrete
// implementation is *Manager (manager.go); tests inject fakes.
type Lifecycle interface {
	IndexList(ctx context.Context) ([]IndexEntry, error)
	IndexGet(ctx context.Context, name string) (*IndexEntryDetail, error)
	IndexReload(ctx context.Context) error

	List(ctx context.Context) ([]Release, error)
	Get(ctx context.Context, releaseName string) (*ReleaseDetail, error)
	Install(ctx context.Context, req InstallRequest) (*Release, error)
	Upgrade(ctx context.Context, releaseName string, req UpgradeRequest) (*Release, error)
	Uninstall(ctx context.Context, releaseName string) error
	Rollback(ctx context.Context, releaseName string, revision int) (*Release, error)

	Events(ctx context.Context, releaseName string) ([]Event, error)
	Logs(ctx context.Context, releaseName string, req LogRequest) (io.ReadCloser, error)
}

// validateReleaseName enforces the DNS-1123 label rules Helm itself
// applies, plus a sane upper bound. Returned errors wrap
// ErrInvalidArgument so handlers map them to 400.
func validateReleaseName(name string) error {
	if name == "" {
		return errInvalid("release name is required")
	}
	if len(name) > 53 {
		return errInvalid("release name must be at most 53 characters")
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' && i > 0 && i < len(name)-1:
		default:
			return errInvalid("release name must match [a-z0-9](-[a-z0-9])*")
		}
	}
	return nil
}

func errInvalid(msg string) error { return wrap(ErrInvalidArgument, msg) }

type wrapErr struct {
	parent error
	msg    string
}

func (w *wrapErr) Error() string { return w.parent.Error() + ": " + w.msg }
func (w *wrapErr) Unwrap() error { return w.parent }

func wrap(parent error, msg string) error { return &wrapErr{parent: parent, msg: msg} }
