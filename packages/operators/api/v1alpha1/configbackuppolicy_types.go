package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ConfigBackupS3 captures an external S3 destination.
type ConfigBackupS3 struct {
	Endpoint          string     `json:"endpoint,omitempty"`
	Bucket            string     `json:"bucket"`
	Region            string     `json:"region,omitempty"`
	Prefix            string     `json:"prefix,omitempty"`
	CredentialsSecret *SecretRef `json:"credentialsSecret,omitempty"`
}

// ConfigBackupDestination is a single backup target.
type ConfigBackupDestination struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:Enum=bucket;s3;cloudBackupTarget;localPath
	Type              string          `json:"type"`
	Bucket            string          `json:"bucket,omitempty"`
	CloudBackupTarget string          `json:"cloudBackupTarget,omitempty"`
	Path              string          `json:"path,omitempty"`
	S3                *ConfigBackupS3 `json:"s3,omitempty"`
}

// ConfigBackupIncludes selects which subsystems are exported.
type ConfigBackupIncludes struct {
	CRDs     bool `json:"crds,omitempty"`
	Keycloak bool `json:"keycloak,omitempty"`
	OpenBao  bool `json:"openbao,omitempty"`
	Postgres bool `json:"postgres,omitempty"`
}

// ConfigBackupEncryption toggles backup-level encryption.
type ConfigBackupEncryption struct {
	Enabled          bool       `json:"enabled"`
	PassphraseSecret *SecretRef `json:"passphraseSecret,omitempty"`
}

// ConfigBackupRetention bounds how many archives are kept.
type ConfigBackupRetention struct {
	// +kubebuilder:validation:Minimum=0
	KeepLast int32 `json:"keepLast"`
}

// ConfigBackupPolicySpec defines the desired state of ConfigBackupPolicy.
type ConfigBackupPolicySpec struct {
	// Cron is a standard 5-field cron expression.
	// +kubebuilder:validation:MinLength=1
	Cron         string                    `json:"cron"`
	Destinations []ConfigBackupDestination `json:"destinations"`
	Include      *ConfigBackupIncludes     `json:"include,omitempty"`
	Encryption   *ConfigBackupEncryption   `json:"encryption,omitempty"`
	Retention    *ConfigBackupRetention    `json:"retention,omitempty"`
}

// ConfigBackupPolicyStatus defines observed state of ConfigBackupPolicy.
type ConfigBackupPolicyStatus struct {
	// +kubebuilder:validation:Enum=Active;Running;Failed
	Phase       string             `json:"phase,omitempty"`
	LastRun     *metav1.Time       `json:"lastRun,omitempty"`
	NextRun     *metav1.Time       `json:"nextRun,omitempty"`
	LastArchive string             `json:"lastArchive,omitempty"`
	Conditions  []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas,shortName=cbp
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cron",type=string,JSONPath=`.spec.cron`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="LastRun",type=date,JSONPath=`.status.lastRun`

// ConfigBackupPolicy — scheduled CR/config snapshot policy.
type ConfigBackupPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ConfigBackupPolicySpec   `json:"spec,omitempty"`
	Status            ConfigBackupPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConfigBackupPolicyList contains a list of ConfigBackupPolicy.
type ConfigBackupPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigBackupPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&ConfigBackupPolicy{}, &ConfigBackupPolicyList{}) }
