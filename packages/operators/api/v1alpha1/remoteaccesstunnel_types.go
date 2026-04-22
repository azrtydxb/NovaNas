package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// RemoteAccessTunnelSpec defines the desired state of RemoteAccessTunnel.
type RemoteAccessTunnelSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for RemoteAccessTunnel.
}

// RemoteAccessTunnelStatus defines observed state of RemoteAccessTunnel.
type RemoteAccessTunnelStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// RemoteAccessTunnel — SD-WAN / Tailscale-style tunnel
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
