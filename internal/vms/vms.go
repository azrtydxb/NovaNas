// Package vms wraps KubeVirt VirtualMachine / VirtualMachineInstance /
// snapshot / restore operations behind a small DTO-shaped interface so
// the HTTP layer never has to look at raw KubeVirt CRDs.
//
// Architectural choices (per docs/vms/README.md):
//
//   - Per-VM namespace `vm-<name>`. Delete cascades by deleting the
//     namespace; RBAC scopes cleanly.
//   - DataVolumes (CDI) for boot disks, sourced from the curated
//     template catalog (deploy/vms/templates.json).
//   - Console: nova-api validates auth, mints a short-lived token, and
//     returns a WebSocket URL the browser opens directly to virt-api.
//     We do NOT proxy the stream through nova-api.
//   - No live migration in v1 (single-node). The migrate path returns
//     a typed error the handler translates to 501.
//
// The KubeVirt typed client (kubevirt.io/client-go) is intentionally not
// pulled in — we keep the implementation behind a small KubeClient
// interface so a fake (in this package, see manager_test.go) drives the
// test suite without spinning up an apiserver.
package vms

import (
	"context"
	"errors"
	"time"
)

// Phase is a coarse VM lifecycle phase exposed to the API.
type Phase string

const (
	PhaseUnknown    Phase = "Unknown"
	PhasePending    Phase = "Pending"
	PhaseScheduling Phase = "Scheduling"
	PhaseScheduled  Phase = "Scheduled"
	PhaseRunning    Phase = "Running"
	PhaseSucceeded  Phase = "Succeeded"
	PhaseFailed     Phase = "Failed"
	PhasePaused     Phase = "Paused"
	PhaseStopped    Phase = "Stopped"
)

// VM is the API-shaped view of a VirtualMachine resource.
type VM struct {
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	UID         string            `json:"uid,omitempty"`
	CPU         int               `json:"cpu"`
	MemoryMB    int               `json:"memoryMB"`
	Running     bool              `json:"running"`
	Phase       Phase             `json:"phase"`
	IP          string            `json:"ip,omitempty"`
	NodeName    string            `json:"nodeName,omitempty"`
	Disks       []VMDisk          `json:"disks,omitempty"`
	Networks    []VMNetwork       `json:"networks,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"createdAt,omitempty"`
	Conditions  []VMCondition     `json:"conditions,omitempty"`
	TemplateID  string            `json:"templateID,omitempty"`
}

// VMDisk is a per-disk DTO. Source is one of:
//
//   - "template:<id>" — boot disk imported from the curated template
//   - "blank"         — empty zvol-backed PVC for data
//   - "url:<https://…>" — operator-supplied HTTP image
type VMDisk struct {
	Name      string `json:"name"`
	SizeGB    int    `json:"sizeGB"`
	Source    string `json:"source"`
	Boot      bool   `json:"boot,omitempty"`
	Bus       string `json:"bus,omitempty"` // virtio (default), sata, scsi
	StorageClass string `json:"storageClass,omitempty"`
}

// VMNetwork picks an attachment type. Type is "pod" (default) or
// "multus:<network-attachment>".
type VMNetwork struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// VMCondition mirrors a kubevirt VM condition.
type VMCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// VMCloudInit is the user-data shape exposed by the API. The manager
// expands this into a full cloud-init NoCloud user-data document.
type VMCloudInit struct {
	User     string   `json:"user,omitempty"`
	Password string   `json:"password,omitempty"`
	SSHKeys  []string `json:"sshKeys,omitempty"`
	Hostname string   `json:"hostname,omitempty"`
	UserData string   `json:"userData,omitempty"` // raw override; if set, takes precedence
}

