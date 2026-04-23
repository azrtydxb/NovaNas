package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AppChartRef points at a chart (OCI or classic Helm repo).
type AppChartRef struct {
	// OCIRef is a pullable OCI reference such as oci://registry/org/chart.
	OCIRef string `json:"ociRef,omitempty"`
	// HelmRepo is the https URL of a traditional chart repo (used when OCIRef is empty).
	HelmRepo string `json:"helmRepo,omitempty"`
	// Name of the chart inside the repo (ignored for OCIRef).
	Name string `json:"name,omitempty"`
	// Version is the chart semver. Required for reproducibility.
	Version string `json:"version,omitempty"`
	// Digest pins a specific content digest (OCI manifest or index entry).
	Digest string `json:"digest,omitempty"`
}

// AppRequirements advertises minimum host capabilities the catalog entry
// expects. Operators use this to gate scheduling.
type AppRequirements struct {
	MinRAMMB    int32   `json:"minRamMB,omitempty"`
	MinCPU      float64 `json:"minCpu,omitempty"`
	RequiresGPU bool    `json:"requiresGpu,omitempty"`
	// Ports advertises ports the app expects (1-65535).
	Ports []int32 `json:"ports,omitempty"`
	// Privileged marks apps that need a privileged pod security context.
	Privileged bool `json:"privileged,omitempty"`
}

// AppSpec defines the desired state of App (a catalog entry projection).
type AppSpec struct {
	// DisplayName is the human-friendly name (required).
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"displayName"`
	// Version is the catalog version string (required).
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`
	// Icon URL or inline data URI.
	Icon        string `json:"icon,omitempty"`
	Description string `json:"description,omitempty"`
	// Chart is the chart reference (required).
	Chart        AppChartRef      `json:"chart"`
	Requirements *AppRequirements `json:"requirements,omitempty"`
	Category     string           `json:"category,omitempty"`
	Homepage     string           `json:"homepage,omitempty"`
	Maintainers  []string         `json:"maintainers,omitempty"`
}

// AppStatus defines observed state of App.
type AppStatus struct {
	// Phase is a coarse lifecycle summary.
	// +kubebuilder:validation:Enum=Pending;Ready;Failed
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// App — Synthesized catalog entry (read-only projection from AppCatalog).
type App struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AppSpec   `json:"spec,omitempty"`
	Status            AppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AppList contains a list of App.
type AppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []App `json:"items"`
}

func init() { SchemeBuilder.Register(&App{}, &AppList{}) }
