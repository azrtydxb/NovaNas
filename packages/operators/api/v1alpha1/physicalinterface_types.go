package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PhysicalInterfaceSpec is intentionally empty — PhysicalInterface is an
// observed resource populated by the host agent, not user-authored.
type PhysicalInterfaceSpec struct{}

// PhysicalInterfaceStatus is the observed state of a physical NIC.
type PhysicalInterfaceStatus struct {
	// MacAddress is the hardware address.
	// +optional
	MacAddress string `json:"macAddress,omitempty"`
	// LinkSpeed in Mbps. 0 means unknown / link down.
	// +optional
	LinkSpeed int64 `json:"linkSpeed,omitempty"`
	// Duplex reports the negotiated duplex mode.
	// +kubebuilder:validation:Enum=full;half;unknown
	// +optional
	Duplex string `json:"duplex,omitempty"`
	// Link reports the operational link state.
	// +kubebuilder:validation:Enum=up;down
	// +optional
	Link string `json:"link,omitempty"`
	// Driver is the kernel driver name.
	// +optional
	Driver string `json:"driver,omitempty"`
	// PcieSlot is the PCI address (e.g. 0000:03:00.0).
	// +optional
	PcieSlot string `json:"pcieSlot,omitempty"`
	// Capabilities exposes ethtool feature flags.
	// +optional
	Capabilities []string `json:"capabilities,omitempty"`
	// UsedBy names the HostInterface / Bond / Vlan that claims this NIC.
	// +optional
	UsedBy string `json:"usedBy,omitempty"`
	// +optional
	Phase string `json:"phase,omitempty"`
	// ObservedGeneration tracks the generation of the last reconcile.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="MAC",type="string",JSONPath=".status.macAddress"
// +kubebuilder:printcolumn:name="Speed",type="integer",JSONPath=".status.linkSpeed"
// +kubebuilder:printcolumn:name="Link",type="string",JSONPath=".status.link"
// +kubebuilder:printcolumn:name="Driver",type="string",JSONPath=".status.driver"

// PhysicalInterface represents a NIC observed from the host.
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
