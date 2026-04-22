package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// GpuDeviceSpec defines the desired state of GpuDevice.
type GpuDeviceSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for GpuDevice.
}

// GpuDeviceStatus defines observed state of GpuDevice.
type GpuDeviceStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// GpuDevice — Observed GPU, passthrough assignment
type GpuDevice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GpuDeviceSpec   `json:"spec,omitempty"`
	Status            GpuDeviceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GpuDeviceList contains a list of GpuDevice.
type GpuDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GpuDevice `json:"items"`
}

func init() { SchemeBuilder.Register(&GpuDevice{}, &GpuDeviceList{}) }
