package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemoteAccessTunnelType selects the tunnel technology.
// +kubebuilder:validation:Enum=sdwan;wireguard;tailscale
type RemoteAccessTunnelType string

// RemoteAccessTunnelEndpoint is the remote peer address.
type RemoteAccessTunnelEndpoint struct {
	// +kubebuilder:validation:MinLength=1
	Hostname string `json:"hostname"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`
}

// RemoteAccessTunnelAuth references the tunnel credentials.
type RemoteAccessTunnelAuth struct {
	// SecretRef names an in-namespace Secret (legacy, simple form).
	// +optional
	SecretRef string `json:"secretRef,omitempty"`
	// Secret is a structured reference to a Secret key.
	// +optional
	Secret *corev1.SecretKeySelector `json:"secret,omitempty"`
}

// RemoteAccessTunnelExpose selects which workloads traverse the tunnel.
type RemoteAccessTunnelExpose struct {
	// +optional
	App string `json:"app,omitempty"`
	// +optional
	Vm string `json:"vm,omitempty"`
	// +kubebuilder:validation:Enum=tunnel;direct
	// +optional
	Via string `json:"via,omitempty"`
}

// RemoteAccessTunnelSpec defines the desired state of RemoteAccessTunnel.
type RemoteAccessTunnelSpec struct {
	Type     RemoteAccessTunnelType     `json:"type"`
	Endpoint RemoteAccessTunnelEndpoint `json:"endpoint"`
	Auth     RemoteAccessTunnelAuth     `json:"auth"`
	// +optional
	Exposes []RemoteAccessTunnelExpose `json:"exposes,omitempty"`
}

// RemoteAccessTunnelStatus defines observed state of RemoteAccessTunnel.
type RemoteAccessTunnelStatus struct {
	// +kubebuilder:validation:Enum=Pending;Connected;Disconnected;Failed;Reconciling;Ready
	// +optional
	Phase string `json:"phase,omitempty"`
	// ConnectedAt is the timestamp of the most recent successful handshake.
	// +optional
	ConnectedAt *metav1.Time `json:"connectedAt,omitempty"`
	// PublicKey is the locally-advertised WireGuard/Tailscale public key.
	// +optional
	PublicKey string `json:"publicKey,omitempty"`
	// AppliedConfigHash is the sha256 of the projected Tunnel spec.
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
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".spec.endpoint.hostname"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

// RemoteAccessTunnel is a persistent outbound tunnel (WireGuard, IPsec,
// Tailscale, or SD-WAN) managed by a systemd unit on the host.
type RemoteAccessTunnel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RemoteAccessTunnelSpec   `json:"spec,omitempty"`
	Status            RemoteAccessTunnelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemoteAccessTunnelList contains a list of RemoteAccessTunnel.
type RemoteAccessTunnelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteAccessTunnel `json:"items"`
}

func init() { SchemeBuilder.Register(&RemoteAccessTunnel{}, &RemoteAccessTunnelList{}) }
