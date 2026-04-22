# ADR 0001: Kubernetes (k3s) as the base

**Status:** Accepted
**Date:** 2026-04-22

## Context

NovaNas targets a single-node NAS appliance that must run user containers
and VMs alongside block, file, and object storage. The orchestration layer
has to schedule and isolate a heterogeneous mix of system services,
user-deployed apps, and VMs; provide a stable API for operators to
reconcile declarative state; and give us a credible path for networking,
storage CSI, and identity integration. It also has to work on modest
hardware: QNAP / TrueNAS-SCALE class boxes with as little as a handful of
cores and 8–16 GB of RAM.

Harvester and TrueNAS SCALE demonstrate that a Kubernetes-based appliance
is viable at this form factor, including on consumer-grade hardware, and
the ecosystem around CSI, CNI, CRDs, and KubeVirt lets us reuse enormous
amounts of prior art rather than reinventing orchestration, scheduling,
and workload lifecycle.

## Decision

NovaNas uses **k3s** (single-node, embedded etcd) as its orchestration
layer. All system services and all user workloads run as pods or KubeVirt
VMs. Kubernetes is treated as an *implementation detail*: it is never
exposed directly in the UI, and the API server is the sole user-facing
control plane. Clustering across boxes is explicitly out of scope for v1.

## Alternatives considered

- **No orchestrator, plain systemd + containers** — works for a fixed
  service set but falls down for user apps/VMs, CRD-style declarative
  reconciliation, and CSI/CNI reuse.
- **Full upstream Kubernetes** — overkill for a single node; higher
  resource overhead, more moving parts.
- **Nomad** — smaller surface but much thinner ecosystem for CSI, CRDs,
  KubeVirt, and operator-pattern controllers.
- **Custom orchestrator** — rewrites the world; huge opportunity cost.

## Consequences

- Positive: leverages the Kubernetes ecosystem (CRDs, controller-runtime,
  CSI, CNI, KubeVirt, Helm, Prometheus operator).
- Positive: operators-as-controllers gives a clean reconciliation model
  for every declarative resource in NovaNas.
- Negative: k3s and its dependencies sit in the critical boot path; we
  own the ops burden even if users never see it.
- Negative: Kubernetes concepts must be hidden carefully; raw YAML must
  never leak into the UX (see ADR 0005).
- Neutral: no cluster/HA story in v1; any multi-node ambition is a new
  design exercise.

## References

- [docs/01-architecture-overview.md](../01-architecture-overview.md)
- [docs/14-decision-log.md](../14-decision-log.md) — F1, F2, F3
- ADR 0005 — Hide Kubernetes behind the NovaNas API
