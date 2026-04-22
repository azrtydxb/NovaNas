package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BucketSpec defines the desired state of Bucket.
type BucketSpec struct {
	Store      string              `json:"store,omitempty"`
	Protection *ProtectionPolicy   `json:"protection,omitempty"`
	Encryption *EncryptionSettings `json:"encryption,omitempty"`
	Tiering    *TieringPolicy      `json:"tiering,omitempty"`
	Versioning string              `json:"versioning,omitempty"`
	ObjectLock *ObjectLockConfig   `json:"objectLock,omitempty"`
	Quota      *BucketQuota        `json:"quota,omitempty"`
	Lifecycle  []BucketLifecycleRule `json:"lifecycle,omitempty"`
}

// ObjectLockConfig configures S3 Object Lock.
type ObjectLockConfig struct {
	Enabled          bool              `json:"enabled,omitempty"`
	Mode             string            `json:"mode,omitempty"`
	DefaultRetention *RetentionPeriod  `json:"defaultRetention,omitempty"`
}

// RetentionPeriod expresses a duration.
type RetentionPeriod struct {
	Period string `json:"period,omitempty"`
}

// BucketQuota sets bucket-level quotas.
type BucketQuota struct {
	HardBytes   int64 `json:"hardBytes,omitempty"`
	HardObjects int64 `json:"hardObjects,omitempty"`
}

// BucketLifecycleRule is a single lifecycle policy entry.
type BucketLifecycleRule struct {
	Prefix       string `json:"prefix,omitempty"`
	ExpireAfter  string `json:"expireAfter,omitempty"`
	TransitionTo string `json:"transitionTo,omitempty"`
}

// BucketStatus defines observed state.
type BucketStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	UsedBytes  int64              `json:"usedBytes,omitempty"`
	Objects    int64              `json:"objects,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=bk,categories=novanas
// +kubebuilder:subresource:status

// Bucket is a native S3 object bucket.
type Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BucketSpec   `json:"spec,omitempty"`
	Status            BucketStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BucketList contains a list of Bucket.
type BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bucket `json:"items"`
}

func init() { SchemeBuilder.Register(&Bucket{}, &BucketList{}) }
