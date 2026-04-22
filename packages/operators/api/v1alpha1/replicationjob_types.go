package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ReplicationJobSpec defines the desired state of ReplicationJob.
type ReplicationJobSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for ReplicationJob.
}

// ReplicationJobStatus defines observed state of ReplicationJob.
type ReplicationJobStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// ReplicationJob — Snapshot-diff replication job
type ReplicationJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ReplicationJobSpec   `json:"spec,omitempty"`
	Status            ReplicationJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ReplicationJobList contains a list of ReplicationJob.
type ReplicationJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReplicationJob `json:"items"`
}

func init() { SchemeBuilder.Register(&ReplicationJob{}, &ReplicationJobList{}) }
