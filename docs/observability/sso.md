# Observability SSO (Keycloak)

This document covers the single sign-on layer in front of the NovaNAS
observability stack. It describes the bootstrap order, the URLs an
operator needs, and the most common failure modes.

## Architecture

```
                    +--------------------------+
                    |    Keycloak (realm:      |
                    |    novanas, :8443)       |
                    +-----------+--------------+
                                |
        OIDC auth-code flow     |     OIDC auth-code flow
        +-----------------------+-----------------------+
        |                       |                       |
        v                       v                       v
+---------------+      +-----------------+      +-----------------+
| Grafana       |      | oauth2-proxy    |      | oauth2-proxy    |
| :3000  HTTPS  |      | -prometheus     |      | -alertmanager   |
| (native OIDC) |      | :9091  HTTPS    |      | :9094  HTTPS    |
+-------+-------+      +--------+--------+      +--------+--------+
        |                       |                        |
        |  Prom/Loki            v                        v
        |  data sources    Prometheus :9090       Alertmanager :9093
        |  (loopback)      (127.0.0.1, HTTPS)     (127.0.0.1, HTTPS)
        |
        |                +-----------------+
        +--------------->| oauth2-proxy    |
                         | -loki           |
                         | :3101  HTTPS    |
                         +--------+--------+
                                  |
                                  v
                            Loki :3100
                            (127.0.0.1, HTTP)
```

The four upstream services keep their existing localhost-only bindings.
Promtail still pushes logs directly to `http://127.0.0.1:3100/loki/api/v1/push`,
so the Loki ingestion path is not affected by SSO.

## Keycloak clients

Realm: `novanas` (already exists, ships with realm roles
`nova-admin`, `nova-operator`, `nova-viewer`).

| Client ID | Type | Used by | Roles allowed |
|-----------|------|---------|---------------|
| `grafana` | confidential, auth-code, PKCE | Grafana OIDC | all (mapped to Admin/Editor/Viewer) |
| `oauth2-proxy-prometheus` | confidential, auth-code | oauth2-proxy on :9091 | `nova-admin`, `nova-operator` |
| `oauth2-proxy-alertmanager` | confidential, auth-code | oauth2-proxy on :9094 | `nova-admin`, `nova-operator` |
| `oauth2-proxy-loki` | confidential, auth-code | oauth2-proxy on :3101 | `nova-admin`, `nova-operator`, `nova-viewer` |

Each client has an OIDC audience mapper so the client ID appears in the
token `aud` claim (oauth2-proxy and Grafana both verify this).

Grafana role mapping uses a JMESPath expression against the
`realm_access.roles` claim (set in `grafana.ini`):

```
contains(realm_access.roles[*], 'nova-admin') && 'Admin'
  || contains(realm_access.roles[*], 'nova-operator') && 'Editor'
  || 'Viewer'
```

`allow_assign_grafana_admin = true` promotes `nova-admin` users to
Grafana Server Admin. The fallback for any authenticated user without
a matching realm role is `Viewer` (`[users] auto_assign_org_role`).

## Bootstrap order

The bootstrap is fully automated by `deploy/observability/setup.sh`
when you export `KC_URL` and `KC_ADMIN_PASS`. The order it follows
(and what to do manually if you bypass setup.sh):

1. **Realm exists.** The `novanas` realm and its three roles must be
   imported first — see `deploy/keycloak/realm-novanas.json` and the
   `nova-keycloak-bootstrap.sh` runner.
2. **Create clients.** Run the two helper scripts:
   ```bash
   export KC_URL=https://192.168.10.204:8443
   export KC_ADMIN_PASS=<admin-pw>

   ./deploy/keycloak/create-grafana-client.sh > /tmp/grafana.json
   ./deploy/keycloak/create-oauth2-proxy-clients.sh > /tmp/o2p.json
   ```
   Both are idempotent: they update existing clients in place and
   rotate the client secret on every run.
3. **Drop secrets in place.**
   ```bash
   # Grafana OIDC secret.
   jq -r .clientSecret /tmp/grafana.json \
     | sudo install -m 0400 -o grafana -g grafana \
         /dev/stdin /etc/grafana/oidc-secret

   # oauth2-proxy client + cookie secrets (one per service).
   for svc in prometheus alertmanager loki; do
     jq -r --arg s "$svc" \
       '.clients[]|select(.service==$s)|.clientSecret' /tmp/o2p.json \
       | sudo install -m 0400 -o oauth2-proxy -g oauth2-proxy \
           /dev/stdin "/etc/oauth2-proxy/${svc}-client-secret"

     jq -r --arg s "$svc" \
       '.clients[]|select(.service==$s)|.cookieSecret' /tmp/o2p.json \
       | sudo install -m 0400 -o oauth2-proxy -g oauth2-proxy \
           /dev/stdin "/etc/oauth2-proxy/${svc}-cookie-secret"
   done
   ```
4. **Install/refresh oauth2-proxy.** The systemd units expect the binary
   at `/usr/local/bin/oauth2-proxy` (pin to v7.6.x, e.g.
   `oauth2-proxy v7.6.0` from the upstream GitHub releases). Verify
   with `oauth2-proxy --version`.
5. **TLS certs.** `issue-certs.sh` already publishes
   `/etc/nova-certs/{prometheus,alertmanager,loki}.{crt,key}`. The
   oauth2-proxy units mount them at `/etc/oauth2-proxy/tls/`.
