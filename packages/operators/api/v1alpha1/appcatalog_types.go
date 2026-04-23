package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AppCatalogSource describes where to fetch chart metadata from.
type AppCatalogSource struct {
	// +kubebuilder:validation:Enum=git;helm;oci;custom
	Type string `json:"type"`
	// URL is the remote location (git repo URL, helm repo URL, or oci ref).
	// +kubebuilder:validation:MinLength=1
	URL    string `json:"url"`
	Branch string `json:"branch,omitempty"`
	Path   string `json:"path,omitempty"`
	// RefreshInterval is a duration (Go or systemd-calendar style).
	RefreshInterval string `json:"refreshInterval,omitempty"`
}

// AppCatalogTrust describes signature requirements for catalog entries.
type AppCatalogTrust struct {
	SignedBy string `json:"signedBy,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// AppCatalogSpec defines the desired state of AppCatalog.
type AppCatalogSpec struct {
	Source AppCatalogSource `json:"source"`
	Trust  *AppCatalogTrust `json:"trust,omitempty"`
}

// AppCatalogStatus defines observed state of AppCatalog.
type AppCatalogStatus struct {
	// +kubebuilder:validation:Enum=Pending;Synced;Failed
	Phase      string             `json:"phase,omitempty"`
	LastSync   *metav1.Time       `json:"lastSync,omitempty"`
	AppCount   int32              `json:"appCount,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.source.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Apps",type=integer,JSONPath=`.status.appCount`

// AppCatalog — remote catalog source (git/helm/oci).
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
