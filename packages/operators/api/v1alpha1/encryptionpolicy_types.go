package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// EncryptionPolicySpec defines the desired state of EncryptionPolicy.
type EncryptionPolicySpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for EncryptionPolicy.
}

// EncryptionPolicyStatus defines observed state of EncryptionPolicy.
type EncryptionPolicyStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// EncryptionPolicy — Cluster defaults for volume encryption
type EncryptionPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              EncryptionPolicySpec   `json:"spec,omitempty"`
	Status            EncryptionPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EncryptionPolicyList contains a list of EncryptionPolicy.
type EncryptionPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EncryptionPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&EncryptionPolicy{}, &EncryptionPolicyList{}) }
