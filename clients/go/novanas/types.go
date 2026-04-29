package novanas

import "time"

// Pool mirrors the Pool schema in api/openapi.yaml.
type Pool struct {
	Name             string `json:"name"`
	SizeBytes        int64  `json:"sizeBytes"`
	Allocated        int64  `json:"allocated"`
	Free             int64  `json:"free"`
	Health           string `json:"health"`
	ReadOnly         bool   `json:"readOnly"`
	FragmentationPct int    `json:"fragmentationPct"`
	CapacityPct      int    `json:"capacityPct"`
	DedupRatio       string `json:"dedupRatio"`
}

// Vdev mirrors the Vdev schema (recursive).
type Vdev struct {
	Type           string `json:"type"`
	Path           string `json:"path"`
	State          string `json:"state"`
	ReadErrors     int    `json:"readErrors"`
	WriteErrors    int    `json:"writeErrors"`
	ChecksumErrors int    `json:"checksumErrors"`
	Children       []Vdev `json:"children,omitempty"`
}

// PoolStatus is the nested "status" field of PoolDetail.
type PoolStatus struct {
	State string `json:"state"`
	Vdevs []Vdev `json:"vdevs,omitempty"`
}

// PoolDetail mirrors the PoolDetail schema.
type PoolDetail struct {
	Pool       Pool              `json:"pool"`
	Properties map[string]string `json:"properties,omitempty"`
	Status     PoolStatus        `json:"status"`
}

// Dataset mirrors the Dataset schema.
type Dataset struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	UsedBytes       int64  `json:"usedBytes"`
	AvailableBytes  int64  `json:"availableBytes"`
	ReferencedBytes int64  `json:"referencedBytes"`
	Mountpoint      string `json:"mountpoint"`
	Compression     string `json:"compression"`
	RecordSizeBytes int64  `json:"recordSizeBytes"`
}

// DatasetType constants for Dataset.Type / CreateDatasetSpec.Type.
const (
	DatasetTypeFilesystem = "filesystem"
	DatasetTypeVolume     = "volume"
)

// DatasetDetail mirrors the DatasetDetail schema.
type DatasetDetail struct {
	Dataset    Dataset           `json:"dataset"`
	Properties map[string]string `json:"properties,omitempty"`
}

// CreateDatasetSpec is the request body for POST /datasets.
type CreateDatasetSpec struct {
	Parent          string            `json:"parent"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	VolumeSizeBytes int64             `json:"volumeSizeBytes,omitempty"`
	Properties      map[string]string `json:"properties,omitempty"`
}

// Snapshot mirrors the Snapshot schema.
type Snapshot struct {
	Name            string `json:"name"`
	Dataset         string `json:"dataset"`
	ShortName       string `json:"shortName"`
	UsedBytes       int64  `json:"usedBytes"`
	ReferencedBytes int64  `json:"referencedBytes"`
	CreationUnix    int64  `json:"creationUnix"`
}

// Job state constants. These are the string values the API uses for
// Job.state — keep in sync with internal/jobs.
const (
	JobStateQueued      = "queued"
	JobStateRunning     = "running"
	JobStateSucceeded   = "succeeded"
	JobStateFailed      = "failed"
	JobStateCancelled   = "cancelled"
	JobStateInterrupted = "interrupted"
)

// Job mirrors the Job schema. ExitCode and Error are pointers because the
// schema marks them nullable; nil means "not set yet" rather than zero.
type Job struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Target     string     `json:"target"`
	State      string     `json:"state"`
	Command    string     `json:"command,omitempty"`
	Stdout     string     `json:"stdout,omitempty"`
	Stderr     string     `json:"stderr,omitempty"`
	ExitCode   *int       `json:"exitCode,omitempty"`
	Error      *string    `json:"error,omitempty"`
	CreatedAt  *time.Time `json:"createdAt,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// IsTerminal reports whether the job has reached a terminal state and will
// not transition further.
func (j *Job) IsTerminal() bool {
	switch j.State {
	case JobStateSucceeded, JobStateFailed, JobStateCancelled, JobStateInterrupted:
		return true
	default:
		return false
	}
}

// Succeeded reports whether the job ended in the succeeded state.
func (j *Job) Succeeded() bool { return j.State == JobStateSucceeded }

// ---- ProtocolShare ---------------------------------------------------------

// Protocol identifies a sharing protocol exposed by a ProtocolShare.
type Protocol string

// Protocol constants. Keep in sync with internal/host/protocolshare.Protocol.
const (
	ProtocolNFS Protocol = "nfs"
	ProtocolSMB Protocol = "smb"
)

// DatasetACE mirrors the DatasetACE schema. It is the wire shape used by
// ProtocolShare.Acls.
type DatasetACE struct {
	Inheritance []string `json:"inheritance,omitempty"`
	Permissions []string `json:"permissions"`
	Principal   string   `json:"principal"`
	Type        string   `json:"type"` // "allow" | "deny"
}

// NfsClientRule mirrors the NfsClientRule schema.
type NfsClientRule struct {
	// Spec is a CIDR, IP, "*", or hostname/wildcard pattern.
	Spec string `json:"spec"`
	// Options is a comma-separated NFS export options string
	// (e.g. "rw,sync,sec=krb5p").
	Options string `json:"options"`
}

// ProtocolNFSOpts mirrors the ProtocolNFSOpts schema.
type ProtocolNFSOpts struct {
	Clients []NfsClientRule `json:"clients"`
}

// ProtocolSMBOpts mirrors the ProtocolSMBOpts schema.
type ProtocolSMBOpts struct {
	Comment    *string  `json:"comment,omitempty"`
	Browseable *bool    `json:"browseable,omitempty"`
	GuestOK    *bool    `json:"guestOk,omitempty"`
	ValidUsers []string `json:"validUsers,omitempty"`
	WriteList  []string `json:"writeList,omitempty"`
}

// ProtocolShare mirrors the ProtocolShare schema.
type ProtocolShare struct {
	Name        string           `json:"name"`
	Pool        string           `json:"pool"`
	DatasetName string           `json:"datasetName"`
	Protocols   []Protocol       `json:"protocols"`
	Acls        []DatasetACE     `json:"acls"`
	QuotaBytes  *int64           `json:"quotaBytes,omitempty"`
	NFS         *ProtocolNFSOpts `json:"nfs,omitempty"`
	SMB         *ProtocolSMBOpts `json:"smb,omitempty"`
}

// ProtocolStatus mirrors the ProtocolStatus schema.
type ProtocolStatus struct {
	Protocol Protocol `json:"protocol"`
	Active   bool     `json:"active"`
	Detail   string   `json:"detail,omitempty"`
}

// ProtocolShareDetail mirrors the ProtocolShareDetail schema returned by GET.
type ProtocolShareDetail struct {
	Share           ProtocolShare    `json:"share"`
	Path            string           `json:"path"`
	Acl             []DatasetACE     `json:"acl"`
	ProtocolsStatus []ProtocolStatus `json:"protocolsStatus"`
}
