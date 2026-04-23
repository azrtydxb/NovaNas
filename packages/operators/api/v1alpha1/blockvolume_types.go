package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BlockVolumeSpec defines the desired state of BlockVolume.
type BlockVolumeSpec struct {
	// +kubebuilder:validation:MinLength=1
	Pool string `json:"pool"`
	// Size is a bytes quantity (e.g. "100Gi").
	// +kubebuilder:validation:MinLength=1
	Size       string              `json:"size"`
	Protection *ProtectionPolicy   `json:"protection,omitempty"`
	Encryption *EncryptionSettings `json:"encryption,omitempty"`
	Tiering    *TieringPolicy      `json:"tiering,omitempty"`
	// AllowDataLoss unblocks deletion even when the volume is bound to
	// a Share/iSCSI/NVMe-oF target.
	AllowDataLoss bool `json:"allowDataLoss,omitempty"`
}

// ProtectionPolicy encodes replication / erasure coding choice.
// Mirror of the discriminated-union Zod schema.
type ProtectionPolicy struct {
	// +kubebuilder:validation:Enum=replication;erasureCoding
	Mode          string                `json:"mode"`
	Replication   *ReplicationProtection `json:"replication,omitempty"`
	ErasureCoding *ErasureCodingSpec    `json:"erasureCoding,omitempty"`
	// Replicas is retained for back-compat with mode=replication.
	Replicas int32 `json:"replicas,omitempty"`
}

// ReplicationProtection holds N-copy params.
type ReplicationProtection struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=8
	Copies int32 `json:"copies"`
}

// ErasureCodingSpec configures EC parameters.
type ErasureCodingSpec struct {
	// +kubebuilder:validation:Minimum=2
	DataShards int32 `json:"dataShards"`
	// +kubebuilder:validation:Minimum=1
	ParityShards int32 `json:"parityShards"`
}

// EncryptionSettings toggles dataset-at-rest encryption.
type EncryptionSettings struct {
	Enabled bool   `json:"enabled,omitempty"`
	KmsKey  string `json:"kmsKey,omitempty"`
}

// TieringPolicy describes cross-pool data movement.
type TieringPolicy struct {
	// +kubebuilder:validation:MinLength=1
	Primary              string `json:"primary,omitempty"`
	DemoteTo             string `json:"demoteTo,omitempty"`
	DemoteAfter          string `json:"demoteAfter,omitempty"`
	PromoteOn            string `json:"promoteOn,omitempty"`
	PromoteAfterAccesses int32  `json:"promoteAfterAccesses,omitempty"`
}

// BlockVolumeStatus defines observed state.
type BlockVolumeStatus struct {
	// +kubebuilder:validation:Enum=Pending;Bound;Available;Failed;Ready;Encrypted
	Phase              string             `json:"phase,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	UsedBytes          int64              `json:"usedBytes,omitempty"`
	// ActualSizeBytes is the raw-bytes footprint on disk after
	// protection/compression overhead.
	ActualSizeBytes int64 `json:"actualSizeBytes,omitempty"`
	// Device is the engine-reported block device path (e.g. /dev/nvme0n1).
	Device string `json:"device,omitempty"`

	// Encryption carries the wrapped DK and Transit key version written
	// by the controller at provision time when spec.encryption.enabled.
	Encryption *EncryptionStatus `json:"encryption,omitempty"`
}

// EncryptionStatus is the observed state of a volume's Dataset Key.
type EncryptionStatus struct {
	Provisioned   bool         `json:"provisioned,omitempty"`
	WrappedDK     []byte       `json:"wrappedDK,omitempty"`
	KeyVersion    uint64       `json:"keyVersion,omitempty"`
	ProvisionedAt *metav1.Time `json:"provisionedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=bv,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Pool",type=string,JSONPath=`.spec.pool`
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.spec.size`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// BlockVolume is a raw block device.
type BlockVolume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BlockVolumeSpec   `json:"spec,omitempty"`
	Status            BlockVolumeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BlockVolumeList contains a list of BlockVolume.
type BlockVolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BlockVolume `json:"items"`
}

func init() { SchemeBuilder.Register(&BlockVolume{}, &BlockVolumeList{}) }
