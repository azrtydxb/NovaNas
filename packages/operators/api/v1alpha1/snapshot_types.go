package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SnapshotSpec defines the desired state of Snapshot.
type SnapshotSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for Snapshot.
}

// SnapshotStatus defines observed state of Snapshot.
type SnapshotStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// Snapshot — Point-in-time snapshot
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
