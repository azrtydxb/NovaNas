package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AlertChannelSpec defines the desired state of AlertChannel.
type AlertChannelSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for AlertChannel.
}

// AlertChannelStatus defines observed state of AlertChannel.
type AlertChannelStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// AlertChannel — Email / webhook / ntfy / pushover
type AlertChannel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AlertChannelSpec   `json:"spec,omitempty"`
	Status            AlertChannelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AlertChannelList contains a list of AlertChannel.
type AlertChannelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AlertChannel `json:"items"`
}

func init() { SchemeBuilder.Register(&AlertChannel{}, &AlertChannelList{}) }
