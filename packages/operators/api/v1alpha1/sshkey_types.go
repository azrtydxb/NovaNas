package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SshKeySpec defines the desired state of SshKey.
type SshKeySpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for SshKey.
}

// SshKeyStatus defines observed state of SshKey.
type SshKeyStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// SshKey — SSH authorized keys
type SshKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SshKeySpec   `json:"spec,omitempty"`
	Status            SshKeyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SshKeyList contains a list of SshKey.
type SshKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SshKey `json:"items"`
}

func init() { SchemeBuilder.Register(&SshKey{}, &SshKeyList{}) }
