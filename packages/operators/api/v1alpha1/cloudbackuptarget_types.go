package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CloudBackupTargetSpec defines the desired state of CloudBackupTarget.
type CloudBackupTargetSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for CloudBackupTarget.
}

// CloudBackupTargetStatus defines observed state of CloudBackupTarget.
type CloudBackupTargetStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// CloudBackupTarget — S3/B2/Azure endpoint
type CloudBackupTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CloudBackupTargetSpec   `json:"spec,omitempty"`
	Status            CloudBackupTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudBackupTargetList contains a list of CloudBackupTarget.
type CloudBackupTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudBackupTarget `json:"items"`
}

func init() { SchemeBuilder.Register(&CloudBackupTarget{}, &CloudBackupTargetList{}) }
