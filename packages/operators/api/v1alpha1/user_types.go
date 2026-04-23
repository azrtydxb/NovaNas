package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// UserSpec defines the desired state of a User.
//
// The User CR is the cluster-authoritative projection of a Keycloak user
// plus a per-tenant OpenBao policy + kubernetes-auth role. Fields mirror
// the Zod User schema in packages/schemas. When a field is empty the
// Keycloak admin client leaves the attribute unchanged on update.
type UserSpec struct {
	// Email address of the user. Synced to Keycloak as `email`.
	// +optional
	Email string `json:"email,omitempty"`

	// FirstName is the user's given name. Synced to Keycloak as `firstName`.
	// +optional
	FirstName string `json:"firstName,omitempty"`

	// LastName is the user's family name. Synced to Keycloak as `lastName`.
	// +optional
	LastName string `json:"lastName,omitempty"`

	// Enabled is the Keycloak `enabled` flag. When false the user exists
	// but cannot authenticate. Defaults to true when unset.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Groups is the list of Keycloak group names the user should be a
	// member of. Group membership is replaced on every reconcile
	// (set-semantics, not merge).
	// +optional
	Groups []string `json:"groups,omitempty"`

	// Realm overrides the default realm name ("novanas"). Typically left
	// unset; useful only for multi-realm deployments.
	// +optional
	Realm string `json:"realm,omitempty"`
}

// UserStatus defines observed state of User.
type UserStatus struct {
	// Phase is a coarse-grained lifecycle indicator: Pending, Ready,
	// Failed. More fine-grained signals live on Conditions.
	Phase string `json:"phase,omitempty"`

	// KeycloakUserID is the UUID returned by Keycloak on first EnsureUser.
	// Stable across reconciles; used by GroupReconciler and downstream
	// auditing.
	KeycloakUserID string `json:"keycloakUserID,omitempty"`

	// Conditions is the Kubernetes-standard condition slice for Ready /
	// Reconciling / Failed signals.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// User — Local user projection of Keycloak.
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              UserSpec   `json:"spec,omitempty"`
	Status            UserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UserList contains a list of User.
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []User `json:"items"`
}

func init() { SchemeBuilder.Register(&User{}, &UserList{}) }
