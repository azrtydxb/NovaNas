package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// TrafficPolicyScopeKind is the kind of the target the policy applies to.
// +kubebuilder:validation:Enum=HostInterface;Namespace;App;Vm;ReplicationJob;ObjectStore
type TrafficPolicyScopeKind string

// TrafficPolicyScope names the target that will be rate-limited.
type TrafficPolicyScope struct {
	Kind TrafficPolicyScopeKind `json:"kind"`
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// TrafficBandwidth is a bandwidth limit (e.g. "100Mbps", "1.5Gbps"). Stored
// as a free-form string; validated at admission time.
type TrafficBandwidth string

// TrafficDirectionLimit is the per-direction limit block.
type TrafficDirectionLimit struct {
	// +optional
	Max TrafficBandwidth `json:"max,omitempty"`
	// +optional
	Burst TrafficBandwidth `json:"burst,omitempty"`
}

// TrafficLimits groups egress/ingress caps.
type TrafficLimits struct {
	// +optional
	Egress *TrafficDirectionLimit `json:"egress,omitempty"`
	// +optional
	Ingress *TrafficDirectionLimit `json:"ingress,omitempty"`
}

// TrafficSchedulingWindow overrides limits on a cron window.
type TrafficSchedulingWindow struct {
	// Cron is the crontab-style spec for when this window is active.
	// +kubebuilder:validation:MinLength=1
	Cron string `json:"cron"`
	// +kubebuilder:validation:Minimum=1
	DurationMinutes int32 `json:"durationMinutes"`
	// +optional
	OverrideEgress *TrafficDirectionLimit `json:"overrideEgress,omitempty"`
	// +optional
	OverrideIngress *TrafficDirectionLimit `json:"overrideIngress,omitempty"`
}

// TrafficPolicySpec defines the desired state of TrafficPolicy.
type TrafficPolicySpec struct {
	Scope TrafficPolicyScope `json:"scope"`
	// +optional
	Limits *TrafficLimits `json:"limits,omitempty"`
	// Scheduling is a map of named windows -> window spec.
	// +optional
	Scheduling map[string]TrafficSchedulingWindow `json:"scheduling,omitempty"`
	// Priority orders overlapping policies; lower runs first.
	// +optional
	Priority int32 `json:"priority,omitempty"`
}

// TrafficPolicyStatus defines observed state of TrafficPolicy.
type TrafficPolicyStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed;Reconciling;Ready
	// +optional
	Phase string `json:"phase,omitempty"`
	// AppliedAt is the timestamp of successful tc/eBPF installation.
	// +optional
	AppliedAt *metav1.Time `json:"appliedAt,omitempty"`
	// AppliedConfigHash is the sha256 of the rendered limiter config.
	// +optional
	AppliedConfigHash string `json:"appliedConfigHash,omitempty"`
	// ObservedGeneration tracks the generation of the last reconcile.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Scope",type="string",JSONPath=".spec.scope.kind"
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.scope.name"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

// TrafficPolicy caps or shapes traffic for a named target.
type TrafficPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TrafficPolicySpec   `json:"spec,omitempty"`
	Status            TrafficPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TrafficPolicyList contains a list of TrafficPolicy.
type TrafficPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TrafficPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&TrafficPolicy{}, &TrafficPolicyList{}) }
