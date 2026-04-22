# 11 — Networking

Two network planes:

- **Host networking** — physical NICs, bonds, VLANs, IP addresses
- **Cluster networking** — pods, services, ingress, VIPs

NovaNas provides declarative CRDs; under the hood nmstate handles the host plane and novanet + novaedge handle the cluster plane.

## Stack

```
Pods (apps, VMs, storage services)
  ↕ novanet (eBPF CNI, overlay, identity policy)
Cluster network (pod CIDR, service CIDR)
  ↕ novaedge (VIPs, ingress, reverse proxy, SD-WAN)
Host network (physical NICs, bonds, VLANs)
  ↕ nmstate / systemd-networkd
Physical: eth0, eth1, ... (NICs)
```

## Host networking CRDs

### PhysicalInterface (observed)

Read-only NIC reflection, like `Disk`:

```yaml
kind: PhysicalInterface
metadata: { name: enp4s0 }
status:
  macAddress: 00:1a:2b:3c:4d:5e
  speedMbps: 10000
  duplex: full
  link: up
  driver: ixgbe
  pcieSlot: "0000:04:00.0"
  capabilities: [rx-checksumming, tx-checksumming, tso, gro, rss]
  usedBy: bond0
```

### Bond

```yaml
kind: Bond
metadata: { name: bond0 }
spec:
  interfaces: [enp4s0, enp5s0]
  mode: 802.3ad          # active-backup | balance-alb | 802.3ad | balance-tlb
  lacp: { rate: fast, aggregatorSelect: bandwidth }
  xmitHashPolicy: layer3+4
  mtu: 9000
```

### Vlan

```yaml
kind: Vlan
metadata: { name: storage-vlan }
spec:
  parent: bond0          # Bond | PhysicalInterface
  vlanId: 42
  mtu: 9000
```

### HostInterface — the IP-bearing interface

```yaml
kind: HostInterface
metadata: { name: storage }
spec:
  backing: storage-vlan
  addresses:
    - { cidr: 10.10.42.10/24, type: static }
    - { cidr: fd42::10/64, type: static }
  gateway: 10.10.42.1
  dns: [10.10.42.1, 1.1.1.1]
  mtu: 9000
  usage: [storage]       # management | storage | cluster | vmBridge | appIngress
```

A single interface can serve multiple `usage` roles. Small boxes with one NIC share everything. Multi-NIC boxes split roles.

### Usage roles

| Role | Traffic |
|---|---|
| `management` | UI, API, SSH, metrics scrape |
| `storage` | iSCSI, NVMe-oF, replication, NFS/SMB, S3 |
| `cluster` | k3s node-local traffic |
| `vmBridge` | L2 bridge to VMs |
| `appIngress` | Inbound to user-facing apps, novaedge VIPs |

## Cluster networking

### ClusterNetwork (singleton)

```yaml
kind: ClusterNetwork
spec:
  podCidr: 10.244.0.0/16
  serviceCidr: 10.96.0.0/12
  overlay:
    type: geneve          # geneve | vxlan | none
    egressInterface: bond0
  policy:
    defaultDeny: true
  mtu: auto               # account for overlay overhead, or explicit
```

Set at install time; rarely changed.

### novanet

- eBPF CNI
- Geneve or VXLAN overlay
- Identity-based policy — policies reference workload labels (`user: pascal`, `namespace: user-pascal`, `app: plex`), not CIDRs
- L4 socket-based load balancing for cluster-internal services
- Native BGP/OSPF/BFD routing via integrated FRR (relevant for future multi-node)
- Real-time flow visibility for observability UI

### novaedge

- Unified LB + ingress + reverse proxy + SD-WAN gateway
- Handles:
  - VIP allocation from `VipPool`
  - Reverse-proxy ingress at `nas.local` and per-app subdomains
  - TLS termination (integrates with OpenBao for certs, preferred; or K8s Secrets synced from OpenBao)
  - Remote access via SD-WAN tunnels

### VipPool

```yaml
kind: VipPool
spec:
  range: 192.168.1.200-192.168.1.240
  interface: mgmt         # HostInterface to advertise on
  announce: arp           # arp | bgp
```

### Ingress

```yaml
kind: Ingress
spec:
  hostname: nas.local
  tls: { certificate: nas-cert }    # wildcard *.nas.local
  rules:
    - host: plex.nas.local
      backend: family-plex.novanas-users-pascal.svc:32400
    - host: nextcloud.nas.local
      backend: nextcloud.novanas-users-pascal.svc:80
```

Admin decision locked in: **subdomain routing** (not path-prefix), wildcard cert.

### RemoteAccessTunnel

```yaml
kind: RemoteAccessTunnel
spec:
  type: sdwan                       # sdwan | wireguard | tailscale
  endpoint:
    hostname: nas.example.com
    port: 443
  auth:
    secretRef: openbao://novanas/tunnel/auth
  exposes:
    - app: family/plex
      via: tunnel
```

