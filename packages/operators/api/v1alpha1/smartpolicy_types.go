package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SmartPolicySpec defines the desired state of SmartPolicy.
type SmartPolicySpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for SmartPolicy.
}

// SmartPolicyStatus defines observed state of SmartPolicy.
type SmartPolicyStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// SmartPolicy — Disk SMART test cadence and thresholds
type SmartPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SmartPolicySpec   `json:"spec,omitempty"`
	Status            SmartPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SmartPolicyList contains a list of SmartPolicy.
type SmartPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SmartPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&SmartPolicy{}, &SmartPolicyList{}) }
