package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DiskState is the lifecycle state of a physical disk.
// +kubebuilder:validation:Enum=UNKNOWN;IDENTIFIED;ASSIGNED;ACTIVE;DEGRADED;FAILED;DRAINING;REMOVABLE;QUARANTINED;WIPED
type DiskState string

const (
	DiskStateUnknown     DiskState = "UNKNOWN"
	DiskStateIdentified  DiskState = "IDENTIFIED"
	DiskStateAssigned    DiskState = "ASSIGNED"
	DiskStateActive      DiskState = "ACTIVE"
	DiskStateDegraded    DiskState = "DEGRADED"
	DiskStateFailed      DiskState = "FAILED"
	DiskStateDraining    DiskState = "DRAINING"
	DiskStateRemovable   DiskState = "REMOVABLE"
	DiskStateQuarantined DiskState = "QUARANTINED"
	DiskStateWiped       DiskState = "WIPED"
)

// DiskRole is either data or spare.
// +kubebuilder:validation:Enum=data;spare
type DiskRole string

// DiskSpec defines the desired state of Disk.
type DiskSpec struct {
	Pool string   `json:"pool,omitempty"`
	Role DiskRole `json:"role,omitempty"`
}

// SmartInfo captures SMART health readings.
type SmartInfo struct {
	// +kubebuilder:validation:Enum=OK;WARN;WARNING;FAIL;FAILED
	OverallHealth      string `json:"overallHealth,omitempty"`
	Temperature        int32  `json:"temperature,omitempty"`
	PowerOnHours       int64  `json:"powerOnHours,omitempty"`
	ReallocatedSectors int64  `json:"reallocatedSectors,omitempty"`
	PendingSectors     int64  `json:"pendingSectors,omitempty"`
	LastShortTest      *metav1.Time `json:"lastShortTest,omitempty"`
	LastLongTest       *metav1.Time `json:"lastLongTest,omitempty"`
}

// DiskLifecycleEvent records a notable disk state transition.
type DiskLifecycleEvent struct {
	Timestamp metav1.Time `json:"timestamp"`
	Type      string      `json:"type"`
	Reason    string      `json:"reason,omitempty"`
	Message   string      `json:"message,omitempty"`
	FromState DiskState   `json:"fromState,omitempty"`
	ToState   DiskState   `json:"toState,omitempty"`
	Actor     string      `json:"actor,omitempty"`
}

// DiskStatus defines observed state of Disk.
type DiskStatus struct {
	State              DiskState            `json:"state,omitempty"`
	Slot               string               `json:"slot,omitempty"`
	Model              string               `json:"model,omitempty"`
	Serial             string               `json:"serial,omitempty"`
	Wwn                string               `json:"wwn,omitempty"`
	SizeBytes          int64                `json:"sizeBytes,omitempty"`
	// +kubebuilder:validation:Enum=nvme;ssd;hdd
	Class              string               `json:"class,omitempty"`
	// +kubebuilder:validation:Enum=nvme;ssd;hdd
	DeviceClass        string               `json:"deviceClass,omitempty"`
	Smart              *SmartInfo           `json:"smart,omitempty"`
	RecentEvents       []DiskLifecycleEvent `json:"recentEvents,omitempty"`
	Conditions         []metav1.Condition   `json:"conditions,omitempty"`
	ObservedGeneration int64                `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Pool",type=string,JSONPath=`.spec.pool`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`
// +kubebuilder:printcolumn:name="Size",type=integer,JSONPath=`.status.sizeBytes`
// +kubebuilder:printcolumn:name="Slot",type=string,JSONPath=`.status.slot`

// Disk represents a physical disk device with a lifecycle state machine.
type Disk struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DiskSpec   `json:"spec,omitempty"`
	Status            DiskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DiskList contains a list of Disk.
type DiskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Disk `json:"items"`
}

func init() { SchemeBuilder.Register(&Disk{}, &DiskList{}) }
