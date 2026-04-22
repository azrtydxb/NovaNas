package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CertificateSpec defines the desired state of Certificate.
type CertificateSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for Certificate.
}

// CertificateStatus defines observed state of Certificate.
type CertificateStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// Certificate — TLS certificate
type Certificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CertificateSpec   `json:"spec,omitempty"`
	Status            CertificateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CertificateList contains a list of Certificate.
type CertificateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Certificate `json:"items"`
}

func init() { SchemeBuilder.Register(&Certificate{}, &CertificateList{}) }
