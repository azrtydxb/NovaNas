package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AlertCondition is the PromQL threshold predicate the policy evaluates.
type AlertCondition struct {
	// Query is the PromQL expression whose scalar value is compared.
	// +kubebuilder:validation:MinLength=1
	Query string `json:"query"`
	// +kubebuilder:validation:Enum=">";"<";">=";"<=";"==";"!="
	Operator string `json:"operator"`
	// Threshold value compared against the query result.
	Threshold float64 `json:"threshold"`
	// For duration the predicate must hold before the alert fires
	// (Prometheus `for`). Example: "5m".
	// +optional
	For string `json:"for,omitempty"`
}

// AlertPolicySpec defines the desired state of AlertPolicy.
type AlertPolicySpec struct {
	// +optional
	Description string `json:"description,omitempty"`
	// +kubebuilder:validation:Enum=info;warning;critical
	Severity  string         `json:"severity"`
	Condition AlertCondition `json:"condition"`
	// Channels references AlertChannel names that should receive the alert.
	// +kubebuilder:validation:MinItems=1
	Channels []string `json:"channels"`
	// Extra Prometheus labels to stamp on the generated rule.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Extra annotations on the generated rule (e.g. runbook URL).
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// Suspended disables rule projection without deleting the CR.
	// +optional
	Suspended bool `json:"suspended,omitempty"`
}

// AlertPolicyStatus defines observed state of AlertPolicy.
type AlertPolicyStatus struct {
	// Phase is one of Pending, Active, Firing, Suspended, Failed.
	Phase string `json:"phase,omitempty"`
	// FiringSince is set the first time the controller observes the policy
	// firing; it is cleared again when the alert resolves.
	FiringSince *metav1.Time `json:"firingSince,omitempty"`
	// LastFired is the most recent time the alert transitioned to firing.
	LastFired *metav1.Time `json:"lastFired,omitempty"`
	// FireCount is the monotonic number of times the policy has fired.
	FireCount int64 `json:"fireCount,omitempty"`
	// RuleHash is a content hash of the last rendered Prometheus rule.
	RuleHash string `json:"ruleHash,omitempty"`
	// ObservedGeneration reflects the latest generation the controller saw.
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// AlertPolicy — PromQL threshold mapped to channels.
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
