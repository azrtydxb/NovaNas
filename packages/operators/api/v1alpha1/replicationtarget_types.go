package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ReplicationSecretRef identifies an auth secret for a replication target.
// Either SecretName (Kubernetes Secret) or SecretRef (openbao://... URI) is used.
type ReplicationSecretRef struct {
	SecretName string `json:"secretName,omitempty"`
	Key        string `json:"key,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	// +kubebuilder:validation:Pattern=`^(openbao|vault)://.+`
	SecretRef string `json:"secretRef,omitempty"`
}

// ReplicationAuth is embedded in ReplicationTargetSpec.
type ReplicationAuth struct {
	// SecretRef is a plain string reference kept for back-compat.
	SecretRef string                `json:"secretRef,omitempty"`
	Secret    *ReplicationSecretRef `json:"secret,omitempty"`
}

// ReplicationBandwidth limits/schedules replication throughput.
type ReplicationBandwidth struct {
	// Limit is a rate string (e.g. "100Mbps").
	Limit    string `json:"limit,omitempty"`
	Schedule string `json:"schedule,omitempty"`
}

// ReplicationTransport tunes the replication stream.
type ReplicationTransport struct {
	// +kubebuilder:validation:Enum=none;zstd;lz4
	Compression string                `json:"compression,omitempty"`
	Encryption  bool                  `json:"encryption,omitempty"`
	Bandwidth   *ReplicationBandwidth `json:"bandwidth,omitempty"`
}

// ReplicationTargetSpec defines the desired state of ReplicationTarget.
type ReplicationTargetSpec struct {
	// Endpoint is the remote NovaNas gRPC/HTTPS URL.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^https?://.+`
	Endpoint  string                `json:"endpoint"`
	Auth      ReplicationAuth       `json:"auth"`
	Transport *ReplicationTransport `json:"transport,omitempty"`
	TlsVerify *bool                 `json:"tlsVerify,omitempty"`
}

// ReplicationTargetStatus defines observed state of ReplicationTarget.
type ReplicationTargetStatus struct {
	// +kubebuilder:validation:Enum=Pending;Connected;Failed;Ready
	Phase              string             `json:"phase,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	RemoteVersion      string             `json:"remoteVersion,omitempty"`
	LastHandshake      *metav1.Time       `json:"lastHandshake,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.spec.endpoint`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// ReplicationTarget is a remote NovaNas cluster/endpoint used by ReplicationJob.
type ReplicationTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ReplicationTargetSpec   `json:"spec,omitempty"`
	Status            ReplicationTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ReplicationTargetList contains a list of ReplicationTarget.
type ReplicationTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReplicationTarget `json:"items"`
}

func init() { SchemeBuilder.Register(&ReplicationTarget{}, &ReplicationTargetList{}) }
