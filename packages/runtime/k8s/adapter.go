package k8s

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	rt "github.com/azrtydxb/novanas/packages/runtime"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
)

// Adapter implements runtime.Adapter against a Kubernetes cluster.
//
// Tenant projection: each runtime tenant maps to a namespace named
// "<NamespacePrefix><tenant>". Defaults to "novanas-".
type Adapter struct {
	cs              kubernetes.Interface
	dyn             dynamic.Interface
	cfg             *rest.Config
	executor        execFactory
	NamespacePrefix string

	mu       sync.Mutex
	watchers map[rt.Tenant][]chan rt.Event
}

// execFactory wraps remotecommand.NewSPDYExecutor so tests can swap
// in a fake. Production wiring goes through realExecFactory below.
type execFactory func(cfg *rest.Config, method string, url *url.URL) (remotecommand.Executor, error)

func New(cs kubernetes.Interface) *Adapter {
	return &Adapter{
		cs:              cs,
		NamespacePrefix: "novanas-",
		watchers:        make(map[rt.Tenant][]chan rt.Event),
		executor:        realExecFactory,
	}
}

// WithDynamicClient wires the dynamic client used for resources the
// adapter does not statically type (KubeVirt VirtualMachine today).
// Splitting it from the constructor keeps the conformance suite —
// which only exercises typed objects — usable with just a fake
// kubernetes.Interface.
func (a *Adapter) WithDynamicClient(dyn dynamic.Interface) *Adapter {
	a.dyn = dyn
	return a
}

// WithRestConfig wires the *rest.Config required for streaming
// endpoints (Logs, Exec). The conformance suite uses a fake clientset
// and skips Logs/Exec, so this is opt-in.
func (a *Adapter) WithRestConfig(cfg *rest.Config) *Adapter {
	a.cfg = cfg
	return a
}

// WithExecutor lets tests substitute the SPDY executor factory.
func (a *Adapter) WithExecutor(f execFactory) *Adapter {
	a.executor = f
	return a
}

func realExecFactory(cfg *rest.Config, method string, u *url.URL) (remotecommand.Executor, error) {
	return remotecommand.NewSPDYExecutor(cfg, method, u)
}

func (a *Adapter) Name() string { return "k8s" }

func (a *Adapter) namespace(t rt.Tenant) string {
	return a.NamespacePrefix + string(t)
}

