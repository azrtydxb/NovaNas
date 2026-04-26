package runtime

import "time"

type Tenant string

type WorkloadRef struct {
	Tenant Tenant
	Name   string
}

type WorkloadKind string

const (
	WorkloadService         WorkloadKind = "service"
	WorkloadStatefulService WorkloadKind = "stateful-service"
	WorkloadDaemon          WorkloadKind = "daemon"
	WorkloadJob             WorkloadKind = "job"
)

type PrivilegeProfile string

const (
	PrivilegeRestricted PrivilegeProfile = "restricted"
	PrivilegeBaseline   PrivilegeProfile = "baseline"
	PrivilegePrivileged PrivilegeProfile = "privileged"
)

type WorkloadSpec struct {
	Ref       WorkloadRef
	Kind      WorkloadKind
	Privilege PrivilegeProfile

	Containers []ContainerSpec
	Volumes    []VolumeSpec
	Network    NetworkAttachment

	Replicas int

	Labels map[string]string
}

type ContainerSpec struct {
	Name         string
	Image        string
	Command      []string
	Args         []string
	Env          map[string]string
	Ports        []PortSpec
	VolumeMounts []VolumeMount
	Resources    ResourceRequirements
}

type PortSpec struct {
	Name          string
	ContainerPort int32
	Protocol      string
}

type VolumeSpec struct {
	Name   string
	Source VolumeSource
}

// VolumeSource is a tagged union: exactly one field must be non-nil.
// Adapters reject specs that violate this with ErrInvalidSpec.
type VolumeSource struct {
	EmptyDir    *EmptyDirSource
	Dataset     *DatasetSource
	BlockVolume *BlockVolumeSource
	HostPath    *HostPathSource
	Secret      *SecretSource
}

type EmptyDirSource struct {
	SizeBytes int64
}

type DatasetSource struct {
	Name     string
	ReadOnly bool
	SubPath  string
}

type BlockVolumeSource struct {
	Name     string
	ReadOnly bool
}

// HostPathSource is rejected unless the workload's PrivilegeProfile is
// PrivilegePrivileged. Validation lives in the adapter so all backends
// enforce identically.
type HostPathSource struct {
	Path string
}

type SecretSource struct {
	OpenBaoPath string
}

type VolumeMount struct {
	Name      string
	MountPath string
	ReadOnly  bool
}

type ResourceRequirements struct {
	CPURequestMilli int32
	CPULimitMilli   int32
	MemoryRequestMB int32
	MemoryLimitMB   int32
}

type NetworkAttachment struct {
	Network string
	Expose  []ExposeRule
}

type ExposeRule struct {
	PortName string
	Scope    string
}

type WorkloadStatus struct {
	Ref      WorkloadRef
	Phase    Phase
	Replicas ReplicaCounts
	Message  string
	Updated  time.Time
}

type Phase string

const (
	PhasePending     Phase = "pending"
	PhaseProgressing Phase = "progressing"
	PhaseReady       Phase = "ready"
	PhaseDegraded    Phase = "degraded"
	PhaseFailed      Phase = "failed"
	PhaseCompleted   Phase = "completed"
)

type ReplicaCounts struct {
	Desired int
	Ready   int
}

type NetworkSpec struct {
	Tenant   Tenant
	Name     string
	Internal bool
}

type ExecRequest struct {
	Ref       WorkloadRef
	Container string
	Command   []string
	TTY       bool
}

type LogOptions struct {
	Ref       WorkloadRef
	Container string
	Follow    bool
	TailLines int64
}

type VMRef struct {
	Tenant Tenant
	Name   string
}

// VMSpec is a runtime-neutral description of a virtual machine. Spec is
// kept as a free-form map so the K8s adapter can pass through KubeVirt
// VirtualMachineSpec shapes without the runtime package having to model
// every KubeVirt field. A typed surface can be added once a Docker
// (libvirt) adapter exists and the union of fields is known.
type VMSpec struct {
	Ref  VMRef
	Spec map[string]any
}

type VMPowerState string

const (
	VMRunning VMPowerState = "Running"
	VMStopped VMPowerState = "Stopped"
	VMPaused  VMPowerState = "Paused"
)

type VMStatus struct {
	Ref     VMRef
	Phase   VMPowerState
	Message string
}
