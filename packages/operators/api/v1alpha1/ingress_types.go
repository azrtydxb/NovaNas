package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// IngressRule is one host/path -> backend mapping.
type IngressRule struct {
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`
	// Backend is the name of the target resource (App, Service, etc.).
	// +kubebuilder:validation:MinLength=1
	Backend string `json:"backend"`
	// +optional
	Path string `json:"path,omitempty"`
}

// IngressTls selects the certificate used to terminate TLS.
type IngressTls struct {
	// Certificate is the name of a Certificate CR in the same namespace.
	// +kubebuilder:validation:MinLength=1
	Certificate string `json:"certificate"`
}

// IngressSpec defines the desired state of Ingress.
type IngressSpec struct {
	// +kubebuilder:validation:MinLength=1
	Hostname string `json:"hostname"`
	// +optional
	Tls *IngressTls `json:"tls,omitempty"`
	// +kubebuilder:validation:MinItems=1
	Rules []IngressRule `json:"rules"`
}

// IngressStatus defines observed state of Ingress.
type IngressStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed;Reconciling;Ready
	// +optional
	Phase string `json:"phase,omitempty"`
	// Vip is the allocated external IP (IPv4 or IPv6).
	// +optional
	Vip string `json:"vip,omitempty"`
	// AppliedConfigHash is the sha256 of the generated novaedge Route spec.
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
// +kubebuilder:printcolumn:name="VIP",type="string",JSONPath=".status.vip"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

// Ingress exposes a backend via novaedge.
type Ingress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              IngressSpec   `json:"spec,omitempty"`
	Status            IngressStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IngressList contains a list of Ingress.
type IngressList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Ingress `json:"items"`
}

func init() { SchemeBuilder.Register(&Ingress{}, &IngressList{}) }
