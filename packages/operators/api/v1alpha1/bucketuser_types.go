package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BucketUserSpec defines the desired state of BucketUser.
type BucketUserSpec struct {
	// TODO(wave-4): mirror fields from packages/schemas Zod schema for BucketUser.
}

// BucketUserStatus defines observed state of BucketUser.
type BucketUserStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas
// +kubebuilder:subresource:status

// BucketUser — S3 credentials and policies
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
