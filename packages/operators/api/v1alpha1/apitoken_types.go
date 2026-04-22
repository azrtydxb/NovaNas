package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ApiTokenSpec defines the desired state of ApiToken.
type ApiTokenSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for ApiToken.
}

// ApiTokenStatus defines observed state of ApiToken.
type ApiTokenStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// ApiToken — Scoped API token
type ApiToken struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ApiTokenSpec   `json:"spec,omitempty"`
	Status            ApiTokenStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApiTokenList contains a list of ApiToken.
type ApiTokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApiToken `json:"items"`
}

func init() { SchemeBuilder.Register(&ApiToken{}, &ApiTokenList{}) }
