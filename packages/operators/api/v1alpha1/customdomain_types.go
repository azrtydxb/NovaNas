package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CustomDomainTlsProvider selects how certificates are obtained.
// +kubebuilder:validation:Enum=letsencrypt;internal;upload
type CustomDomainTlsProvider string

// CustomDomainTls configures certificate acquisition.
type CustomDomainTls struct {
	Provider CustomDomainTlsProvider `json:"provider"`
	// Certificate is the name of a Certificate CR (when Provider=upload or
	// =internal and a pre-created cert is used).
	// +optional
	Certificate string `json:"certificate,omitempty"`
}

// CustomDomainSpec defines the desired state of CustomDomain.
type CustomDomainSpec struct {
	// Hostname is the FQDN (e.g. "files.example.com").
	// +kubebuilder:validation:MinLength=1
	Hostname string `json:"hostname"`
	// Target references the downstream CR that should receive traffic.
	Target corev1.ObjectReference `json:"target"`
	Tls    CustomDomainTls        `json:"tls"`
}

// CustomDomainStatus defines observed state of CustomDomain.
type CustomDomainStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed;Reconciling;Ready
	// +optional
	Phase string `json:"phase,omitempty"`
	// +kubebuilder:validation:Enum=Pending;Issued;Expired;Failed
	// +optional
	CertificateStatus string `json:"certificateStatus,omitempty"`
	// AppliedConfigHash is the sha256 of the projected HostnameBinding.
	// +optional
	AppliedConfigHash string `json:"appliedConfigHash,omitempty"`
	// ObservedGeneration tracks the generation of the last reconcile.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Hostname",type="string",JSONPath=".spec.hostname"
// +kubebuilder:printcolumn:name="Cert",type="string",JSONPath=".status.certificateStatus"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

// CustomDomain binds an external hostname to a NovaNas-exposed resource.
type CustomDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CustomDomainSpec   `json:"spec,omitempty"`
	Status            CustomDomainStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CustomDomainList contains a list of CustomDomain.
type CustomDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CustomDomain `json:"items"`
}

func init() { SchemeBuilder.Register(&CustomDomain{}, &CustomDomainList{}) }
