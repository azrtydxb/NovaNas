package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SLOIndicator expresses a SLI as two PromQL queries.
type SLOIndicator struct {
	// GoodQuery counts events meeting the SLO.
	// +kubebuilder:validation:Required
	GoodQuery string `json:"goodQuery"`

	// TotalQuery counts all SLI-eligible events.
	// +kubebuilder:validation:Required
	TotalQuery string `json:"totalQuery"`
}

// SLOBurnRateAlert configures a multi-window burn-rate alert.
type SLOBurnRateAlert struct {
	// ShortWindow is the fast-burn window, e.g. "5m".
	ShortWindow string `json:"shortWindow,omitempty"`
	// LongWindow is the slow-burn window, e.g. "1h".
	LongWindow string `json:"longWindow,omitempty"`
	// Threshold is the multiple of the error budget consumption rate
	// that triggers the alert (default 14.4 for 2% budget/hour).
	Threshold string `json:"threshold,omitempty"`
	// Severity maps to an AlertSeverity.
	// +kubebuilder:validation:Enum=info;warning;critical
	Severity string `json:"severity,omitempty"`
}

// ServiceLevelObjectiveSpec defines desired state.
type ServiceLevelObjectiveSpec struct {
	Description string `json:"description,omitempty"`

	// Target is the SLO target percentage as a decimal string (0..100,
	// e.g. "99.9"). Stored as string to preserve precision across
	// JSON round-trips; the controller parses it to a float64.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^(100(\.0+)?|[0-9]{1,2}(\.[0-9]+)?)$`
	Target string `json:"target"`

	// Window is the rolling evaluation window, e.g. "30d".
	// +kubebuilder:validation:Required
	Window string `json:"window"`

	// +kubebuilder:validation:Required
	Indicator SLOIndicator `json:"indicator"`

	// PrometheusURL is the base URL of the Prometheus HTTP API used to
	// query the current SLI. Defaults to
	// http://prometheus.novanas-system:9090 when empty.
	PrometheusURL string `json:"prometheusURL,omitempty"`

	// BurnRateAlerts configures per-window burn-rate alerts projected
	// into PrometheusRule.
	BurnRateAlerts []SLOBurnRateAlert `json:"burnRateAlerts,omitempty"`

	// AlertChannels is the channel set notified on burn-rate alerts.
	AlertChannels []string `json:"alertChannels,omitempty"`

	// EvalIntervalSeconds controls how often the controller polls
	// Prometheus for the current SLI. Default 60 when unset.
	EvalIntervalSeconds int32 `json:"evalIntervalSeconds,omitempty"`
}

// ServiceLevelObjectiveStatus reports observed state.
type ServiceLevelObjectiveStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;AtRisk;Breached;Failed
	Phase string `json:"phase,omitempty"`

	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// CurrentSLI is the most recent computed SLI as a percentage string.
	CurrentSLI string `json:"currentSLI,omitempty"`

	// BurnRate is the current normalised error-budget burn rate.
	BurnRate string `json:"burnRate,omitempty"`

	// ErrorBudgetRemainingSeconds is the remaining budget expressed as
	// "unavailability time" within Window (lower is worse).
	ErrorBudgetRemainingSeconds int64 `json:"errorBudgetRemainingSeconds,omitempty"`

	// ErrorBudgetRemainingPercent is the remaining budget as a percentage.
	ErrorBudgetRemainingPercent string `json:"errorBudgetRemainingPercent,omitempty"`

	// LastEvaluation is the time of the last Prometheus query.
	LastEvaluation *metav1.Time `json:"lastEvaluation,omitempty"`

	// LastError is the most recent evaluation error, if any.
	LastError string `json:"lastError,omitempty"`

	// RenderedRuleName is the child PrometheusRule name.
	RenderedRuleName string `json:"renderedRuleName,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=".spec.target"
// +kubebuilder:printcolumn:name="Window",type=string,JSONPath=".spec.window"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="SLI",type=string,JSONPath=".status.currentSLI"
// +kubebuilder:printcolumn:name="BudgetLeft",type=string,JSONPath=".status.errorBudgetRemainingPercent"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// ServiceLevelObjective describes an SLI/SLO with burn-rate alerting.
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
