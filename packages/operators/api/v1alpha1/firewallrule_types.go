package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// FirewallScope selects the enforcement plane for a rule.
// +kubebuilder:validation:Enum=host;pod
type FirewallScope string

// FirewallDirection selects the traffic direction.
// +kubebuilder:validation:Enum=inbound;outbound
type FirewallDirection string

// FirewallAction is the terminal disposition for a matching packet.
// +kubebuilder:validation:Enum=allow;deny;reject;log
type FirewallAction string

// FirewallProtocol is the L3/L4 protocol filter.
// +kubebuilder:validation:Enum=tcp;udp;icmp;any
type FirewallProtocol string

// FirewallEndpoint is the match block for a source/destination.
type FirewallEndpoint struct {
	// Cidrs is a list of IPv4/IPv6 CIDRs to match.
	// +optional
	Cidrs []string `json:"cidrs,omitempty"`
	// Labels matches pods by label (only meaningful when scope=pod).
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Ports is the list of L4 ports to match.
	// +optional
	Ports []int32 `json:"ports,omitempty"`
	// Protocol narrows the match to a single L4 protocol.
	// +optional
	Protocol FirewallProtocol `json:"protocol,omitempty"`
}

// FirewallRuleSpec defines the desired state of FirewallRule.
type FirewallRuleSpec struct {
	Scope     FirewallScope     `json:"scope"`
	Direction FirewallDirection `json:"direction"`
	Action    FirewallAction    `json:"action"`
	// Interface restricts the rule to traffic on this NIC (scope=host only).
	// +optional
	Interface string `json:"interface,omitempty"`
	// +optional
	Source *FirewallEndpoint `json:"source,omitempty"`
	// +optional
	Destination *FirewallEndpoint `json:"destination,omitempty"`
	// Priority orders rules; lower runs first. Defaults to 1000.
	// +optional
	Priority int32 `json:"priority,omitempty"`
}

// FirewallRuleStatus defines observed state of FirewallRule.
type FirewallRuleStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed;Reconciling;Ready
	// +optional
	Phase string `json:"phase,omitempty"`
	// InstalledAt is the timestamp of successful rule installation.
	// +optional
	InstalledAt *metav1.Time `json:"installedAt,omitempty"`
	// AppliedConfigHash is the sha256 of the rendered nftables/eBPF blob.
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
// +kubebuilder:printcolumn:name="Scope",type="string",JSONPath=".spec.scope"
// +kubebuilder:printcolumn:name="Direction",type="string",JSONPath=".spec.direction"
// +kubebuilder:printcolumn:name="Action",type="string",JSONPath=".spec.action"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

// FirewallRule is a single allow/deny rule programmed into nftables or eBPF.
type FirewallRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FirewallRuleSpec   `json:"spec,omitempty"`
	Status            FirewallRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FirewallRuleList contains a list of FirewallRule.
type FirewallRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FirewallRule `json:"items"`
}

func init() { SchemeBuilder.Register(&FirewallRule{}, &FirewallRuleList{}) }
