package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// KmsKeyRotation describes a rotation schedule.
type KmsKeyRotation struct {
	// Enabled toggles automatic rotation.
	Enabled bool `json:"enabled"`
	// Period is a Go duration string (e.g. "720h"). When empty the
	// controller uses a 30-day default.
	// +optional
	Period string `json:"period,omitempty"`
}

// KmsKeySpec mirrors packages/schemas/src/crypto/kms-key.ts plus two
// operator-internal fields (KeyID, TransitPath) carried forward since
// the original S3 gateway integration.
type KmsKeySpec struct {
	// Description is a human-readable note for the UI.
	// +optional
	Description string `json:"description,omitempty"`

	// Rotation configures automatic key rotation through OpenBao Transit.
	// +optional
	Rotation *KmsKeyRotation `json:"rotation,omitempty"`

	// DeletionProtection blocks deletion while true.
	// +optional
	DeletionProtection bool `json:"deletionProtection,omitempty"`

	// KeyID is the stable external identifier referenced by S3 clients
	// in x-amz-server-side-encryption-aws-kms-key-id headers, e.g.
	// "arn:novanas:kms:::key/finance-sse-kms".
	// +optional
	KeyID string `json:"keyId,omitempty"`

	// TransitPath is the OpenBao Transit mount + key path used to unwrap
	// the DK (e.g. "transit/keys/finance-sse-kms").
	// +optional
	TransitPath string `json:"transitPath,omitempty"`
}

// KmsKeyStatus is the observed state of the key.
type KmsKeyStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Rotating;Disabled;Destroyed
	// +optional
	Phase string `json:"phase,omitempty"`

	// KeyID is the concrete backend key identifier (Transit key name).
	// +optional
	KeyID string `json:"keyId,omitempty"`

	// KeyVersion is the active Transit key version.
	// +kubebuilder:validation:Minimum=0
	// +optional
	KeyVersion int32 `json:"keyVersion,omitempty"`

	// CreatedAt is when the backing key was first provisioned.
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// LastRotatedAt is when the key was last rotated.
	// +optional
	LastRotatedAt *metav1.Time `json:"lastRotatedAt,omitempty"`

	// WrappedDK is the OpenBao-wrapped (Transit-encrypted) Dataset Key
	// in its vault "vault:v1:..." form. The S3 gateway's kms_resolver
	// unwraps it via Transit at request time.
	// +optional
	WrappedDK string `json:"wrappedDK,omitempty"`

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
// +kubebuilder:printcolumn:name="Version",type=integer,JSONPath=`.status.keyVersion`
// +kubebuilder:printcolumn:name="LastRotated",type=date,JSONPath=`.status.lastRotatedAt`

// KmsKey — Named data key backed by OpenBao Transit.
type KmsKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KmsKeySpec   `json:"spec,omitempty"`
	Status            KmsKeyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KmsKeyList contains a list of KmsKey.
type KmsKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KmsKey `json:"items"`
}

func init() { SchemeBuilder.Register(&KmsKey{}, &KmsKeyList{}) }
