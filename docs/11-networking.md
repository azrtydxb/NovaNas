# 11 — Networking

Two network planes:

- **Host networking** — physical NICs, bonds, VLANs, IP addresses
- **Cluster networking** — pods, services, ingress, VIPs

NovaNas exposes both planes as API resources owned by the API server (Postgres-backed; no CRDs). Under the hood, nmstate handles the host plane and novanet + novaedge handle the cluster plane — both are driven by NovaNas controllers reading the API resources, not by users authoring Kubernetes objects.

## Stack

```
Containers / VMs (apps, storage services)
  ↕ novanet (eBPF, overlay, identity policy) — runtime adapter wires this in
Workload network (overlay CIDR, service VIPs)
  ↕ novaedge (VIPs, ingress, reverse proxy, SD-WAN)
Host network (physical NICs, bonds, VLANs)
  ↕ nmstate / systemd-networkd (host-agent, runtime-agnostic)
Physical: eth0, eth1, ... (NICs)
```

## Host networking resources

These are API-server-owned resources. The host-agent (a runtime-neutral process) reads them via `/api/v1/*` and applies them with nmstate. There are no CRDs.

### physicalInterface (observed)

Read-only NIC reflection, populated by the host-agent:

```json
{
  "name": "enp4s0",
  "status": {
    "macAddress": "00:1a:2b:3c:4d:5e",
    "speedMbps": 10000,
    "duplex": "full",
    "link": "up",
    "driver": "ixgbe",
    "pcieSlot": "0000:04:00.0",
    "capabilities": ["rx-checksumming", "tx-checksumming", "tso", "gro", "rss"],
    "usedBy": "bond0"
  }
}
```

### bond

`POST /api/v1/bonds`:

```json
{
  "name": "bond0",
  "interfaces": ["enp4s0", "enp5s0"],
  "mode": "802.3ad",
  "lacp": { "rate": "fast", "aggregatorSelect": "bandwidth" },
  "xmitHashPolicy": "layer3+4",
  "mtu": 9000
}
```

### vlan

`POST /api/v1/vlans`:

```json
{
  "name": "storage-vlan",
  "parent": "bond0",
  "vlanId": 42,
  "mtu": 9000
}
```

### hostInterface — the IP-bearing interface

`POST /api/v1/hostInterfaces`:

```json
{
  "name": "storage",
  "backing": "storage-vlan",
  "addresses": [
    { "cidr": "10.10.42.10/24", "type": "static" },
    { "cidr": "fd42::10/64", "type": "static" }
  ],
  "gateway": "10.10.42.1",
  "dns": ["10.10.42.1", "1.1.1.1"],
  "mtu": 9000,
  "usage": ["storage"]
}
```

A single interface can serve multiple `usage` roles. Small boxes with one NIC share everything. Multi-NIC boxes split roles.

### Usage roles

| Role | Traffic |
|---|---|
| `management` | UI, API, SSH, metrics scrape |
| `storage` | iSCSI, NVMe-oF, replication, NFS/SMB, S3 |
| `cluster` | Runtime-internal traffic (k3s node-local on K8s adapter; docker bridge on Docker adapter) |
| `vmBridge` | L2 bridge to VMs |
| `appIngress` | Inbound to user-facing apps, novaedge VIPs |

## Cluster networking

### clusterNetwork (singleton)

`PUT /api/v1/clusterNetwork`:

```json
{
  "podCidr": "10.244.0.0/16",
  "serviceCidr": "10.96.0.0/12",
  "overlay": {
    "type": "geneve",
    "egressInterface": "bond0"
  },
  "policy": { "defaultDeny": true },
  "mtu": "auto"
}
```

Set at install time; rarely changed. Consumed by the runtime adapter to configure the runtime's networking (CNI on K8s; equivalent bridge/network setup on Docker).

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
  - VIP allocation from `vipPool`
  - Reverse-proxy ingress at `nas.local` and per-app subdomains
  - TLS termination (integrates with OpenBao for certs)
  - Remote access via SD-WAN tunnels

novaedge is configured exclusively via the NovaNas API. Its config bundle is rendered by the networking controller from `vipPool` / `ingress` / `remoteAccessTunnel` / `customDomain` API resources.

### vipPool

`POST /api/v1/vipPools`:

