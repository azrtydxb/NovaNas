package k8s

import (
	"fmt"
	"strings"

	rt "github.com/azrtydxb/novanas/packages/runtime"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// labelTenant and labelWorkload are stamped on every emitted runtime
// object so DeleteWorkload can find them again and so an external
// watcher can correlate Pods/Services back to the API resource.
const (
	labelTenant   = "novanas.io/tenant"
	labelWorkload = "novanas.io/workload"
)

func ownerLabels(ref rt.WorkloadRef, extra map[string]string) map[string]string {
	out := map[string]string{
		labelTenant:   string(ref.Tenant),
		labelWorkload: ref.Name,
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func selector(ref rt.WorkloadRef) map[string]string {
	return map[string]string{
		labelTenant:   string(ref.Tenant),
		labelWorkload: ref.Name,
	}
}

func toDeployment(ns string, spec rt.WorkloadSpec) *appsv1.Deployment {
	replicas := int32(spec.Replicas)
	if replicas == 0 {
		replicas = 1
	}
	labels := ownerLabels(spec.Ref, spec.Labels)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Ref.Name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector(spec.Ref)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       toPodSpec(spec),
			},
		},
	}
}

func toJob(ns string, spec rt.WorkloadSpec) *batchv1.Job {
	labels := ownerLabels(spec.Ref, spec.Labels)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Ref.Name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       toPodSpec(spec),
			},
		},
	}
}

