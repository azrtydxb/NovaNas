package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// IsoLibrarySpec defines the desired state of IsoLibrary.
type IsoLibrarySpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for IsoLibrary.
}

// IsoLibraryStatus defines observed state of IsoLibrary.
type IsoLibraryStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// IsoLibrary — Managed ISO collection for VM install
type IsoLibrary struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              IsoLibrarySpec   `json:"spec,omitempty"`
	Status            IsoLibraryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IsoLibraryList contains a list of IsoLibrary.
type IsoLibraryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IsoLibrary `json:"items"`
}

func init() { SchemeBuilder.Register(&IsoLibrary{}, &IsoLibraryList{}) }
