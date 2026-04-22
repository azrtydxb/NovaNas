package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// NfsServerSpec defines the desired state of NfsServer.
type NfsServerSpec struct {
	Interface string   `json:"interface,omitempty"`
	Replicas  int32    `json:"replicas,omitempty"`
	Versions  []string `json:"versions,omitempty"`
}

// NfsServerStatus defines observed state.
type NfsServerStatus struct {
	Phase         string             `json:"phase,omitempty"`
	Conditions    []metav1.Condition `json:"conditions,omitempty"`
	ReadyReplicas int32              `json:"readyReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// NfsServer is a knfsd operator config.
type NfsServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NfsServerSpec   `json:"spec,omitempty"`
	Status            NfsServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NfsServerList contains a list of NfsServer.
type NfsServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NfsServer `json:"items"`
}

func init() { SchemeBuilder.Register(&NfsServer{}, &NfsServerList{}) }
