package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AlertChannelType enumerates the supported delivery backends.
// +kubebuilder:validation:Enum=email;webhook;slack;pagerduty;ntfy;pushover;discord;telegram
type AlertChannelType string

// SecretKeyRef is a reference to a key within a Secret.
type SecretKeyRef struct {
	// Name of the Secret.
	Name string `json:"name"`
	// Namespace of the Secret. Defaults to the channel namespace or
	// "novanas-system" when empty.
	Namespace string `json:"namespace,omitempty"`
	// Key inside the Secret's data map.
	Key string `json:"key"`
}

// EmailChannelConfig configures SMTP delivery.
type EmailChannelConfig struct {
	To             []string      `json:"to"`
	From           string        `json:"from,omitempty"`
	SmtpServer     string        `json:"smtpServer,omitempty"`
	SmtpPort       int32         `json:"smtpPort,omitempty"`
	UsernameSecret *SecretKeyRef `json:"usernameSecret,omitempty"`
	PasswordSecret *SecretKeyRef `json:"passwordSecret,omitempty"`
	StartTLS       bool          `json:"startTls,omitempty"`
}

// WebhookChannelConfig configures a generic JSON webhook target.
type WebhookChannelConfig struct {
	URL          string            `json:"url"`
	Method       string            `json:"method,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	SecretRef    *SecretKeyRef     `json:"secretRef,omitempty"`
	TimeoutSecs  int32             `json:"timeoutSecs,omitempty"`
	InsecureSkip bool              `json:"insecureSkipTlsVerify,omitempty"`
}

// SlackChannelConfig wraps a Slack webhook URL secret.
type SlackChannelConfig struct {
	WebhookURLSecret SecretKeyRef `json:"webhookUrlSecret"`
	Channel          string       `json:"channel,omitempty"`
	Username         string       `json:"username,omitempty"`
	IconEmoji        string       `json:"iconEmoji,omitempty"`
}

// PagerDutyChannelConfig wraps the PD Events v2 routing key.
type PagerDutyChannelConfig struct {
	IntegrationKeySecret SecretKeyRef `json:"integrationKeySecret"`
	Severity             string       `json:"severity,omitempty"`
	Component            string       `json:"component,omitempty"`
}

// NtfyChannelConfig configures the ntfy.sh family of push destinations.
type NtfyChannelConfig struct {
	Server     string        `json:"server,omitempty"`
	Topic      string        `json:"topic"`
	AuthSecret *SecretKeyRef `json:"authSecret,omitempty"`
	Priority   string        `json:"priority,omitempty"`
}

// PushoverChannelConfig configures the Pushover API.
type PushoverChannelConfig struct {
	UserKeySecret SecretKeyRef `json:"userKeySecret"`
	TokenSecret   SecretKeyRef `json:"tokenSecret"`
	Device        string       `json:"device,omitempty"`
	Priority      int32        `json:"priority,omitempty"`
}

// AlertChannelSpec defines desired state.
type AlertChannelSpec struct {
	// +kubebuilder:validation:Required
	Type AlertChannelType `json:"type"`

	// MinSeverity gates which alerts are dispatched on this channel.
	// +kubebuilder:validation:Enum=info;warning;critical
	MinSeverity string `json:"minSeverity,omitempty"`

	Email     *EmailChannelConfig     `json:"email,omitempty"`
	Webhook   *WebhookChannelConfig   `json:"webhook,omitempty"`
	Slack     *SlackChannelConfig     `json:"slack,omitempty"`
	PagerDuty *PagerDutyChannelConfig `json:"pagerduty,omitempty"`
	Ntfy      *NtfyChannelConfig      `json:"ntfy,omitempty"`
	Pushover  *PushoverChannelConfig  `json:"pushover,omitempty"`

	// Suspended silences the channel without deleting it.
	Suspended bool `json:"suspended,omitempty"`

	// RateLimitPerMinute bounds delivery attempts; 0 means unlimited.
	RateLimitPerMinute int32 `json:"rateLimitPerMinute,omitempty"`
}

// AlertChannelStatus reports observed state.
type AlertChannelStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed;Suspended
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the .metadata.generation reconciled last.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastDeliveryAt is the time of the most recent delivery attempt.
	LastDeliveryAt *metav1.Time `json:"lastDeliveryAt,omitempty"`

	// LastSuccessfulDeliveryAt is the last delivery that succeeded.
	LastSuccessfulDeliveryAt *metav1.Time `json:"lastSuccessfulDeliveryAt,omitempty"`

	// DeliveryCount is the total lifetime count of successful deliveries.
	DeliveryCount int64 `json:"deliveryCount,omitempty"`

	// ConsecutiveFailures counts back-to-back delivery failures;
	// resets on first success.
	ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`

	// LastError is the most recent dispatch error message.
	LastError string `json:"lastError,omitempty"`

	// ResolvedSecretRef echoes the generated/normalised Secret reference
	// used by the downstream dispatcher.
	ResolvedSecretRef *corev1.LocalObjectReference `json:"resolvedSecretRef,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Failures",type=integer,JSONPath=".status.consecutiveFailures"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// AlertChannel is an email/webhook/slack/pagerduty/ntfy/pushover destination.
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
