package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AlertSeverity is the severity tier of an AlertPolicy.
// +kubebuilder:validation:Enum=info;warning;critical
type AlertSeverity string

// AlertCondition is a PromQL threshold evaluation.
type AlertCondition struct {
	// Query is the PromQL expression evaluated by Prometheus.
	// +kubebuilder:validation:Required
	Query string `json:"query"`

	// Operator compares Query result vs Threshold.
	// +kubebuilder:validation:Enum=">";"<";">=";"<=";"==";"!="
	Operator string `json:"operator"`

	// Threshold is the numeric value Query is compared to.
	Threshold string `json:"threshold"`

	// For is the firing duration (e.g. "5m").
	For string `json:"for,omitempty"`
}

// AlertPolicySpec defines desired state.
type AlertPolicySpec struct {
	Description string         `json:"description,omitempty"`
	Severity    AlertSeverity  `json:"severity"`
	Condition   AlertCondition `json:"condition"`

	// Channels lists AlertChannel names to dispatch to.
	// +kubebuilder:validation:MinItems=1
	Channels []string `json:"channels"`

	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`

	// Suspended disables rule rendering / firing without deletion.
	Suspended bool `json:"suspended,omitempty"`

	// RuntimeNamespace overrides where the projected PrometheusRule is
	// written. Defaults to "novanas-system".
	RuntimeNamespace string `json:"runtimeNamespace,omitempty"`
}

// AlertPolicyStatus reports observed state.
type AlertPolicyStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Firing;Suspended;Failed
	Phase string `json:"phase,omitempty"`

	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// RuleHash is a SHA-256 of the rendered PrometheusRule spec; used to
	// short-circuit reconciliation when the spec is unchanged.
	RuleHash string `json:"ruleHash,omitempty"`

	// FiringSince is the first observed firing time for the current
	// incident. Cleared on resolve.
	FiringSince *metav1.Time `json:"firingSince,omitempty"`

	// LastFiredAt is the most recent firing timestamp observed.
	LastFiredAt *metav1.Time `json:"lastFiredAt,omitempty"`

	// FireCount is the total number of transitions into a firing state.
	FireCount int64 `json:"fireCount,omitempty"`

	// RenderedRuleName is the child PrometheusRule name.
	RenderedRuleName string `json:"renderedRuleName,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Severity",type=string,JSONPath=".spec.severity"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="FireCount",type=integer,JSONPath=".status.fireCount"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// AlertPolicy maps a PromQL-driven threshold to one or more channels.
type AlertPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AlertPolicySpec   `json:"spec,omitempty"`
	Status            AlertPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AlertPolicyList contains a list of AlertPolicy.
type AlertPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AlertPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&AlertPolicy{}, &AlertPolicyList{}) }