func (a *Adapter) EnsureTenant(ctx context.Context, t rt.Tenant) error {
	if t == "" {
		return fmt.Errorf("%w: empty tenant", rt.ErrInvalidSpec)
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: a.namespace(t)}}
	_, err := a.cs.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (a *Adapter) DeleteTenant(ctx context.Context, t rt.Tenant) error {
	nsName := a.namespace(t)
	ds, err := a.cs.AppsV1().Deployments(nsName).List(ctx, metav1.ListOptions{Limit: 1})
	if err == nil && len(ds.Items) > 0 {
		return fmt.Errorf("%w: tenant %q has workloads", rt.ErrInvalidSpec, t)
	}
	js, err := a.cs.BatchV1().Jobs(nsName).List(ctx, metav1.ListOptions{Limit: 1})
	if err == nil && len(js.Items) > 0 {
		return fmt.Errorf("%w: tenant %q has workloads", rt.ErrInvalidSpec, t)
	}
	if err := a.cs.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func (a *Adapter) EnsureNetwork(ctx context.Context, spec rt.NetworkSpec) error {
	if spec.Name == "" || spec.Tenant == "" {
		return fmt.Errorf("%w: network name and tenant required", rt.ErrInvalidSpec)
	}
	nsName := a.namespace(spec.Tenant)
	if _, err := a.cs.CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: tenant %q not found", rt.ErrNotFound, spec.Tenant)
		}
		return err
	}
	np := toNetworkPolicy(nsName, spec)
	_, err := a.cs.NetworkingV1().NetworkPolicies(nsName).Create(ctx, np, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (a *Adapter) DeleteNetwork(ctx context.Context, t rt.Tenant, name string) error {
	nsName := a.namespace(t)
	pods, err := a.cs.CoreV1().Pods(nsName).List(ctx, metav1.ListOptions{
		LabelSelector: labelTenant + "=" + string(t),
		Limit:         1,
	})
	// Best-effort attached-check: any pods labelled for this tenant
	// count as "still attached" so the network can't be ripped from
	// under live workloads.
	if err == nil && len(pods.Items) > 0 {
		return fmt.Errorf("%w: network %q still attached", rt.ErrInvalidSpec, name)
	}
	if err := a.cs.NetworkingV1().NetworkPolicies(nsName).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func (a *Adapter) EnsureWorkload(ctx context.Context, spec rt.WorkloadSpec) (rt.WorkloadStatus, error) {
	if err := validateSpec(spec); err != nil {
		return rt.WorkloadStatus{}, err
	}
	if err := validateForK8s(spec); err != nil {
		return rt.WorkloadStatus{}, err
	}
	nsName := a.namespace(spec.Ref.Tenant)
	if _, err := a.cs.CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return rt.WorkloadStatus{}, fmt.Errorf("%w: tenant %q not found", rt.ErrNotFound, spec.Ref.Tenant)
		}
		return rt.WorkloadStatus{}, err
	}

	switch spec.Kind {
	case rt.WorkloadService:
		dep := toDeployment(nsName, spec)
		if _, err := a.cs.AppsV1().Deployments(nsName).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return rt.WorkloadStatus{}, err
			}
			if _, err := a.cs.AppsV1().Deployments(nsName).Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
				return rt.WorkloadStatus{}, err
			}
		}
		if svc := toService(nsName, spec); svc != nil {
			if _, err := a.cs.CoreV1().Services(nsName).Create(ctx, svc, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
				return rt.WorkloadStatus{}, err
			}
		}
	case rt.WorkloadJob:
		j := toJob(nsName, spec)
		if _, err := a.cs.BatchV1().Jobs(nsName).Create(ctx, j, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return rt.WorkloadStatus{}, err
		}
	default:
		return rt.WorkloadStatus{}, fmt.Errorf("%w: kind %s", rt.ErrInvalidSpec, spec.Kind)
	}

	status := initialStatus(spec)
	a.fanout(spec.Ref.Tenant, rt.Event{Ref: spec.Ref, Status: status})
	return status, nil
}

