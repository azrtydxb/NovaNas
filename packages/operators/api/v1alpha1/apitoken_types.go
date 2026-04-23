package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ApiTokenSpec mirrors packages/schemas/src/identity/api-token.ts. A
// scoped bearer token created on behalf of a User, rotated on a schedule
// set by the controller. The plaintext token is returned exactly once in
// Status.RawTokenSecret and scrubbed on the next reconcile.
type ApiTokenSpec struct {
	// Owner is the User CR name that the token authenticates as.
	// +kubebuilder:validation:MinLength=1
	Owner string `json:"owner"`

	// Scopes is the list of capability strings the token is permitted to
	// use. Scope semantics are enforced by the API layer, not the operator.
	// +kubebuilder:validation:MinItems=1
	Scopes []string `json:"scopes"`

	// ExpiresAt is an optional absolute expiry. If nil the token does not
	// expire (subject to rotation).
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// Description is a free-form human-readable note shown in the UI.
	// +optional
	Description string `json:"description,omitempty"`

	// RotationPeriod triggers automatic rotation at the given interval.
	// Must be a valid Go duration string (e.g. "720h"). Zero disables
	// rotation.
	// +optional
	RotationPeriod string `json:"rotationPeriod,omitempty"`
}

// ApiTokenStatus reports the token lifecycle. The plaintext token lives
// in RawTokenSecret for exactly one reconcile after creation or rotation
// and is scrubbed on the next pass.
type ApiTokenStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Expired;Revoked
	// +optional
	Phase string `json:"phase,omitempty"`

	// TokenID is a stable public identifier (SHA-256 of the token).
	// +optional
	TokenID string `json:"tokenID,omitempty"`

	// SecretRef names the Secret holding the token hash; the plaintext
	// itself is delivered via RawTokenSecret below.
	// +optional
	SecretRef string `json:"secretRef,omitempty"`

	// CreatedAt is when the current token was minted.
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// LastUsedAt is when the token was most recently presented.
	// +optional
	LastUsedAt *metav1.Time `json:"lastUsedAt,omitempty"`

	// LastRotatedAt is when the token was last rotated.
	// +optional
	LastRotatedAt *metav1.Time `json:"lastRotatedAt,omitempty"`

	// RawTokenSecret is the plaintext token delivered exactly once on
	// create or rotation. The controller clears this field on the next
	// reconcile, so clients must capture it immediately.
	// +optional
	RawTokenSecret string `json:"rawTokenSecret,omitempty"`

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
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="LastRotated",type=date,JSONPath=`.status.lastRotatedAt`

// ApiToken — Scoped API token projection with hashed secret storage.
type ApiToken struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ApiTokenSpec   `json:"spec,omitempty"`
	Status            ApiTokenStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApiTokenList contains a list of ApiToken.
type ApiTokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApiToken `json:"items"`
}

func init() { SchemeBuilder.Register(&ApiToken{}, &ApiTokenList{}) }
