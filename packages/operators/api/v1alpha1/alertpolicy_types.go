package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AlertPolicySpec defines the desired state of AlertPolicy.
type AlertPolicySpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for AlertPolicy.
}

// AlertPolicyStatus defines observed state of AlertPolicy.
type AlertPolicyStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// AlertPolicy — Metric threshold to channel mapping
type AlertPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AlertPolicySpec   `json:"spec,omitempty"`
	Status            AlertPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AlertPolicyList contains a list of AlertPolicy.
type AlertPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AlertPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&AlertPolicy{}, &AlertPolicyList{}) }
