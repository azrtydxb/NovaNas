package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SshKeySpec mirrors packages/schemas/src/identity/ssh-key.ts. The
// controller pushes the public key to the host's authorized_keys via
// the node agent and records the SHA-256 fingerprint in status.
type SshKeySpec struct {
	// Owner is the User CR that owns the key. The node agent installs
	// the key into ~owner/.ssh/authorized_keys on targeted hosts.
	// +kubebuilder:validation:MinLength=1
	Owner string `json:"owner"`

	// PublicKey is the OpenSSH-format public key line
	// (e.g. "ssh-ed25519 AAAA... comment").
	// +kubebuilder:validation:MinLength=1
	PublicKey string `json:"publicKey"`

	// Comment overrides the comment field in the authorized_keys entry.
	// +optional
	Comment string `json:"comment,omitempty"`

	// ExpiresAt disables the key after the given instant by removing it
	// from authorized_keys. Nil means no expiry.
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
}

// SshKeyStatus records projection state for the key.
type SshKeyStatus struct {
	// +kubebuilder:validation:Enum=Active;Expired;Revoked
	// +optional
	Phase string `json:"phase,omitempty"`

	// Fingerprint is the SHA-256 fingerprint of the public key, base64
	// encoded (matches `ssh-keygen -lf`).
	// +optional
	Fingerprint string `json:"fingerprint,omitempty"`

	// KeyType is the SSH key algorithm ("ssh-ed25519", "ssh-rsa", etc.).
	// +optional
	KeyType string `json:"keyType,omitempty"`

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
// +kubebuilder:printcolumn:name="Owner",type=string,JSONPath=`.spec.owner`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.status.keyType`
// +kubebuilder:printcolumn:name="Fingerprint",type=string,JSONPath=`.status.fingerprint`

// SshKey — SSH authorized_keys projection with fingerprint tracking.
type SshKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SshKeySpec   `json:"spec,omitempty"`
	Status            SshKeyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SshKeyList contains a list of SshKey.
type SshKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SshKey `json:"items"`
}

func init() { SchemeBuilder.Register(&SshKey{}, &SshKeyList{}) }
