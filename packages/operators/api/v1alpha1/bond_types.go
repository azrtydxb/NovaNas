package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BondMode is the kernel bond mode string (see bonding.txt in the Linux
// kernel tree). The enum mirrors packages/schemas/src/networking/bond.ts.
// +kubebuilder:validation:Enum=active-backup;balance-alb;balance-tlb;802.3ad;balance-rr;balance-xor;broadcast
type BondMode string

// BondLacp configures LACP-specific parameters when Mode is 802.3ad.
type BondLacp struct {
	// +kubebuilder:validation:Enum=slow;fast
	// +optional
	Rate string `json:"rate,omitempty"`
	// +kubebuilder:validation:Enum=stable;bandwidth;count
	// +optional
	AggregatorSelect string `json:"aggregatorSelect,omitempty"`
}

// BondSpec defines the desired state of Bond. Mirrors BondSpecSchema.
type BondSpec struct {
	// Interfaces is the ordered list of member NIC names.
	// +kubebuilder:validation:MinItems=1
	Interfaces []string `json:"interfaces"`
	// Mode is the kernel bonding mode.
	Mode BondMode `json:"mode"`
	// Lacp is the LACP tuning block (only meaningful when Mode=802.3ad).
	// +optional
	Lacp *BondLacp `json:"lacp,omitempty"`
	// XmitHashPolicy selects the transmit hash algorithm.
	// +kubebuilder:validation:Enum=layer2;layer2+3;layer3+4;encap2+3;encap3+4
	// +optional
	XmitHashPolicy string `json:"xmitHashPolicy,omitempty"`
	// Mtu overrides the aggregate interface MTU.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Mtu *int32 `json:"mtu,omitempty"`
	// Miimon is the MII link-check interval in milliseconds.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Miimon *int32 `json:"miimon,omitempty"`
}

// BondStatus defines observed state of Bond.
type BondStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Degraded;Failed;Reconciling;Ready
	// +optional
	Phase string `json:"phase,omitempty"`
	// ActiveMembers lists members currently up/carrying traffic.
	// +optional
	ActiveMembers []string `json:"activeMembers,omitempty"`
	// AppliedConfigHash is the sha256 of the nmstate YAML last applied to
	// the host for this bond. Used to detect drift.
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
// +kubebuilder:printcolumn:name="Mode",type="string",JSONPath=".spec.mode"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Bond — LACP / active-backup / balance interface.
type Bond struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BondSpec   `json:"spec,omitempty"`
	Status            BondStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BondList contains a list of Bond.
type BondList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bond `json:"items"`
}

func init() { SchemeBuilder.Register(&Bond{}, &BondList{}) }
