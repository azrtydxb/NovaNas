package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// NvmeofListen is the portal configuration for an NVMe-oF target.
type NvmeofListen struct {
	// +kubebuilder:validation:MinLength=1
	HostInterface string `json:"hostInterface"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`
}

// NvmeofTargetSpec defines the desired state of NvmeofTarget.
type NvmeofTargetSpec struct {
	// BlockVolume names the backing BlockVolume CR.
	// +kubebuilder:validation:MinLength=1
	BlockVolume string `json:"blockVolume"`
	// SubsystemNqn overrides the auto-generated NQN.
	SubsystemNqn string `json:"subsystemNqn,omitempty"`
	// +kubebuilder:validation:Enum=tcp;rdma
	Transport       string       `json:"transport,omitempty"`
	Listen          NvmeofListen `json:"listen"`
	AllowedHostNqns []string     `json:"allowedHostNqns,omitempty"`
}

// NvmeofTargetStatus defines observed state of NvmeofTarget.
type NvmeofTargetStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed;Ready
	Phase              string             `json:"phase,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	SubsystemNqn       string             `json:"subsystemNqn,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Volume",type=string,JSONPath=`.spec.blockVolume`
// +kubebuilder:printcolumn:name="Transport",type=string,JSONPath=`.spec.transport`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// NvmeofTarget is an NVMe-oF subsystem exporting a BlockVolume.
type NvmeofTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NvmeofTargetSpec   `json:"spec,omitempty"`
	Status            NvmeofTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NvmeofTargetList contains a list of NvmeofTarget.
type NvmeofTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NvmeofTarget `json:"items"`
}

func init() { SchemeBuilder.Register(&NvmeofTarget{}, &NvmeofTargetList{}) }
