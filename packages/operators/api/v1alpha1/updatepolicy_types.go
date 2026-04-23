package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// UpdateMaintenanceWindow defines when updates are allowed to apply.
type UpdateMaintenanceWindow struct {
	// Cron is a 5-field cron expression that matches the start of the window.
	// +kubebuilder:validation:MinLength=1
	Cron string `json:"cron"`
	// DurationMinutes is how long the window stays open.
	// +kubebuilder:validation:Minimum=1
	DurationMinutes int32 `json:"durationMinutes"`
}

// UpdatePolicySpec defines the desired state of UpdatePolicy.
type UpdatePolicySpec struct {
	// +kubebuilder:validation:Enum=stable;beta;edge;manual
	Channel           string                   `json:"channel"`
	AutoUpdate        bool                     `json:"autoUpdate,omitempty"`
	AutoReboot        bool                     `json:"autoReboot,omitempty"`
	MaintenanceWindow *UpdateMaintenanceWindow `json:"maintenanceWindow,omitempty"`
	SkipVersions      []string                 `json:"skipVersions,omitempty"`
}

// UpdatePolicyStatus defines observed state of UpdatePolicy.
type UpdatePolicyStatus struct {
	// +kubebuilder:validation:Enum=Idle;Checking;Downloading;Installing;PendingReboot;Failed
	Phase            string             `json:"phase,omitempty"`
	CurrentVersion   string             `json:"currentVersion,omitempty"`
	AvailableVersion string             `json:"availableVersion,omitempty"`
	LastCheck        *metav1.Time       `json:"lastCheck,omitempty"`
	Conditions       []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Channel",type=string,JSONPath=`.spec.channel`
// +kubebuilder:printcolumn:name="Current",type=string,JSONPath=`.status.currentVersion`
// +kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.status.availableVersion`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// UpdatePolicy — RAUC A/B update channel, cadence, and maintenance windows.
type UpdatePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              UpdatePolicySpec   `json:"spec,omitempty"`
	Status            UpdatePolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UpdatePolicyList contains a list of UpdatePolicy.
type UpdatePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UpdatePolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&UpdatePolicy{}, &UpdatePolicyList{}) }
