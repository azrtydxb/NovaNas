package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SystemSmtpSettings configures outbound notification email.
type SystemSmtpSettings struct {
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
	// +kubebuilder:validation:Enum=none;starttls;tls
	Encryption string     `json:"encryption,omitempty"`
	From       string     `json:"from"`
	AuthSecret *SecretRef `json:"authSecret,omitempty"`
}

// SystemNtpSettings configures the host clock source.
type SystemNtpSettings struct {
	Servers []string `json:"servers"`
	Enabled *bool    `json:"enabled,omitempty"`
}

// SystemSettingsSpec defines the desired state of SystemSettings.
type SystemSettingsSpec struct {
	Hostname       string              `json:"hostname,omitempty"`
	Timezone       string              `json:"timezone,omitempty"`
	Locale         string              `json:"locale,omitempty"`
	NTP            *SystemNtpSettings  `json:"ntp,omitempty"`
	SMTP           *SystemSmtpSettings `json:"smtp,omitempty"`
	MOTD           string              `json:"motd,omitempty"`
	SupportContact string              `json:"supportContact,omitempty"`
}

// SystemSettingsStatus defines observed state of SystemSettings.
type SystemSettingsStatus struct {
	// +kubebuilder:validation:Enum=Applied;Failed
	Phase      string             `json:"phase,omitempty"`
	AppliedAt  *metav1.Time       `json:"appliedAt,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Hostname",type=string,JSONPath=`.spec.hostname`
// +kubebuilder:printcolumn:name="Timezone",type=string,JSONPath=`.spec.timezone`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// SystemSettings — Hostname, timezone, NTP, locale, SMTP cluster knobs.
type SystemSettings struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SystemSettingsSpec   `json:"spec,omitempty"`
	Status            SystemSettingsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SystemSettingsList contains a list of SystemSettings.
type SystemSettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SystemSettings `json:"items"`
}

func init() { SchemeBuilder.Register(&SystemSettings{}, &SystemSettingsList{}) }
