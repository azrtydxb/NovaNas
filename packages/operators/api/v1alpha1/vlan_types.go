package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// VlanSpec defines the desired state of Vlan.
type VlanSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for Vlan.
}

// VlanStatus defines observed state of Vlan.
type VlanStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// Vlan — 802.1Q virtual interface
type Vlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VlanSpec   `json:"spec,omitempty"`
	Status            VlanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VlanList contains a list of Vlan.
type VlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Vlan `json:"items"`
}

func init() { SchemeBuilder.Register(&Vlan{}, &VlanList{}) }
