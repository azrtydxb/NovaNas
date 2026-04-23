package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// EncryptionMasterKeySpec describes how the master key is sealed.
type EncryptionMasterKeySpec struct {
	// +kubebuilder:validation:Enum=tpm;passphrase;both
	// +optional
	SealedBy string `json:"sealedBy,omitempty"`
}

// EncryptionPolicySpec mirrors packages/schemas/src/crypto/encryption-policy.ts.
// The policy is consulted by volume-provisioning controllers
// (BlockVolume, Dataset, Bucket) via a shared helper
// (reconciler.ResolveEncryptionPolicy). It can be cluster-scoped
// (default) or narrowed by namespace selector.
type EncryptionPolicySpec struct {
	// DefaultEnabled sets whether new volumes are encrypted by default
	// when they don't specify an explicit choice.
	// +optional
	DefaultEnabled *bool `json:"defaultEnabled,omitempty"`

	// Cipher selects the symmetric cipher used on the data path.
	// +kubebuilder:validation:Enum=AES256-GCM;AES128-GCM;XChaCha20-Poly1305
	// +optional
	Cipher string `json:"cipher,omitempty"`

	// MasterKey configures sealing of the cluster master key.
	// +optional
	MasterKey *EncryptionMasterKeySpec `json:"masterKey,omitempty"`

	// RequireForTiers lists storage tier names where encryption is
	// mandatory (e.g. "hot", "cold").
	// +optional
	RequireForTiers []string `json:"requireForTiers,omitempty"`

	// NamespaceSelector narrows a policy to a subset of namespaces.
	// Nil selector means cluster-wide.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// EncryptionPolicyStatus reports whether the master key is currently
// sealed and available to the data-plane.
type EncryptionPolicyStatus struct {
	// +kubebuilder:validation:Enum=Active;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// MasterKeySealed is true when the configured sealing mechanism
	// currently holds the master key.
	// +optional
	MasterKeySealed bool `json:"masterKeySealed,omitempty"`

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
// +kubebuilder:printcolumn:name="Cipher",type=string,JSONPath=`.spec.cipher`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// EncryptionPolicy — Cluster defaults for volume encryption.
type EncryptionPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              EncryptionPolicySpec   `json:"spec,omitempty"`
	Status            EncryptionPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EncryptionPolicyList contains a list of EncryptionPolicy.
type EncryptionPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EncryptionPolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&EncryptionPolicy{}, &EncryptionPolicyList{}) }
