package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BlockVolumeSpec defines the desired state of BlockVolume.
type BlockVolumeSpec struct {
	Pool       string              `json:"pool,omitempty"`
	Size       string              `json:"size,omitempty"`
	Protection *ProtectionPolicy   `json:"protection,omitempty"`
	Encryption *EncryptionSettings `json:"encryption,omitempty"`
	Tiering    *TieringPolicy      `json:"tiering,omitempty"`
}

// ProtectionPolicy encodes replication / erasure coding choice.
type ProtectionPolicy struct {
	Mode          string              `json:"mode,omitempty"`
	Replicas      int32               `json:"replicas,omitempty"`
	ErasureCoding *ErasureCodingSpec  `json:"erasureCoding,omitempty"`
}

// ErasureCodingSpec configures EC parameters.
type ErasureCodingSpec struct {
	DataShards   int32 `json:"dataShards,omitempty"`
	ParityShards int32 `json:"parityShards,omitempty"`
}

// EncryptionSettings toggles dataset-at-rest encryption.
type EncryptionSettings struct {
	Enabled bool   `json:"enabled,omitempty"`
	KmsKey  string `json:"kmsKey,omitempty"`
}

// TieringPolicy describes cross-pool data movement.
type TieringPolicy struct {
	Primary      string `json:"primary,omitempty"`
	DemoteTo     string `json:"demoteTo,omitempty"`
	DemoteAfter  string `json:"demoteAfter,omitempty"`
	PromoteOn    string `json:"promoteOn,omitempty"`
}

// BlockVolumeStatus defines observed state.
type BlockVolumeStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	UsedBytes  int64              `json:"usedBytes,omitempty"`

	// Encryption carries the wrapped DK and Transit key version written
	// by the controller at provision time when spec.encryption.enabled.
	Encryption *EncryptionStatus `json:"encryption,omitempty"`
}

// EncryptionStatus is the observed state of a volume's Dataset Key.
// Populated by the controller on first successful ProvisionVolume call.
type EncryptionStatus struct {
	// Provisioned is true once the DK has been generated and wrapped.
	Provisioned bool `json:"provisioned,omitempty"`
	// WrappedDK is the OpenBao Transit-wrapped Dataset Key, base64-encoded
	// by the JSON marshaller.
	WrappedDK []byte `json:"wrappedDK,omitempty"`
	// KeyVersion is the Transit master-key version used to wrap the DK.
	KeyVersion uint64 `json:"keyVersion,omitempty"`
	// ProvisionedAt records when the DK was created.
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
