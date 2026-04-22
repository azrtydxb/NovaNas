# Networking troubleshooting

NovaEdge VIPs, NovaNet policies, DNS resolution, mDNS discovery, NFS
hangs.

## NovaEdge VIP stuck

**Symptom.** After a node loss the VIP did not move to a healthy node;
clients see a stalled connection.

**Diagnose.**

```sh
novanasctl vip list -o wide
# expected-node vs current-node; status should be Active.

kubectl -n novanas-system logs -l app=novaedge --tail=500 \
  | grep -E 'bgp|vip|advert'
```

On the new primary node:

```sh
birdc show protocols            # or the vendor equivalent
birdc show route for <vip>
```

**Root causes.**

1. BGP session with the ToR did not re-establish (e.g., MD5 secret
   mismatch after a key rotation).
2. The upstream router still has a cached route to the dead node
   and does not withdraw it (MED/LOCAL_PREF misconfig).
3. NovaEdge pod on the new primary is crashing before it advertises.

**Remediate.**

- Force a BGP session reset from the router side (if you control it),
  or bounce the NovaEdge pod to retrigger:

  ```sh
  kubectl -n novanas-system rollout restart deploy/novaedge
  ```

- Clear stuck routes upstream. The specific command depends on your
  router vendor.
- If the pod keeps crashing, inspect the logs for config errors (most
  commonly an IP in `VipPool.spec.cidr` that overlaps a local
  interface).

## NovaNet policy blocks

**Symptom.** Pod-to-pod or pod-to-service traffic drops; user reports
"the app can't reach the database".

**Diagnose.**

```sh
kubectl get networkpolicies -A
kubectl get servicepolicies -A     # NovaNet CRD layer

# Capture packets on the source pod for 10s.
kubectl -n <ns> exec <pod> -- tcpdump -nn -c 100 host <dst-ip>

# Inspect denials in NovaNet:
kubectl -n novanas-system logs -l app=novanet --tail=500 \
  | grep -E 'drop|deny'
```

**Root causes.**

1. A new `ServicePolicy` with tighter rules was applied and didn't
   allow the required traffic.
2. Egress to DNS is blocked (most common cause of apparent "nothing
   works"); check for a policy that allows 53/UDP explicitly.
3. `TrafficPolicy` is shaping the flow below a usable rate.

**Remediate.**

- Amend the offending policy; NovaNet reconciliation is eventual — give
  it 10s after the edit.
- For DNS, ensure a global egress-to-kube-dns policy exists:

  ```sh
  kubectl -n novanas-system get servicepolicy allow-dns
  ```

- If desperate and the environment is under attack, remove the policy
  enforcement annotation on the namespace to fall back to pass-through
  while you investigate:

  ```sh
  kubectl label ns <ns> novanet.io/enforce=disabled --overwrite
  ```

  Restore enforcement as soon as possible.

## DNS resolution failure

**Symptom.** `getaddrinfo` failures; clients cannot resolve
`nas.local` or a pod-injected name.

**Diagnose.**

```sh
kubectl -n kube-system get pods -l k8s-app=kube-dns
dig @<kube-dns-svc-ip> novanas.novanas-system.svc.cluster.local

# From a client:
dig nas.local
```

**Root causes.**

1. CoreDNS is unhealthy (OOM, crashlooping).
2. The upstream resolver is unreachable — check the NovaNas host's
   `/etc/resolv.conf` and corporate DNS uptime.
3. Split-horizon: client resolves via corporate DNS which doesn't
   know about `.local`.

**Remediate.**

- Restart CoreDNS if it is sick:

  ```sh
  kubectl -n kube-system rollout restart deploy/coredns
  ```

- Add a forwarder to the corporate DNS that has an A record for the
  VIP.
- For `.local` discovery, verify mDNS (Avahi) is running on the
  NovaNas host (see next section).

## mDNS discovery

**Symptom.** `nas.local` works from one subnet but not another; Bonjour
browser shows the NAS from some clients but not others.

**Diagnose.**

```sh
# On the NovaNas host:
avahi-browse -a -r -t -p | head
systemctl status avahi-daemon

# From a client on the failing subnet:
dns-sd -B _smb._tcp           # macOS
avahi-browse -a               # Linux
```

**Root causes.**

1. mDNS is link-local only; a router between the client and the NAS
   is not forwarding 224.0.0.251.
2. Avahi is not advertising on the interface that faces the client
   subnet (check `/etc/avahi/avahi-daemon.conf`
   `allow-interfaces=`).

**Remediate.**

- Enable mDNS repeater on the router (e.g., `avahi-daemon` in
  `reflector` mode on the router), or configure IGMP querier.
- Add the facing interface to Avahi's `allow-interfaces`.
- As a fallback, hand out the VIP via DHCP option 119 (domain search)
  plus a DNS A record — more reliable than mDNS across subnets.

## NFS hang

**Symptom.** NFS mount on a client hangs indefinitely; `ls`, `df`
block.

**Diagnose.**

```sh
# On the client:
cat /proc/mounts | grep nfs
mount -v                   # flags used
nfsstat -c                 # RPC errors

# On the NovaNas side:
kubectl -n novanas-system get nfsserver <nfs>
novanasctl share get <share> -o json | jq '.status'
```

**Root causes.**

1. Server entered grace period (just restarted); clients using
   `hard,intr` wait for up to 90s.
2. The VIP is mid-failover (see [VIP stuck](#novaedge-vip-stuck)).
3. Client used `nosoft` without `intr`; cannot be interrupted even by
   Ctrl-C. Wait for grace or force-unmount with `umount -f -l`.
4. Kerberos ticket expired on the client.

**Remediate.**

- Wait 90s; most mounts recover.
- Check the NFS server pod's logs for `rpc.mountd` denials.
- Re-kinit on the client if sec=krb5.

```sh
kinit user@REALM
```
