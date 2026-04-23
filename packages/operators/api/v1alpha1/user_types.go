package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// UserSpec mirrors packages/schemas/src/identity/user.ts. It describes
// the desired state of a NovaNas User, which is projected into Keycloak
// (users, realm membership, group membership, password rotation) by the
// User controller. The spec also carries POSIX identity attributes that
// the node agent consults when rendering /etc/passwd entries.
type UserSpec struct {
	// Username is the realm-unique login name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Username string `json:"username"`

	// Email is the user's primary email. Used by Keycloak for password
	// recovery and notifications.
	// +kubebuilder:validation:Pattern=`^[^@\s]+@[^@\s]+\.[^@\s]+$`
	// +optional
	Email string `json:"email,omitempty"`

	// DisplayName is the human-readable name shown in the UI.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// Groups is the list of Group CR names this user belongs to.
	// +optional
	Groups []string `json:"groups,omitempty"`

	// Admin grants cluster-administrator rights via RBAC.
	// +optional
	Admin bool `json:"admin,omitempty"`

	// Enabled toggles login without deleting the user. Defaults to true.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Realm overrides the operator's default Keycloak realm for this user.
	// +optional
	Realm string `json:"realm,omitempty"`

	// Federated is true when the user is sourced from an external IdP
	// (LDAP/AD/OIDC) and should not receive a local password.
	// +optional
	Federated bool `json:"federated,omitempty"`

	// UID is the POSIX user-id projected into /etc/passwd.
	// +kubebuilder:validation:Minimum=0
	// +optional
	UID *int64 `json:"uid,omitempty"`

	// PrimaryGID is the primary POSIX group-id.
	// +kubebuilder:validation:Minimum=0
	// +optional
	PrimaryGID *int64 `json:"primaryGid,omitempty"`

	// HomeDataset names a Dataset CR mounted as the user's home directory.
	// +optional
	HomeDataset string `json:"homeDataset,omitempty"`

	// Shell overrides the default login shell (/bin/bash).
	// +optional
	Shell string `json:"shell,omitempty"`
}

// UserStatus is the observed state of the User projection.
type UserStatus struct {
	// Phase is a high-level lifecycle state for dashboards.
	// +kubebuilder:validation:Enum=Pending;Active;Disabled;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// KeycloakID is the internal Keycloak UUID returned by the admin API
	// after EnsureUser. Empty while the projection has never succeeded.
	// +optional
	KeycloakID string `json:"keycloakID,omitempty"`

	// LastLogin is the timestamp of the most recent successful login
	// reported by Keycloak. Populated by the sync loop.
	// +optional
	LastLogin *metav1.Time `json:"lastLogin,omitempty"`

	// Conditions carries Ready/Progressing/Degraded conditions.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="KeycloakID",type=string,JSONPath=`.status.keycloakID`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// User — Local projection of a Keycloak user.
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
