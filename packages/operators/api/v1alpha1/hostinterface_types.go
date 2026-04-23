package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// HostInterfaceUsage tags an interface with its functional role(s).
// +kubebuilder:validation:Enum=management;storage;cluster;vmBridge;appIngress
type HostInterfaceUsage string

// HostInterfaceAddress is an IP address assignment.
type HostInterfaceAddress struct {
	// Cidr is an IP/prefix (e.g. 10.0.0.2/24 or fe80::1/64).
	// +kubebuilder:validation:MinLength=1
	Cidr string `json:"cidr"`
	// Type selects address acquisition mode.
	// +kubebuilder:validation:Enum=static;dhcp;slaac
	Type string `json:"type"`
}

// HostInterfaceSpec defines the desired state of HostInterface.
type HostInterfaceSpec struct {
	// Backing names the physical NIC, Bond, or Vlan this HostInterface
	// consumes.
	// +kubebuilder:validation:MinLength=1
	Backing string `json:"backing"`
	// Addresses is the list of IP assignments.
	// +optional
	Addresses []HostInterfaceAddress `json:"addresses,omitempty"`
	// Gateway is the default gateway (optional).
	// +optional
	Gateway string `json:"gateway,omitempty"`
	// Dns is the list of DNS resolver IPs.
	// +optional
	Dns []string `json:"dns,omitempty"`
	// Mtu overrides the interface MTU.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Mtu *int32 `json:"mtu,omitempty"`
	// Usage declares the functional role(s) of this interface.
	// +kubebuilder:validation:MinItems=1
	Usage []HostInterfaceUsage `json:"usage"`
}

// HostInterfaceStatus defines observed state of HostInterface.
type HostInterfaceStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed;Reconciling;Ready
	// +optional
	Phase string `json:"phase,omitempty"`
	// EffectiveAddresses lists the addresses actually programmed.
	// +optional
	EffectiveAddresses []string `json:"effectiveAddresses,omitempty"`
	// +kubebuilder:validation:Enum=up;down
	// +optional
	Link string `json:"link,omitempty"`
	// AppliedConfigHash is the sha256 of the last-applied nmstate YAML.
	// +optional
	AppliedConfigHash string `json:"appliedConfigHash,omitempty"`
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
// +kubebuilder:printcolumn:name="Backing",type="string",JSONPath=".spec.backing"
// +kubebuilder:printcolumn:name="Link",type="string",JSONPath=".status.link"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

// HostInterface represents a host-level IP-bearing interface.
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
