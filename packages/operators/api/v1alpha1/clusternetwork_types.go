package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// OverlayType selects the data-plane encapsulation mode.
// +kubebuilder:validation:Enum=geneve;vxlan;none
type OverlayType string

// ClusterNetworkOverlay specifies the cluster overlay configuration.
type ClusterNetworkOverlay struct {
	Type OverlayType `json:"type"`
	// EgressInterface is the NIC used for overlay egress traffic.
	// +optional
	EgressInterface string `json:"egressInterface,omitempty"`
}

// ClusterNetworkPolicy tunes default policy behavior.
type ClusterNetworkPolicy struct {
	// DefaultDeny installs a default-deny policy for pod traffic.
	// +optional
	DefaultDeny bool `json:"defaultDeny,omitempty"`
}

// ClusterNetworkSpec defines the desired state of ClusterNetwork.
type ClusterNetworkSpec struct {
	// PodCidr is the pod IP allocation CIDR (IPv4 or IPv6).
	// +kubebuilder:validation:MinLength=1
	PodCidr string `json:"podCidr"`
	// ServiceCidr is the cluster service IP CIDR.
	// +kubebuilder:validation:MinLength=1
	ServiceCidr string `json:"serviceCidr"`
	// Overlay selects the data-plane encapsulation.
	// +optional
	Overlay *ClusterNetworkOverlay `json:"overlay,omitempty"`
	// Policy tunes default pod policy behavior.
	// +optional
	Policy *ClusterNetworkPolicy `json:"policy,omitempty"`
	// Mtu is the cluster-wide MTU. Either an integer or the string "auto".
	// +kubebuilder:validation:XIntOrString
	// +optional
	Mtu *intstr.IntOrString `json:"mtu,omitempty"`
}

// ClusterNetworkStatus defines observed state of ClusterNetwork.
type ClusterNetworkStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed;Reconciling;Ready
	// +optional
	Phase string `json:"phase,omitempty"`
	// EffectiveMtu is the resolved MTU applied to the overlay.
	// +optional
	EffectiveMtu int32 `json:"effectiveMtu,omitempty"`
	// AppliedConfigHash is the sha256 of the generated CNI/cluster-net config.
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
// +kubebuilder:printcolumn:name="Pods",type="string",JSONPath=".spec.podCidr"
// +kubebuilder:printcolumn:name="Services",type="string",JSONPath=".spec.serviceCidr"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

// ClusterNetwork is the cluster-singleton owning cluster-wide CNI config.
type ClusterNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterNetworkSpec   `json:"spec,omitempty"`
	Status            ClusterNetworkStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterNetworkList contains a list of ClusterNetwork.
type ClusterNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterNetwork `json:"items"`
}

func init() { SchemeBuilder.Register(&ClusterNetwork{}, &ClusterNetworkList{}) }
