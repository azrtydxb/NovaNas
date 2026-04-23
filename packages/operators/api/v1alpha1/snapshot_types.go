package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// VolumeSourceRef is a discriminated reference to a snapshottable source.
// Kind must be one of BlockVolume, Dataset, Bucket, AppInstance, Vm.
type VolumeSourceRef struct {
	// +kubebuilder:validation:Enum=BlockVolume;Dataset;Bucket;AppInstance;Vm
	Kind string `json:"kind"`
	// +kubebuilder:validation:MinLength=1
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// SnapshotSpec defines the desired state of Snapshot.
type SnapshotSpec struct {
	// Source is the volume-like object the snapshot is taken from.
	Source VolumeSourceRef `json:"source"`
	// Locked prevents deletion while true. When set, controllers refuse
	// to remove the finalizer unless spec.allowDataLoss=true.
	Locked bool `json:"locked,omitempty"`
	// RetainUntil is an RFC-3339 timestamp after which the snapshot is
	// eligible for automatic pruning by retention policy.
	RetainUntil *metav1.Time `json:"retainUntil,omitempty"`
	// Labels are free-form annotations propagated to the backend.
	Labels map[string]string `json:"labels,omitempty"`
	// AllowDataLoss bypasses Locked + data-present guards on deletion.
	AllowDataLoss bool `json:"allowDataLoss,omitempty"`
}

// SnapshotStatus defines observed state of Snapshot.
type SnapshotStatus struct {
	// Phase summarises the snapshot lifecycle.
	// +kubebuilder:validation:Enum=Pending;Ready;Failed;Deleted
	Phase              string             `json:"phase,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	// SizeBytes is the logical bytes captured at snapshot time.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	// CreatedAt is when the backend accepted the snapshot.
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`
	// ReadyAt is when the snapshot transitioned to Ready.
	ReadyAt *metav1.Time `json:"readyAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=snap,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.source.name`
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.spec.source.kind`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Size",type=integer,JSONPath=`.status.sizeBytes`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Snapshot is a point-in-time image of a BlockVolume, Dataset, or other
// snapshottable source.
type Snapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SnapshotSpec   `json:"spec,omitempty"`
	Status            SnapshotStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SnapshotList contains a list of Snapshot.
type SnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Snapshot `json:"items"`
}

func init() { SchemeBuilder.Register(&Snapshot{}, &SnapshotList{}) }
