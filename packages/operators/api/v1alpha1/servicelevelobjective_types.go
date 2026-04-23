package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SloIndicator is a ratio SLI built from two PromQL range vectors:
// goodQuery / totalQuery over the rolling window.
type SloIndicator struct {
	// +kubebuilder:validation:MinLength=1
	GoodQuery string `json:"goodQuery"`
	// +kubebuilder:validation:MinLength=1
	TotalQuery string `json:"totalQuery"`
}

// ServiceLevelObjectiveSpec defines the desired state of ServiceLevelObjective.
type ServiceLevelObjectiveSpec struct {
	// +optional
	Description string `json:"description,omitempty"`
	// Target is the SLO percentage (0..100 inclusive).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Target float64 `json:"target"`
	// Window is the rolling evaluation window (e.g. "30d", "7d").
	// +kubebuilder:validation:Pattern=`^\d+(\.\d+)?(ns|us|ms|s|m|h|d|w)$`
	Window    string       `json:"window"`
	Indicator SloIndicator `json:"indicator"`
	// AlertOnBurnRate enables multi-window burn-rate projection.
	// +optional
	AlertOnBurnRate bool `json:"alertOnBurnRate,omitempty"`
	// AlertChannels are the names of AlertChannels to notify on burn.
	// +optional
	AlertChannels []string `json:"alertChannels,omitempty"`
}

// ServiceLevelObjectiveStatus defines observed state of ServiceLevelObjective.
type ServiceLevelObjectiveStatus struct {
	// Phase is one of Pending, Active, Failed.
	Phase string `json:"phase,omitempty"`
	// CurrentSLI is the measured success ratio over the window (0..1).
	CurrentSLI *float64 `json:"currentSLI,omitempty"`
	// CurrentObjective is the SLO target expressed as a ratio (target/100).
	CurrentObjective *float64 `json:"currentObjective,omitempty"`
	// ErrorBudgetRemaining is the fraction of the error budget still
	// unconsumed; negative values indicate budget exhaustion.
	ErrorBudgetRemaining *float64 `json:"errorBudgetRemaining,omitempty"`
	// ErrorBudgetRemainingSeconds is the remaining seconds of error
	// budget left in the current window (budget * window_seconds).
	ErrorBudgetRemainingSeconds *int64 `json:"errorBudgetRemainingSeconds,omitempty"`
	// BurnRate is the short-window burn rate (5m by convention).
	BurnRate *float64 `json:"burnRate,omitempty"`
	// LastEvaluatedAt is the last successful SLI computation time.
	LastEvaluatedAt *metav1.Time `json:"lastEvaluatedAt,omitempty"`
	// ObservedGeneration reflects the latest generation the controller saw.
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// ServiceLevelObjective — ratio SLI + error-budget target.
type ServiceLevelObjective struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ServiceLevelObjectiveSpec   `json:"spec,omitempty"`
	Status            ServiceLevelObjectiveStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceLevelObjectiveList contains a list of ServiceLevelObjective.
type ServiceLevelObjectiveList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceLevelObjective `json:"items"`
}

func init() { SchemeBuilder.Register(&ServiceLevelObjective{}, &ServiceLevelObjectiveList{}) }
