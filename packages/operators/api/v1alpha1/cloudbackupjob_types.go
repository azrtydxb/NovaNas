package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// VolumeSourceRef refers to a source volume for a backup job.
type VolumeSourceRef struct {
	// Kind is one of "BlockVolume", "Dataset", "Snapshot".
	// +kubebuilder:validation:Enum=BlockVolume;Dataset;Snapshot
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Namespace of the source; defaults to the job's namespace.
	Namespace string `json:"namespace,omitempty"`
}

// CloudBackupRetention is a minimal restic-style retention policy.
type CloudBackupRetention struct {
	KeepLast    int32 `json:"keepLast,omitempty"`
	KeepHourly  int32 `json:"keepHourly,omitempty"`
	KeepDaily   int32 `json:"keepDaily,omitempty"`
	KeepWeekly  int32 `json:"keepWeekly,omitempty"`
	KeepMonthly int32 `json:"keepMonthly,omitempty"`
	KeepYearly  int32 `json:"keepYearly,omitempty"`
}

// CloudBackupJobSpec defines desired state.
type CloudBackupJobSpec struct {
	// +kubebuilder:validation:Required
	Source VolumeSourceRef `json:"source"`

	// Target is the name of a CloudBackupTarget in the same namespace.
	// +kubebuilder:validation:Required
	Target string `json:"target"`

	// Cron is an optional schedule; when empty the job is one-shot.
	Cron string `json:"cron,omitempty"`

	Retention *CloudBackupRetention `json:"retention,omitempty"`
	Excludes  []string              `json:"excludes,omitempty"`

	Suspended bool `json:"suspended,omitempty"`

	// Timeout bounds an individual run, e.g. "4h".
	Timeout string `json:"timeout,omitempty"`

	// Parallelism bounds concurrent upload streams (engine-specific).
	Parallelism int32 `json:"parallelism,omitempty"`
}

// CloudBackupJobStatus reports observed state.
type CloudBackupJobStatus struct {
	// +kubebuilder:validation:Enum=Pending;Scheduled;Running;Succeeded;Failed;Suspended;Cancelled
	Phase string `json:"phase,omitempty"`

	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastRun is the start time of the most recent run.
	LastRun *metav1.Time `json:"lastRun,omitempty"`

	// LastSuccessfulRun is the start time of the most recent successful run.
	LastSuccessfulRun *metav1.Time `json:"lastSuccessfulRun,omitempty"`

	// NextRun is the next scheduled invocation (cron jobs only).
	NextRun *metav1.Time `json:"nextRun,omitempty"`

	// BytesTransferred is the cumulative bytes uploaded in the last run.
	BytesTransferred int64 `json:"bytesTransferred,omitempty"`

	// BytesTotal is the expected total bytes for the current run.
	BytesTotal int64 `json:"bytesTotal,omitempty"`

	// ProgressPercent is 0..100 for the current run.
	ProgressPercent int32 `json:"progressPercent,omitempty"`

	// SnapshotID is the engine snapshot identifier produced by the last run.
	SnapshotID string `json:"snapshotId,omitempty"`

	// FilesProcessed is the count of files in the last run.
	FilesProcessed int64 `json:"filesProcessed,omitempty"`

	// FailureCount is the cumulative failure count.
	FailureCount int64 `json:"failureCount,omitempty"`

	// LastError is the most recent run error.
	LastError string `json:"lastError,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=".spec.target"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Progress",type=integer,JSONPath=".status.progressPercent"
// +kubebuilder:printcolumn:name="LastRun",type=date,JSONPath=".status.lastRun"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// CloudBackupJob is a volume-to-cloud backup job (one-shot or cron).
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
