package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// VipPoolSpec defines the desired state of VipPool.
type VipPoolSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for VipPool.
}

// VipPoolStatus defines observed state of VipPool.
type VipPoolStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// VipPool — novaedge LAN VIP range
type VipPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VipPoolSpec   `json:"spec,omitempty"`
	Status            VipPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VipPoolList contains a list of VipPool.
type VipPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VipPool `json:"items"`
}

func init() { SchemeBuilder.Register(&VipPool{}, &VipPoolList{}) }
