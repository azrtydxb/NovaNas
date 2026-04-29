# NovaNAS object storage (RustFS)

NovaNAS exposes S3-compatible object storage via [RustFS](https://rustfs.com)
running directly on the storage host (systemd, not k3s). Identity is federated
to the Nova Keycloak realm so the same `nova-admin` / `nova-operator` /
`nova-viewer` realm roles that gate everything else also gate object storage.

> Deployment, install scripts, and systemd unit live under
> `deploy/rustfs/` and `deploy/systemd/rustfs.service`. This document is
> the operator-facing runbook.

## What you get

- **S3 API** on `https://<host>:9000` — works with any S3 SDK (AWS, boto3,
  go-aws-sdk-v2, s3cmd, rclone, mc, …).
- **Web console** on `https://<host>:9001/rustfs/console` — protected by Keycloak SSO
  (auth-code flow). Log in as a user with realm role `nova-admin`,
  `nova-operator`, or `nova-viewer`.
- **ZFS-native storage**: data lives on the dataset `tank/objects`, so you
  get snapshots, send/recv replication, scrubs, and compression for free.
- **TLS** terminated in RustFS itself, using a cert signed by the local
  Nova CA at `/etc/nova-ca/ca.crt`.

## How Keycloak SSO works for the console

The flow is the standard OAuth 2.0 / OIDC auth-code flow:

1. User browses to `https://<host>:9001/rustfs/console`.
2. RustFS console redirects them to Keycloak
   (`https://192.168.10.204:8443/realms/novanas`).
3. User authenticates against Keycloak (password, mTLS, whatever the realm
   is configured for).
4. Keycloak redirects back to RustFS with an authorization code.
5. RustFS exchanges the code for an access token. The token carries:
   - `aud`: includes `rustfs` (added by the audience mapper that
     `create-rustfs-client.sh` installs).
   - `groups`: a flat list of the user's realm roles —
     e.g. `["nova-operator"]` — emitted by the realm-roles-as-groups
     protocol mapper.
6. RustFS reads the `groups` claim
   (`RUSTFS_IDENTITY_OPENID_GROUPS_CLAIM=groups`) and uses each value as
   a key into its IAM policy mappings.

### OIDC token claim layout

```json
{
  "iss": "https://192.168.10.204:8443/realms/novanas",
  "aud": ["rustfs", "account"],
  "preferred_username": "alice",
  "groups": ["nova-operator"],
  "roles": ["nova-operator"],
  "realm_access": { "roles": ["nova-operator", "default-roles-novanas"] }
}
```

### Realm role -> RustFS policy mapping

| Realm role     | Recommended RustFS policy | What they can do |
| -------------- | ------------------------- | ---------------- |
| `nova-admin`    | `consoleAdmin`            | Full admin: any bucket, any object, IAM. |
| `nova-operator` | `readwrite`               | Create buckets, put/get/delete objects in any bucket. |
| `nova-viewer`   | `readonly`                | List buckets, get objects. No mutations. |

Attach policies to groups (one-time setup, after RustFS is up and a
`nova-admin` user has logged in once so RustFS knows the group exists):

```bash
# Using the AWS CLI with root credentials from /etc/rustfs/rustfs.env
aws --endpoint-url https://<host>:9000 --no-verify-ssl \
    iam attach-group-policy --group-name nova-admin    --policy-arn arn:aws:iam::aws:policy/consoleAdmin
aws --endpoint-url https://<host>:9000 --no-verify-ssl \
    iam attach-group-policy --group-name nova-operator --policy-arn arn:aws:iam::aws:policy/readwrite
aws --endpoint-url https://<host>:9000 --no-verify-ssl \
    iam attach-group-policy --group-name nova-viewer   --policy-arn arn:aws:iam::aws:policy/readonly
```

> **How RustFS resolves an OIDC user to a policy** (verified against
> [`crates/iam/src/oidc.rs`](https://github.com/rustfs/rustfs/blob/main/crates/iam/src/oidc.rs)):
> on each request, RustFS reads the configured `groups_claim` from the
> JWT and looks up an IAM policy of the same name. The Keycloak protocol
> mapper we install emits realm roles (`nova-admin`, `nova-operator`,
> `nova-viewer`) flattened into the `groups` claim, so each role name
> needs an IAM policy of that exact name to exist on the RustFS side.
> The `aws iam create-policy` + `attach-group-policy` calls above are
> the supported path; the RustFS console UI (Identity → Policies) is
> the no-CLI alternative. `RUSTFS_IDENTITY_OPENID_ROLE_POLICY` lets you
> set a baseline policy attached to every authenticated user as a
> safety net — see `rustfs.env.template`.

## Quickstart for end users

### AWS CLI with OIDC token (STS)

For browser-driven workflows, the simplest path is: log in to the console
once, generate a "service account" access key from the console UI for your
user, then use it from the AWS CLI.

```bash
aws configure --profile nova
# AWS Access Key ID:    <key from RustFS console>
# AWS Secret Access Key: <secret from RustFS console>
# Default region:        us-east-1   (RustFS ignores this but the CLI insists)

aws --profile nova --endpoint-url https://<host>:9000 \
    --ca-bundle /etc/nova-ca/ca.crt \
    s3 ls
```

For purely token-driven access (e.g. CI), exchange a Keycloak access token
for temporary S3 credentials via the AssumeRoleWithWebIdentity STS endpoint:

```bash
TOKEN=$(curl -s -k \
    -d "grant_type=password" \
    -d "client_id=rustfs" \
    -d "client_secret=$RUSTFS_CLIENT_SECRET" \
    -d "username=$USER" -d "password=$PASS" \
    -d "scope=openid" \
    https://192.168.10.204:8443/realms/novanas/protocol/openid-connect/token \
    | jq -r .access_token)

CREDS=$(curl -s --cacert /etc/nova-ca/ca.crt \
    "https://<host>:9000/?Action=AssumeRoleWithWebIdentity&Version=2011-06-15&WebIdentityToken=$TOKEN&DurationSeconds=3600")

# Parse AccessKeyId / SecretAccessKey / SessionToken from $CREDS XML and
# export as AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN.
```

### Create a bucket and upload an object

```bash
aws --profile nova --endpoint-url https://<host>:9000 \
    --ca-bundle /etc/nova-ca/ca.crt \
    s3 mb s3://photos

echo "hello, novanas" > /tmp/hello.txt
aws --profile nova --endpoint-url https://<host>:9000 \
    --ca-bundle /etc/nova-ca/ca.crt \
    s3 cp /tmp/hello.txt s3://photos/hello.txt

aws --profile nova --endpoint-url https://<host>:9000 \
    --ca-bundle /etc/nova-ca/ca.crt \
    s3 ls s3://photos/
```

### s3cmd

```ini
# ~/.s3cfg
host_base = <host>:9000
host_bucket = <host>:9000
access_key = <key>
secret_key = <secret>
use_https = True
ca_certs_file = /etc/nova-ca/ca.crt
signature_v2 = False
```

```bash
s3cmd mb s3://archives
s3cmd put bigfile.tar.gz s3://archives/
```

## Troubleshooting

### 403 AccessDenied on every request

- **Token is missing the `rustfs` audience.** Decode the token at
  <https://jwt.io> and check `aud`. If `rustfs` is not present, re-run
  `deploy/keycloak/create-rustfs-client.sh` — it installs the audience
  mapper.
- **`groups` claim is empty.** RustFS only sees `groups`, not
  `realm_access.roles`. The realm-roles-as-groups protocol mapper that
  `create-rustfs-client.sh` installs is what flattens realm roles into
  `groups`. Confirm via the Keycloak admin UI under Clients -> rustfs ->
  Client scopes -> Mappers.
- **No IAM policy attached to the group.** RustFS treats unmapped groups
  as having zero permissions. See "Realm role -> RustFS policy mapping"
  above.

### `Invalid login token` in the console

- Clock skew between the RustFS host and Keycloak — sync NTP.
- The realm signing key was rotated; restart `rustfs.service` so it
  re-fetches the JWKS from
  `https://192.168.10.204:8443/realms/novanas/protocol/openid-connect/certs`.

### `unable to get local issuer certificate`

The S3 client must trust the Nova CA. Either pass `--ca-bundle
/etc/nova-ca/ca.crt` (AWS CLI) or `--cacert ...` (curl) on every call,
or install the CA into the system trust store (`update-ca-certificates`
on Debian/Ubuntu).

### Service won't start: "address already in use"

```bash
sudo ss -ltnp | grep -E ':(9000|9001)\b'
```

Some other service (often a leftover MinIO) is on 9000/9001. Stop it or
change `RUSTFS_ADDRESS` / `RUSTFS_CONSOLE_ADDRESS` in
`/etc/rustfs/rustfs.env`.

### OIDC env-var reference

All `RUSTFS_IDENTITY_OPENID_*` variables in `rustfs.env.template` are
verified against the canonical source-of-truth file in the RustFS repo:
[`crates/config/src/constants/oidc.rs`](https://github.com/rustfs/rustfs/blob/main/crates/config/src/constants/oidc.rs).
That file declares every `ENV_IDENTITY_OPENID_*` constant the daemon reads.
If a variable looks like it's being ignored on your pinned release, check
the same file at the matching tag on GitHub.

## Backup and snapshot

Object storage data lives on the ZFS dataset `tank/objects`. All the
standard ZFS tools work:

### Snapshot

```bash
sudo zfs snapshot tank/objects@daily-$(date +%Y%m%d)
sudo zfs list -t snapshot tank/objects
```

### Restore an entire dataset (rollback)

This is destructive and rolls back every object in every bucket to the
state at the snapshot time. Use it only for disaster recovery.

```bash
sudo systemctl stop rustfs.service
sudo zfs rollback -r tank/objects@daily-20260429
sudo systemctl start rustfs.service
```

### Granular restore (single bucket / object)

ZFS snapshots are mounted under `tank/objects/.zfs/snapshot/<name>/`.
Copy back what you need:

```bash
sudo cp -a /var/lib/rustfs/data/.zfs/snapshot/daily-20260429/photos/hello.txt \
          /var/lib/rustfs/data/photos/hello.txt
```

> **Caveat:** copying files directly into the data directory bypasses
> RustFS's metadata layer. For a single-node deployment it works, but for
> anything beyond ad-hoc recovery prefer the API path:
> `aws s3 cp ...` from a snapshot mount.

### Off-host replication

```bash
sudo zfs send -i tank/objects@yesterday tank/objects@today \
    | ssh backup zfs recv backup/rustfs-mirror
```

## See also

- `deploy/rustfs/README.md` — install / upgrade / pinned version details.
- `deploy/systemd/rustfs.service` — the systemd unit (sandbox flags, etc).
- `deploy/keycloak/create-rustfs-client.sh` — Keycloak client provisioning.
