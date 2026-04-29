# NovaNAS deployment

Complete inventory of services + reproducible build runbook.

> **This is the manifest for the eventual NovaNAS OS image.** Every step
> required to bring a blank Debian box to a running NovaNAS is captured
> here. When the OS image build pipeline is set up, it must produce a
> system equivalent to following this runbook.

The reference deployment is the dev box at `192.168.10.204` (Debian 13
"trixie", kernel 6.12, x86_64, Intel i5-14400T, 64 GB RAM, 6× WD HDD +
2× Samsung SSD + 2× BIWIN NVMe). Versions captured 2026-04-29 reflect
that box's actual state. The previous, stale version of this doc has
been superseded; see git history if you need it.

---

## 1. Service inventory

### 1.1 Running on the dev box

| Service | Port(s) | Auth | Backed by | Config |
|---|---|---|---|---|
| **nova-api** | 8444 (HTTPS), 8090 (HTTP redirect) | Keycloak JWT | Postgres + Redis + ZFS | `/etc/nova-nas/`, env in unit |
| **Keycloak** | 8443 (HTTPS), 9095 (mgmt) | bootstrap admin | Postgres `keycloak` DB | `/etc/keycloak/`, `/opt/keycloak/` |
| **OpenBao** | 8200, 8201 (raft) | TPM-unsealed root token | raft on local disk | `/etc/openbao/`, `/var/lib/openbao/` |
| **PostgreSQL 17** | 5432 | password | local disk | `/etc/postgresql/17/`, `/var/lib/postgresql/` |
| **Redis** | 6379 | none (loopback) | RDB on disk | `/etc/redis/redis.conf` |
| **Prometheus** | 9090 | none (loopback) | local TSDB | `/etc/prometheus/prometheus.yml` + `rules/*.yml` |
| Prometheus alertmanager | 9093 | none | — | `/etc/prometheus/alertmanager.yml` |
| Prometheus node-exporter | 9100 | none | — | systemd default |
| **Grafana** | 3000 (HTTPS) | Keycloak OIDC + break-glass admin | sqlite | `/etc/grafana/grafana.ini`, `/etc/grafana/oidc-secret` |
| **Loki** | 3100 | none (loopback) | filesystem | `/etc/loki/loki-config.yaml` |
| **Promtail** | 9080 | none | journald scrape | `/etc/promtail/promtail-config.yaml` |
| **RustFS** | 9000 (S3), 9001 (console at `/rustfs/console`) | OIDC `rustfs` client | ZFS dataset `tank/objects` mounted at `/var/lib/rustfs/data` | `/etc/rustfs/rustfs.env`, `/etc/rustfs/certs/` |
| **k3s** | 6443 | k3s certs | embedded etcd | `/etc/rancher/k3s/` |
| KubeVirt (virt-api / virt-controller / virt-handler / virt-operator) | (in-cluster) | k8s SA | k3s | manifests in `deploy/k3s/` |
| CDI (apiserver / deployment / operator / uploadproxy) | (in-cluster) | k8s SA | k3s | manifests in `deploy/k3s/` |
| Cluster snapshot-controller | (in-cluster) | k8s SA | k3s | manifests in `deploy/k3s/` |
| **nova-csi controller / node** | UDS | OIDC client_credentials → nova-api | k3s | `deploy/csi/manifests/` |
| **Samba** smbd / nmbd / winbind | 445 / 137-139 | passdb | ZFS via `vfs_zfsacl` | `/etc/samba/smb.conf` + `smb.conf.d/` |
| **NFS server** | 2049 | sec=sys (krb5p when KDC online) | ZFS auto-export | `/etc/exports.d/*.exports` |
| NFS rpc-gssd / svcgssd | UDS | machine cred via `/etc/krb5.keytab` | host keytab | `/etc/default/nfs-common`, `rpc-gssd.service.d/` |
| iscsid | 3260 (initiator) | optional CHAP | LIO via configfs | `/etc/iscsi/`, `/etc/target/saveconfig.json` |
| smartmontools | — | — | textfile collector for Prometheus | `/etc/smartd.conf` |
| nova-bao-tpm-unseal | — | (oneshot) | TPM | `/etc/openbao/unseal/keys.enc` |