func toService(ns string, spec rt.WorkloadSpec) *corev1.Service {
	ports := []corev1.ServicePort{}
	for _, expose := range spec.Network.Expose {
		for _, c := range spec.Containers {
			for _, p := range c.Ports {
				if p.Name != expose.PortName {
					continue
				}
				ports = append(ports, corev1.ServicePort{
					Name:       p.Name,
					Port:       p.ContainerPort,
					TargetPort: intstr.FromInt32(p.ContainerPort),
					Protocol:   protocolFor(p.Protocol),
				})
			}
		}
	}
	if len(ports) == 0 {
		return nil
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Ref.Name,
			Namespace: ns,
			Labels:    ownerLabels(spec.Ref, spec.Labels),
		},
		Spec: corev1.ServiceSpec{
			Selector: selector(spec.Ref),
			Ports:    ports,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
}

func toPodSpec(spec rt.WorkloadSpec) corev1.PodSpec {
	ps := corev1.PodSpec{
		Containers:    make([]corev1.Container, 0, len(spec.Containers)),
		RestartPolicy: corev1.RestartPolicyAlways,
	}
	if spec.Kind == rt.WorkloadJob {
		ps.RestartPolicy = corev1.RestartPolicyOnFailure
	}
	if spec.Privilege == rt.PrivilegeRestricted {
		runAsNonRoot := true
		ps.SecurityContext = &corev1.PodSecurityContext{RunAsNonRoot: &runAsNonRoot}
	}
	for _, v := range spec.Volumes {
		ps.Volumes = append(ps.Volumes, toVolume(v))
	}
	for _, c := range spec.Containers {
		ps.Containers = append(ps.Containers, toContainer(c))
	}
	return ps
}

func toContainer(c rt.ContainerSpec) corev1.Container {
	cont := corev1.Container{
		Name:    c.Name,
		Image:   c.Image,
		Command: c.Command,
		Args:    c.Args,
	}
	for k, v := range c.Env {
		cont.Env = append(cont.Env, corev1.EnvVar{Name: k, Value: v})
	}
	for _, p := range c.Ports {
		cont.Ports = append(cont.Ports, corev1.ContainerPort{
			Name:          p.Name,
			ContainerPort: p.ContainerPort,
			Protocol:      protocolFor(p.Protocol),
		})
	}
	for _, vm := range c.VolumeMounts {
		cont.VolumeMounts = append(cont.VolumeMounts, corev1.VolumeMount{
			Name:      vm.Name,
			MountPath: vm.MountPath,
			ReadOnly:  vm.ReadOnly,
		})
	}
	cont.Resources = toResourceRequirements(c.Resources)
	return cont
}

func toResourceRequirements(r rt.ResourceRequirements) corev1.ResourceRequirements {
	out := corev1.ResourceRequirements{}
	if r.CPURequestMilli > 0 || r.MemoryRequestMB > 0 {
		out.Requests = corev1.ResourceList{}
		if r.CPURequestMilli > 0 {
			out.Requests[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(r.CPURequestMilli), resource.DecimalSI)
		}
		if r.MemoryRequestMB > 0 {
			out.Requests[corev1.ResourceMemory] = *resource.NewQuantity(int64(r.MemoryRequestMB)*1024*1024, resource.BinarySI)
		}
	}
	if r.CPULimitMilli > 0 || r.MemoryLimitMB > 0 {
		out.Limits = corev1.ResourceList{}
		if r.CPULimitMilli > 0 {
			out.Limits[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(r.CPULimitMilli), resource.DecimalSI)
		}
		if r.MemoryLimitMB > 0 {
			out.Limits[corev1.ResourceMemory] = *resource.NewQuantity(int64(r.MemoryLimitMB)*1024*1024, resource.BinarySI)
		}
	}
	return out
}

// toVolume maps a runtime VolumeSource onto a Kubernetes Volume. Only
// the source variants the K8s adapter supports today are handled here;
// Dataset/BlockVolume are deferred until the storage data plane lands
// PVCs (#50).
func toVolume(v rt.VolumeSpec) corev1.Volume {
	out := corev1.Volume{Name: v.Name}
	switch {
	case v.Source.EmptyDir != nil:
		ed := &corev1.EmptyDirVolumeSource{}
		if v.Source.EmptyDir.SizeBytes > 0 {
			q := *resource.NewQuantity(v.Source.EmptyDir.SizeBytes, resource.BinarySI)
			ed.SizeLimit = &q
		}
		out.VolumeSource = corev1.VolumeSource{EmptyDir: ed}
	case v.Source.HostPath != nil:
		out.VolumeSource = corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{Path: v.Source.HostPath.Path},
		}
	case v.Source.Secret != nil:
		// OpenBao Agent Injector is the actual mount path; here we
		// emit a placeholder Secret name. Wiring to OpenBao is in a
		// later PR alongside the AppInstance migration.
		secretName := openBaoPathToSecretName(v.Source.Secret.OpenBaoPath)
		out.VolumeSource = corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: secretName},
		}
	default:
		// Dataset / BlockVolume — emit an EmptyDir placeholder so the
		// Pod still validates. Replaced with real PVCs in #50.
		out.VolumeSource = corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}
	}
	return out
}

func openBaoPathToSecretName(path string) string {
	s := strings.TrimPrefix(path, "openbao://")
	s = strings.ReplaceAll(s, "/", "-")
	if s == "" {
		s = "openbao-secret"
	}
	return s
}

func protocolFor(p string) corev1.Protocol {
	switch strings.ToUpper(p) {
	case "UDP":
		return corev1.ProtocolUDP
	default:
		return corev1.ProtocolTCP
	}
}

func toNetworkPolicy(ns string, spec rt.NetworkSpec) *networkingv1.NetworkPolicy {
	policyTypes := []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: ns,
			Labels: map[string]string{
				labelTenant: string(spec.Tenant),
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: policyTypes,
		},
	}
	if !spec.Internal {
		np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
	}
	return np
}

// validateForK8s checks invariants the adapter cannot represent. Runs
// before the runtime-neutral validateSpec to surface the K8s-specific
// rejection cases in conformance.
func validateForK8s(spec rt.WorkloadSpec) error {
	switch spec.Kind {
	case rt.WorkloadService, rt.WorkloadJob:
	case rt.WorkloadStatefulService, rt.WorkloadDaemon:
		return fmt.Errorf("%w: %s not yet supported by k8s adapter", rt.ErrInvalidSpec, spec.Kind)
	default:
		return fmt.Errorf("%w: workload kind required", rt.ErrInvalidSpec)
	}
	return nil
}
