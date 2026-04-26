# 04 — Tenancy & Isolation

NovaNas hosts both **its own appliance services** and **user-deployed containers and VMs** on one box. Isolation between these is enforced by policy, not by separate runtimes. The user-facing model is *tenants*: system, apps-system, per-user, per-group, plus a VM tenant. The runtime adapter projects each tenant onto the runtime's native scope (Kubernetes namespace + PSA on K8s; Docker network + label set + cgroup profile on Docker).

## Tenant layout

| Tenant | Purpose | Privilege profile |
|---|---|---|
| `novanas-system` | All NovaNas-owned containers (storage, protocols, API, UI, controllers, observability, Keycloak, OpenBao, Postgres, Redis) | `privileged` |
| `novanas-apps-system` | System-level installs for catalog apps that need elevated permissions | `baseline` |
| `novanas-users/<user>` | One per user — their apps, volumes, secrets | `restricted` |
| `novanas-vms` | All VMs | `restricted` |
| `novanas-shared/<group>` | Group-shared workloads (e.g., family-shared apps) | `restricted` |
| Runtime-internal | The runtime's own controlplane (k3s `kube-system` on K8s, dockerd internals on Docker) — opaque to NovaNas | `privileged` |

Each user gets at least one tenant. Groups may share a tenant for jointly-owned resources.

## Enforcement layers

All applied together — defense in depth:

### 1. AuthZ at the API server

- System tenants: only the API server's own service token and a small set of controller tokens can write
- User tokens (Keycloak-issued, scoped at the API server): can only read/write resources whose tenant matches their scope
- Admin "escape hatch" — a privileged shell into the runtime can be requested via a deliberately-guarded API call; use is audited

### 2. Privilege profile, applied by the runtime adapter

- `novanas-system`: `privileged` — components like the NFS host-agent, SPDK dataplane, and the VM hypervisor need host access. K8s adapter: PSA `privileged`. Docker adapter: container started with required caps + bind mounts.
- `novanas-apps-system`: `baseline` — curated apps have moderate privilege needs (mapped to PSA `baseline` on K8s; equivalent restricted-but-not-rootless profile on Docker).
- `novanas-users/*`, `novanas-vms`, `novanas-shared/*`: `restricted` — no host filesystem, no host network, no host PID, no extra capabilities, run-as-non-root enforced, read-only root filesystem preferred. The runtime adapter rejects any container spec that violates this regardless of how it was generated.

User apps that legitimately need more privilege must be installed from the **Official catalog** (where chart manifests are reviewed and targeted at `novanas-apps-system`), not as user-tenant installs.

### 3. NetworkPolicy via novanet

novanet provides **identity-based policy** (workload labels, not CIDRs). Applied to every namespace:

- **Default deny** intra-tenant and inter-tenant
- User tenants whitelist:
  - Egress to internet (if permitted by user's role and `firewallRule` API resources)
  - Egress to novaedge-published VIPs (their own apps' service IPs)
  - Ingress from novaedge only
  - No egress to the system tenant or runtime-internal identities
- System workloads get `identity: system`; user policies can't target it

### 4. API-server admission

There is no Kubernetes admission webhook — the NovaNas API server is itself the admission point. Every request validates:

- Users cannot reference pools they don't own when creating volumes / datasets
- Dataset quotas strictly enforced (hard cap)
- Users cannot request `hostPath`-style mounts (the runtime adapter rejects any controller-emitted spec that escalates beyond the user's tenant profile)
- Users cannot exceed CPU / memory / object quotas attached to their tenant
- System-scope resources (`pool`, `systemSettings`, etc.) reject writes from user-scope tokens
- Object Lock rules enforced (see 03-access-protocols)

### 5. NovaNas API server — the authoritative layer

The API server is the **single authorization boundary**. All state changes flow through it, and it:

- Resolves the user's Keycloak token → permissions
- Validates every operation against the user's scope (tenant ownership, resource ownership)
- Persists the change to Postgres (sole source of truth)
- Audits every action
- Hands off to the runtime adapter, which executes with its own privileged credentials against the runtime (kube-apiserver token on K8s, Docker socket on Docker)

The runtime's native authorization (K8s RBAC, Docker socket access) is intentionally coarse — only NovaNas controllers and the runtime adapter ever speak it. Fine-grained per-user authorization lives exclusively in the API server. Simpler to reason about, simpler to test, and runtime-agnostic.

## Per-user scaffolding

When a `User` is created (via UI, CLI, or Keycloak group-sync), the operator provisions:

- Tenant `novanas-users/<user>` with restricted profile
- Quota record on the API server: CPU, memory, storage tier limits based on the `user` API resource
- Default per-container sizing applied by the runtime adapter (LimitRange on K8s; cgroup defaults on Docker)
- Network policy default-deny + whitelist for internet (if allowed) and own-app VIPs
- Role binding to the `novanas-user` role, scoped to this tenant
- Default storage tier pointing at the user's assigned pool
- Dataset quota entries in any shared datasets they're granted access to
- OpenBao policy permitting access to their tenant's secrets path

Users can only create:
- Apps (via `appInstance`) in their own tenant
- VMs in the VM tenant subject to quota
- Datasets, block volumes, buckets within their storage quota
- Snapshots of resources they own
- Their own API tokens and SSH keys

## Single UI, role-driven visibility

One React SPA. What a user sees is gated by their role:

- **Admin** — full UI, all namespaces, system settings, pools, disks, identity, networking, updates, audit log
- **User** — their namespace(s): their datasets, shares they own or have access to, their apps, their VMs, their snapshots, plus read-only views of overall system health
- **Viewer / share-only** — may not see the UI at all; just consumes shared datasets via SMB/NFS/S3 with their identity

The UI is a thin client: it hides tabs based on permissions the API returns, and gracefully handles 403s. Enforcement is in the API server, not the UI.

## Apps and VMs as users of the tenancy model

- `appInstance` lives in a user tenant. Its containers, volumes, secrets stay in-tenant.
- App storage (`appInstance.storage`) auto-creates datasets/block-volumes in the same user tenant, quota-accounted.
- Apps exposed externally go through novaedge — users can advertise via mDNS/LAN, NAS-reverse-proxy, or internet (internet exposure may require admin permission per `servicePolicy`).
- VMs live in the VM tenant with labels identifying their owner; the API server scopes VM actions to owners + admins.
- App and VM CPU/memory count against the owner's quota record.

## Escape hatches

- **Privileged runtime shell**: admin can request from the API server (generates scoped token, audits issuance). Manifests as a kubeconfig on K8s, a Docker socket bind on Docker. Intended for debugging, not routine use.
- **SSH to host**: disabled by default; enabled via `servicePolicy` with key-only access. Admins only.
- **Raw Keycloak admin console**: accessible for identity power-users but not needed for normal operation.
- **Raw Grafana**: accessible for metric power-users; deep-linked from NovaNas UI.

All escape-hatch usage is audited.

## What this model does NOT defend against

- **Kernel exploits**: a container escape via kernel vuln bypasses tenant profile and authZ. Mitigated by regular RAUC updates and minimal attack surface, not by architecture.
- **Compromise of the NovaNas API server**: holds privileged credentials to the runtime; compromise = full access. Mitigated by minimal attack surface, input validation, limited egress, full audit.
- **Malicious admins**: admins can do anything. Audit trail catches misuse after the fact.
- **Physical access to disks**: chunk encryption + TPM-sealed keys mitigate (disk pulled from running box is useless without MK unlock).
