package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ServicePolicySpec defines the desired state of ServicePolicy.
type ServicePolicySpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for ServicePolicy.
}

// ServicePolicyStatus defines observed state of ServicePolicy.
type ServicePolicyStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// ServicePolicy — Master enable/disable for SSH, SMB, NFS
type ServicePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ServicePolicySpec   `json:"spec,omitempty"`
	Status            ServicePolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServicePolicyList contains a list of ServicePolicy.
type ServicePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServicePolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&ServicePolicy{}, &ServicePolicyList{}) }
