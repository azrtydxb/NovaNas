package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// VmSpec defines the desired state of Vm.
type VmSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for Vm.
}

// VmStatus defines observed state of Vm.
type VmStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,categories=novanas
// +kubebuilder:subresource:status

// Vm — KubeVirt VM with NAS-friendly UX
type Vm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VmSpec   `json:"spec,omitempty"`
	Status            VmStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VmList contains a list of Vm.
type VmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Vm `json:"items"`
}

func init() { SchemeBuilder.Register(&Vm{}, &VmList{}) }
