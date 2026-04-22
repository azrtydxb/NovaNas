# 04 — Tenancy & Isolation

NovaNas hosts both **its own appliance services** and **user-deployed containers and VMs** on one Kubernetes node. Isolation between these is enforced by policy, not by separate clusters. On a single-node cluster sharing one kernel, defense-in-depth is required.

## Namespace layout

| Namespace | Purpose | PSA |
|---|---|---|
| `novanas-system` | All NovaNas-owned pods (storage, protocols, API, UI, operators, observability, Keycloak, OpenBao, Postgres, Redis) | `privileged` |
| `novanas-apps-system` | System-level Helm releases for catalog apps (shared services) | `baseline` |
| `novanas-users/<user>` | One per user — their apps, PVCs, secrets | `restricted` |
| `novanas-vms` | All KubeVirt VMs | `restricted` |
| `novanas-shared/<group>` | Group-shared workloads (e.g., family-shared apps) | `restricted` |
| `kube-system` | k3s internals | `privileged` |

Each user gets at least one namespace. Groups may share a namespace for jointly-owned resources.

## Enforcement layers

All applied together — defense in depth:

### 1. RBAC

- System namespaces: only the `novanas-api` ServiceAccount and a small set of operator SAs have cluster-wide write permissions
- User roles (projected into `kube-apiserver` via the API server, not exposed directly): scoped to their own namespace(s)
- Admin "escape hatch" — a cluster-admin kubeconfig can be generated via a deliberately-guarded API call; use is audited

### 2. Pod Security Admission

- `novanas-system`: `privileged` — components like `novanas-nfs-operator`, the SPDK dataplane, and KubeVirt virt-handler need host access
- `novanas-apps-system`: `baseline` — curated apps have moderate privilege needs
- `novanas-users/*`, `novanas-vms`, `novanas-shared/*`: `restricted` — no hostPath, no hostNetwork, no hostPID, no privileged containers, no added capabilities, runAsNonRoot enforced, read-only root FS preferred

User apps that legitimately need more privilege must be installed from the **Official catalog** (where chart manifests are reviewed and targeted at `novanas-apps-system`), not as user-namespace installs.

### 3. NetworkPolicy via novanet

novanet provides **identity-based policy** (workload labels, not CIDRs). Applied to every namespace:

- **Default deny** intra-namespace and inter-namespace
- User namespaces whitelist:
  - Egress to internet (if permitted by user's role and `FirewallRule` cluster policy)
  - Egress to novaedge-published VIPs (their own apps' service IPs)
  - Ingress from novaedge only
  - No egress to `novanas-system` or `kube-system` identities
- System pods get `identity: system`; user policies can't target it

### 4. Admission webhook

`novanas-admission` validates CRD operations:

- Users cannot create PVCs bound to pools they don't own
- Dataset quotas strictly enforced (hard cap)
- Users cannot bind `hostPath` volumes
- Users cannot exceed `ResourceQuota` / `LimitRange` in their namespace
- System CRDs (`StoragePool`, `SystemSettings`, etc.) rejected from user namespaces
- Object Lock rules enforced (see 03-access-protocols)

### 5. NovaNas API server — the authoritative layer

Because K8s is hidden, the API server is the **de facto authorization boundary**. All state changes flow through it, and it:

- Resolves the user's Keycloak token → permissions
- Validates every operation against the user's scope (namespace ownership, resource ownership)
- Constructs K8s calls using its own cluster-admin-ish token (coarse K8s RBAC)
- Audits every action

This means K8s RBAC itself is coarse (API server can do a lot); fine-grained auth lives in one place — the API server. Simpler to reason about and test.

## Per-user scaffolding

When a `User` is created (via UI, CLI, or Keycloak group-sync), the operator provisions:

- Namespace `novanas-users/<user>` with PSA `restricted`
- `ResourceQuota` — CPU, memory, storage class limits based on the User CR spec
- `LimitRange` — default/max pod sizing
- NetworkPolicy default-deny + whitelist for internet (if allowed) and own-app VIPs
- RoleBindings referencing the `novanas-user` ClusterRole, scoped to this namespace
- Default `StorageClass` pointing at the user's assigned pool tier
- Dataset quota entries in any shared datasets they're granted access to
- OpenBao policy permitting access to their namespace's secrets path

Users can only create:
- Apps (via `AppInstance`) into their own namespaces
- VMs into `novanas-vms` subject to quota
- Datasets, BlockVolumes, Buckets within their storage quota
- Snapshots of resources they own
- Their own API tokens and SSH keys

## Single UI, role-driven visibility

One React SPA. What a user sees is gated by their role:

- **Admin** — full UI, all namespaces, system settings, pools, disks, identity, networking, updates, audit log
- **User** — their namespace(s): their datasets, shares they own or have access to, their apps, their VMs, their snapshots, plus read-only views of overall system health
- **Viewer / share-only** — may not see the UI at all; just consumes shared datasets via SMB/NFS/S3 with their identity

The UI is a thin client: it hides tabs based on permissions the API returns, and gracefully handles 403s. Enforcement is in the API server, not the UI.

## Apps and VMs as users of the tenancy model

- `AppInstance` lives in a user namespace. Its pods, PVCs, secrets stay in-namespace.
- App storage (`AppInstance.spec.storage`) auto-creates Datasets/BlockVolumes in the same user namespace, quota-accounted.
- Apps exposed externally go through novaedge — users can advertise via mDNS/LAN, NAS-reverse-proxy, or internet (internet exposure may require admin permission per `ServicePolicy`).
- VMs live in `novanas-vms` with labels identifying their owner; RBAC scopes VM actions to owners + admins.
- App and VM CPU/memory count against the owner's `ResourceQuota`.

## Escape hatches

- **Cluster-admin kubeconfig**: admin can request from API server (generates scoped token, audits issuance). Intended for debugging, not routine use.
- **SSH to host**: disabled by default; enabled via `ServicePolicy` with key-only access. Admins only.
- **Raw Keycloak admin console**: accessible for identity power-users but not needed for normal operation.
- **Raw Grafana**: accessible for metric power-users; deep-linked from NovaNas UI.

All escape-hatch usage is audited.

## What this model does NOT defend against

- **Kernel exploits**: a container escape via kernel vuln bypasses PSA and RBAC. Mitigated by regular RAUC updates and minimal attack surface, not by architecture.
- **Compromise of the NovaNas API server**: has cluster-admin-ish power; compromise = full access. Mitigated by minimal attack surface, input validation, limited egress, full audit.
- **Malicious admins**: admins can do anything. Audit trail catches misuse after the fact.
- **Physical access to disks**: chunk encryption + TPM-sealed keys mitigate (disk pulled from running box is useless without MK unlock).
