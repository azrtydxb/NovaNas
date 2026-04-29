// Package csi implements the Container Storage Interface (CSI) gRPC services
// for NovaNAS. It targets a single-node k3s deployment where the storage host
// and the kubelet share the same machine, so volumes are local (ZFS datasets
// and zvols) and the Node service uses bind mounts.
package csi

import (
	"context"
	"time"
)

// NovaNASClient is the subset of the NovaNAS HTTP SDK that the CSI driver
// depends on. The real client lives in clients/go/novanas; this interface
// allows the CSI package to build today and to be tested with stubs.
type NovaNASClient interface {
	GetDataset(ctx context.Context, fullname string) (*Dataset, error)
	CreateDataset(ctx context.Context, spec CreateDatasetSpec) (*Job, error)
	DestroyDataset(ctx context.Context, fullname string, recursive bool) (*Job, error)
	SetDatasetProps(ctx context.Context, fullname string, props map[string]string) (*Job, error)
	CreateSnapshot(ctx context.Context, dataset, shortName string, recursive bool) (*Job, error)
	DestroySnapshot(ctx context.Context, fullname string) (*Job, error)
	CloneSnapshot(ctx context.Context, snapshot, target string, properties map[string]string) (*Job, error)
	CreateProtocolShare(ctx context.Context, share ProtocolShareSpec) (*Job, error)
	GetProtocolShare(ctx context.Context, name, pool, dataset string) (*ProtocolShareDetail, error)
	DeleteProtocolShare(ctx context.Context, name, pool, dataset string) (*Job, error)
	WaitJob(ctx context.Context, id string, pollInterval time.Duration) (*Job, error)
	IsNotFound(err error) bool
}

// NFSClientRule mirrors the SDK's NfsClientRule. Duplicated here so the CSI
// package's NovaNASClient interface stays SDK-free.
type NFSClientRule struct {
	Spec    string
	Options string
}

// ProtocolShareSpec is the CSI-side request shape used to create a
// ProtocolShare via the NovaNAS API. Only the NFS-bearing subset is wired in
// today; SMB is a server-side concern when a future kind=smb is added.
type ProtocolShareSpec struct {
	Name        string
	Pool        string
	DatasetName string
	Protocols   []string // typically ["nfs"]
	QuotaBytes  int64
	NFSClients  []NFSClientRule
}

// ProtocolShareDetail is the read-side projection of the API's
// ProtocolShareDetail used by the CSI driver. Only the fields the driver
// actually inspects are projected here.
type ProtocolShareDetail struct {
	Name        string
	Pool        string
	DatasetName string
	Path        string
}

// Dataset is a minimal projection of a ZFS dataset/zvol.
type Dataset struct {
	Name           string
	Type           string // "filesystem" | "volume"
	UsedBytes      int64
	AvailableBytes int64
	Mountpoint     string
	// Volsize is the configured size for zvols (type="volume"). For
	// filesystem datasets, capacity is governed by quota/refquota.
	Volsize int64
	// Quota is the configured quota for filesystem datasets.
	Quota int64
}

// Job is a NovaNAS asynchronous job handle.
type Job struct {
	ID    string
	State string // queued|running|done|failed
	Error *string
}

// Done reports whether the job is terminal.
func (j *Job) Done() bool { return j.State == "done" || j.State == "failed" }

// CreateDatasetSpec describes a dataset to create.
type CreateDatasetSpec struct {
	Parent          string // e.g. "tank/csi"
	Name            string // leaf name, e.g. "pvc-<uuid>"
	Type            string // "filesystem" | "volume"
	VolumeSizeBytes int64  // required for type=volume
	Properties      map[string]string
}

// FullName returns the dataset's full ZFS path "<parent>/<name>".
func (s CreateDatasetSpec) FullName() string {
	if s.Parent == "" {
		return s.Name
	}
	return s.Parent + "/" + s.Name
}
