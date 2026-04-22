package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// HostInterfaceSpec defines the desired state of HostInterface.
type HostInterfaceSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for HostInterface.
}

// HostInterfaceStatus defines observed state of HostInterface.
type HostInterfaceStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// HostInterface — IP-bearing interface with role
type HostInterface struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              HostInterfaceSpec   `json:"spec,omitempty"`
	Status            HostInterfaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HostInterfaceList contains a list of HostInterface.
type HostInterfaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostInterface `json:"items"`
}

func init() { SchemeBuilder.Register(&HostInterface{}, &HostInterfaceList{}) }
