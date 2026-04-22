package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ServiceLevelObjectiveSpec defines the desired state of ServiceLevelObjective.
type ServiceLevelObjectiveSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for ServiceLevelObjective.
}

// ServiceLevelObjectiveStatus defines observed state of ServiceLevelObjective.
type ServiceLevelObjectiveStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// ServiceLevelObjective — SLO config
type ServiceLevelObjective struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ServiceLevelObjectiveSpec   `json:"spec,omitempty"`
	Status            ServiceLevelObjectiveStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceLevelObjectiveList contains a list of ServiceLevelObjective.
type ServiceLevelObjectiveList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceLevelObjective `json:"items"`
}

func init() { SchemeBuilder.Register(&ServiceLevelObjective{}, &ServiceLevelObjectiveList{}) }
