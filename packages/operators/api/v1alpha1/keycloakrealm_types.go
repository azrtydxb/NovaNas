package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// RealmFederationConnection describes LDAP/AD/OIDC connection settings.
type RealmFederationConnection struct {
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`
	// +optional
	BaseDN string `json:"baseDn,omitempty"`
	// +optional
	UsersDN string `json:"usersDn,omitempty"`
	// +optional
	GroupsDN string `json:"groupsDn,omitempty"`
	// +optional
	BindDN string `json:"bindDn,omitempty"`
	// +optional
	BindSecret *SecretKeyReference `json:"bindSecret,omitempty"`
	// +optional
	StartTLS bool `json:"startTls,omitempty"`
}

// RealmFederation is one identity provider attached to the realm.
type RealmFederation struct {
	// +kubebuilder:validation:Enum=activeDirectory;ldap;oidc
	Type string `json:"type"`
	// +optional
	DisplayName string                    `json:"displayName,omitempty"`
	Connection  RealmFederationConnection `json:"connection"`
	// SyncPeriod is a Go duration string for the federation sync cadence.
	// +optional
	SyncPeriod string `json:"syncPeriod,omitempty"`
}

// RealmMFA configures the realm's MFA requirements.
type RealmMFA struct {
	// +optional
	TOTP bool `json:"totp,omitempty"`
	// +optional
	WebAuthn bool `json:"webauthn,omitempty"`
	// +optional
	Required bool `json:"required,omitempty"`
}

// KeycloakRealmSpec mirrors packages/schemas/src/identity/keycloak-realm.ts.
type KeycloakRealmSpec struct {
	// +optional
	DisplayName string `json:"displayName,omitempty"`
	// +optional
	DefaultLocale string `json:"defaultLocale,omitempty"`
	// +optional
	Federations []RealmFederation `json:"federations,omitempty"`
	// +optional
	MFA *RealmMFA `json:"mfa,omitempty"`
	// PasswordPolicy is the Keycloak password policy expression, e.g.
	// "length(12) and upperCase(1) and digits(1)".
	// +optional
	PasswordPolicy string `json:"passwordPolicy,omitempty"`
}

// KeycloakRealmStatus reports sync state for the realm.
type KeycloakRealmStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed
	// +optional
	Phase string `json:"phase,omitempty"`
	// +kubebuilder:validation:Minimum=0
	// +optional
	UserCount int32 `json:"userCount,omitempty"`
	// +kubebuilder:validation:Minimum=0
	// +optional
	GroupCount int32 `json:"groupCount,omitempty"`
	// +optional
	LastSync *metav1.Time `json:"lastSync,omitempty"`
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
// +kubebuilder:printcolumn:name="Users",type=integer,JSONPath=`.status.userCount`
// +kubebuilder:printcolumn:name="Groups",type=integer,JSONPath=`.status.groupCount`

// KeycloakRealm — Keycloak realm configuration with federations and MFA.
type KeycloakRealm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KeycloakRealmSpec   `json:"spec,omitempty"`
	Status            KeycloakRealmStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakRealmList contains a list of KeycloakRealm.
type KeycloakRealmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakRealm `json:"items"`
}

func init() { SchemeBuilder.Register(&KeycloakRealm{}, &KeycloakRealmList{}) }