6. **Start oauth2-proxy units.** They have `Requires=` on the upstream
   services, so the upstream must be running first:
   ```bash
   sudo systemctl start prometheus alertmanager loki
   sudo systemctl start oauth2-proxy-prometheus
   sudo systemctl start oauth2-proxy-alertmanager
   sudo systemctl start oauth2-proxy-loki
   ```
7. **Restart Grafana** so it picks up `[auth.generic_oauth]`:
   ```bash
   sudo systemctl restart grafana.service
   ```

## URLs

After bootstrap:

| URL | Purpose |
|-----|---------|
| `https://novanas.local:3000` | Grafana (Sign in with Keycloak) |
| `https://novanas.local:9091` | Prometheus (oauth2-proxy) |
| `https://novanas.local:9094` | Alertmanager (oauth2-proxy) |
| `https://novanas.local:3101` | Loki (oauth2-proxy) |

The `oauth2/sign_in`, `oauth2/start`, `oauth2/callback`, `oauth2/sign_out`
endpoints are mounted by oauth2-proxy at each of the proxied URLs.

Internal endpoints not exposed via SSO (used by health probes and
sibling services on the box):

- `https://127.0.0.1:9090/-/healthy` (Prometheus)
- `https://127.0.0.1:9093/-/healthy` (Alertmanager)
- `http://127.0.0.1:3100/ready` (Loki)
- `http://127.0.0.1:3100/loki/api/v1/push` (Promtail ingestion)

## Break-glass admin

Grafana keeps a local `admin` user. Set or rotate its password via:

```bash
sudo /usr/sbin/grafana-cli admin reset-admin-password '<new-pw>'
```

The local login form remains available at `https://novanas.local:3000/login`
even when Keycloak is down.

## Troubleshooting

### Grafana "user does not have a valid login" / always Viewer

- Check that `realm_access.roles` is in the access token. From
  Keycloak admin: Clients -> grafana -> Client scopes -> `roles` must
  be in `defaultClientScopes`.
- The script sets this; if you reverted the client manually, re-run
  `create-grafana-client.sh`.
- Confirm `[auth.generic_oauth] role_attribute_path` is intact in
  `/etc/grafana/grafana.ini`. The expression is evaluated against the
  userinfo response — `realm_access` requires the `roles` mapper.

### Grafana TLS handshake error talking to Keycloak

- Keycloak's cert chains to `/etc/nova-ca/ca.crt`.
- `[auth.generic_oauth] tls_client_ca = /etc/nova-ca/ca.crt` must be
  set; do **not** flip `tls_skip_verify_insecure = true` in production.

### oauth2-proxy: `403 Forbidden` after a successful login

The user has authenticated but lacks one of the realm roles in
`allowed_groups`. Either:

- Add the realm role to the user in Keycloak, or
- Edit `/etc/oauth2-proxy/<service>.cfg` to widen `allowed_groups` and
  restart `oauth2-proxy-<service>.service`.

oauth2-proxy maps Keycloak realm roles into the `groups` claim via the
`keycloak-oidc` provider; verify with:

```bash
sudo journalctl -u oauth2-proxy-prometheus.service -n 50
```

### oauth2-proxy: `cookie secret must be 16, 24, or 32 bytes`

The cookie secret file is empty/garbled. Regenerate:

```bash
openssl rand -base64 32 \
  | sudo install -m 0400 -o oauth2-proxy -g oauth2-proxy \
      /dev/stdin /etc/oauth2-proxy/prometheus-cookie-secret
sudo systemctl restart oauth2-proxy-prometheus.service
```

(Or just re-run `create-oauth2-proxy-clients.sh` — it emits fresh
cookie secrets.)

### Browser warns "ERR_CERT_AUTHORITY_INVALID"

The oauth2-proxy listeners use the host certs issued by the local CA.
Trust `/etc/nova-ca/ca.crt` in your OS/browser, or use the cert pinned
NovaFlow web client.

### Promtail can no longer push logs

It shouldn't be affected — Promtail uses the loopback HTTP endpoint at
`http://127.0.0.1:3100/loki/api/v1/push`, which bypasses oauth2-proxy.
If Promtail logs show 401/403 errors, double-check it isn't pointed at
`https://127.0.0.1:3101` by mistake.

### Rotating secrets

All three creator scripts rotate secrets on each run. Procedure:

```bash
sudo -E ./deploy/keycloak/create-grafana-client.sh > /tmp/grafana.json
# install secret
sudo systemctl restart grafana.service

sudo -E ./deploy/keycloak/create-oauth2-proxy-clients.sh > /tmp/o2p.json
# install client + cookie secrets per service
sudo systemctl restart oauth2-proxy-prometheus oauth2-proxy-alertmanager oauth2-proxy-loki
```

Active sessions are invalidated when the cookie secret changes.

## Files of interest

- `deploy/keycloak/create-grafana-client.sh` — Grafana OIDC client.
- `deploy/keycloak/create-oauth2-proxy-clients.sh` — three oauth2-proxy clients.
- `deploy/oauth2-proxy/{prometheus,alertmanager,loki}.cfg` — proxy configs.
- `deploy/systemd/oauth2-proxy-{prometheus,alertmanager,loki}.service` — units.
- `deploy/grafana/grafana.ini` — `[auth.generic_oauth]` block.
- `deploy/observability/setup.sh` — step 14 wires it all together.
