package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// RetentionPolicy expresses GFS-style snapshot retention.
type RetentionPolicy struct {
	Hourly   int32 `json:"hourly,omitempty"`
	Daily    int32 `json:"daily,omitempty"`
	Weekly   int32 `json:"weekly,omitempty"`
	Monthly  int32 `json:"monthly,omitempty"`
	Yearly   int32 `json:"yearly,omitempty"`
	KeepLast int32 `json:"keepLast,omitempty"`
}

// SnapshotScheduleSpec defines the desired state of SnapshotSchedule.
type SnapshotScheduleSpec struct {
	// Source volume-like object to snapshot.
	Source VolumeSourceRef `json:"source"`
	// Cron is a 5 or 6-field cron expression.
	// +kubebuilder:validation:MinLength=1
	Cron      string           `json:"cron"`
	Retention *RetentionPolicy `json:"retention,omitempty"`
	// NamingFormat is a strftime-like template used for snapshot names.
	NamingFormat string `json:"namingFormat,omitempty"`
	// Locked prevents snapshots produced by this schedule from being
	// pruned by other retention flows.
	Locked bool `json:"locked,omitempty"`
	// Suspended halts new snapshot creation without deleting the schedule.
	Suspended bool `json:"suspended,omitempty"`
}

// SnapshotScheduleStatus defines observed state of SnapshotSchedule.
type SnapshotScheduleStatus struct {
	// +kubebuilder:validation:Enum=Active;Suspended;Failed;Scheduled
	Phase              string             `json:"phase,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	LastRun            *metav1.Time       `json:"lastRun,omitempty"`
	NextRun            *metav1.Time       `json:"nextRun,omitempty"`
	SnapshotsCreated   int64              `json:"snapshotsCreated,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=snapsched,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.source.name`
// +kubebuilder:printcolumn:name="Cron",type=string,JSONPath=`.spec.cron`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Next",type=string,JSONPath=`.status.nextRun`

// SnapshotSchedule creates Snapshot CRs on a cron cadence.
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