### 1.2 Committed but not yet deployed on the dev box

These are present in this repo but `systemctl enable` has not been run:

| Service | Purpose | Source |
|---|---|---|
| `krb5kdc.service` | MIT KDC daemon | `deploy/systemd/krb5kdc.service` + `deploy/krb5/kdc.conf` |
| `kadmind.service` | KDC admin server | `deploy/systemd/kadmind.service` + `deploy/krb5/kadm5.acl` |
| `nova-kdc-bootstrap.service` | First-boot `kdb5_util create -s` | `deploy/krb5/nova-kdc-bootstrap.sh` |
| `nova-kdc-unseal.service` | TPM-unseal KDC master key into tmpfs | `cmd/nova-kdc-unseal/` |
| `nova-krb5-sync.service` | Keycloak ↔ KDC user-principal sync | `cmd/nova-krb5-sync/` |
| `nova-zfs-keyload.service` | TPM-unseal ZFS dataset keys at boot | `cmd/nova-zfs-keyload/` |
| `oauth2-proxy-prometheus.service` | OIDC SSO in front of Prometheus | `deploy/systemd/` |
| `oauth2-proxy-alertmanager.service` | same for Alertmanager | `deploy/systemd/` |
| `oauth2-proxy-loki.service` | same for Loki | `deploy/systemd/` |
| `nova-iscsi-restore.service` | restore LIO config at boot | `cmd/nova-iscsi-restore/` |
| `nova-nvmet-restore.service` | restore NVMe-oF config | `cmd/nova-nvmet-restore/` |
| Prometheus alert rules (47 rules) | full alerting catalog | `deploy/prometheus/rules/*.yml` |
| Grafana OIDC | Keycloak SSO for the dashboard | `deploy/grafana/grafana.ini` (config present, secret + restart pending) |

### 1.3 On-disk locations (all hosts)

```
/etc/nova-nas/tls/cert.pem, key.pem    nova-api HTTPS material
/etc/nova-nas/secrets/                 file-backed secrets manager root
/etc/nova-ca/{ca.crt,ca.key,ca.srl}    local CA — signs all internal certs

/etc/keycloak/tls/                     Keycloak server cert
/etc/keycloak/keycloak.conf            Keycloak runtime config (when used)
/opt/keycloak/                         Keycloak install (26.0.7)

/etc/grafana/grafana.ini               Grafana + OIDC stub
/etc/grafana/oidc-secret               grafana Keycloak client secret (mode 0400)
/etc/grafana/{grafana.crt,grafana.key} Grafana TLS cert (signed by Nova CA)
/etc/grafana/provisioning/             datasources + dashboards

/etc/openbao/config.hcl                raft + TLS
/etc/openbao/unseal/keys.enc           TPM-sealed unseal keys
/var/lib/openbao/raft/                 OpenBao state

/etc/prometheus/prometheus.yml
/etc/prometheus/rules/*.yml            recording + alerting rules (47 rules across 6 files)
/etc/alertmanager/alertmanager.yml     routes + receivers
/etc/loki/loki-config.yaml
/etc/promtail/promtail-config.yaml

/etc/rustfs/rustfs.env                 RustFS config (OIDC client secret + root creds)
/etc/rustfs/certs/{rustfs_cert.pem, rustfs_key.pem}

/etc/exports.d/*.exports               NFS exports (per-share files written by nova-api)
/etc/samba/smb.conf                    base config
/etc/samba/smb.conf.d/*.conf           per-share files written by nova-api

/etc/krb5.conf                         Kerberos client config
/etc/krb5.keytab                       host machine principal (NFS sec=krb5p)
/etc/krb5kdc/{kdc.conf,kadm5.acl}      KDC server config
/etc/idmapd.conf                       NFSv4 uid/gid mapping (Domain=novanas.local)
/etc/default/nfs-common                rpc.gssd flags (GSSDARGS="-n" for machine creds)
/etc/systemd/system/rpc-gssd.service.d/override.conf

/var/lib/krb5kdc/                      KDC database
/run/krb5kdc/.k5.NOVANAS.LOCAL         TPM-unsealed master key (tmpfs at boot)

/etc/iscsi/initiatorname.iscsi
/etc/target/saveconfig.json            LIO persisted config
/etc/nvmet/                            NVMe-oF persisted config

/etc/nova-csi/                         CSI controller secrets (oidc-client-id/secret, ca.crt)
/etc/nova-kdc/                         master.enc (TPM-sealed KDC stash) when sealed
/etc/nova-krb5-sync/oidc-client-secret OIDC client secret for the sync daemon

/var/lib/rustfs/data/                  RustFS objects == ZFS dataset tank/objects
/var/lib/grafana/grafana.db
/var/lib/{prometheus,alertmanager,loki,promtail}/

/etc/rancher/k3s/                      k3s config
/var/lib/rancher/k3s/                  k3s state
```

