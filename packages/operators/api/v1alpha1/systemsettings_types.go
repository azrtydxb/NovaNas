package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SystemSettingsSpec defines the desired state of SystemSettings.
type SystemSettingsSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for SystemSettings.
}

// SystemSettingsStatus defines observed state of SystemSettings.
type SystemSettingsStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// SystemSettings — Hostname, timezone, NTP, locale, SMTP
type SystemSettings struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SystemSettingsSpec   `json:"spec,omitempty"`
	Status            SystemSettingsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SystemSettingsList contains a list of SystemSettings.
type SystemSettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SystemSettings `json:"items"`
}

func init() { SchemeBuilder.Register(&SystemSettings{}, &SystemSettingsList{}) }
