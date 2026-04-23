package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CloudBackupTargetSpec defines the desired state of CloudBackupTarget.
type CloudBackupTargetSpec struct {
	// +kubebuilder:validation:Enum=s3;b2;azure;gcs;swift;webdav
	Provider string `json:"provider"`
	// Endpoint is the object-store endpoint URL. S3-compatible providers
	// (B2, MinIO) require this; AWS can omit it.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`
	// Region for AWS S3 / GCS; optional for others.
	// +optional
	Region string `json:"region,omitempty"`
	// Prefix applied under the bucket for NovaNas-owned objects.
	// +optional
	Prefix string `json:"prefix,omitempty"`
	// CredentialsSecret holds provider-specific credentials:
	// S3/B2/Azure require "access_key" + "secret_key" (or SAS URL);
	// GCS requires "service_account_json"; WebDAV requires "username"+"password".
	CredentialsSecret SecretKeyRef `json:"credentialsSecret"`
	// RepositoryPasswordSecret is the restic/kopia/borg repo password.
	// +optional
	RepositoryPasswordSecret *SecretKeyRef `json:"repositoryPasswordSecret,omitempty"`
	// +kubebuilder:validation:Enum=restic;borg;kopia
	// +optional
	Engine string `json:"engine,omitempty"`
}

// CloudBackupTargetCapabilities records what the probe discovered about the
// target's object-store features.
type CloudBackupTargetCapabilities struct {
	// Multipart indicates the endpoint advertises multipart uploads.
	Multipart bool `json:"multipart,omitempty"`
	// ServerSideEncryption is true when the endpoint announces SSE support.
	ServerSideEncryption bool `json:"serverSideEncryption,omitempty"`
	// Versioning is true when the bucket has object versioning enabled.
	Versioning bool `json:"versioning,omitempty"`
	// ObjectLock is true when the bucket has object-lock configured.
	ObjectLock bool `json:"objectLock,omitempty"`
}

// CloudBackupTargetStatus defines observed state of CloudBackupTarget.
type CloudBackupTargetStatus struct {
	// Phase is one of Pending, Ready, Failed.
	Phase string `json:"phase,omitempty"`
	// RepositoryInitialized tracks whether the backup engine has
	// initialised its on-bucket repository layout.
	RepositoryInitialized bool `json:"repositoryInitialized,omitempty"`
	// Reachable is true iff the last connectivity probe succeeded.
	Reachable bool `json:"reachable,omitempty"`
	// LastProbeAt is the RFC3339 timestamp of the last probe.
	LastProbeAt *metav1.Time `json:"lastProbeAt,omitempty"`
	// LastProbeError records the error from the last failed probe.
	LastProbeError string `json:"lastProbeError,omitempty"`
	// Capabilities is the cached feature discovery from the last probe.
	Capabilities *CloudBackupTargetCapabilities `json:"capabilities,omitempty"`
	// ObservedGeneration reflects the latest generation the controller saw.
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// CloudBackupTarget — cloud object-store endpoint (S3/B2/Azure/GCS/Swift/WebDAV).
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
