package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// KeycloakRealmSpec defines the desired state of KeycloakRealm.
type KeycloakRealmSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for KeycloakRealm.
}

// KeycloakRealmStatus defines observed state of KeycloakRealm.
type KeycloakRealmStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// KeycloakRealm — Realm federation config
type KeycloakRealm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KeycloakRealmSpec   `json:"spec,omitempty"`
	Status            KeycloakRealmStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakRealmList contains a list of KeycloakRealm.
type KeycloakRealmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakRealm `json:"items"`
}

func init() { SchemeBuilder.Register(&KeycloakRealm{}, &KeycloakRealmList{}) }
