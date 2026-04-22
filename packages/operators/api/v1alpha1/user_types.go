package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// UserSpec defines the desired state of User.
type UserSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for User.
}

// UserStatus defines observed state of User.
type UserStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// User — Local user projection of Keycloak
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              UserSpec   `json:"spec,omitempty"`
	Status            UserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UserList contains a list of User.
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []User `json:"items"`
}

func init() { SchemeBuilder.Register(&User{}, &UserList{}) }
