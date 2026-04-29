// Package replication implements NovaNAS's general replication subsystem.
//
// Three backends are supported:
//
//   - ZFS native (zfs send | zfs receive) for peer-to-peer NovaNAS hosts.
//     Highest fidelity; preserves snapshots, properties and ACLs. Default
//     for NovaNAS-to-NovaNAS.
//   - S3 push/pull for backups to any S3-compatible object store.
//   - rsync-over-SSH for legacy non-NovaNAS targets.
//
// A replication is described by a [Job]: it has a direction (push/pull),
// a source, a destination, a cron schedule (empty = manual-only), and a
// retention policy. The [Manager] owns persisting jobs, scheduling them
// (via the existing scheduler package) and dispatching individual runs
// onto Asynq.
//
// The on-disk job/run schema is intentionally not declared here. The
// Manager is parameterised by a [Store] interface so this package can be
// unit-tested without touching the database, and so the database
// migration / sqlc generation lives with the rest of the schema in
// internal/store/migrations and internal/store/queries (see the
// docs/replication/README.md for the migration layout this package
// expects).
package replication

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
)

// BackendKind identifies which replication backend a job uses.
type BackendKind string

const (
	BackendZFS   BackendKind = "zfs"
	BackendS3    BackendKind = "s3"
	BackendRsync BackendKind = "rsync"
)

// Direction is push (source-local, destination-remote) or pull (the
// reverse). For S3 push means upload, pull means download.
type Direction string

const (
	DirectionPush Direction = "push"
	DirectionPull Direction = "pull"
)

// RunOutcome is the terminal state of a single replication run.
type RunOutcome string

const (
	RunPending   RunOutcome = "pending"
	RunRunning   RunOutcome = "running"
	RunSucceeded RunOutcome = "succeeded"
	RunFailed    RunOutcome = "failed"
	RunCancelled RunOutcome = "cancelled"
)

// Source describes where data is read from for a replication run.
//
// Only the fields relevant to the Backend should be populated; unused
// fields are ignored. For example, a ZFS source uses Dataset only; an
// S3-pull source uses Bucket+Prefix.
type Source struct {
	// Dataset is the local ZFS dataset (e.g. "tank/data") for the ZFS
	// backend, or the local mount path for the rsync backend.
	Dataset string `json:"dataset,omitempty"`
	// Path is a host filesystem path (rsync push, S3 push from a
	// non-ZFS source).
	Path string `json:"path,omitempty"`
	// Host / SSHUser are used by ZFS pull and rsync.
	Host    string `json:"host,omitempty"`
	SSHUser string `json:"sshUser,omitempty"`
	// Bucket / Prefix are used by S3 pull.
	Bucket string `json:"bucket,omitempty"`
	Prefix string `json:"prefix,omitempty"`
	// Endpoint is the optional S3 endpoint URL (RustFS, MinIO, etc.).
	Endpoint string `json:"endpoint,omitempty"`
	// Region is the S3 region.
	Region string `json:"region,omitempty"`
}

