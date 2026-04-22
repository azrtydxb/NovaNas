# @novanas/operators

Kubernetes controllers for every NovaNas CRD under `novanas.io/v1alpha1`.

This is a scaffold: every reconciler is a no-op that logs the request and
returns `ctrl.Result{}, nil`. Real logic lands in Wave 4+.

## Layout

- `api/v1alpha1/` — Go types for all CRD kinds, plus a hand-rolled deep-copy
  stub. Replace with `controller-gen object` output when wired into the build.
- `internal/controllers/` — one controller per kind.
- `internal/reconciler/` — shared `BaseReconciler`.
- `internal/logging/` — zap + logr bootstrap.
- `internal/metrics/` — Prometheus registration.
- `config/rbac/role.yaml` — minimal cluster role.
- `config/manager/manager.yaml` — operator Deployment + SA + binding.
- `config/crd/` — placeholder; real CRD YAMLs are generated from the types
  later via `controller-gen crd`.

## Build

```sh
cd packages/operators
go build ./...
```

## Run locally

```sh
KUBECONFIG=~/.kube/config ./manager --development --leader-elect=false
```

The manager exposes:

- `:8080` — Prometheus metrics (`/metrics`)
- `:8081` — liveness (`/healthz`) and readiness (`/readyz`)

## Regenerating deep-copy and CRD manifests

Real deep-copy methods and CRD YAML are produced by
[`controller-gen`](https://book.kubebuilder.io/reference/controller-gen.html).
Until the Makefile target lands in a later wave, run it by hand:

```sh
go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
controller-gen object paths=./api/v1alpha1/...
controller-gen crd paths=./api/v1alpha1/... output:crd:dir=./config/crd
```

The `zz_generated.deepcopy.go` checked in today is a shallow-copy stub
sufficient for the scheme to accept the types at registration time.

## Container image

```sh
docker build -t novanas/operators:dev .
```

Multi-stage: `golang:1.23` to build, `distroless/static:nonroot` for runtime.
