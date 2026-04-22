package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// IscsiTargetSpec defines the desired state of IscsiTarget.
type IscsiTargetSpec struct {
	BlockVolume string   `json:"blockVolume,omitempty"`
	Iqn         string   `json:"iqn,omitempty"`
	PortalIps   []string `json:"portalIps,omitempty"`
	Port        int32    `json:"port,omitempty"`
	Initiators  []string `json:"initiators,omitempty"`
}

// IscsiTargetStatus defines observed state.
type IscsiTargetStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// IscsiTarget is an iSCSI portal binding a BlockVolume.
type IscsiTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              IscsiTargetSpec   `json:"spec,omitempty"`
	Status            IscsiTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IscsiTargetList contains a list of IscsiTarget.
type IscsiTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IscsiTarget `json:"items"`
}

func init() { SchemeBuilder.Register(&IscsiTarget{}, &IscsiTargetList{}) }
