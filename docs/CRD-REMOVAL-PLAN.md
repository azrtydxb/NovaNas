# CRD removal & runtime-adapter refactor — task list

Tracking doc for the follow-up to the doc-cleanup pass that landed
alongside the amended [ADR 0005](adr/0005-hide-kubernetes-behind-api.md).

**Goal:** delete every CRD and replace the kubebuilder operators with
runtime-neutral controllers driven by the API server, sitting on top of
a runtime adapter (k8s today, docker tomorrow). When done, replacing
k3s with Docker should be a config change, not a port.

The previous wave (#52, commit `80a1eef`) deleted 28 CRDs whose state
moved to Postgres. **This plan deletes the remaining 24 CRDs** and the
controller-runtime scaffolding that depends on them.

## Scope of code to remove

### CRD type packages

- [ ] `packages/operators/api/v1alpha1/` — every `*_types.go` (app,
  appinstance, blockvolume, bond, clusternetwork, customdomain,
  firewallrule, gpudevice, hostinterface, ingress, iscsitarget,
  nfsserver, nvmeoftarget, objectstore, physicalinterface,
  remoteaccesstunnel, servicepolicy, share, smbserver, sshkey,
  trafficpolicy, vippool, vlan, vm), `groupversion_info.go`,
  `common_types.go`, `iam_common_types.go`, `zz_generated.deepcopy.go`.
- [ ] `storage/api/v1alpha1/` — `types.go`, `groupversion_info.go`,
  `zz_generated.deepcopy.go`, `zz_generated_deepcopy_manual.go`.
  (Storage's CRD set is internal but still a CRD — goes too.)

### CRD manifests

- [ ] `packages/operators/config/crd/bases/novanas.io_*.yaml` (all 24
  files).
- [ ] Any `helm/charts/*/templates/*crd*` if added later. (Currently no
  `helm/charts/` directory; verify and re-check when it lands.)

### Controllers & reconcilers (reshape, don't just delete)

- [ ] `packages/operators/internal/controllers/*_controller.go` —
  rewrite each as a runtime-neutral controller that:
  1. Reads desired state from the NovaNas API server (Postgres-backed),
     not from a CRD watch.
  2. Computes runtime intent.
  3. Calls the runtime adapter to converge it.
  4. Writes observed status back to the API server.
- [ ] `packages/operators/internal/reconciler/*.go` — same shape; many
  of these (openbao, keycloak, certificates, keyprovisioner) are
  already mostly API-server-driven and need only the CRD-watch removal.
- [ ] `packages/operators/main.go` — drop `controller-runtime`
  manager; replace with a simple goroutine-per-controller supervisor
  that takes the API client + runtime adapter as constructor args.

### Runtime adapter (new package)

- [ ] `packages/runtime/` — new Go module exposing one interface (rough
  shape):

  ```go
  type Adapter interface {
      EnsureWorkload(ctx, WorkloadSpec)  (WorkloadStatus, error)
      DeleteWorkload(ctx, WorkloadRef)   error
      ObserveWorkload(ctx, WorkloadRef)  (WorkloadStatus, error)
      EnsureNetwork(ctx, NetworkSpec)    error
      ApplyHostNetwork(ctx, HostNetSpec) error
      // ... event stream, log tail, exec, etc.
  }
  ```

- [ ] `packages/runtime/k8s/` — first impl. Uses typed Pod / Deployment
  / Service / NetworkPolicy / etc. via `client-go`. **No CRDs**, only
  built-in Kubernetes kinds.
- [ ] `packages/runtime/docker/` — second impl. Uses `dockerode`
  equivalent / `docker/client` Go SDK. Maps the same `WorkloadSpec`
  onto containers, networks, volumes.
- [ ] Conformance test suite that runs every controller's unit tests
  against both adapter impls. Lives in `packages/runtime/conformance/`.

### CSI driver

- [ ] `packages/csi/` stays for the K8s adapter only. Mark as
  k8s-adapter-specific in build tags. Docker adapter mounts volumes
  directly on the host; no CSI abstraction needed there.

### Helm

- [ ] App-catalog Helm charts (`apps/`) stay — they are *packaging*,
  rendered by NovaNas at deploy time and translated through the
  runtime adapter, not deployed via `helm install` against
  kube-apiserver.
- [ ] Any system-level Helm chart for NovaNas itself (umbrella, any
  subchart that installs NovaNas components) is removed; the NovaNas
  appliance bootstraps via the runtime adapter directly. Vendored
  upstream charts (Keycloak, OpenBao, Postgres, Redis,
  Prometheus/Loki/Tempo/Grafana/Alloy) are repackaged as
  runtime-adapter manifests.

### Helm umbrella consumers

- [ ] Search the repo for `helm install`, `helm upgrade`,
  `helm template` invocations in CI / installer / OS layer; replace
  with runtime-adapter calls.

## Suggested PR sequence

1. **PR 1 — runtime adapter interface + k8s impl**. Empty docker impl
   that returns "not implemented". Conformance harness in place.
2. **PR 2 — controller refactor, one resource family at a time**.
   Order: simplest first.
   1. servicePolicy, firewallRule, trafficPolicy (config-only, no
      runtime workloads to materialize)
   2. bond, vlan, hostInterface, physicalInterface (host-agent already
      runtime-agnostic)
   3. share, smbServer, nfsServer, iscsiTarget, nvmeofTarget (single
      workload each)
   4. objectStore, ingress, vipPool, customDomain, remoteAccessTunnel
      (novaedge-driven)
   5. clusterNetwork (one-shot at install)
   6. app, appInstance (Helm-rendered → runtime adapter)
   7. vm (KubeVirt today, libvirt path planned for Docker adapter)
   8. gpuDevice, sshKey (host-agent reports)
   9. blockVolume (chunk-engine integration)
3. **PR 3 — delete CRD types + manifests**. Lands only after every
   controller in PR 2 has been migrated and tested.
4. **PR 4 — main.go & controller-runtime removal**. Replace with
   plain goroutine supervisor; drop `sigs.k8s.io/controller-runtime`
   from go.mod.
5. **PR 5 — Helm umbrella retirement**. Replace boot-time Helm with
   runtime-adapter manifests.
6. **PR 6 — docker adapter impl**. Conformance suite green; first end-
   to-end QEMU smoke against a Docker-backed appliance image.

## Non-goals

- Removing Kubernetes as a *supported* runtime. K8s stays as the
  default and best-tested adapter. Goal is parity with Docker, not
  replacement.
- Touching the data path. Chunk engine, dataplane, gRPC contracts,
  storage Postgres tables stay as-is.
- Touching the API surface. `/api/v1/*` routes do not change shape;
  this refactor only reshapes what runs *behind* the API.

## How we'll know it's done

- `git grep -i "kind: " packages storage helm` returns zero matches
  outside of comments and `apps/` (catalog charts).
- `git grep -E "novanas\.io/v1alpha1|kubebuilder" packages storage`
  returns zero matches.
- `go.mod` for the controllers module no longer imports
  `sigs.k8s.io/controller-runtime` or `k8s.io/apiextensions-apiserver`.
- `packages/runtime/conformance/` passes against both `k8s` and
  `docker` impls in CI.
- The first-boot installer can pick `k8s` or `docker` as the runtime
  and bring the appliance up to a green health-check on either.
