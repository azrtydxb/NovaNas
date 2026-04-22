package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SmbServerSpec defines the desired state of SmbServer.
type SmbServerSpec struct {
	Interface  string `json:"interface,omitempty"`
	Workgroup  string `json:"workgroup,omitempty"`
	Replicas   int32  `json:"replicas,omitempty"`
	MinVersion string `json:"minVersion,omitempty"`
	MaxVersion string `json:"maxVersion,omitempty"`
}

// SmbServerStatus defines observed state.
type SmbServerStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	ReadyReplicas int32           `json:"readyReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// SmbServer is a Samba pod deployment config.
type SmbServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SmbServerSpec   `json:"spec,omitempty"`
	Status            SmbServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SmbServerList contains a list of SmbServer.
type SmbServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SmbServer `json:"items"`
}

func init() { SchemeBuilder.Register(&SmbServer{}, &SmbServerList{}) }