### 1.4 ZFS layout

```
tank                  raidz2 over 6× WD HDD (~7.14 TiB usable)
├── csi/              CSI-managed PVC datasets (provisioned by nova-csi)
│   └── pvc-*         per-PVC datasets (auto-mounted) or zvols (block)
├── objects           RustFS data dir (recordsize=1M, atime=off, compression=lz4,
│                     mountpoint=/var/lib/rustfs/data)
└── (postgres, redis, observability datasets — split out from / when scaling)

logs vdev:  BIWIN X570 PRO NVMe (SLOG)
cache vdev: 2× Samsung SSD (L2ARC)
```

NVMe namespace 0 (the OS NVMe) is **never touched** by NovaNAS automation.

---

## 2. From-scratch reproduction

This is the canonical build sequence. Run in order; each step references
the relevant `deploy/` artifact in this repo.

### 2.1 Base OS

- Debian 13 (trixie), x86_64, kernel 6.12 line
- During install: minimal base, no desktop, OpenSSH server only
- Network: static IP on the management LAN (RFC1918 only)
- Hostname: `novanas` (matches `kubernetes.io/hostname` selectors in
  `deploy/csi/manifests/`)

### 2.2 Packages

```bash
sudo apt update && sudo apt install -y \
  zfsutils-linux \
  nfs-kernel-server nfs-common \
  samba samba-common-bin winbind \
  open-iscsi targetcli-fb \
  nvme-cli nvmetcli \
  smartmontools \
  postgresql-17 redis-server \
  prometheus prometheus-alertmanager prometheus-node-exporter \
  grafana \
  krb5-kdc krb5-admin-server krb5-user libpam-krb5 \
  tpm2-tools \
  ca-certificates curl jq unzip openssl \
  build-essential git \
  openjdk-21-jre-headless
```

Loki and Promtail are not in Debian's main archive at the right version.
Install from Grafana's apt repo:

```bash
curl -fsSL https://apt.grafana.com/gpg.key | sudo tee /etc/apt/keyrings/grafana.asc
echo "deb [signed-by=/etc/apt/keyrings/grafana.asc] https://apt.grafana.com stable main" \
  | sudo tee /etc/apt/sources.list.d/grafana.list
sudo apt update && sudo apt install -y loki promtail
```

### 2.3 Local CA

```bash
sudo mkdir -p /etc/nova-ca && cd /etc/nova-ca
sudo openssl genrsa -out ca.key 4096
sudo openssl req -x509 -new -nodes -key ca.key -sha256 -days 365 \
  -subj "/CN=NovaNAS Local CA" -out ca.crt
sudo chmod 644 ca.crt && sudo chmod 600 ca.key
```

### 2.4 ZFS pool

Use **WWN identifiers**, not `/dev/sd?` (those are not stable across
kernel boots).

```bash
sudo zpool create -o ashift=12 tank \
  raidz2 wwn-0x... wwn-0x... wwn-0x... wwn-0x... wwn-0x... wwn-0x... \
  log nvme-... \
  cache wwn-0x... wwn-0x...

sudo zfs create -o atime=off tank/csi
sudo zfs create -o recordsize=1M -o atime=off -o compression=lz4 \
  -o mountpoint=/var/lib/rustfs/data tank/objects
```

