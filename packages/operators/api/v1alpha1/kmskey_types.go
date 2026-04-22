package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// KmsKeySpec defines the desired state of KmsKey.
//
// NOTE: these fields are intentionally minimal/additive. Full field
// mirroring from the packages/schemas Zod schema is tracked under
// TODO(wave-4); the A7 resolver only needs the transit path + wrapped DK.
type KmsKeySpec struct {
	// KeyID is the stable external identifier that S3 clients reference
	// in x-amz-server-side-encryption-aws-kms-key-id headers, e.g.
	// "arn:novanas:kms:::key/finance-sse-kms".
	KeyID string `json:"keyId,omitempty"`
	// TransitPath is the OpenBao Transit mount + key path used to unwrap
	// the DK (e.g. "transit/keys/finance-sse-kms").
	TransitPath string `json:"transitPath,omitempty"`
}

// KmsKeyStatus defines observed state of KmsKey.
type KmsKeyStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// WrappedDK is the OpenBao-wrapped (Transit-encrypted) Dataset Key
	// for this KmsKey, base64-encoded in its vault "vault:v1:..." form.
	// The controller populates it; the S3 gateway's kms_resolver unwraps
	// via Transit at request time.
	WrappedDK string `json:"wrappedDK,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// KmsKey — Named data key for SSE-KMS usage
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
