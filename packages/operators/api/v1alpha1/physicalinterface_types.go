package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PhysicalInterfaceSpec defines the desired state of PhysicalInterface.
type PhysicalInterfaceSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for PhysicalInterface.
}

// PhysicalInterfaceStatus defines observed state of PhysicalInterface.
type PhysicalInterfaceStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// PhysicalInterface — Observed NIC (status-only)
type PhysicalInterface struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PhysicalInterfaceSpec   `json:"spec,omitempty"`
	Status            PhysicalInterfaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PhysicalInterfaceList contains a list of PhysicalInterface.
type PhysicalInterfaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PhysicalInterface `json:"items"`
}

func init() { SchemeBuilder.Register(&PhysicalInterface{}, &PhysicalInterfaceList{}) }