### CustomDomain

For user-supplied branded hostnames (e.g., `movies.example.com`):

```yaml
kind: CustomDomain
spec:
  hostname: movies.example.com
  target: { kind: AppInstance, namespace: novanas-users/pascal, name: family-plex }
  tls:
    provider: letsencrypt           # ACME via novaedge, keys in OpenBao
```

## Discovery

`novanas-discovery` pod advertises presence on the LAN:

- **mDNS / Bonjour** (Avahi) — Mac/Linux native
- **SSDP / UPnP** — older Windows Network Neighborhood
- **WS-Discovery** — modern Windows (since SMBv1/NetBIOS deprecation)

Advertisements:
- `nas.local` (IPv4 + IPv6)
- Per-share SMB (`_smb._tcp`)
- Web UI (`_https._tcp`)
- Per-app `<app>.nas.local` — default on, opt-out per-app via `AppInstance.spec.network.expose`

## DNS

Two stories:

**Inbound** (clients resolving names):
- mDNS/SSDP/WS-Discovery on LAN
- Admin-configured public DNS for WAN (e.g., `nas.example.com` with wildcard) used via novaedge SD-WAN tunnel

**Outbound** (NAS resolving):
- systemd-resolved on host using DNS from HostInterface DHCP or static
- In-cluster: CoreDNS (k3s default) forwards to host

## IPv6

- **Enabled by default** when the network provides v6 (SLAAC or DHCPv6 detected)
- Every HostInterface can hold v6 addresses
- novanet and novaedge support v6 VIPs
- v6-only mode deferred; plumbing in place

## Firewall

### Host firewall

```yaml
kind: FirewallRule
metadata: { name: management-access }
spec:
  scope: host                       # host | pod
  direction: inbound
  action: allow
  interface: mgmt
  source:
    cidrs: [192.168.1.0/24]
  destination:
    ports: [22, 443]
    protocol: tcp
  priority: 100
```

**No default host firewall.** All NovaNas service ports are open per `ServicePolicy`. Admin opts into restrictions.

### Pod firewall

`FirewallRule` with `scope: pod` synthesizes novanet identity policies. User-friendly CRD; novanet provides the eBPF enforcement.

## Traffic QoS

Unified `TrafficPolicy` CRD:

```yaml
kind: TrafficPolicy
metadata: { name: storage-isolation }
spec:
  scope:
    kind: HostInterface             # HostInterface | Namespace | App | Vm | ReplicationJob | ObjectStore
    name: storage
  limits:
    egress: { max: 1Gbps, burst: 2Gbps }
    ingress: { max: 10Gbps }
  scheduling:
    offHours:
      cron: "0 22 * * *"
      durationMinutes: 480
      overrideEgress: { max: 500Mbps }
  priority: 100
```

Replaces scattered bandwidth fields across `ReplicationTarget`, `AppInstance`, etc. Those fields become sugar that synthesize `TrafficPolicy` under the hood.

## Installer networking

First-boot wizard network step is minimal:

1. Show detected NICs (green = link up)
2. Pick management NIC
3. DHCP (default) or static IP
4. Hostname
5. Done — advanced topology (bonds, VLANs, multiple NICs) in web UI post-install

## Observability

Per-interface metrics exposed to Prometheus:
- `novanas_net_bytes_total{interface=, direction=}`
- `novanas_net_packets_total{interface=, direction=}`
- `novanas_net_errors_total{interface=}`
- `novanas_net_drops_total{interface=}`
- `novanas_bond_member_up{bond=, member=}`
- `novanas_vlan_tag_count{vlan=}`
- `novaedge_vip_sessions{vip=}`
- `novanet_flow_rate{identity_src=, identity_dst=}`

UI: per-interface graphs on the Network page; per-app request metrics via novaedge stats.

## Example topologies

### Single 1 GbE NIC (home user)

- One HostInterface, DHCP
- `usage: [management, storage, cluster, appIngress]` on the single NIC
- mDNS advertises `nas.local` on LAN
- Works out of the box

### 2×10 GbE LAG with storage VLAN

- Bond(802.3ad) across both NICs, MTU 9000
- HostInterface(mgmt) on untagged bond0
- Vlan(42), HostInterface(storage, MTU 9000) for NFS/SMB/iSCSI
- `usage: [storage]` separates the backup/replication traffic from user access

### VM bridge

- HostInterface(vm-br0) with `usage: [vmBridge]` — no IP, pure bridge
- KubeVirt VMs attach to `bridge: vm-br0` for L2 access to the physical LAN

### Remote access

- `RemoteAccessTunnel` provisions novaedge SD-WAN tunnel
- Per-app `expose: internet` flag opts apps into being reachable from outside
- Admin-gated permission; users cannot self-expose