```json
{
  "name": "lan",
  "range": "192.168.1.200-192.168.1.240",
  "interface": "mgmt",
  "announce": "arp"
}
```

### ingress

`POST /api/v1/ingresses`:

```json
{
  "name": "default",
  "hostname": "nas.local",
  "tls": { "certificate": "nas-cert" },
  "rules": [
    { "host": "plex.nas.local",      "backend": { "appInstance": "family-plex",  "port": 32400 } },
    { "host": "nextcloud.nas.local", "backend": { "appInstance": "nextcloud",    "port": 80    } }
  ]
}
```

Admin decision locked in: **subdomain routing** (not path-prefix), wildcard cert. Backends reference API resources by name, not Kubernetes Services — the runtime adapter resolves the service endpoint.

### remoteAccessTunnel

`POST /api/v1/remoteAccessTunnels`:

```json
{
  "name": "wan",
  "type": "sdwan",
  "endpoint": { "hostname": "nas.example.com", "port": 443 },
  "auth": { "secretRef": "openbao://novanas/tunnel/auth" },
  "exposes": [{ "app": "family/plex", "via": "tunnel" }]
}
```

### customDomain

For user-supplied branded hostnames (e.g., `movies.example.com`):

`POST /api/v1/customDomains`:

```json
{
  "hostname": "movies.example.com",
  "target": { "kind": "appInstance", "owner": "pascal", "name": "family-plex" },
  "tls": { "provider": "letsencrypt" }
}
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
- Per-app `<app>.nas.local` — default on, opt-out per-app via `appInstance.network.expose`

## DNS

Two stories:

**Inbound** (clients resolving names):
- mDNS/SSDP/WS-Discovery on LAN
- Admin-configured public DNS for WAN (e.g., `nas.example.com` with wildcard) used via novaedge SD-WAN tunnel

**Outbound** (NAS resolving):
- systemd-resolved on host using DNS from `hostInterface` DHCP or static
- For workload-internal lookup the runtime adapter wires DNS (CoreDNS on K8s, embedded resolver on Docker) to forward to the host resolver

## IPv6

- **Enabled by default** when the network provides v6 (SLAAC or DHCPv6 detected)
- Every `hostInterface` can hold v6 addresses
- novanet and novaedge support v6 VIPs
- v6-only mode deferred; plumbing in place

## Firewall

### Host firewall

`POST /api/v1/firewallRules`:

```json
{
  "name": "management-access",
  "scope": "host",
  "direction": "inbound",
  "action": "allow",
  "interface": "mgmt",
  "source": { "cidrs": ["192.168.1.0/24"] },
  "destination": { "ports": [22, 443], "protocol": "tcp" },
  "priority": 100
}
```

**No default host firewall.** All NovaNas service ports are open per `servicePolicy`. Admin opts into restrictions.

### Workload firewall

A `firewallRule` with `scope: workload` is translated into novanet identity policies by the networking controller. Users author the API resource; the controller emits the runtime-native enforcement objects (eBPF programs via novanet on K8s; equivalent ipset/nftables-driven novanet config on Docker).

## Traffic QoS

Unified `trafficPolicy` API resource:

`POST /api/v1/trafficPolicies`:

```json
{
  "name": "storage-isolation",
  "scope": { "kind": "hostInterface", "name": "storage" },
  "limits": {
    "egress":  { "max": "1Gbps",  "burst": "2Gbps" },
    "ingress": { "max": "10Gbps" }
  },
  "scheduling": {
    "offHours": {
      "cron": "0 22 * * *",
      "durationMinutes": 480,
      "overrideEgress": { "max": "500Mbps" }
    }
  },
  "priority": 100
}
```

`scope.kind` may be `hostInterface | tenant | app | vm | replicationJob | objectStore`. Replaces scattered bandwidth fields across `replicationTarget`, `appInstance`, etc. — those fields become sugar that synthesize a `trafficPolicy` under the hood.

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

- `hostInterface(vm-br0)` with `usage: [vmBridge]` — no IP, pure bridge
- VMs attach to `bridge: vm-br0` for L2 access to the physical LAN; the runtime adapter wires the VM NIC to the bridge (KubeVirt on K8s, libvirt on Docker)

### Remote access

- `remoteAccessTunnel` provisions a novaedge SD-WAN tunnel
- Per-app `expose: internet` flag opts apps into being reachable from outside
- Admin-gated permission; users cannot self-expose
