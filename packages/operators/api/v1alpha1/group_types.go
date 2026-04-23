package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// GroupSpec mirrors packages/schemas/src/identity/group.ts. A Group is
// projected into Keycloak and also drives K8s RBAC RoleBinding generation
// so cluster tools can authorise on group membership.
type GroupSpec struct {
	// Name is the realm-unique group name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// DisplayName is a human-readable label for the UI.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// Members is the list of User CR names that should belong to this
	// group. The controller reconciles membership to Keycloak in both
	// directions (adds and removes).
	// +optional
	Members []string `json:"members,omitempty"`

	// Realm overrides the operator's default Keycloak realm.
	// +optional
	Realm string `json:"realm,omitempty"`

	// Federated is true for groups mirrored from an external IdP.
	// +optional
	Federated bool `json:"federated,omitempty"`

	// GID is the POSIX group-id projected into /etc/group.
	// +kubebuilder:validation:Minimum=0
	// +optional
	GID *int64 `json:"gid,omitempty"`

	// ClusterRole optionally names a Kubernetes ClusterRole that the
	// controller should bind to this group via a generated
	// ClusterRoleBinding.
	// +optional
	ClusterRole string `json:"clusterRole,omitempty"`
}

// GroupStatus is the observed state of the Group projection.
type GroupStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// KeycloakID is the internal UUID returned by the Keycloak admin API.
	// +optional
	KeycloakID string `json:"keycloakID,omitempty"`

	// MemberCount is the number of resolved members on the last sync.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MemberCount int32 `json:"memberCount,omitempty"`

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
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Members",type=integer,JSONPath=`.status.memberCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Group — Keycloak group projection with optional K8s RBAC binding.
type Group struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GroupSpec   `json:"spec,omitempty"`
	Status            GroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GroupList contains a list of Group.
type GroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Group `json:"items"`
}

func init() { SchemeBuilder.Register(&Group{}, &GroupList{}) }
