# Kerberos NFSv4 in NovaNAS

This document walks through the end-to-end Kerberos NFS path so
operators (and future agents) can debug each hop.

## The five-step picture

```
+----------+    1. kinit / keytab     +-------+
|          | <----------------------- |  KDC  |
|  host    |                          +-------+
|  keytab  |
|          |    2. rpc.gssd reads keytab (machine creds, -n)
+----+-----+
     |
     | 3. mount -t nfs4 -o sec=krb5p server:/share /mnt
     v
+----+-----+    4. GSS context (krb5p: integrity + privacy)
|  client  | <==========================================> +--------+
|  kernel  |                                              | nfsd   |
|          |    5. server validates ticket vs its keytab  | + idm  |
+----------+                                              +--------+
                                                              |
                                       6. uid/gid wire <-> name
                                       via idmapd nsswitch lookup
                                       (drives ZFS ACL evaluation)
```

## Components and where NovaNAS configures them

| Step | Component                | Configured by                                      |
|------|--------------------------|----------------------------------------------------|
| 1    | KDC + principals         | Agent A (`internal/host/krb5/kdc.go`, bootstrap)   |
| 1    | `nfs/<host>@REALM` keytab| `internal/host/krb5` `UploadKeytab`                |
| 2    | `rpc.gssd` machine creds | `internal/host/krb5` `SetGssdDefaults` (`-n`) plus |
|      |                          |   `deploy/systemd/rpc-gssd.service.d/override.conf` |
| 3    | mount.nfs4 invocation    | Agent B (`internal/csi/`)                          |
| 4    | export `sec=krb5p`       | `internal/host/nfs` `RequireKerberos=true`         |
| 5    | server keytab validation | Linux kernel (reads `/etc/krb5.keytab` directly)   |
| 6    | `idmapd` Domain match    | `internal/host/krb5` `SetIdmapdConfig`             |

## Why machine creds (`rpc.gssd -n`)

NovaNAS is a Kubernetes storage backend. CSI mounts happen as `root`
inside the kubelet's mount namespace, with no user TGT in any
credentials cache. Without `-n`, `rpc.gssd` would fail the upcall and
the mount would hang. With `-n`, `rpc.gssd` falls back to the host's
own service principal (`nfs/<client-fqdn>@REALM`) — which is exactly
what NFSv4 expects for system mounts.

The same `nfs/<host>@REALM` principal is used on both ends:

- On a NovaNAS storage node acting as **server**, `nfsd` reads
  `/etc/krb5.keytab` and uses `nfs/<server>@REALM` to accept inbound
  contexts.
- On a NovaNAS storage node acting as **client** (rare, but happens
  during replication or fail-over), `rpc.gssd` reads the same keytab
  and uses the same principal for outbound contexts.

A single keytab covering both directions keeps provisioning trivial.

## Why `Method = nsswitch` in idmapd

NFSv4 sends ownership over the wire as a string of the form
`alice@DOMAIN`, not a numeric uid. Both ends call `nfsidmap` to
translate. With `Method = nsswitch`:

- A locally-managed setup (operators editing `/etc/passwd`) Just Works.
- An AD-joined setup (SSSD providing `passwd`/`group` via nss_sss)
  Just Works — SSSD already maps AD SIDs to stable uids.
- A FreeIPA-joined setup is identical to the AD case.

If the client's idmapd Domain does not match the server's, the wire
name "alice@CLIENT.DOMAIN" cannot be translated on the server and gets
squashed to `nobody`. NovaNAS pins both ends to the same realm-derived
domain to avoid this class of bug.

## Why `sec=krb5p` (vs krb5 / krb5i)

| Flavor   | Authentication | Integrity | Encryption | Use case                            |
|----------|----------------|-----------|------------|-------------------------------------|
| `krb5`   | yes            | no        | no         | trusted network, perf-sensitive     |
| `krb5i`  | yes            | yes       | no         | LAN; tampering matters, snooping ok |
| `krb5p`  | yes            | yes       | yes        | default for NovaNAS                 |

NovaNAS defaults to `krb5p` because storage traffic crosses cluster
networks that may include in-network observability tooling and
multi-tenant overlays. The CPU cost of AES-NI-accelerated encryption
is negligible compared to disk and network latency.

## Operator opt-down

`internal/host/nfs` honors a caller-supplied `sec=` override per
client rule. So an operator can publish a single export with mixed
flavors:

```
/tank/share1 10.0.0.0/24(sec=krb5p,rw) trusted.host(sec=krb5i,rw)
```

This is intentional. The global `RequireKerberos` flag enforces a
*minimum* (no `sec=sys` slipping in by accident); explicit callers
choosing a different krb5 flavor are still authenticated.

## Failure modes and where to look

| Symptom                              | Likely cause                                 | Look here                              |
|--------------------------------------|----------------------------------------------|----------------------------------------|
| Mount hangs forever                  | `rpc.gssd` not running, or no `-n`           | `systemctl status rpc-gssd`            |
| `mount: access denied`               | client keytab missing `nfs/<client>@REALM`   | `klist -k -t -e /etc/krb5.keytab`      |
| Files all owned by `nobody:nogroup`  | idmapd Domain mismatch                       | `/etc/idmapd.conf` on both sides       |
| `Server-rejected GSS_INIT`           | server keytab has wrong KVNO (rotated)       | re-upload keytab via NovaNAS API       |
| Time skew error in journal           | clock drift between client/server/KDC > 5min | NTP config                             |
