package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DatasetSpec defines the desired state of Dataset.
type DatasetSpec struct {
	Pool        string              `json:"pool,omitempty"`
	Size        string              `json:"size,omitempty"`
	Filesystem  string              `json:"filesystem,omitempty"`
	AclMode     string              `json:"aclMode,omitempty"`
	Compression string              `json:"compression,omitempty"`
	Protection  *ProtectionPolicy   `json:"protection,omitempty"`
	Encryption  *EncryptionSettings `json:"encryption,omitempty"`
	Tiering     *TieringPolicy      `json:"tiering,omitempty"`
	Quota       *DatasetQuota       `json:"quota,omitempty"`
	Defaults    *DatasetDefaults    `json:"defaults,omitempty"`
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
	Mode  string `json:"mode,omitempty"`
}

// DatasetStatus defines observed state.
type DatasetStatus struct {
	Phase       string             `json:"phase,omitempty"`
	Conditions  []metav1.Condition `json:"conditions,omitempty"`
	UsedBytes   int64              `json:"usedBytes,omitempty"`
	MountPath   string             `json:"mountPath,omitempty"`
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