// CreateRequest is what POST /api/v1/vms accepts.
type CreateRequest struct {
	Namespace  string      `json:"namespace,omitempty"` // optional; default vm-<name>
	Name       string      `json:"name"`
	TemplateID string      `json:"templateID,omitempty"`
	CPU        int         `json:"cpu"`
	MemoryMB   int         `json:"memoryMB"`
	Disks      []VMDisk    `json:"disks,omitempty"`
	Networks   []VMNetwork `json:"networks,omitempty"`
	CloudInit  VMCloudInit `json:"cloudInit,omitempty"`
	StartOnCreate bool     `json:"startOnCreate,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

// PatchRequest is the writable subset for PATCH.
type PatchRequest struct {
	CPU      *int     `json:"cpu,omitempty"`
	MemoryMB *int     `json:"memoryMB,omitempty"`
	Disks    []VMDisk `json:"disks,omitempty"` // when present, replaces disks
	Labels   map[string]string `json:"labels,omitempty"`
}

// Snapshot is the API view of a VirtualMachineSnapshot.
type Snapshot struct {
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	VMName    string    `json:"vmName"`
	Phase     string    `json:"phase,omitempty"`
	ReadyToUse bool     `json:"readyToUse,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

// CreateSnapshotRequest is the body for POST /api/v1/vm-snapshots.
type CreateSnapshotRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	VMName    string `json:"vmName"`
}

// Restore is the API view of a VirtualMachineRestore.
type Restore struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	VMName     string `json:"vmName"`
	SnapshotName string `json:"snapshotName"`
	Complete   bool   `json:"complete"`
}

// CreateRestoreRequest is the body for POST /api/v1/vm-restores.
type CreateRestoreRequest struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	VMName       string `json:"vmName"`
	SnapshotName string `json:"snapshotName"`
}

// ConsoleSession is what GET /…/console returns. The browser opens a
// direct WebSocket to virt-api using these fields. ExpiresAt is the
// hard deadline after which virt-api will reject the token; the GUI
// should request a fresh session before that.
type ConsoleSession struct {
	WSURL     string    `json:"wsUrl"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	Kind      string    `json:"kind"` // "vnc" | "spice" | "serial"
}

// Page wraps a paginated list response.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// ListOptions is the input to Manager.List.
type ListOptions struct {
	NamespacePrefix string // default "vm-"
	Cursor          string
	PageSize        int // default 50
}

// Errors returned by Manager.
var (
	ErrNotFound       = errors.New("vms: not found")
	ErrAlreadyExists  = errors.New("vms: already exists")
	ErrInvalidRequest = errors.New("vms: invalid request")
	ErrNotImplemented = errors.New("vms: not implemented")
	ErrConflict       = errors.New("vms: conflict")
)

// KubeClient abstracts the small set of KubeVirt operations the manager
// needs. Real implementations wrap a dynamic client; tests use a fake
// in-memory implementation (manager_test.go).
type KubeClient interface {
	ListNamespaces(ctx context.Context, prefix string) ([]string, error)
	CreateNamespace(ctx context.Context, name string, labels map[string]string) error
	DeleteNamespace(ctx context.Context, name string) error

	ListVMs(ctx context.Context, namespace string) ([]VM, error)
	GetVM(ctx context.Context, namespace, name string) (*VM, error)
	CreateVM(ctx context.Context, vm *VM, cloudInit VMCloudInit, templateID string) (*VM, error)
	PatchVM(ctx context.Context, namespace, name string, p PatchRequest) (*VM, error)
	DeleteVM(ctx context.Context, namespace, name string) error
	SetVMRunning(ctx context.Context, namespace, name string, running bool) error
	RestartVM(ctx context.Context, namespace, name string) error
	PauseVM(ctx context.Context, namespace, name string) error
	UnpauseVM(ctx context.Context, namespace, name string) error
	MigrateVM(ctx context.Context, namespace, name string) error

	CountReadyNodes(ctx context.Context) (int, error)

	ListSnapshots(ctx context.Context, namespace string) ([]Snapshot, error)
	CreateSnapshot(ctx context.Context, s Snapshot) (*Snapshot, error)
	DeleteSnapshot(ctx context.Context, namespace, name string) error

	ListRestores(ctx context.Context, namespace string) ([]Restore, error)
	CreateRestore(ctx context.Context, r Restore) (*Restore, error)
	DeleteRestore(ctx context.Context, namespace, name string) error

	// MintConsoleToken issues a short-lived token for a virt-api
	// /vnc, /spice, or /serial subresource WebSocket. Real
	// implementations call kubevirt's TokenRequest API or a
	// service-account TokenRequest with appropriate scopes.
	MintConsoleToken(ctx context.Context, namespace, name, kind string, ttl time.Duration) (token string, expiresAt time.Time, err error)
}