### 2.5 Postgres + Redis

```bash
sudo systemctl enable --now postgresql@17-main
sudo -u postgres psql <<EOF
CREATE USER novanas WITH PASSWORD 'novanas';
CREATE DATABASE novanas OWNER novanas;
CREATE USER keycloak WITH PASSWORD 'kcpass';
CREATE DATABASE keycloak OWNER keycloak;
EOF

sudo systemctl enable --now redis-server   # default: bind 127.0.0.1, no auth
```

### 2.6 OpenBao

Install from <https://github.com/openbao/openbao/releases> (v2.4.1 at
this snapshot). Install systemd units `deploy/systemd/openbao.service`
and `deploy/systemd/nova-bao-tpm-unseal.service`.

```bash
sudo systemctl enable --now nova-bao-tpm-unseal openbao

# First boot: unseal --init reads plaintext keys from stdin, TPM-seals,
# writes the encrypted blob, and the operator shred-deletes the plaintext.
echo '["<key1>","<key2>","<key3>"]' \
  | sudo /usr/local/bin/nova-bao-unseal --init
```

### 2.7 Keycloak

```bash
sudo mkdir -p /opt/keycloak && cd /tmp
curl -fsSL https://github.com/keycloak/keycloak/releases/download/26.0.7/keycloak-26.0.7.tar.gz \
  | sudo tar xz -C /opt/keycloak --strip-components=1

sudo /opt/keycloak/bin/kc.sh build --db=postgres --health-enabled=true

sudo cp deploy/systemd/keycloak.service /etc/systemd/system/
sudo cp -r deploy/systemd/keycloak.service.d /etc/systemd/system/  # management-port override
sudo systemctl daemon-reload
sudo systemctl enable --now keycloak

# Bootstrap admin: admin / adminpw  -- ROTATE BEFORE PRODUCTION
# Realm import:
sudo /opt/keycloak/bin/kcadm.sh config credentials \
  --server https://192.168.10.204:8443 --realm master \
  --user admin --password adminpw --config /tmp/.kcadm.config

sudo /opt/keycloak/bin/kcadm.sh create realms \
  -f deploy/keycloak/realm-novanas.json \
  --config /tmp/.kcadm.config
```

The realm has roles `nova-admin` / `nova-operator` / `nova-viewer` and
the `nova-tenant` / `nova-platform-nfs` user-profile attributes.

### 2.8 Per-service Keycloak clients

Each script in `deploy/keycloak/` provisions one client:

```bash
export KC_URL=https://192.168.10.204:8443
export KC_ADMIN_PASS=adminpw
export KCADM=/opt/keycloak/bin/kcadm.sh
export TLS_INSECURE=true

deploy/keycloak/create-csi-client.sh           > /tmp/csi.json
deploy/keycloak/create-rustfs-client.sh        > /tmp/rustfs.json
deploy/keycloak/create-grafana-client.sh       > /tmp/grafana.json
deploy/keycloak/create-oauth2-proxy-clients.sh > /tmp/o2p.json
deploy/keycloak/create-krb5-sync-client.sh     > /tmp/krb5sync.json
```

Each emits a JSON object with a freshly-rotated client secret. Each
service's README explains how to drop the secret into its config.

**Known kcadm gotchas** (handled in the scripts since `49a6c96`):

- `--config` must come **after** the subcommand on Keycloak 26+. The
  scripts use `HOME=$CFGDIR` so kcadm uses its default config path.
- `--format csv --noquotes` doesn't emit a header row on K26+, so the
  scripts no longer use `tail -n +2`.
- `kcadm create -i` doesn't reliably echo the new ID; the scripts
  re-query `get clients -q clientId=...` instead.

### 2.9 nova-api

