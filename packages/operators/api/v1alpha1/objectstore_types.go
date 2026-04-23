package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ObjectStoreTLS toggles listener TLS.
type ObjectStoreTLS struct {
	Enabled     bool   `json:"enabled"`
	Certificate string `json:"certificate,omitempty"`
}

// ObjectStoreFeatures enables/disables feature flags on the S3 listener.
type ObjectStoreFeatures struct {
	Versioning    bool `json:"versioning,omitempty"`
	ObjectLock    bool `json:"objectLock,omitempty"`
	Replication   bool `json:"replication,omitempty"`
	Website       bool `json:"website,omitempty"`
	Select        bool `json:"select,omitempty"`
	Notifications bool `json:"notifications,omitempty"`
}

// ObjectStoreSpec defines the desired state of ObjectStore.
type ObjectStoreSpec struct {
	// BindInterface is a host interface name; empty means all interfaces.
	BindInterface string `json:"bindInterface,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port     int32                `json:"port,omitempty"`
	TLS      *ObjectStoreTLS      `json:"tls,omitempty"`
	Region   string               `json:"region,omitempty"`
	Features *ObjectStoreFeatures `json:"features,omitempty"`
}

// ObjectStoreStatus defines observed state of ObjectStore.
type ObjectStoreStatus struct {
	// +kubebuilder:validation:Enum=Pending;Running;Failed
	Phase      string             `json:"phase,omitempty"`
	Endpoint   string             `json:"endpoint,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// TotalObjects is the S3 gateway's observed object count.
	TotalObjects int64 `json:"totalObjects,omitempty"`
	// UsedBytes is the current occupied capacity across all buckets served here.
	UsedBytes int64 `json:"usedBytes,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=novanas,shortName=os
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Port",type=integer,JSONPath=`.spec.port`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.endpoint`

// ObjectStore — S3 gateway service config.
type ObjectStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ObjectStoreSpec   `json:"spec,omitempty"`
	Status            ObjectStoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ObjectStoreList contains a list of ObjectStore.
type ObjectStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ObjectStore `json:"items"`
}

func init() { SchemeBuilder.Register(&ObjectStore{}, &ObjectStoreList{}) }
