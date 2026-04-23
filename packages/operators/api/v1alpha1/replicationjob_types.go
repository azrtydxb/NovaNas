package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ReplicationJobSpec defines the desired state of ReplicationJob.
type ReplicationJobSpec struct {
	// Source volume-like object.
	Source VolumeSourceRef `json:"source"`
	// Target is the name of a ReplicationTarget CR.
	// +kubebuilder:validation:MinLength=1
	Target string `json:"target"`
	// Direction selects push (send local snapshots) or pull.
	// +kubebuilder:validation:Enum=push;pull
	Direction string `json:"direction"`
	// Cron schedules incremental jobs. Omit for one-shot.
	Cron      string           `json:"cron,omitempty"`
	Retention *RetentionPolicy `json:"retention,omitempty"`
	// RemoteName is the volume/dataset name on the remote side.
	RemoteName string `json:"remoteName,omitempty"`
	Suspended  bool   `json:"suspended,omitempty"`
}

// ReplicationJobStatus defines observed state of ReplicationJob.
type ReplicationJobStatus struct {
	// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Suspended;Cancelled
	Phase              string             `json:"phase,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	LastRun            *metav1.Time       `json:"lastRun,omitempty"`
	NextRun            *metav1.Time       `json:"nextRun,omitempty"`
	BytesTransferred   int64              `json:"bytesTransferred,omitempty"`
	LastSnapshot       string             `json:"lastSnapshot,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.source.name`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.target`
// +kubebuilder:printcolumn:name="Direction",type=string,JSONPath=`.spec.direction`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// ReplicationJob drives snapshot-diff replication between pools/clusters.
type ReplicationJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ReplicationJobSpec   `json:"spec,omitempty"`
	Status            ReplicationJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ReplicationJobList contains a list of ReplicationJob.
type ReplicationJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReplicationJob `json:"items"`
}

func init() { SchemeBuilder.Register(&ReplicationJob{}, &ReplicationJobList{}) }
