package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ScrubScheduleSpec defines the desired state of ScrubSchedule.
type ScrubScheduleSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for ScrubSchedule.
}

// ScrubScheduleStatus defines observed state of ScrubSchedule.
type ScrubScheduleStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// ScrubSchedule — Per-pool integrity scrub cadence
type ScrubSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ScrubScheduleSpec   `json:"spec,omitempty"`
	Status            ScrubScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScrubScheduleList contains a list of ScrubSchedule.
type ScrubScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScrubSchedule `json:"items"`
}

func init() { SchemeBuilder.Register(&ScrubSchedule{}, &ScrubScheduleList{}) }
