package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// GroupSpec defines the desired state of Group.
type GroupSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for Group.
}

// GroupStatus defines observed state of Group.
type GroupStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// Group — Group projection of Keycloak
type Group struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GroupSpec   `json:"spec,omitempty"`
	Status            GroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GroupList contains a list of Group.
type GroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Group `json:"items"`
}

func init() { SchemeBuilder.Register(&Group{}, &GroupList{}) }
