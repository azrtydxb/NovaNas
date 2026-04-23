package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// VipPoolAnnounce selects the L2/L3 announcement method.
// +kubebuilder:validation:Enum=arp;bgp;ndp
type VipPoolAnnounce string

// VipPoolSpec defines the desired state of VipPool.
type VipPoolSpec struct {
	// Range is a CIDR or "start-end" IP range.
	// +kubebuilder:validation:MinLength=1
	Range string `json:"range"`
	// Interface is the NIC used to announce VIPs.
	// +kubebuilder:validation:MinLength=1
	Interface string `json:"interface"`
	// Announce selects the announcement protocol. Defaults to "arp".
	// +optional
	Announce VipPoolAnnounce `json:"announce,omitempty"`
}

// VipAllocation records which IP is bound to which target.
type VipAllocation struct {
	Ip string `json:"ip"`
	// +optional
	Owner string `json:"owner,omitempty"`
	// +optional
	OwnerUid string `json:"ownerUid,omitempty"`
}

// VipPoolStatus defines observed state of VipPool.
type VipPoolStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed;Reconciling;Ready
	// +optional
	Phase string `json:"phase,omitempty"`
	// Allocated is the number of VIPs currently assigned.
	// +optional
	Allocated int32 `json:"allocated,omitempty"`
	// Available is the number of VIPs still free to allocate.
	// +optional
	Available int32 `json:"available,omitempty"`
	// Allocations is the set of active VIP bindings.
	// +optional
	Allocations []VipAllocation `json:"allocations,omitempty"`
	// AppliedConfigHash is the sha256 of the projected IPAddressPool spec.
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
// +kubebuilder:printcolumn:name="Range",type="string",JSONPath=".spec.range"
// +kubebuilder:printcolumn:name="Interface",type="string",JSONPath=".spec.interface"
// +kubebuilder:printcolumn:name="Allocated",type="integer",JSONPath=".status.allocated"
// +kubebuilder:printcolumn:name="Available",type="integer",JSONPath=".status.available"

// VipPool is a pool of virtual IPs announced via MetalLB/keepalived.
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
