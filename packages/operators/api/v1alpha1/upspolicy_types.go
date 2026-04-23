package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// UpsThresholds encodes battery / runtime trigger points.
type UpsThresholds struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	BatteryPercent *int32 `json:"batteryPercent,omitempty"`
	// +kubebuilder:validation:Minimum=0
	RuntimeSeconds *int32 `json:"runtimeSeconds,omitempty"`
}

// UpsPolicySpec defines the desired state of UpsPolicy.
type UpsPolicySpec struct {
	// +kubebuilder:validation:Enum=nut;apcupsd
	Integration string `json:"integration"`
	Host        string `json:"host,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port       int32          `json:"port,omitempty"`
	DeviceName string         `json:"deviceName,omitempty"`
	AuthSecret *SecretRef     `json:"authSecret,omitempty"`
	Thresholds *UpsThresholds `json:"thresholds,omitempty"`
	// OnBattery is the ordered list of actions when utility power fails.
	OnBattery []string `json:"onBattery,omitempty"`
	// OnLowBattery triggers when BatteryPercent/RuntimeSeconds crosses the threshold.
	OnLowBattery []string `json:"onLowBattery,omitempty"`
}

// UpsPolicyStatus defines observed state of UpsPolicy.
type UpsPolicyStatus struct {
	// +kubebuilder:validation:Enum=Active;Disconnected;Failed
	Phase          string             `json:"phase,omitempty"`
	BatteryPercent string             `json:"batteryPercent,omitempty"`
	RuntimeSeconds int32              `json:"runtimeSeconds,omitempty"`
	OnBattery      bool               `json:"onBattery,omitempty"`
	Conditions     []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Integration",type=string,JSONPath=`.spec.integration`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="OnBattery",type=boolean,JSONPath=`.status.onBattery`

// UpsPolicy — NUT/apcupsd integration and shutdown orchestration.
type UpsPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              UpsPolicySpec   `json:"spec,omitempty"`
	Status            UpsPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UpsPolicyList contains a list of UpsPolicy.
type UpsPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UpsPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&UpsPolicy{}, &UpsPolicyList{}) }
