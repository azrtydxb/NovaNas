package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// FirewallRuleSpec defines the desired state of FirewallRule.
type FirewallRuleSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for FirewallRule.
}

// FirewallRuleStatus defines observed state of FirewallRule.
type FirewallRuleStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// FirewallRule — Host-level nftables or pod-level policy
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
