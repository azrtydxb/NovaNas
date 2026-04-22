package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// NvmeofTargetSpec defines the desired state of NvmeofTarget.
type NvmeofTargetSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for NvmeofTarget.
}

// NvmeofTargetStatus defines observed state of NvmeofTarget.
type NvmeofTargetStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// NvmeofTarget — NVMe-oF subsystem binding a BlockVolume
type NvmeofTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NvmeofTargetSpec   `json:"spec,omitempty"`
	Status            NvmeofTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NvmeofTargetList contains a list of NvmeofTarget.
type NvmeofTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NvmeofTarget `json:"items"`
}

func init() { SchemeBuilder.Register(&NvmeofTarget{}, &NvmeofTargetList{}) }
