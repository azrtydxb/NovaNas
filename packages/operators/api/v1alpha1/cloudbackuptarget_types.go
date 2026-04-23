package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CloudBackupProvider enumerates supported object-storage providers.
// +kubebuilder:validation:Enum=s3;b2;azure;gcs;swift
type CloudBackupProvider string

// CloudBackupEngine enumerates supported backup engines.
// +kubebuilder:validation:Enum=restic;borg;kopia
type CloudBackupEngine string

// CloudBackupTargetSpec defines desired state.
type CloudBackupTargetSpec struct {
	// +kubebuilder:validation:Required
	Provider CloudBackupProvider `json:"provider"`

	// Endpoint overrides the provider's default endpoint (S3-compat only).
	Endpoint string `json:"endpoint,omitempty"`

	// +kubebuilder:validation:Required
	Bucket string `json:"bucket"`

	Region string `json:"region,omitempty"`
	Prefix string `json:"prefix,omitempty"`

	// CredentialsSecret is a Secret containing provider credentials.
	// For S3/B2: AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY.
	// For Azure: AZURE_ACCOUNT_NAME / AZURE_ACCOUNT_KEY.
	// +kubebuilder:validation:Required
	CredentialsSecret SecretKeyRef `json:"credentialsSecret"`

	// RepositoryPasswordSecret is the encryption password secret for
	// restic/borg/kopia.
	RepositoryPasswordSecret *SecretKeyRef `json:"repositoryPasswordSecret,omitempty"`

	Engine CloudBackupEngine `json:"engine,omitempty"`

	// StorageClass is the provider storage class, e.g. "STANDARD_IA".
	StorageClass string `json:"storageClass,omitempty"`

	// ReachabilityProbeIntervalSeconds controls how often the controller
	// probes the target. Default 300 when unset.
	ReachabilityProbeIntervalSeconds int32 `json:"reachabilityProbeIntervalSeconds,omitempty"`
}

// CloudBackupCapability lists detected provider capabilities.
type CloudBackupCapability struct {
	Versioning       bool `json:"versioning,omitempty"`
	ObjectLock       bool `json:"objectLock,omitempty"`
	ServerSideCrypt  bool `json:"serverSideEncryption,omitempty"`
	MultipartUpload  bool `json:"multipartUpload,omitempty"`
	LifecyclePolicy  bool `json:"lifecyclePolicy,omitempty"`
}

// CloudBackupTargetStatus reports observed state.
type CloudBackupTargetStatus struct {
	// +kubebuilder:validation:Enum=Pending;Probing;Ready;Unreachable;Failed
	Phase string `json:"phase,omitempty"`

	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Reachable reflects the last probe outcome.
	Reachable bool `json:"reachable,omitempty"`

	// RepositoryInitialized is true once the engine has initialised the
	// remote repository (e.g. `restic init`).
	RepositoryInitialized bool `json:"repositoryInitialized,omitempty"`

	// LastProbeAt is the last probe timestamp.
	LastProbeAt *metav1.Time `json:"lastProbeAt,omitempty"`

	// LastProbeError is the most recent probe error, if any.
	LastProbeError string `json:"lastProbeError,omitempty"`

	// Capabilities is the detected capability set cached at probe time.
	Capabilities *CloudBackupCapability `json:"capabilities,omitempty"`

	// ResolvedEndpoint is the fully resolved endpoint used at probe time.
	ResolvedEndpoint string `json:"resolvedEndpoint,omitempty"`

	// UsedBytes is an informational size of all objects under Prefix.
	UsedBytes int64 `json:"usedBytes,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=".spec.provider"
// +kubebuilder:printcolumn:name="Bucket",type=string,JSONPath=".spec.bucket"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Reachable",type=boolean,JSONPath=".status.reachable"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// CloudBackupTarget is an S3/B2/Azure/GCS/Swift object-storage endpoint.
type CloudBackupTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CloudBackupTargetSpec   `json:"spec,omitempty"`
	Status            CloudBackupTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudBackupTargetList contains a list of CloudBackupTarget.
type CloudBackupTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudBackupTarget `json:"items"`
}

func init() { SchemeBuilder.Register(&CloudBackupTarget{}, &CloudBackupTargetList{}) }
