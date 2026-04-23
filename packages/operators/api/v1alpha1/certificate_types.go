package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CertificateProvider is one of the supported issuance backends.
// +kubebuilder:validation:Enum=acme;internalPki;upload
type CertificateProvider string

const (
	// CertProviderACME issues via an ACME CA (Let's Encrypt, ZeroSSL, etc.).
	CertProviderACME CertificateProvider = "acme"
	// CertProviderInternalPKI issues via OpenBao PKI.
	CertProviderInternalPKI CertificateProvider = "internalPki"
	// CertProviderUpload consumes key+cert from referenced Secrets.
	CertProviderUpload CertificateProvider = "upload"
)

// CertificateACMESpec configures an ACME-based issuance.
type CertificateACMESpec struct {
	// +kubebuilder:validation:Enum=letsencrypt;letsencrypt-staging;zerossl;custom
	Issuer string `json:"issuer"`
	// +kubebuilder:validation:Pattern=`^[^@\s]+@[^@\s]+\.[^@\s]+$`
	// +optional
	Email string `json:"email,omitempty"`
	// +optional
	DirectoryURL string `json:"directoryUrl,omitempty"`
}

// CertificateUploadSpec points at Secrets holding a pre-issued bundle.
type CertificateUploadSpec struct {
	CertSecret SecretKeyReference `json:"certSecret"`
	KeySecret  SecretKeyReference `json:"keySecret"`
}

// CertificateSpec mirrors packages/schemas/src/crypto/certificate.ts. A
// Certificate is issued either by an ACME-compliant CA (via cert-manager
// where available) or by the internal OpenBao PKI. Uploaded bundles are
// consumed verbatim. The resulting material lands in a child Secret
// named "<cert-name>-tls" with an owner reference for GC.
type CertificateSpec struct {
	Provider CertificateProvider `json:"provider"`

	// CommonName is the primary subject CN.
	// +kubebuilder:validation:MinLength=1
	CommonName string `json:"commonName"`

	// DNSNames are additional SAN DNS entries.
	// +optional
	DNSNames []string `json:"dnsNames,omitempty"`

	// IPAddresses are additional SAN IP entries.
	// +optional
	IPAddresses []string `json:"ipAddresses,omitempty"`

	// +optional
	ACME *CertificateACMESpec `json:"acme,omitempty"`

	// +optional
	Upload *CertificateUploadSpec `json:"upload,omitempty"`

	// RenewBeforeDays triggers a renew this many days before NotAfter.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=30
	// +optional
	RenewBeforeDays int32 `json:"renewBeforeDays,omitempty"`
}

// CertificateStatus reports issuance state.
type CertificateStatus struct {
	// +kubebuilder:validation:Enum=Pending;Issued;Renewing;Expired;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// SerialNumber is the hex serial of the currently-installed cert.
	// +optional
	SerialNumber string `json:"serialNumber,omitempty"`

	// Issuer is the issuing CA's subject DN.
	// +optional
	Issuer string `json:"issuer,omitempty"`

	// NotBefore is the cert's NotBefore validity instant.
	// +optional
	NotBefore *metav1.Time `json:"notBefore,omitempty"`

	// NotAfter is the cert's NotAfter validity instant (expiry).
	// +optional
	NotAfter *metav1.Time `json:"notAfter,omitempty"`

	// SecretRef names the TLS Secret holding the issued material.
	// +optional
	SecretRef string `json:"secretRef,omitempty"`

	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,categories=novanas,shortName=cert
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="CN",type=string,JSONPath=`.spec.commonName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="NotAfter",type=date,JSONPath=`.status.notAfter`

// Certificate — Namespaced TLS certificate managed by the operator.
type Certificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CertificateSpec   `json:"spec,omitempty"`
	Status            CertificateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CertificateList contains a list of Certificate.
type CertificateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Certificate `json:"items"`
}

func init() { SchemeBuilder.Register(&Certificate{}, &CertificateList{}) }
