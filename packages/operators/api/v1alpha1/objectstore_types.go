package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ObjectStoreSpec defines the desired state of ObjectStore.
type ObjectStoreSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for ObjectStore.
}

// ObjectStoreStatus defines observed state of ObjectStore.
type ObjectStoreStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// ObjectStore — S3 gateway service config
type ObjectStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ObjectStoreSpec   `json:"spec,omitempty"`
	Status            ObjectStoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ObjectStoreList contains a list of ObjectStore.
type ObjectStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ObjectStore `json:"items"`
}

func init() { SchemeBuilder.Register(&ObjectStore{}, &ObjectStoreList{}) }
