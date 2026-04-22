package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// TrafficPolicySpec defines the desired state of TrafficPolicy.
type TrafficPolicySpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for TrafficPolicy.
}

// TrafficPolicyStatus defines observed state of TrafficPolicy.
type TrafficPolicyStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// TrafficPolicy — QoS limits by scope
type TrafficPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TrafficPolicySpec   `json:"spec,omitempty"`
	Status            TrafficPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TrafficPolicyList contains a list of TrafficPolicy.
type TrafficPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TrafficPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&TrafficPolicy{}, &TrafficPolicyList{}) }