func (a *Adapter) DeleteWorkload(ctx context.Context, ref rt.WorkloadRef) error {
	nsName := a.namespace(ref.Tenant)
	if err := a.cs.AppsV1().Deployments(nsName).Delete(ctx, ref.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := a.cs.CoreV1().Services(nsName).Delete(ctx, ref.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := a.cs.BatchV1().Jobs(nsName).Delete(ctx, ref.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	a.fanout(ref.Tenant, rt.Event{Ref: ref, Status: rt.WorkloadStatus{Ref: ref, Phase: rt.PhaseFailed, Message: "deleted"}})
	return nil
}

func (a *Adapter) ObserveWorkload(ctx context.Context, ref rt.WorkloadRef) (rt.WorkloadStatus, error) {
	nsName := a.namespace(ref.Tenant)
	if dep, err := a.cs.AppsV1().Deployments(nsName).Get(ctx, ref.Name, metav1.GetOptions{}); err == nil {
		desired := int(*dep.Spec.Replicas)
		ready := int(dep.Status.ReadyReplicas)
		phase := rt.PhaseProgressing
		if ready >= desired && desired > 0 {
			phase = rt.PhaseReady
		}
		return rt.WorkloadStatus{
			Ref:      ref,
			Phase:    phase,
			Replicas: rt.ReplicaCounts{Desired: desired, Ready: ready},
		}, nil
	}
	if j, err := a.cs.BatchV1().Jobs(nsName).Get(ctx, ref.Name, metav1.GetOptions{}); err == nil {
		phase := rt.PhaseProgressing
		if j.Status.Succeeded > 0 {
			phase = rt.PhaseCompleted
		} else if j.Status.Failed > 0 {
			phase = rt.PhaseFailed
		}
		return rt.WorkloadStatus{Ref: ref, Phase: phase}, nil
	}
	return rt.WorkloadStatus{}, rt.ErrNotFound
}

// Logs streams a workload's container log into out. We resolve the
// workload to its Pod by label-selector (the same labels EnsureWorkload
// stamps), then call CoreV1().Pods.GetLogs on the first match. Picking
// the first replica is a deliberate simplification — multi-pod log
// fanout belongs to a higher layer.
func (a *Adapter) Logs(ctx context.Context, opts rt.LogOptions, out io.Writer) error {
	nsName := a.namespace(opts.Ref.Tenant)
	pods, err := a.cs.CoreV1().Pods(nsName).List(ctx, metav1.ListOptions{
		LabelSelector: labelWorkload + "=" + opts.Ref.Name,
		Limit:         1,
	})
	if err != nil {
		return err
	}
	if len(pods.Items) == 0 {
		return rt.ErrNotFound
	}
	pod := pods.Items[0]
	logOpts := &corev1.PodLogOptions{Follow: opts.Follow, Container: opts.Container}
	if opts.TailLines > 0 {
		logOpts.TailLines = &opts.TailLines
	}
	req := a.cs.CoreV1().Pods(nsName).GetLogs(pod.Name, logOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	_, err = io.Copy(out, stream)
	return err
}

// Exec runs a one-shot command in the named workload's first matching
// pod. Streams stdout / stderr through the writers. Requires a
// *rest.Config (WithRestConfig) since SPDY streaming bypasses the
// typed clientset.
func (a *Adapter) Exec(ctx context.Context, req rt.ExecRequest, stdout, stderr io.Writer) (int, error) {
	if a.cfg == nil {
		return -1, fmt.Errorf("k8s adapter: rest.Config required for Exec (call WithRestConfig)")
	}
	nsName := a.namespace(req.Ref.Tenant)
	pods, err := a.cs.CoreV1().Pods(nsName).List(ctx, metav1.ListOptions{
		LabelSelector: labelWorkload + "=" + req.Ref.Name,
		Limit:         1,
	})
	if err != nil {
		return -1, err
	}
	if len(pods.Items) == 0 {
		return -1, rt.ErrNotFound
	}
	pod := pods.Items[0]
	container := req.Container
	if container == "" && len(pod.Spec.Containers) > 0 {
		container = pod.Spec.Containers[0].Name
	}
	host := strings.TrimRight(a.cfg.Host, "/")
	u, err := url.Parse(fmt.Sprintf("%s/api/v1/namespaces/%s/pods/%s/exec", host, nsName, pod.Name))
	if err != nil {
		return -1, err
	}
	q := u.Query()
	if container != "" {
		q.Set("container", container)
	}
	for _, c := range req.Command {
		q.Add("command", c)
	}
	if stdout != nil {
		q.Set("stdout", "true")
	}
	if stderr != nil {
		q.Set("stderr", "true")
	}
	if req.TTY {
		q.Set("tty", "true")
	}
	u.RawQuery = q.Encode()
	executor, err := a.executor(a.cfg, "POST", u)
	if err != nil {
		return -1, err
	}
	streamErr := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
		Tty:    req.TTY,
	})
	if streamErr != nil {
		var codeErr utilexec.CodeExitError
		if errors.As(streamErr, &codeErr) {
			return codeErr.Code, nil
		}
		return -1, streamErr
	}
	return 0, nil
}

func (a *Adapter) WatchEvents(ctx context.Context, t rt.Tenant) (<-chan rt.Event, error) {
	ch := make(chan rt.Event, 16)
	a.mu.Lock()
	a.watchers[t] = append(a.watchers[t], ch)
	a.mu.Unlock()

	nsName := a.namespace(t)
	depW, derr := a.cs.AppsV1().Deployments(nsName).Watch(ctx, metav1.ListOptions{})
	jobW, jerr := a.cs.BatchV1().Jobs(nsName).Watch(ctx, metav1.ListOptions{})

	go func() {
		<-ctx.Done()
		if depW != nil {
			depW.Stop()
		}
		if jobW != nil {
			jobW.Stop()
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		watchers := a.watchers[t]
		for i, w := range watchers {
			if w == ch {
				a.watchers[t] = append(watchers[:i], watchers[i+1:]...)
				break
			}
		}
		close(ch)
	}()

	if derr == nil {
		go a.bridgeWatch(ctx, depW, t, "Deployment")
	}
	if jerr == nil {
		go a.bridgeWatch(ctx, jobW, t, "Job")
	}

	return ch, nil
}

func (a *Adapter) bridgeWatch(ctx context.Context, w watch.Interface, t rt.Tenant, kind string) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.ResultChan():
			if !ok {
				return
			}
			obj, _ := ev.Object.(metav1.Object)
			if obj == nil {
				continue
			}
			labels := obj.GetLabels()
			name := labels[labelWorkload]
			if name == "" {
				name = obj.GetName()
			}
			ref := rt.WorkloadRef{Tenant: t, Name: name}
			phase := rt.PhaseProgressing
			if ev.Type == watch.Deleted {
				phase = rt.PhaseFailed
			}
			_ = kind
			a.fanout(t, rt.Event{Ref: ref, Status: rt.WorkloadStatus{Ref: ref, Phase: phase}})
		}
	}
}

