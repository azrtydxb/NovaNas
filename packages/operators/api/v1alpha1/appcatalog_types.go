package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AppCatalogSpec defines the desired state of AppCatalog.
type AppCatalogSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for AppCatalog.
}

// AppCatalogStatus defines observed state of AppCatalog.
type AppCatalogStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// AppCatalog — Catalog source (git/helm/oci)
type AppCatalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AppCatalogSpec   `json:"spec,omitempty"`
	Status            AppCatalogStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AppCatalogList contains a list of AppCatalog.
type AppCatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppCatalog `json:"items"`
}

func init() { SchemeBuilder.Register(&AppCatalog{}, &AppCatalogList{}) }
