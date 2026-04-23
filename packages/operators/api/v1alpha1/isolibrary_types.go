package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// IsoSource declares a remote ISO the library should cache locally.
type IsoSource struct {
	// +kubebuilder:validation:MinLength=1
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
	Name   string `json:"name,omitempty"`
}

// IsoLibrarySpec defines the desired state of IsoLibrary.
type IsoLibrarySpec struct {
	// Dataset names the backing Dataset (mount point) where ISOs are cached.
	// +kubebuilder:validation:MinLength=1
	Dataset string      `json:"dataset"`
	Sources []IsoSource `json:"sources,omitempty"`
}

// IsoLibraryEntry is a single materialised ISO on the local store.
type IsoLibraryEntry struct {
	Name         string       `json:"name"`
	SizeBytes    int64        `json:"sizeBytes"`
	SHA256       string       `json:"sha256"`
	DownloadedAt *metav1.Time `json:"downloadedAt,omitempty"`
}

// IsoLibraryStatus defines observed state of IsoLibrary.
type IsoLibraryStatus struct {
	// +kubebuilder:validation:Enum=Pending;Ready;Failed
	Phase      string             `json:"phase,omitempty"`
	Entries    []IsoLibraryEntry  `json:"entries,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Dataset",type=string,JSONPath=`.spec.dataset`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// IsoLibrary — Managed ISO collection for VM install media.
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
