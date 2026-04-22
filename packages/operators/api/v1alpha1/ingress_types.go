package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// IngressSpec defines the desired state of Ingress.
type IngressSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for Ingress.
}

// IngressStatus defines observed state of Ingress.
type IngressStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// Ingress — novaedge reverse-proxy ingress rules
type Ingress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              IngressSpec   `json:"spec,omitempty"`
	Status            IngressStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IngressList contains a list of Ingress.
type IngressList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Ingress `json:"items"`
}

func init() { SchemeBuilder.Register(&Ingress{}, &IngressList{}) }
