package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ShareSpec defines the desired state of Share.
type ShareSpec struct {
	Dataset   string           `json:"dataset,omitempty"`
	Path      string           `json:"path,omitempty"`
	Protocols ShareProtocols   `json:"protocols,omitempty"`
	Access    []ShareAccessRule `json:"access,omitempty"`
}

// ShareProtocols configures per-protocol export options.
type ShareProtocols struct {
	Smb *SmbShareConfig `json:"smb,omitempty"`
	Nfs *NfsShareConfig `json:"nfs,omitempty"`
}

// SmbShareConfig holds SMB export options.
type SmbShareConfig struct {
	Server        string `json:"server,omitempty"`
	ShadowCopies  bool   `json:"shadowCopies,omitempty"`
	CaseSensitive bool   `json:"caseSensitive,omitempty"`
}

// NfsShareConfig holds NFS export options.
type NfsShareConfig struct {
	Server          string   `json:"server,omitempty"`
	Squash          string   `json:"squash,omitempty"`
	AllowedNetworks []string `json:"allowedNetworks,omitempty"`
}

// ShareAccessRule is a single access-control entry.
type ShareAccessRule struct {
	Principal SharePrincipal `json:"principal,omitempty"`
	Mode      string         `json:"mode,omitempty"`
}

// SharePrincipal identifies a user or group.
type SharePrincipal struct {
	User  string `json:"user,omitempty"`
	Group string `json:"group,omitempty"`
}

// ShareStatus defines observed state.
type ShareStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// Share is a multi-protocol export (SMB + NFS) of a Dataset path.
type Share struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ShareSpec   `json:"spec,omitempty"`
	Status            ShareStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ShareList contains a list of Share.
type ShareList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Share `json:"items"`
}

func init() { SchemeBuilder.Register(&Share{}, &ShareList{}) }
