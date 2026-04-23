package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// VlanSpec defines the desired state of Vlan.
type VlanSpec struct {
	// Parent is the underlying interface (ethernet, bond, etc.).
	// +kubebuilder:validation:MinLength=1
	Parent string `json:"parent"`
	// VlanId is the 802.1Q VLAN tag (1-4094).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4094
	VlanId int32 `json:"vlanId"`
	// Mtu overrides the child interface MTU.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Mtu *int32 `json:"mtu,omitempty"`
}

// VlanStatus defines observed state of Vlan.
type VlanStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed;Reconciling;Ready
	// +optional
	Phase string `json:"phase,omitempty"`
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
// +kubebuilder:printcolumn:name="Parent",type="string",JSONPath=".spec.parent"
// +kubebuilder:printcolumn:name="VlanID",type="integer",JSONPath=".spec.vlanId"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

// Vlan is an 802.1Q VLAN child interface.
type Vlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VlanSpec   `json:"spec,omitempty"`
	Status            VlanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VlanList contains a list of Vlan.
type VlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Vlan `json:"items"`
}

func init() { SchemeBuilder.Register(&Vlan{}, &VlanList{}) }
