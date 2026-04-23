package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BucketUserCredentials points at Secrets holding generated AK/SK pairs.
type BucketUserCredentials struct {
	AccessKeySecret *SecretRef `json:"accessKeySecret,omitempty"`
	SecretKeySecret *SecretRef `json:"secretKeySecret,omitempty"`
}

// BucketUserPolicy is an IAM-like per-bucket permission block.
type BucketUserPolicy struct {
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix,omitempty"`
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Enum=read;write;delete;list;manage;bypassGovernance
	Actions []string `json:"actions"`
	// +kubebuilder:validation:Enum=allow;deny
	Effect string `json:"effect,omitempty"`
}

// BucketUserSpec defines the desired state of BucketUser.
type BucketUserSpec struct {
	DisplayName string                `json:"displayName,omitempty"`
	Credentials BucketUserCredentials `json:"credentials"`
	Policies    []BucketUserPolicy    `json:"policies,omitempty"`
}

// BucketUserStatus defines observed state of BucketUser.
type BucketUserStatus struct {
	// +kubebuilder:validation:Enum=Pending;Active;Failed
	Phase       string             `json:"phase,omitempty"`
	AccessKeyID string             `json:"accessKeyId,omitempty"`
	Conditions  []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas,shortName=bku
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="AccessKey",type=string,JSONPath=`.status.accessKeyId`

// BucketUser — S3 credentials and policies scoped to one or more buckets.
type BucketUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BucketUserSpec   `json:"spec,omitempty"`
	Status            BucketUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BucketUserList contains a list of BucketUser.
type BucketUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BucketUser `json:"items"`
}

func init() { SchemeBuilder.Register(&BucketUser{}, &BucketUserList{}) }
