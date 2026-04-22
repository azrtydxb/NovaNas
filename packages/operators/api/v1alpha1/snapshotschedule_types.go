package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SnapshotScheduleSpec defines the desired state of SnapshotSchedule.
type SnapshotScheduleSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for SnapshotSchedule.
}

// SnapshotScheduleStatus defines observed state of SnapshotSchedule.
type SnapshotScheduleStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// SnapshotSchedule — Periodic snapshot schedule
type SnapshotSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SnapshotScheduleSpec   `json:"spec,omitempty"`
	Status            SnapshotScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SnapshotScheduleList contains a list of SnapshotSchedule.
type SnapshotScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SnapshotSchedule `json:"items"`
}

func init() { SchemeBuilder.Register(&SnapshotSchedule{}, &SnapshotScheduleList{}) }
