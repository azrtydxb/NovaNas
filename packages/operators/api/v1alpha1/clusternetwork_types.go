package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ClusterNetworkSpec defines the desired state of ClusterNetwork.
type ClusterNetworkSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for ClusterNetwork.
}

// ClusterNetworkStatus defines observed state of ClusterNetwork.
type ClusterNetworkStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// ClusterNetwork — Pod/service CIDRs, overlay config
type ClusterNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterNetworkSpec   `json:"spec,omitempty"`
	Status            ClusterNetworkStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterNetworkList contains a list of ClusterNetwork.
type ClusterNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterNetwork `json:"items"`
}

func init() { SchemeBuilder.Register(&ClusterNetwork{}, &ClusterNetworkList{}) }
