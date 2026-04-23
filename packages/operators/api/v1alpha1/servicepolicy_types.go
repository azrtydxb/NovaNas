package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ServiceName enumerates manageable cluster services.
// +kubebuilder:validation:Enum=ssh;smb;nfs;iscsi;nvmeof;s3;api;ui;grafana;prometheus;loki;keycloak;openbao
type ServiceName string

// ServiceToggle enables/disables one service with optional bindings.
type ServiceToggle struct {
	Name    ServiceName `json:"name"`
	Enabled bool        `json:"enabled"`
	// BindInterface restricts the service to a specific NIC (when
	// supported by the underlying daemon).
	// +optional
	BindInterface string `json:"bindInterface,omitempty"`
	// Port overrides the default listening port.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`
}

// ServicePolicySpec defines the desired state of ServicePolicy.
type ServicePolicySpec struct {
	// +kubebuilder:validation:MinItems=1
	Services []ServiceToggle `json:"services"`
}

// ServicePolicyStatus defines observed state of ServicePolicy.
type ServicePolicyStatus struct {
	// +kubebuilder:validation:Enum=Pending;Applied;Failed;Reconciling;Ready
	// +optional
	Phase string `json:"phase,omitempty"`
	// AppliedAt is the timestamp of the last successful apply.
	// +optional
	AppliedAt *metav1.Time `json:"appliedAt,omitempty"`
	// AppliedConfigHash is the sha256 of the rendered ConfigMap payload.
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
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

// ServicePolicy toggles cluster-level services on/off.
type ServicePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ServicePolicySpec   `json:"spec,omitempty"`
	Status            ServicePolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServicePolicyList contains a list of ServicePolicy.
type ServicePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServicePolicy `json:"items"`
}

func init() { SchemeBuilder.Register(&ServicePolicy{}, &ServicePolicyList{}) }
