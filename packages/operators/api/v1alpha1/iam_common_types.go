package v1alpha1

// SecretKeyReference names a Secret (optionally in a specific namespace)
// plus an optional key. It mirrors packages/schemas common
// SecretReferenceSchema and is used by IAM-domain specs (Certificate,
// AuditPolicy, KeycloakRealm) to reference bind credentials, webhook
// auth material, and ACME/upload bundles.
type SecretKeyReference struct {
	// Name is the Secret name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace defaults to the referencing CR's namespace when empty.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Key selects a specific key inside the Secret. When empty, the
	// consumer picks a type-appropriate default (e.g. "tls.crt").
	// +optional
	Key string `json:"key,omitempty"`
}
