# NovaNas — Design Documentation

NovaNas is a Kubernetes-native single-node NAS appliance providing unified block, file, and object storage with integrated container and VM hosting. It is forked from NovaStor (an SDS project) and evolves independently.

## Product summary

- **Form factor**: single-node appliance (QNAP/TrueNAS class hardware and up)
- **Storage protocols**: iSCSI, NVMe-oF, NFS, SMB, S3
- **Compute**: user-deployable containers (Helm) and VMs (KubeVirt)
- **Orchestration**: k3s (single-node Kubernetes)
- **Identity**: Keycloak (local users, AD/LDAP, OIDC)
- **Secrets**: OpenBao (Vault-compatible, TPM auto-unseal)
- **Networking**: novanet (eBPF CNI) + novaedge (LB/ingress/SD-WAN)
- **OS**: immutable Debian with RAUC A/B updates

## Document index

| # | Document | Scope |
|---|----------|-------|
| 01 | [Architecture Overview](01-architecture-overview.md) | Layered architecture, component stack, data flow |
| 02 | [Storage Architecture](02-storage-architecture.md) | Pools, volumes, chunks, protection, tiering, encryption |
| 03 | [Access Protocols](03-access-protocols.md) | Block, NFS, SMB, S3 — how each is served |
| 04 | [Tenancy & Isolation](04-tenancy-isolation.md) | System vs user workloads, RBAC, PSA, NetworkPolicy |
| 05 | [CRD Reference](05-crd-reference.md) | Every NovaNas CRD with example manifests |
| 06 | [Boot, Install, Update](06-boot-install-update.md) | OS layering, RAUC, installer, factory reset, config backup |
| 07 | [Disk Lifecycle](07-disk-lifecycle.md) | Disk states, hot-swap, rebuild, foreign imports |
| 08 | [Apps & VMs](08-apps-and-vms.md) | App catalog, AppInstance, KubeVirt integration |
| 09 | [UI & API](09-ui-and-api.md) | Web UI, API server, stack choices, auth flow |
| 10 | [Identity & Secrets](10-identity-and-secrets.md) | Keycloak, OpenBao, TPM unseal, PKI |
| 11 | [Networking](11-networking.md) | NICs, bonds, VLANs, novanet, novaedge, discovery |
| 12 | [Observability](12-observability.md) | Prometheus, Loki, Tempo, Grafana, alerts, SLOs |
| 13 | [Build & Release](13-build-and-release.md) | Monorepo, CI, RAUC bundles, signing |
| 14 | [Decision Log](14-decision-log.md) | Summary of every locked-in design decision |

## Developer docs

- [Developer guide](developer/README.md) — onboarding, workflow, testing, debugging
- [Architecture Decision Records](adr/README.md) — historical context for major decisions

## Operations

- [Production runbook](runbook/README.md) — hardware expansion, disk
  replacement, off-site replication, OS upgrades, ransomware response,
  disaster recovery
- [Troubleshooting guide](troubleshooting/README.md) — storage,
  networking, identity, performance diagnostics

## Planning

- [CRD consolidation plan](CRD-CONSOLIDATION-PLAN.md) — tracking doc
  for unifying `storage/api/v1alpha1` and
  `packages/operators/api/v1alpha1`

## Scope status

**Out of scope for v1**: multi-box clustering.

**Deferred detailed design**: Web UI visual design (separate design pass after this documentation).

## Licensing

Apache License 2.0. Preserves attribution to the NovaStor project from which the storage engine is forked.
