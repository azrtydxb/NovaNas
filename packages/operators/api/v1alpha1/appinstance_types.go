package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AppInstanceSpec defines the desired state of AppInstance.
type AppInstanceSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for AppInstance.
}

// AppInstanceStatus defines observed state of AppInstance.
type AppInstanceStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,categories=novanas
// +kubebuilder:subresource:status

// AppInstance — User-installed app
type AppInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AppInstanceSpec   `json:"spec,omitempty"`
	Status            AppInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AppInstanceList contains a list of AppInstance.
type AppInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppInstance `json:"items"`
}

func init() { SchemeBuilder.Register(&AppInstance{}, &AppInstanceList{}) }
