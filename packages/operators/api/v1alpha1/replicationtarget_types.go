package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ReplicationTargetSpec defines the desired state of ReplicationTarget.
type ReplicationTargetSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for ReplicationTarget.
}

// ReplicationTargetStatus defines observed state of ReplicationTarget.
type ReplicationTargetStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// ReplicationTarget — Remote NovaNas endpoint
type ReplicationTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ReplicationTargetSpec   `json:"spec,omitempty"`
	Status            ReplicationTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ReplicationTargetList contains a list of ReplicationTarget.
type ReplicationTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReplicationTarget `json:"items"`
}

func init() { SchemeBuilder.Register(&ReplicationTarget{}, &ReplicationTargetList{}) }
