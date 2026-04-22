package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AppSpec defines the desired state of App.
type AppSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for App.
}

// AppStatus defines observed state of App.
type AppStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// App — Synthesized catalog entry (read-only)
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
