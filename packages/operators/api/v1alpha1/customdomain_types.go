package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CustomDomainSpec defines the desired state of CustomDomain.
type CustomDomainSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for CustomDomain.
}

// CustomDomainStatus defines observed state of CustomDomain.
type CustomDomainStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// CustomDomain — User-supplied hostname
type CustomDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CustomDomainSpec   `json:"spec,omitempty"`
	Status            CustomDomainStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CustomDomainList contains a list of CustomDomain.
type CustomDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CustomDomain `json:"items"`
}

func init() { SchemeBuilder.Register(&CustomDomain{}, &CustomDomainList{}) }
