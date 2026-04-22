package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// KmsKeySpec defines the desired state of KmsKey.
type KmsKeySpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for KmsKey.
}

// KmsKeyStatus defines observed state of KmsKey.
type KmsKeyStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// KmsKey — Named data key for SSE-KMS usage
type KmsKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KmsKeySpec   `json:"spec,omitempty"`
	Status            KmsKeyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KmsKeyList contains a list of KmsKey.
type KmsKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KmsKey `json:"items"`
}

func init() { SchemeBuilder.Register(&KmsKey{}, &KmsKeyList{}) }
