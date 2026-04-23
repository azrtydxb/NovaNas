package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ScrubScheduleSpec defines the desired state of ScrubSchedule.
type ScrubScheduleSpec struct {
	// Pool is the StoragePool name to scrub.
	// +kubebuilder:validation:MinLength=1
	Pool string `json:"pool"`
	// Cron is a 5 or 6-field cron expression.
	// +kubebuilder:validation:MinLength=1
	Cron string `json:"cron"`
	// Priority influences engine scheduling of the scrub task.
	// +kubebuilder:validation:Enum=low;normal;high
	Priority string `json:"priority,omitempty"`
	// Repair, when true, asks the scrubber to repair recoverable chunks
	// in-place instead of just reporting.
	Repair    bool `json:"repair,omitempty"`
	Suspended bool `json:"suspended,omitempty"`
}

// ScrubScheduleStatus defines observed state of ScrubSchedule.
type ScrubScheduleStatus struct {
	// +kubebuilder:validation:Enum=Active;Running;Suspended;Failed;Scheduled
	Phase              string             `json:"phase,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	LastRun            *metav1.Time       `json:"lastRun,omitempty"`
	NextRun            *metav1.Time       `json:"nextRun,omitempty"`
	ChunksScrubbed     int64              `json:"chunksScrubbed,omitempty"`
	ErrorsRepaired     int64              `json:"errorsRepaired,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Pool",type=string,JSONPath=`.spec.pool`
// +kubebuilder:printcolumn:name="Cron",type=string,JSONPath=`.spec.cron`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// ScrubSchedule drives per-pool integrity scrubs.
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