Build from repo root:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -o /usr/local/bin/nova-api ./cmd/nova-api
```

Required env (full reference: `internal/config/config.go`):

```
DATABASE_URL=postgres://novanas:novanas@127.0.0.1:5432/novanas?sslmode=disable
REDIS_URL=redis://127.0.0.1:6379
LISTEN_ADDR=0.0.0.0:8090
TLS_HTTPS_ADDR=0.0.0.0:8444
TLS_CERT_PATH=/etc/nova-nas/tls/cert.pem
TLS_KEY_PATH=/etc/nova-nas/tls/key.pem
OIDC_ISSUER_URL=https://192.168.10.204:8443/realms/novanas
OIDC_AUDIENCE=nova-api
OIDC_REQUIRED_ROLE_PREFIX=nova-
SECRETS_BACKEND=file
SECRETS_FILE_ROOT=/etc/nova-nas/secrets

# Opt-in features (default false):
NFS_REQUIRE_KERBEROS=true       # only when KDC + host keytab exist
KRB5_KDC_ENABLED=true           # only when embedded KDC is running
KRB5_REALM=NOVANAS.LOCAL
SECRETS_FILE_TPM_SEAL=true      # encrypt secrets at rest with TPM

# SMTP (optional — can also be set at runtime via PUT /api/v1/notifications/smtp):
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USERNAME=...
SMTP_PASSWORD_FILE=/etc/nova-nas/smtp.password
SMTP_FROM=nova-api@novanas.local
SMTP_TLS_MODE=starttls
```

Migrations (no embedded migrator yet):

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
DB_URL='postgres://novanas:novanas@127.0.0.1:5432/novanas?sslmode=disable' \
  make migrate-up
```

Migration files in `internal/store/migrations/`: `0001_init.sql`,
`0002_scheduler.sql`, `0003_scrub_policies.sql`, `0004_replication.sql`.

```bash
sudo cp deploy/systemd/nova-api.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now nova-api
```

### 2.10 Samba + NFS

NFS exports + Samba shares are managed declaratively by nova-api:
`internal/host/nfs/` writes `/etc/exports.d/nova-nas-<share>.exports`,
`internal/host/samba/` writes `/etc/samba/smb.conf.d/`. Globals are a
one-time bootstrap.

```bash
sudo systemctl enable --now nfs-kernel-server smbd nmbd winbind

# Apply Samba globals (vfs_zfsacl, ACL inheritance, etc.):
TOKEN=$(...)  # admin Keycloak access token
curl -k --cacert /etc/nova-ca/ca.crt -X PUT \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d @deploy/samba/globals.json \
  https://localhost:8444/api/v1/samba/globals
```

For NFSv4 ACL parity over Samba you need a **patched** `nfs4-acl-tools`
that understands ZFS's XDR-encoded ACL xattr. Tracked in **#85**.

### 2.11 iSCSI / NVMe-oF

Block exports go through LIO via configfs. Persistence:

```bash
sudo systemctl enable --now iscsid
sudo systemctl enable nova-iscsi-restore   # oneshot, replays JSON at boot
sudo systemctl enable nova-nvmet-restore   # same for nvmet
```

### 2.12 k3s + KubeVirt + nova-csi

```bash
# k3s — single-node, no traefik, no servicelb
curl -sfL https://get.k3s.io \
  | INSTALL_K3S_EXEC="server --disable=traefik --disable=servicelb" sh -

# KubeVirt + CDI + cluster snapshot-controller
sudo k3s kubectl apply -f deploy/k3s/kubevirt-operator.yaml
sudo k3s kubectl apply -f deploy/k3s/kubevirt-cr.yaml
sudo k3s kubectl apply -f deploy/k3s/cdi-operator.yaml
sudo k3s kubectl apply -f deploy/k3s/cdi-cr.yaml
sudo k3s kubectl apply -f deploy/k3s/snapshot-controller.yaml

# nova-csi
deploy/csi/build-image.sh
sudo k3s ctr images import deploy/csi/novanas-csi.tar
sudo k3s kubectl apply -f deploy/csi/manifests/

# OIDC client secret for the CSI controller:
SECRET=$(jq -r .clientSecret /tmp/csi.json)
sudo k3s kubectl -n nova-csi create secret generic nova-csi-auth \
  --from-literal=oidc-client-id=nova-csi \
  --from-literal=oidc-client-secret="$SECRET" \
  --from-file=ca.crt=/etc/nova-ca/ca.crt
```

