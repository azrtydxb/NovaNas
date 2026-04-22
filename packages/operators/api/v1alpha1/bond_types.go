package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BondSpec defines the desired state of Bond.
type BondSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for Bond.
}

// BondStatus defines observed state of Bond.
type BondStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// Bond — LACP / active-backup / balance interface
type Bond struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BondSpec   `json:"spec,omitempty"`
	Status            BondStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BondList contains a list of Bond.
type BondList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bond `json:"items"`
}

func init() { SchemeBuilder.Register(&Bond{}, &BondList{}) }
