package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// IscsiPortal is the network-facing listener of an iSCSI target.
type IscsiPortal struct {
	// +kubebuilder:validation:MinLength=1
	HostInterface string `json:"hostInterface"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`
}

// IscsiChapSecretRef points at a Kubernetes Secret holding CHAP creds.
type IscsiChapSecretRef struct {
	SecretName string `json:"secretName,omitempty"`
	Key        string `json:"key,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	// +kubebuilder:validation:Pattern=`^(openbao|vault)://.+`
	SecretRef string `json:"secretRef,omitempty"`
}

// IscsiChapAuth toggles CHAP authentication on the target.
type IscsiChapAuth struct {
	Enabled      bool                `json:"enabled"`
	Mutual       bool                `json:"mutual,omitempty"`
	UserSecret   *IscsiChapSecretRef `json:"userSecret,omitempty"`
	MutualSecret *IscsiChapSecretRef `json:"mutualSecret,omitempty"`
}

// IscsiTargetSpec defines the desired state of IscsiTarget.
type IscsiTargetSpec struct {
	// BlockVolume names the backing BlockVolume CR.
	// +kubebuilder:validation:MinLength=1
	BlockVolume string      `json:"blockVolume"`
	Portal      IscsiPortal `json:"portal"`
	Iqn         string      `json:"iqn,omitempty"`
	// +kubebuilder:validation:Enum=any;whitelist
	AclMode            string         `json:"aclMode,omitempty"`
	InitiatorAllowList []string       `json:"initiatorAllowList,omitempty"`
	Chap               *IscsiChapAuth `json:"chap,omitempty"`
}

// IscsiTargetStatus defines observed state.
type IscsiTargetStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed;Ready
	Phase              string             `json:"phase,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Iqn                string             `json:"iqn,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Volume",type=string,JSONPath=`.spec.blockVolume`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// IscsiTarget is an iSCSI portal binding a BlockVolume.
type IscsiTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              IscsiTargetSpec   `json:"spec,omitempty"`
	Status            IscsiTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IscsiTargetList contains a list of IscsiTarget.
type IscsiTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IscsiTarget `json:"items"`
}

func init() { SchemeBuilder.Register(&IscsiTarget{}, &IscsiTargetList{}) }