### 2.13 Observability

```bash
# Prometheus + Alertmanager
sudo cp deploy/prometheus/prometheus.yml /etc/prometheus/
sudo mkdir -p /etc/prometheus/rules
sudo cp deploy/prometheus/rules/*.yml /etc/prometheus/rules/
promtool check rules /etc/prometheus/rules/*.yml
sudo cp deploy/alertmanager/alertmanager.yml /etc/alertmanager/   # or merge with .example
sudo systemctl restart prometheus prometheus-alertmanager

# Grafana
sudo cp deploy/grafana/grafana.ini /etc/grafana/
sudo cp -r deploy/grafana/provisioning /etc/grafana/
sudo cp -r deploy/grafana/dashboards /var/lib/grafana/dashboards

SECRET=$(jq -r .clientSecret /tmp/grafana.json)
echo -n "$SECRET" | sudo install -m 0400 -o grafana -g grafana \
  /dev/stdin /etc/grafana/oidc-secret

# Issue Grafana TLS cert from the Nova CA (mirror the rustfs cert recipe
# in deploy/rustfs/README.md, with CN=grafana.novanas.local)
sudo systemctl restart grafana-server

# Loki + Promtail
sudo cp deploy/loki/loki-config.yaml /etc/loki/
sudo cp deploy/promtail/promtail-config.yaml /etc/promtail/
sudo systemctl enable --now loki promtail
```

For SSO in front of Prometheus / Alertmanager / Loki, install
`oauth2-proxy` and apply `deploy/systemd/oauth2-proxy-*.service` after
running `deploy/keycloak/create-oauth2-proxy-clients.sh`.

### 2.14 RustFS (object storage)

```bash
deploy/rustfs/install.sh   # downloads pinned 1.0.0-beta.1, installs binary + unit

# Issue TLS cert from the Nova CA (see deploy/rustfs/README.md TLS section)

# Inject the Keycloak client secret + rotate the root S3 keys:
SECRET=$(jq -r .clientSecret /tmp/rustfs.json)
sudo sed -i \
  "s|^RUSTFS_IDENTITY_OPENID_CLIENT_SECRET=.*|RUSTFS_IDENTITY_OPENID_CLIENT_SECRET=$SECRET|" \
  /etc/rustfs/rustfs.env
sudo sed -i "s|^RUSTFS_SECRET_KEY=.*|RUSTFS_SECRET_KEY=$(openssl rand -hex 16)|" \
  /etc/rustfs/rustfs.env

sudo systemctl enable --now rustfs
```

Browser: `https://<host>:9001/rustfs/console`. S3: `https://<host>:9000`.

### 2.15 KDC (when ready to enable)

```bash
go build -o /usr/local/bin/nova-kdc-unseal ./cmd/nova-kdc-unseal

sudo cp deploy/systemd/{krb5kdc,kadmind,nova-kdc-bootstrap,nova-kdc-unseal}.service \
  /etc/systemd/system/
sudo cp deploy/krb5/kdc.conf /etc/krb5kdc/kdc.conf
sudo cp deploy/krb5/kadm5.acl /etc/krb5kdc/kadm5.acl

# First boot: bootstrap creates the KDB + TPM-seals master key
sudo NOVA_KDC_TPM_SEAL=1 deploy/krb5/nova-kdc-bootstrap.sh

sudo systemctl enable --now nova-kdc-unseal krb5kdc kadmind
```

Enable host-side Kerberos for NFS:

