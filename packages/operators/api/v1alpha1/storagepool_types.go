package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StoragePoolSpec defines the desired state of StoragePool.
type StoragePoolSpec struct {
	// Tier classifies the pool (e.g., fast, warm, cold).
	Tier string `json:"tier,omitempty"`
	// DeviceFilter constrains disk selection.
	DeviceFilter *DeviceFilter `json:"deviceFilter,omitempty"`
	// RecoveryRate controls rebuild bandwidth (e.g., slow, balanced, fast).
	RecoveryRate string `json:"recoveryRate,omitempty"`
	// RebalanceOnAdd controls behaviour when new disks are added.
	RebalanceOnAdd string `json:"rebalanceOnAdd,omitempty"`
}

// DeviceFilter narrows device eligibility for a StoragePool.
type DeviceFilter struct {
	PreferredClass string `json:"preferredClass,omitempty"`
	MinSize        string `json:"minSize,omitempty"`
	MaxSize        string `json:"maxSize,omitempty"`
}

// StoragePoolStatus defines the observed state of StoragePool.
type StoragePoolStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	DiskCount  int32              `json:"diskCount,omitempty"`
	CapacityBytes int64           `json:"capacityBytes,omitempty"`
	UsedBytes  int64              `json:"usedBytes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=sp,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Tier",type=string,JSONPath=`.spec.tier`
// +kubebuilder:printcolumn:name="Disks",type=integer,JSONPath=`.status.diskCount`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// StoragePool is a bag of disks with a tier label.
type StoragePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StoragePoolSpec   `json:"spec,omitempty"`
	Status StoragePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// StoragePoolList contains a list of StoragePool.
type StoragePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StoragePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StoragePool{}, &StoragePoolList{})
}