// fanout: holds a.mu briefly. Drops events on full receivers; the
// memory adapter's "best-effort" semantics carry over so conformance
// asserts only at-least-once delivery within the test's timeout.
func (a *Adapter) fanout(t rt.Tenant, ev rt.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, ch := range a.watchers[t] {
		select {
		case ch <- ev:
		default:
		}
	}
}

func initialStatus(spec rt.WorkloadSpec) rt.WorkloadStatus {
	desired := spec.Replicas
	if desired == 0 {
		desired = 1
	}
	phase := rt.PhaseProgressing
	if spec.Kind == rt.WorkloadJob {
		phase = rt.PhaseProgressing
	}
	return rt.WorkloadStatus{
		Ref:      spec.Ref,
		Phase:    phase,
		Replicas: rt.ReplicaCounts{Desired: desired},
	}
}

// validateSpec mirrors the runtime-neutral checks the memory adapter
// already enforces. Centralised here so all adapters reject the same
// inputs.
func validateSpec(spec rt.WorkloadSpec) error {
	if spec.Ref.Name == "" || spec.Ref.Tenant == "" {
		return fmt.Errorf("%w: workload name and tenant required", rt.ErrInvalidSpec)
	}
	if spec.Kind == "" {
		return fmt.Errorf("%w: workload kind required", rt.ErrInvalidSpec)
	}
	if len(spec.Containers) == 0 {
		return fmt.Errorf("%w: at least one container required", rt.ErrInvalidSpec)
	}
	for _, v := range spec.Volumes {
		if err := validateVolumeSource(v.Source, spec.Privilege); err != nil {
			return fmt.Errorf("volume %q: %w", v.Name, err)
		}
	}
	if strings.TrimSpace(spec.Ref.Name) == "" {
		return fmt.Errorf("%w: workload name must not be whitespace", rt.ErrInvalidSpec)
	}
	return nil
}

func validateVolumeSource(src rt.VolumeSource, profile rt.PrivilegeProfile) error {
	count := 0
	if src.EmptyDir != nil {
		count++
	}
	if src.Dataset != nil {
		count++
	}
	if src.BlockVolume != nil {
		count++
	}
	if src.HostPath != nil {
		count++
	}
	if src.Secret != nil {
		count++
	}
	if count != 1 {
		return errors.New("exactly one volume source must be set")
	}
	if src.HostPath != nil && profile != rt.PrivilegePrivileged {
		return fmt.Errorf("%w: hostPath requires privileged profile", rt.ErrInvalidSpec)
	}
	return nil
}
