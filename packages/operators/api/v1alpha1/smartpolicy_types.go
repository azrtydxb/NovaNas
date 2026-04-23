package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SmartAppliesTo narrows which disks the policy monitors.
type SmartAppliesTo struct {
	All     bool     `json:"all,omitempty"`
	Pools   []string `json:"pools,omitempty"`
	Disks   []string `json:"disks,omitempty"`
	// +kubebuilder:validation:items:Enum=nvme;ssd;hdd
	Classes []string `json:"classes,omitempty"`
}

// SmartTest configures a scheduled self-test (short/long) by cron expression.
type SmartTest struct {
	// +kubebuilder:validation:MinLength=1
	Cron string `json:"cron"`
}

// SmartThreshold pairs warning/critical levels for a single attribute.
type SmartThreshold struct {
	Warning  string `json:"warning"`
	Critical string `json:"critical"`
}

// SmartThresholds bundles per-attribute warning/critical values.
type SmartThresholds struct {
	ReallocatedSectors *SmartThreshold `json:"reallocatedSectors,omitempty"`
	PendingSectors     *SmartThreshold `json:"pendingSectors,omitempty"`
	Temperature        *SmartThreshold `json:"temperature,omitempty"`
	PowerOnHours       *SmartThreshold `json:"powerOnHours,omitempty"`
}

// SmartActions ties threshold breaches to operator actions.
type SmartActions struct {
	// +kubebuilder:validation:Enum=alert;alertAndMarkDegraded;markDegraded;none
	OnWarning string `json:"onWarning,omitempty"`
	// +kubebuilder:validation:Enum=alert;alertAndMarkDegraded;markDegraded;none
	OnCritical string `json:"onCritical,omitempty"`
}

// SmartPolicySpec defines the desired state of SmartPolicy.
type SmartPolicySpec struct {
	AppliesTo  SmartAppliesTo   `json:"appliesTo"`
	ShortTest  *SmartTest       `json:"shortTest,omitempty"`
	LongTest   *SmartTest       `json:"longTest,omitempty"`
	Thresholds *SmartThresholds `json:"thresholds,omitempty"`
	Actions    *SmartActions    `json:"actions,omitempty"`
}

// SmartPolicyStatus defines observed state of SmartPolicy.
type SmartPolicyStatus struct {
	// +kubebuilder:validation:Enum=Active;Failed
	Phase      string             `json:"phase,omitempty"`
	DiskCount  int32              `json:"diskCount,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas,shortName=smartp
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Disks",type=integer,JSONPath=`.status.diskCount`

// SmartPolicy — SMART self-test cadence and threshold-driven disk actions.
type SmartPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SmartPolicySpec   `json:"spec,omitempty"`
	Status            SmartPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SmartPolicyList contains a list of SmartPolicy.
type SmartPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SmartPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&SmartPolicy{}, &SmartPolicyList{}) }
