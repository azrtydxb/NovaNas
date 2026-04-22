package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ConfigBackupPolicySpec defines the desired state of ConfigBackupPolicy.
type ConfigBackupPolicySpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for ConfigBackupPolicy.
}

// ConfigBackupPolicyStatus defines observed state of ConfigBackupPolicy.
type ConfigBackupPolicyStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// ConfigBackupPolicy — Config snapshot cron and destinations
type ConfigBackupPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ConfigBackupPolicySpec   `json:"spec,omitempty"`
	Status            ConfigBackupPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConfigBackupPolicyList contains a list of ConfigBackupPolicy.
type ConfigBackupPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigBackupPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&ConfigBackupPolicy{}, &ConfigBackupPolicyList{}) }
