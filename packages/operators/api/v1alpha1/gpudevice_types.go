package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ResourceRef is a simple name/namespace/kind pointer used by Status fields.
type ResourceRef struct {
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// GpuDeviceSpec defines the desired state of GpuDevice.
type GpuDeviceSpec struct {
	// Passthrough requests the device be rebound to vfio-pci for VM use.
	Passthrough bool `json:"passthrough,omitempty"`
}

// GpuDeviceStatus defines observed state of GpuDevice.
type GpuDeviceStatus struct {
	// +kubebuilder:validation:Enum=Detecting;Available;Assigned;Unavailable;Failed
	Phase      string             `json:"phase,omitempty"`
	Vendor     string             `json:"vendor,omitempty"`
	Model      string             `json:"model,omitempty"`
	PCIAddress string             `json:"pciAddress,omitempty"`
	DeviceID   string             `json:"deviceId,omitempty"`
	Driver     string             `json:"driver,omitempty"`
	VfioBound  bool               `json:"vfioBound,omitempty"`
	IommuGroup int32              `json:"iommuGroup,omitempty"`
	AssignedTo *ResourceRef       `json:"assignedTo,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Vendor",type=string,JSONPath=`.status.vendor`
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.status.model`
// +kubebuilder:printcolumn:name="Passthrough",type=boolean,JSONPath=`.spec.passthrough`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// GpuDevice — Observed GPU with passthrough assignment.
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
