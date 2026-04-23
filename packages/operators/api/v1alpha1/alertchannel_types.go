package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SecretKeyRef points at a key inside a Kubernetes Secret. It mirrors
// the SecretReference union from packages/schemas/src/common/references.ts
// (classic variant). The OpenBao variant is represented by SecretURI.
type SecretKeyRef struct {
	// Name is the secret's metadata.name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Key is the map key within the secret. Defaults to "token" when empty.
	// +optional
	Key string `json:"key,omitempty"`
	// Namespace of the secret; defaults to the CR's namespace when empty.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// URI is an alternate addressing form (openbao://path or vault://path).
	// Mutually exclusive with Name.
	// +optional
	URI string `json:"uri,omitempty"`
}

// EmailChannel holds SMTP recipients for an email alert channel.
type EmailChannel struct {
	// +kubebuilder:validation:MinItems=1
	To   []string `json:"to"`
	From string   `json:"from,omitempty"`
}

// WebhookChannel is a generic outgoing webhook.
type WebhookChannel struct {
	// +kubebuilder:validation:Pattern=`^https?://.+`
	URL     string            `json:"url"`
	Secret  *SecretKeyRef     `json:"secret,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// SlackChannel is a Slack incoming-webhook destination.
type SlackChannel struct {
	// +kubebuilder:validation:Pattern=`^https://hooks\.slack\.com/.+`
	WebhookURL string `json:"webhookURL"`
	Channel    string `json:"channel,omitempty"`
	Username   string `json:"username,omitempty"`
}

// PagerDutyChannel is a PagerDuty Events v2 integration.
type PagerDutyChannel struct {
	// RoutingKeySecret references the Events v2 integration key.
	RoutingKeySecret SecretKeyRef `json:"routingKeySecret"`
	// Severity maps NovaNas severity to PagerDuty severity when set;
	// otherwise NovaNas severity passes through.
	// +optional
	Severity string `json:"severity,omitempty"`
}

// NtfyChannel is an ntfy.sh (or self-hosted) topic subscription.
type NtfyChannel struct {
	Server     string        `json:"server,omitempty"`
	Topic      string        `json:"topic"`
	AuthSecret *SecretKeyRef `json:"authSecret,omitempty"`
}

// PushoverChannel pushes to the Pushover service.
type PushoverChannel struct {
	UserKey SecretKeyRef `json:"userKey"`
	Token   SecretKeyRef `json:"token"`
}

// AlertChannelSpec defines the desired state of AlertChannel.
type AlertChannelSpec struct {
	// +kubebuilder:validation:Enum=email;webhook;ntfy;pushover;slack;discord;telegram;browserPush;pagerduty
	Type string `json:"type"`

	Email     *EmailChannel     `json:"email,omitempty"`
	Webhook   *WebhookChannel   `json:"webhook,omitempty"`
	Slack     *SlackChannel     `json:"slack,omitempty"`
	PagerDuty *PagerDutyChannel `json:"pagerduty,omitempty"`
	Ntfy      *NtfyChannel      `json:"ntfy,omitempty"`
	Pushover  *PushoverChannel  `json:"pushover,omitempty"`

	// +kubebuilder:validation:Enum=info;warning;critical
	// +optional
	MinSeverity string `json:"minSeverity,omitempty"`
}

// AlertChannelStatus defines observed state of AlertChannel.
type AlertChannelStatus struct {
	// Phase is one of Pending, Active, Degraded, Failed.
	Phase string `json:"phase,omitempty"`
	// LastDeliveryAt is the RFC3339 timestamp of the most recent
	// successful probe or alert delivery.
	LastDeliveryAt *metav1.Time `json:"lastDeliveryAt,omitempty"`
	// LastProbeAt is when the controller last tried to probe the endpoint.
	LastProbeAt *metav1.Time `json:"lastProbeAt,omitempty"`
	// LastProbeError is the error from the last failed probe, if any.
	LastProbeError string `json:"lastProbeError,omitempty"`
	// ConsecutiveFailures is a rolling health counter; it resets on success.
	ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`
	// ObservedGeneration reflects the latest generation the controller saw.
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// AlertChannel — notification destination (email / webhook / Slack / PagerDuty / ntfy / pushover).
type AlertChannel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AlertChannelSpec   `json:"spec,omitempty"`
	Status            AlertChannelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AlertChannelList contains a list of AlertChannel.
type AlertChannelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AlertChannel `json:"items"`
}

func init() { SchemeBuilder.Register(&AlertChannel{}, &AlertChannelList{}) }
