package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CloudBackupJobSpec defines the desired state of CloudBackupJob.
type CloudBackupJobSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for CloudBackupJob.
}

// CloudBackupJobStatus defines observed state of CloudBackupJob.
type CloudBackupJobStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// CloudBackupJob — Volume-to-cloud backup job
type CloudBackupJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CloudBackupJobSpec   `json:"spec,omitempty"`
	Status            CloudBackupJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudBackupJobList contains a list of CloudBackupJob.
type CloudBackupJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudBackupJob `json:"items"`
}

func init() { SchemeBuilder.Register(&CloudBackupJob{}, &CloudBackupJobList{}) }
