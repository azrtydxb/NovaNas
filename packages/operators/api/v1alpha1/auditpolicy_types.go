package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AuditPolicySpec defines the desired state of AuditPolicy.
type AuditPolicySpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for AuditPolicy.
}

// AuditPolicyStatus defines observed state of AuditPolicy.
type AuditPolicyStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// AuditPolicy — What to audit, where to send it
type AuditPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AuditPolicySpec   `json:"spec,omitempty"`
	Status            AuditPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AuditPolicyList contains a list of AuditPolicy.
type AuditPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuditPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&AuditPolicy{}, &AuditPolicyList{}) }
