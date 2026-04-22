package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// UpdatePolicySpec defines the desired state of UpdatePolicy.
type UpdatePolicySpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for UpdatePolicy.
}

// UpdatePolicyStatus defines observed state of UpdatePolicy.
type UpdatePolicyStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// UpdatePolicy — Channel, auto-update, maintenance window
type UpdatePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              UpdatePolicySpec   `json:"spec,omitempty"`
	Status            UpdatePolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UpdatePolicyList contains a list of UpdatePolicy.
type UpdatePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UpdatePolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&UpdatePolicy{}, &UpdatePolicyList{}) }
