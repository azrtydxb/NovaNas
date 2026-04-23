package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CloudBackupJobSpec defines the desired state of CloudBackupJob.
type CloudBackupJobSpec struct {
	Source VolumeSourceRef `json:"source"`
	// Target is the name of a CloudBackupTarget CR.
	// +kubebuilder:validation:MinLength=1
	Target string `json:"target"`
	// Cron is a five-field POSIX cron expression. Empty means on-demand.
	// +optional
	Cron string `json:"cron,omitempty"`
	// +optional
	Retention *RetentionPolicy `json:"retention,omitempty"`
	// +optional
	Excludes []string `json:"excludes,omitempty"`
	// +optional
	Suspended bool `json:"suspended,omitempty"`
	// MaxRetries is the max number of consecutive failed runs before
	// the controller marks the job Failed. Defaults to 3.
	// +optional
	MaxRetries int32 `json:"maxRetries,omitempty"`
}

// CloudBackupJobStatus defines observed state of CloudBackupJob.
type CloudBackupJobStatus struct {
	// Phase is one of Pending, Running, Succeeded, Failed, Suspended.
	Phase string `json:"phase,omitempty"`
	// LastRun is the start time of the most recent run.
	LastRun *metav1.Time `json:"lastRun,omitempty"`
	// NextRun is the next scheduled run (cron).
	NextRun *metav1.Time `json:"nextRun,omitempty"`
	// LastSuccessfulRun tracks the most recent Succeeded run for dedup.
	LastSuccessfulRun *metav1.Time `json:"lastSuccessfulRun,omitempty"`
	// LastSnapshotID is the source snapshot the last successful run used.
	LastSnapshotID string `json:"lastSnapshotID,omitempty"`
	// SnapshotID is the source snapshot the current/last run is streaming.
	SnapshotID string `json:"snapshotID,omitempty"`
	// BytesTotal is the expected payload size of the current run.
	BytesTotal int64 `json:"bytesTotal,omitempty"`
	// BytesTransferred is progress for the current run (monotonic until reset).
	BytesTransferred int64 `json:"bytesTransferred,omitempty"`
	// BytesUploaded is the cumulative bytes uploaded across all runs.
	BytesUploaded int64 `json:"bytesUploaded,omitempty"`
	// Progress is 0..100 for the current run.
	Progress int32 `json:"progress,omitempty"`
	// ConsecutiveFailures is reset on each success.
	ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`
	// Message is a human-readable status message.
	Message string `json:"message,omitempty"`
	// ObservedGeneration reflects the latest generation the controller saw.
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// CloudBackupJob — scheduled volume-to-cloud backup job.
type CloudBackupJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CloudBackupJobSpec   `json:"spec,omitempty"`
	Status            CloudBackupJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudBackupJobList contains a list of CloudBackupJob.
type CloudBackupJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudBackupJob `json:"items"`
}

func init() { SchemeBuilder.Register(&CloudBackupJob{}, &CloudBackupJobList{}) }
