package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// VmOS describes the guest OS family.
type VmOS struct {
	// +kubebuilder:validation:Enum=linux;windows;other
	Type    string `json:"type"`
	Variant string `json:"variant,omitempty"`
}

// VmResources captures CPU and memory allocation.
type VmResources struct {
	// +kubebuilder:validation:Minimum=1
	CPU int32 `json:"cpu"`
	// +kubebuilder:validation:Minimum=1
	MemoryMiB int32 `json:"memoryMiB"`
	Sockets   int32 `json:"sockets,omitempty"`
	Cores     int32 `json:"cores,omitempty"`
	Threads   int32 `json:"threads,omitempty"`
}

// VmDiskSource is a discriminated union; exactly one of Dataset / BlockVolume
// / ISO / Clone should be populated. Type selects which.
type VmDiskSource struct {
	// +kubebuilder:validation:Enum=dataset;blockVolume;iso;clone
	Type        string `json:"type"`
	Dataset     string `json:"dataset,omitempty"`
	Size        string `json:"size,omitempty"`
	BlockVolume string `json:"blockVolume,omitempty"`
	IsoLibrary  string `json:"isoLibrary,omitempty"`
	SourceVM    string `json:"sourceVm,omitempty"`
	SourceDisk  string `json:"sourceDisk,omitempty"`
}

// VmDisk is a virtual disk attachment.
type VmDisk struct {
	// +kubebuilder:validation:MinLength=1
	Name   string       `json:"name"`
	Source VmDiskSource `json:"source"`
	// +kubebuilder:validation:Enum=virtio;scsi;sata;ide
	Bus      string `json:"bus,omitempty"`
	Boot     int32  `json:"boot,omitempty"`
	ReadOnly bool   `json:"readOnly,omitempty"`
}

// VmCdromSource identifies the ISO-backed source for a CD-ROM.
type VmCdromSource struct {
	// +kubebuilder:validation:Enum=iso
	Type       string `json:"type"`
	IsoLibrary string `json:"isoLibrary,omitempty"`
}

// VmCdrom is a CD-ROM attachment backed by an entry in an IsoLibrary.
type VmCdrom struct {
	// +kubebuilder:validation:MinLength=1
	Name   string        `json:"name"`
	Source VmCdromSource `json:"source"`
}

// VmNetwork attaches the VM to a host bridge / pod network / masquerade.
type VmNetwork struct {
	// +kubebuilder:validation:Enum=bridge;pod;masquerade
	Type   string `json:"type"`
	Bridge string `json:"bridge,omitempty"`
	MAC    string `json:"mac,omitempty"`
	// +kubebuilder:validation:Enum=virtio;e1000;rtl8139
	Model string `json:"model,omitempty"`
}

// VmGpuPassthroughEntry pins a specific PCI device to the VM.
type VmGpuPassthroughEntry struct {
	Vendor     string `json:"vendor,omitempty"`
	Device     string `json:"device"`
	DeviceName string `json:"deviceName,omitempty"`
}

// VmGpu configures passthrough GPU assignment.
type VmGpu struct {
	Passthrough []VmGpuPassthroughEntry `json:"passthrough,omitempty"`
}

// VmGraphics toggles the console (SPICE/VNC).
type VmGraphics struct {
	Enabled bool `json:"enabled"`
	// +kubebuilder:validation:Enum=spice;vnc
	Type string `json:"type,omitempty"`
}

// VmSpec defines the desired state of Vm.
type VmSpec struct {
	Owner     string      `json:"owner,omitempty"`
	OS        VmOS        `json:"os"`
	Resources VmResources `json:"resources"`
	Disks     []VmDisk    `json:"disks,omitempty"`
	Cdrom     []VmCdrom   `json:"cdrom,omitempty"`
	Network   []VmNetwork `json:"network,omitempty"`
	GPU       *VmGpu      `json:"gpu,omitempty"`
	Graphics  *VmGraphics `json:"graphics,omitempty"`
	// +kubebuilder:validation:Enum=never;onBoot;always
	Autostart string `json:"autostart,omitempty"`
	// PowerState is the desired runtime state.
	// +kubebuilder:validation:Enum=Running;Stopped;Paused
	PowerState string `json:"powerState,omitempty"`
}

// VmStatus defines observed state of Vm.
type VmStatus struct {
	// +kubebuilder:validation:Enum=Pending;Running;Stopped;Paused;Failed
	Phase      string             `json:"phase,omitempty"`
	ConsoleURL string             `json:"consoleUrl,omitempty"`
	IP         string             `json:"ip,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,categories=novanas
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Power",type=string,JSONPath=`.spec.powerState`
// +kubebuilder:printcolumn:name="IP",type=string,JSONPath=`.status.ip`

// Vm — KubeVirt VirtualMachine with NAS-friendly UX.
type Vm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VmSpec   `json:"spec,omitempty"`
	Status            VmStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VmList contains a list of Vm.
type VmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Vm `json:"items"`
}

func init() { SchemeBuilder.Register(&Vm{}, &VmList{}) }