```bash
sudo cp deploy/krb5/krb5.conf.template /etc/krb5.conf  # edit realm if needed
sudo cp deploy/krb5/idmapd.conf /etc/idmapd.conf
sudo cp deploy/systemd/rpc-gssd.service.d/override.conf \
  /etc/systemd/system/rpc-gssd.service.d/override.conf

# Create the NFS host service principal + keytab via the new KDC API:
TOKEN=...  # admin Keycloak token
curl -k --cacert /etc/nova-ca/ca.crt -X POST \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"nfs/novanas.novanas.local","randkey":true}' \
  https://localhost:8444/api/v1/krb5/principals

curl -k --cacert /etc/nova-ca/ca.crt -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -o /etc/krb5.keytab \
  https://localhost:8444/api/v1/krb5/principals/nfs%2Fnovanas.novanas.local/keytab

# Flip nova-api to require krb5p on all NFS exports:
sudo sed -i 's|^NFS_REQUIRE_KERBEROS=.*|NFS_REQUIRE_KERBEROS=true|' /etc/default/nova-api
sudo sed -i 's|^KRB5_KDC_ENABLED=.*|KRB5_KDC_ENABLED=true|' /etc/default/nova-api
sudo systemctl restart nova-api rpc-gssd nfs-kernel-server
```

### 2.16 Keycloak ↔ KDC sync

```bash
go build -o /usr/local/bin/nova-krb5-sync ./cmd/nova-krb5-sync
sudo cp deploy/systemd/nova-krb5-sync.service /etc/systemd/system/

SECRET=$(jq -r .clientSecret /tmp/krb5sync.json)
echo -n "$SECRET" | sudo install -m 0400 \
  -o nova-krb5-sync -g nova-krb5-sync \
  /dev/stdin /etc/nova-krb5-sync/oidc-client-secret

sudo systemctl enable --now nova-krb5-sync
```

### 2.17 ZFS encryption + key escrow

```bash
go build -o /usr/local/bin/nova-zfs-keyload ./cmd/nova-zfs-keyload
sudo cp deploy/systemd/nova-zfs-keyload.service /etc/systemd/system/
sudo systemctl enable nova-zfs-keyload
# Runs Before=zfs-mount + nfs-server + smbd, Requires=openbao.service.
```

Provision encrypted datasets via:
`POST /api/v1/datasets/{full}/encryption`.

---

## 3. nova-api endpoint catalog

Verified live on the dev box at `https://192.168.10.204:8444` against
the latest `main`:

| Endpoint | Method | Auth | Purpose |
|---|---|---|---|
| `/healthz` | GET | none | liveness |
| `/metrics` | GET | none | Prometheus scrape (91 nova_* metrics) |
| `/openapi.yaml` | GET | none | spec |
| `/api/v1/pools` | GET / POST | viewer / operator | ZFS pool CRUD |
| `/api/v1/pools/{name}/scrub` | POST | operator | trigger scrub |
| `/api/v1/datasets` | GET / POST | viewer / operator | dataset CRUD |
| `/api/v1/datasets/{full}/encryption` | POST | operator | provision encrypted dataset |
| `/api/v1/datasets/{full}/encryption/{load,unload,recover}-key` | POST | operator / admin | encryption key ops |
| `/api/v1/snapshots` | GET / POST | viewer / operator | snapshots |
| `/api/v1/snapshot-policies` | GET / PUT / DELETE | operator | snapshot scheduling |
| `/api/v1/scrub-policies` | GET / POST / PATCH / DELETE | viewer / operator | scrub scheduling |
| `/api/v1/replication-jobs` | GET / POST / PATCH / DELETE | viewer / operator | replication CRUD |
| `/api/v1/replication-jobs/{id}/run` | POST | operator | trigger replication |
| `/api/v1/replication-jobs/{id}/runs` | GET | viewer | run history |
| `/api/v1/protocol-shares` | GET / POST / PATCH / DELETE | viewer / operator | unified NFS+SMB shares |
| `/api/v1/iscsi/...` | full CRUD | viewer / operator | iSCSI |
| `/api/v1/nvmeof/...` | full CRUD | viewer / operator | NVMe-oF |
| `/api/v1/krb5/principals` | full CRUD + `/keytab` | viewer / admin | KDC principal management (when `KRB5_KDC_ENABLED=true`) |
| `/api/v1/krb5/kdc/status` | GET | viewer | KDC daemon status |
| `/api/v1/notifications/smtp` | GET / PUT / `POST /test` | admin | SMTP relay config + test send |
| `/api/v1/audit` | GET | operator | audit log read (cursor-paginated) |
| `/api/v1/audit/summary` | GET | operator | aggregate counts |
| `/api/v1/audit/export?format=csv\|jsonl` | GET | operator | streaming export |
| `/api/v1/network/interfaces` | GET | viewer | NIC inventory |
| `/api/v1/network/configs` | full CRUD | viewer / operator | network config |
| `/api/v1/disks` | GET | viewer | disk + SMART |
| `/api/v1/secrets/...` | GET / PUT / DELETE | admin | secrets manager (file or OpenBao backend) |
| `/api/v1/jobs` | GET | viewer | Asynq job state |

