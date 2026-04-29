# NFSv4 + Kerberos on Debian

This directory contains the host-side artifacts NovaNAS needs to serve
NFSv4 with `sec=krb5p`. NovaNAS targets Debian 12+ (bookworm); other
distros use different paths and unit names.

## Packages

```
sudo apt update
sudo apt install -y \
    nfs-common \
    nfs-kernel-server \
    krb5-user \
    libpam-krb5 \
    libnss-sss \
    rpcbind
```

`krb5-user` pulls in the client tools (`kinit`, `klist`, `kadmin`).
`nfs-common` provides `rpc.gssd` and `rpc.idmapd` (or the kernel
`nfsidmap` upcall, depending on kernel version). `nfs-kernel-server`
provides `rpc.svcgssd` if the kernel is built without nfsd's in-kernel
GSS path; on modern kernels this is unnecessary.

## Files NovaNAS manages

| Path                                               | Owner                                |
|----------------------------------------------------|--------------------------------------|
| `/etc/krb5.conf`                                   | `internal/host/krb5` (rendered)      |
| `/etc/krb5.keytab`                                 | `internal/host/krb5` (uploaded)      |
| `/etc/idmapd.conf`                                 | `internal/host/krb5` (rendered)      |
| `/etc/default/nfs-common`                          | `internal/host/krb5` (rendered)      |
| `/etc/exports.d/nova-nas-*.exports`                | `internal/host/nfs`  (rendered)      |
| `/etc/systemd/system/rpc-gssd.service.d/override.conf` | shipped from `deploy/systemd/`   |

## Enable the units

```
sudo systemctl daemon-reload
sudo systemctl enable --now rpcbind
sudo systemctl enable --now nfs-server
sudo systemctl enable --now rpc-gssd
sudo systemctl enable --now nfs-idmapd        # if not already pulled in
```

## Verify

```
# Confirm rpc.gssd is running with -n (machine creds).
ps -ef | grep rpc.gssd

# Confirm the host keytab contains nfs/<fqdn>@REALM.
sudo klist -k -t -e /etc/krb5.keytab

# Confirm the export is published with sec=krb5p.
sudo exportfs -v
```

## Mounting from a Linux client

The client also needs `nfs-common` and a populated keytab:

```
sudo mount -t nfs4 -o sec=krb5p,vers=4.2 nas.novanas.local:/share1 /mnt
```

When the kernel asks `rpc.gssd` for a context, `rpc.gssd` reads the
client's `/etc/krb5.keytab` (machine creds, because of the `-n` flag),
acquires a TGT for `nfs/<client-host>@REALM`, and presents it to the
server. The server validates against its own keytab. From that point
on, all RPC traffic on the wire is integrity-protected and encrypted.
