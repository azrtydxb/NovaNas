package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DatasetSpec defines the desired state of Dataset.
type DatasetSpec struct {
	// +kubebuilder:validation:MinLength=1
	Pool string `json:"pool"`
	// +kubebuilder:validation:MinLength=1
	Size string `json:"size"`
	// +kubebuilder:validation:Enum=xfs;ext4;btrfs;zfs
	Filesystem string `json:"filesystem"`
	// +kubebuilder:validation:Enum=posix;nfsv4;smb;mixed
	AclMode string `json:"aclMode,omitempty"`
	// +kubebuilder:validation:Enum=none;lz4;zstd;gzip
	Compression string              `json:"compression,omitempty"`
	Protection  *ProtectionPolicy   `json:"protection,omitempty"`
	Encryption  *EncryptionSettings `json:"encryption,omitempty"`
	Tiering     *TieringPolicy      `json:"tiering,omitempty"`
	Quota       *DatasetQuota       `json:"quota,omitempty"`
	Defaults    *DatasetDefaults    `json:"defaults,omitempty"`
	// AllowDataLoss permits deletion even when Snapshots/Shares reference
	// this Dataset. Without it the controller cascades Snapshots first.
	AllowDataLoss bool `json:"allowDataLoss,omitempty"`
}

// DatasetQuota sets quota thresholds.
type DatasetQuota struct {
	Hard string `json:"hard,omitempty"`
	Soft string `json:"soft,omitempty"`
}

// DatasetDefaults sets default ownership and permissions.
type DatasetDefaults struct {
	Owner string `json:"owner,omitempty"`
	Group string `json:"group,omitempty"`
	// +kubebuilder:validation:Pattern=`^0?[0-7]{3,4}$`
	Mode string `json:"mode,omitempty"`
}

// DatasetStatus defines observed state.
type DatasetStatus struct {
	// +kubebuilder:validation:Enum=Pending;Mounted;Degraded;Failed;Ready
	Phase              string             `json:"phase,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	UsedBytes          int64              `json:"usedBytes,omitempty"`
	MountPath          string             `json:"mountPath,omitempty"`
	MountPoint         string             `json:"mountPoint,omitempty"`

	// Encryption carries the wrapped DK produced at provision time.
	Encryption *EncryptionStatus `json:"encryption,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=ds,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Pool",type=string,JSONPath=`.spec.pool`
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.spec.size`
// +kubebuilder:printcolumn:name="FS",type=string,JSONPath=`.spec.filesystem`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Dataset is a BlockVolume + filesystem + mountable storage area.
type Dataset struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DatasetSpec   `json:"spec,omitempty"`
	Status            DatasetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatasetList contains a list of Dataset.
type DatasetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Dataset `json:"items"`
}

func init() { SchemeBuilder.Register(&Dataset{}, &DatasetList{}) }
