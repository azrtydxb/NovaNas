package novanas

// Resource type definitions for the kinds the storage data plane
// reads. Each carries only the fields the controller actually needs;
// extending the api server's schema doesn't break consumers because
// json unmarshal ignores unknown fields.

type ObjectMeta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type Pool struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       PoolSpec   `json:"spec"`
	Status     PoolStatus `json:"status,omitempty"`
}

// LabelSelector mirrors metav1.LabelSelector. Kept local to avoid
// dragging the k8s.io/apimachinery dep into every consumer.
type LabelSelector struct {
	MatchLabels      map[string]string          `json:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

type LabelSelectorRequirement struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

// DeviceFilter mirrors the API wire shape (preferredClass / minSize /
// maxSize). The legacy storage CRD's `Type` / `MinSize` fields are not
// supported by the API server — they were dropped when the resource
// moved to Postgres.
type DeviceFilter struct {
	PreferredClass string `json:"preferredClass,omitempty"` // nvme | ssd | hdd
	MinSize        string `json:"minSize,omitempty"`
	MaxSize        string `json:"maxSize,omitempty"`
}

// FileBackendSpec is the Pool-level file-backend config (loop-mounted
// file). Wire shape mirrors PoolFileBackendSchema in
// packages/schemas/src/storage/storage-pool.ts.
type FileBackendSpec struct {
	Path      string `json:"path,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
}

type PoolSpec struct {
	NodeSelector *LabelSelector   `json:"nodeSelector,omitempty"`
	BackendType  string           `json:"backendType,omitempty"`
	DeviceFilter *DeviceFilter    `json:"deviceFilter,omitempty"`
	FileBackend  *FileBackendSpec `json:"fileBackend,omitempty"`
	Tier         string           `json:"tier,omitempty"`
}

type PoolStatus struct {
	Phase         string      `json:"phase,omitempty"`
	NodeCount     int         `json:"nodeCount,omitempty"`
	TotalCapacity string      `json:"totalCapacity,omitempty"`
	Conditions    []Condition `json:"conditions,omitempty"`
}

type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

// BackendAssignment binds a StoragePool to a node and tracks the
// backing SPDK bdev provisioned by the agent. The wire shape mirrors
// packages/schemas/src/storage/backend-assignment.ts.
type BackendAssignment struct {
	APIVersion string                  `json:"apiVersion"`
	Kind       string                  `json:"kind"`
	Metadata   ObjectMeta              `json:"metadata"`
	Spec       BackendAssignmentSpec   `json:"spec"`
	Status     BackendAssignmentStatus `json:"status,omitempty"`
}

type BackendAssignmentSpec struct {
	PoolRef      string                  `json:"poolRef"`
	NodeName     string                  `json:"nodeName"`
	BackendType  string                  `json:"backendType"`
	DeviceFilter *APIDeviceFilter        `json:"deviceFilter,omitempty"`
	FileBackend  *APIFileBackendSpec     `json:"fileBackend,omitempty"`
}

// APIDeviceFilter mirrors the API wire shape (preferredClass/minSize/
// maxSize). Distinct from the legacy DeviceFilter type that uses
// Type/MinSize on the storage CRD side.
type APIDeviceFilter struct {
	PreferredClass string `json:"preferredClass,omitempty"` // nvme | ssd | hdd
	MinSize        string `json:"minSize,omitempty"`
	MaxSize        string `json:"maxSize,omitempty"`
}

type APIFileBackendSpec struct {
	Path      string `json:"path,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
}

type BackendAssignmentStatus struct {
	Phase      string      `json:"phase,omitempty"`
	Device     string      `json:"device,omitempty"`
	PCIeAddr   string      `json:"pcieAddr,omitempty"`
	BdevName   string      `json:"bdevName,omitempty"`
	Capacity   int64       `json:"capacity,omitempty"`
	Message    string      `json:"message,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
}

type Disk struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       DiskSpec   `json:"spec"`
	Status     DiskStatus `json:"status,omitempty"`
}

type DiskSpec struct {
	NodeName string `json:"nodeName,omitempty"`
	Path     string `json:"path,omitempty"`
}

type DiskStatus struct {
	Phase string `json:"phase,omitempty"`
	Size  string `json:"size,omitempty"`
}
