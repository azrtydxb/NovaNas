package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// UpsPolicySpec defines the desired state of UpsPolicy.
type UpsPolicySpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for UpsPolicy.
}

// UpsPolicyStatus defines observed state of UpsPolicy.
type UpsPolicyStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// UpsPolicy — NUT/apcupsd integration
type UpsPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              UpsPolicySpec   `json:"spec,omitempty"`
	Status            UpsPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UpsPolicyList contains a list of UpsPolicy.
type UpsPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UpsPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&UpsPolicy{}, &UpsPolicyList{}) }
