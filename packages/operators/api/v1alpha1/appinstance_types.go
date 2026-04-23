package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AppInstanceStorage binds an application volume slot to a backing resource.
type AppInstanceStorage struct {
	// Name is the slot name the chart refers to.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Dataset references a Dataset CR (mutually exclusive with BlockVolume/Bucket).
	Dataset     string `json:"dataset,omitempty"`
	BlockVolume string `json:"blockVolume,omitempty"`
	Bucket      string `json:"bucket,omitempty"`
	// Size is a bytes quantity; for datasets it's advisory, for block volumes required.
	Size string `json:"size,omitempty"`
	// +kubebuilder:validation:Enum=ReadWrite;ReadOnly
	Mode      string `json:"mode,omitempty"`
	MountPath string `json:"mountPath,omitempty"`
}

// AppInstanceTLS ties an exposed port to a Certificate resource.
type AppInstanceTLS struct {
	Certificate string `json:"certificate"`
}

// AppInstanceExpose opens a chart port through the cluster edge.
type AppInstanceExpose struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
	// +kubebuilder:validation:Enum=TCP;UDP
	Protocol string `json:"protocol,omitempty"`
	// +kubebuilder:validation:Enum=internal;lan;public
	Advertise string          `json:"advertise,omitempty"`
	TLS       *AppInstanceTLS `json:"tls,omitempty"`
	Hostname  string          `json:"hostname,omitempty"`
}

// AppInstanceNetwork groups network-level settings.
type AppInstanceNetwork struct {
	Expose []AppInstanceExpose `json:"expose,omitempty"`
}

// AppInstanceUpdates carries auto-update preferences.
type AppInstanceUpdates struct {
	AutoUpdate bool   `json:"autoUpdate,omitempty"`
	Channel    string `json:"channel,omitempty"`
}

// AppInstanceSpec defines the desired state of AppInstance.
type AppInstanceSpec struct {
	// App is the name of an App catalog entry (required).
	// +kubebuilder:validation:MinLength=1
	App string `json:"app"`
	// Version pins the app version. Empty means "track the App's current version".
	Version string `json:"version,omitempty"`
	// Values is a free-form chart values object (validated against App.schema).
	Values  *runtime.RawExtension `json:"values,omitempty"`
	Storage []AppInstanceStorage  `json:"storage,omitempty"`
	Network *AppInstanceNetwork   `json:"network,omitempty"`
	Updates *AppInstanceUpdates   `json:"updates,omitempty"`
}

// AppInstanceReplicaStatus mirrors appsv1.DeploymentStatus fields the UI wants.
type AppInstanceReplicaStatus struct {
	Desired     int32 `json:"desired"`
	Ready       int32 `json:"ready"`
	Updated     int32 `json:"updated"`
	Available   int32 `json:"available"`
	Unavailable int32 `json:"unavailable,omitempty"`
}

// AppInstanceStatus defines observed state of AppInstance.
type AppInstanceStatus struct {
	// +kubebuilder:validation:Enum=Pending;Running;Stopped;Failed;Updating
	Phase      string             `json:"phase,omitempty"`
	Healthy    bool               `json:"healthy,omitempty"`
	Revision   int64              `json:"revision,omitempty"`
	ExposedAt  string             `json:"exposedAt,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Replicas reports the total / desired / ready replica count of the
	// underlying Deployment so the dashboard can render rollout progress.
	Replicas *AppInstanceReplicaStatus `json:"replicas,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,categories=novanas,shortName=appinst
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="App",type=string,JSONPath=`.spec.app`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.replicas.ready`

// AppInstance — a user-installed app, rendered from a chart.
type AppInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AppInstanceSpec   `json:"spec,omitempty"`
	Status            AppInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AppInstanceList contains a list of AppInstance.
type AppInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppInstance `json:"items"`
}

func init() { SchemeBuilder.Register(&AppInstance{}, &AppInstanceList{}) }
