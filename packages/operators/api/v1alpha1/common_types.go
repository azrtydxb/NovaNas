package v1alpha1

// SecretRef is the NovaNas shared reference type for K8s Secrets used by
// operator-managed CRs (UPS credentials, SMTP auth, AD join, backup
// encryption, etc.). It mirrors packages/schemas SecretReferenceSchema.
type SecretRef struct {
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName"`
	// Key inside the Secret. Defaults are controller-specific.
	Key string `json:"key,omitempty"`
	// Namespace of the Secret. Defaults to the CR's namespace for
	// namespaced CRs, or the operator namespace for cluster-scoped CRs.
	Namespace string `json:"namespace,omitempty"`
}