All `/api/v1/*` requires `Authorization: Bearer <Keycloak access token>`.

Smoke test on the dev box (live):

```bash
$ curl -sk --cacert /etc/nova-ca/ca.crt https://127.0.0.1:8444/healthz
{"status":"ok"}

$ for path in /api/v1/notifications/smtp /api/v1/audit /api/v1/scrub-policies \
              /api/v1/replication-jobs /api/v1/krb5/kdc/status; do
    code=$(curl -sk --cacert /etc/nova-ca/ca.crt -o /dev/null -w "%{http_code}" \
              "https://127.0.0.1:8444${path}")
    printf "%-35s %s\n" "$path" "$code"
  done
/api/v1/notifications/smtp          401  # auth required, route exists
/api/v1/audit                       401
/api/v1/scrub-policies              401
/api/v1/replication-jobs            401
/api/v1/krb5/kdc/status             401
```

---

## 4. From a fresh Debian 13 to a working NovaNAS — at-a-glance

| # | Section | Time | Notes |
|---|---|---|---|
| 1 | Base OS install | 15 min | Debian 13 minimal |
| 2 | Packages (apt + Grafana repo) | 5 min | §2.2 |
| 3 | Local CA | 1 min | §2.3 |
| 4 | ZFS pool + datasets | 5 min | §2.4 |
| 5 | Postgres + Redis | 2 min | §2.5 |
| 6 | OpenBao + first-boot unseal | 10 min | §2.6 |
| 7 | Keycloak + realm import | 15 min | §2.7 |
| 8 | Per-service Keycloak clients | 5 min | §2.8 |
| 9 | nova-api + DB migrations | 10 min | §2.9 |
| 10 | Samba + NFS bootstrap | 5 min | §2.10 |
| 11 | iSCSI / NVMe-oF | 2 min | §2.11 |
| 12 | k3s + KubeVirt + CDI + nova-csi | 20 min | §2.12 |
| 13 | Observability stack | 10 min | §2.13 |
| 14 | RustFS | 5 min | §2.14 |
| 15 | KDC + krb5 host enable + sync daemon | 15 min | §2.15–2.16 |
| 16 | ZFS encryption + keyload | 5 min | §2.17 |
| **Total** | | **≈2 h** | for someone with this runbook in front of them |

---

## 5. Open issues / undeployed gaps

- **#85** — patched `nfs4-acl-tools` for ZFS XDR ACL xattr
- **#86** — Web GUI / React frontend
- **#87** — 2FA / MFA enforcement on the realm
- **#88** — UPS / NUT integration
- **#89** — Per-user / per-project ZFS quotas
- **#90** — Package Center / app ecosystem
- **#91** — Hardware bay-to-device mapping

Plus the undeployed-on-dev-box items in §1.2.

---

## 6. Versions captured (2026-04-29)

```
Debian          13 (trixie)   kernel 6.12.74+deb13+1-amd64
zfsutils-linux  2.3.2-2
samba           4.22.8
nfs-common      2.8.3
postgresql      17.9
redis-server    (Debian default)
prometheus      2.53.3
grafana         13.0.1
loki            3.7.1
promtail        3.6.10
smartmontools   7.4-3
krb5            1.21.3
nova-ca         CN=NovaNAS Local CA, valid 2026-04-29 → 2027-04-29
keycloak        26.0.7
openbao         2.4.1
k3s             v1.35.4+k3s1 (go 1.25.9)
rustfs          1.0.0-beta.1
```