// Destination mirrors Source on the receiving side.
type Destination struct {
	Dataset  string `json:"dataset,omitempty"`
	Path     string `json:"path,omitempty"`
	Host     string `json:"host,omitempty"`
	SSHUser  string `json:"sshUser,omitempty"`
	Bucket   string `json:"bucket,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Region   string `json:"region,omitempty"`
}

// RetentionPolicy controls how many runs / remote backup objects are
// kept after a successful run. Zero values are interpreted as "no
// limit" for that bucket.
type RetentionPolicy struct {
	// KeepLastN is the simplest policy: keep the N most recent
	// successful runs/backups; older ones are eligible for deletion.
	KeepLastN int `json:"keepLastN,omitempty"`
	// KeepDaily / KeepWeekly / KeepMonthly are sanoid-style retention
	// buckets applied to the timestamps of successful runs.
	KeepDaily   int `json:"keepDaily,omitempty"`
	KeepWeekly  int `json:"keepWeekly,omitempty"`
	KeepMonthly int `json:"keepMonthly,omitempty"`
	KeepYearly  int `json:"keepYearly,omitempty"`
}

// IsZero reports whether no retention is configured. When true,
// retention application is a no-op.
func (r RetentionPolicy) IsZero() bool {
	return r.KeepLastN == 0 && r.KeepDaily == 0 && r.KeepWeekly == 0 &&
		r.KeepMonthly == 0 && r.KeepYearly == 0
}

// Job is the persistent description of a replication.
type Job struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Backend     BackendKind     `json:"backend"`
	Direction   Direction       `json:"direction"`
	Source      Source          `json:"source"`
	Destination Destination     `json:"destination"`
	// Schedule is a standard cron expression. Empty = manual only.
	Schedule  string          `json:"schedule"`
	Retention RetentionPolicy `json:"retention"`
	Enabled   bool            `json:"enabled"`
	// SecretRef is the OpenBao key prefix where backend-specific
	// credentials are stored (e.g. "nova/replication/<job-id>"). The
	// concrete schema under this prefix is backend-specific:
	//
	//   ZFS    : ssh_key (PEM)
	//   S3     : access_key, secret_key
	//   rsync  : ssh_key (PEM)
	//
	// Credentials must NEVER be in the DB.
	SecretRef string `json:"secretRef,omitempty"`

	// LastSnapshot records the last snapshot replicated for incremental
	// ZFS sends. Empty means the next run is a full send.
	LastSnapshot string `json:"lastSnapshot,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Run is the record of a single replication execution.
type Run struct {
	ID               uuid.UUID  `json:"id"`
	JobID            uuid.UUID  `json:"jobId"`
	StartedAt        time.Time  `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
	Outcome          RunOutcome `json:"outcome"`
	BytesTransferred int64      `json:"bytesTransferred"`
	// Snapshot is the full snapshot name that was replicated (ZFS
	// only). For S3/rsync this is empty.
	Snapshot string `json:"snapshot,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Validate returns the first user-visible error in j, or nil if j is
// well-formed enough to enqueue. Backend-specific deeper validation
// happens inside the corresponding Backend.Validate hook.
func (j *Job) Validate() error {
	if j == nil {
		return errors.New("replication: nil job")
	}
	if j.Name == "" {
		return errors.New("replication: name is required")
	}
	switch j.Backend {
	case BackendZFS, BackendS3, BackendRsync:
	default:
		return errors.New("replication: backend must be zfs|s3|rsync")
	}
	switch j.Direction {
	case DirectionPush, DirectionPull:
	default:
		return errors.New("replication: direction must be push|pull")
	}
	return nil
}

// RunResult is what a Backend returns from Execute. It is the source of
// truth for the bytes/snapshot fields recorded on the matching Run row.
type RunResult struct {
	BytesTransferred int64
	Snapshot         string
}

// ExecuteContext bundles the per-run inputs a Backend.Execute call gets.
// Stdout/Stderr are optional sinks for backend-specific progress output;
// nil discards.
type ExecuteContext struct {
	Job    Job
	Stdout io.Writer
	Stderr io.Writer
}

// Backend is the per-protocol replication implementation. Each backend
// owns its on-the-wire format, credential lookup, and incremental-state
// bookkeeping (returned via RunResult.Snapshot for ZFS).
type Backend interface {
	// Kind returns the BackendKind this backend implements.
	Kind() BackendKind

	// Validate performs deeper, backend-specific validation of a Job
	// at create/update time. Cheap; never touches the network.
	Validate(ctx context.Context, j Job) error

	// Execute runs one full replication pass synchronously. Long
	// operations must respect ctx cancellation. Returning a non-nil
	// error marks the run as RunFailed; a nil return is RunSucceeded.
	Execute(ctx context.Context, in ExecuteContext) (RunResult, error)
}
