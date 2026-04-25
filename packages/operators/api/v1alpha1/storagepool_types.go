package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StoragePoolSpec defines the desired state of StoragePool.
type StoragePoolSpec struct {
	// Tier is the pool's performance level. "1" is the fastest (NVMe,
	// hot data); "4" is the slowest (cold archive). Tiering policies
	// compare lexically — lower number = faster.
	// +kubebuilder:validation:Enum="1";"2";"3";"4"
	Tier string `json:"tier,omitempty"`
	// DeviceFilter constrains disk selection.
	DeviceFilter *DeviceFilter `json:"deviceFilter,omitempty"`
	// RecoveryRate controls rebuild bandwidth.
	// +kubebuilder:validation:Enum=slow;balanced;fast
	RecoveryRate string `json:"recoveryRate,omitempty"`
	// RebalanceOnAdd controls behaviour when new disks are added.
	// +kubebuilder:validation:Enum=auto;manual;never
	RebalanceOnAdd string `json:"rebalanceOnAdd,omitempty"`
	// Disks pins the member Disk CR names. Empty means filter-only.
	Disks []string `json:"disks,omitempty"`
	// AllowDataLoss permits deletion while BlockVolumes still reference the pool.
	AllowDataLoss bool `json:"allowDataLoss,omitempty"`
}

// DeviceFilter narrows device eligibility for a StoragePool.
type DeviceFilter struct {
	// +kubebuilder:validation:Enum=nvme;ssd;hdd
	PreferredClass string `json:"preferredClass,omitempty"`
	MinSize        string `json:"minSize,omitempty"`
	MaxSize        string `json:"maxSize,omitempty"`
}

// PoolCapacity reports observed pool-level capacity.
type PoolCapacity struct {
	TotalBytes     int64 `json:"totalBytes,omitempty"`
	UsedBytes      int64 `json:"usedBytes,omitempty"`
	AvailableBytes int64 `json:"availableBytes,omitempty"`
}

// StoragePoolStatus defines the observed state of StoragePool.
type StoragePoolStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Degraded;Failed;Ready
	Phase              string             `json:"phase,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	DiskCount          int32              `json:"diskCount,omitempty"`
	CapacityBytes      int64              `json:"capacityBytes,omitempty"`
	UsedBytes          int64              `json:"usedBytes,omitempty"`
	Capacity           *PoolCapacity      `json:"capacity,omitempty"`
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
