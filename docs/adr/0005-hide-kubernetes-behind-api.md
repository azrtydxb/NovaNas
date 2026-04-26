# ADR 0005: API server is the sole control plane; runtime is swappable

**Status:** Accepted (2026-04-22). Amended 2026-04-26 to forbid CRDs entirely and require a runtime-pluggable architecture.
**Date:** 2026-04-22 / amended 2026-04-26

## Context

NovaNas is sold and experienced as a NAS appliance, not as a Kubernetes
platform. Its users are admins and end users of storage services, not
cluster operators. Exposing `kubectl`, YAML, or CRD semantics directly in
the UI or CLI would:

- Bleed cluster-shaped concepts (pods, namespaces, StatefulSets) into a
  domain where users think in pools, datasets, shares, apps, and VMs.
- Force Kubernetes RBAC — which is coarse and cluster-scoped by default
  — to carry authorization for fine-grained, per-resource operations it
  was not designed for.
- Prevent us from shaping composite operations (e.g., "create a Dataset
  with a Share and a default Snapshot schedule") as single transactions
  with coherent validation and audit.
- Make future refactors difficult: anything users have scripted against
  the raw K8s API becomes a backwards-compatibility constraint.

## Decision

The **NovaNas API server is the sole control plane and the sole source
of truth**. Every admin UI action, CLI command, and SDK call goes through
`/api/v1/*` (Fastify, Zod-validated REST + WebSocket). All business state
— pools, volumes, datasets, shares, snapshots, replication, apps, VMs,
networking, identity, alerting, settings — lives in Postgres, owned by
the API server.

The API server is also the **authorization boundary**: it authenticates
users via Keycloak OIDC, applies fine-grained policy, persists desired
state in Postgres, and streams status back out via WebSocket.

`novanasctl` talks to this API — not to kube-apiserver. The web UI has
no raw YAML editor.

### No CRDs anywhere

NovaNas defines **zero Custom Resource Definitions** in its own code.
CRDs were the original plumbing inherited from NovaStor, but they
- couple the data model to a Kubernetes-specific extension API,
- introduce a second source of truth (CRD spec/status) that drifts from
  Postgres,
- prevent the system from running on a non-Kubernetes runtime.

All resources — including pools, volumes, shares, disks, datasets,
snapshots, apps, VMs, network primitives, and protocol targets — are
**API-server-owned objects backed by Postgres**. Operators are replaced
by **runtime-neutral controllers** that read desired state from the API
server and converge it by emitting runtime-native objects (Pods,
Deployments, Services, NetworkPolicies on Kubernetes; equivalent
container/network primitives on Docker) through a **runtime adapter**.

### Runtime is swappable

Kubernetes (k3s) is the default runtime, but it is not load-bearing.
The system must run on Docker (or any other OCI runtime) by switching
the runtime adapter. The architectural test: *if Kubernetes is replaced
with Docker, the user-visible API and behaviour stay the same; only the
runtime adapter changes.* This rules out:
- CRDs (Kubernetes-specific)
- kubectl-driven flows in any internal component
- Reconcilers that watch kube-apiserver as their input
- Helm charts as the source of resource definitions for NovaNas's own
  workloads (Helm may still be used for vendored upstream components,
  but the runtime adapter, not Helm, drives NovaNas's runtime objects)

## Alternatives considered

- **Thin UI wrapping kubectl/YAML** — fastest to build, worst UX; see
  Rancher.
- **Expose K8s API directly with RBAC** — pushes authorization into a
  model that does not express NovaNas's intent.
- **Two APIs (domain + raw YAML escape hatch)** — splits the contract,
  doubles the audit surface, encourages users to route around the
  domain API.
- **Keep CRDs as "internal-only" plumbing** — considered and rejected.
  CRDs as internal plumbing still couple the system to Kubernetes,
  still create a second source of truth (CRD spec drifts from Postgres),
  and still block the runtime-pluggable goal. The boundary is therefore
  not "users don't see CRDs" but "no CRDs exist at all".

## Consequences

- Positive: consistent domain-shaped UX; users never see cluster
  concepts.
- Positive: API owns composite operations, validation, and audit.
- Positive: runtime is swappable — Kubernetes today, Docker or other
  OCI runtimes tomorrow, no rewrite of the data model.
- Positive: a single source of truth (Postgres) — no spec/status drift
  between two stores.
- Negative: every feature must pass through the API — there is no
  "bypass for power users" in the normal flow.
- Negative: API server is on the critical path for every user operation
  and must be robust and fast.
- Negative: requires a runtime adapter abstraction over container
  runtime primitives (pods/containers, services, networking). Adds an
  internal interface to maintain.
- Neutral: a documented read-only `kubectl` (or `docker ps`) escape
  hatch exists for deep debugging by engineers, but is not an official
  user interface.

## References

- [docs/09-ui-and-api.md](../09-ui-and-api.md)
- [docs/14-decision-log.md](../14-decision-log.md) — U4, U7, U15, T5
- ADR 0001 — Kubernetes base
