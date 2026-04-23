package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AuditSinkLoki configures a Loki sink.
type AuditSinkLoki struct {
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`
	// +optional
	AuthSecret *SecretKeyReference `json:"authSecret,omitempty"`
}

// AuditSinkS3 configures an S3 object-store sink.
type AuditSinkS3 struct {
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`
	// +optional
	Prefix            string             `json:"prefix,omitempty"`
	CredentialsSecret SecretKeyReference `json:"credentialsSecret"`
}

// AuditSinkSyslog configures a syslog sink.
type AuditSinkSyslog struct {
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
	// +kubebuilder:validation:Enum=udp;tcp;tls
	Protocol string `json:"protocol"`
}

// AuditSinkFile configures an on-disk sink.
type AuditSinkFile struct {
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
}

// AuditSinkWebhook configures a generic HTTPS webhook sink.
type AuditSinkWebhook struct {
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`
	// +optional
	AuthSecret *SecretKeyReference `json:"authSecret,omitempty"`
}

// AuditSink is one configured destination for audit records. Exactly
// one of the backend-specific sub-structs must be set, matching Type.
type AuditSink struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:Enum=loki;s3;syslog;file;webhook
	Type string `json:"type"`
	// +optional
	Loki *AuditSinkLoki `json:"loki,omitempty"`
	// +optional
	S3 *AuditSinkS3 `json:"s3,omitempty"`
	// +optional
	Syslog *AuditSinkSyslog `json:"syslog,omitempty"`
	// +optional
	File *AuditSinkFile `json:"file,omitempty"`
	// +optional
	Webhook *AuditSinkWebhook `json:"webhook,omitempty"`
}

// AuditPolicySpec mirrors packages/schemas/src/ops/audit-policy.ts. The
// controller serialises the spec into a ConfigMap that the audit agent
// reads; touching the ConfigMap triggers an agent restart via rollout
// annotation.
type AuditPolicySpec struct {
	// Events is a list of audit event types to emit (e.g. "user.login",
	// "dataset.mount"). Empty means "all".
	// +optional
	Events []string `json:"events,omitempty"`

	// Severity is the minimum severity level recorded.
	// +kubebuilder:validation:Enum=info;warning;critical
	// +optional
	Severity string `json:"severity,omitempty"`

	// Sinks is the list of backends receiving records.
	// +kubebuilder:validation:MinItems=1
	Sinks []AuditSink `json:"sinks"`

	// Retention is a Go duration string (e.g. "720h") applied to
	// retention-capable sinks.
	// +optional
	Retention string `json:"retention,omitempty"`
}

// AuditPolicyStatus is the observed state of the audit pipeline.
type AuditPolicyStatus struct {
	// +kubebuilder:validation:Enum=Active;Failed;Reconciling;Ready
	// +optional
	Phase string `json:"phase,omitempty"`

	// EventsEmitted is a best-effort counter of forwarded events.
	// +kubebuilder:validation:Minimum=0
	// +optional
	EventsEmitted int64 `json:"eventsEmitted,omitempty"`

	// ConfigMapRef names the ConfigMap the controller wrote.
	// +optional
	ConfigMapRef string `json:"configMapRef,omitempty"`

	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// AuditPolicy — What to audit and where to send it.
type AuditPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AuditPolicySpec   `json:"spec,omitempty"`
	Status            AuditPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AuditPolicyList contains a list of AuditPolicy.
type AuditPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuditPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&AuditPolicy{}, &AuditPolicyList{}) }
