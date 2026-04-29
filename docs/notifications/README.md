# NovaNAS Outbound Notifications (SMTP)

NovaNAS speaks plain SMTP to a relay you bring. There are two consumers:

1. **nova-api** — transactional email (password reset, invite, weekly
   summary) and on-demand test sends from the API.
2. **Alertmanager** — pages from Prometheus alert rules.

Both use the same relay. Configuring it once is enough.

---

## 1. Pick a relay

| Provider | Host | Port | TLS mode |
|---|---|---|---|
| SendGrid | `smtp.sendgrid.net` | 587 | `starttls` |
| Mailgun  | `smtp.mailgun.org`  | 587 | `starttls` |
| Postmark | `smtp.postmarkapp.com` | 587 | `starttls` |
| Gmail (app password) | `smtp.gmail.com` | 587 | `starttls` |
| AWS SES  | `email-smtp.<region>.amazonaws.com` | 587 | `starttls` |
| On-prem Postfix | `relay.example.lan` | 25  | `none` (loopback/VLAN only) |

Implicit-TLS port 465 is also supported via `tlsMode: tls`.

### Gmail app-password example

1. In a Google account with 2FA enabled, create an *App password*:
   <https://myaccount.google.com/apppasswords>.
2. Username is the full email address (`alerts@your-domain.com`).
3. Password is the 16-character app password (no spaces).

---

## 2. Configure nova-api

Two paths: env vars at boot, or runtime PUT.

### 2a. Env vars (preferred — survives a restart)

```ini
# /etc/nova-nas/nova-api.env
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USERNAME=apikey
SMTP_PASSWORD_FILE=/etc/nova-nas/smtp_password
SMTP_FROM=alerts@example.com
SMTP_TLS_MODE=starttls
SMTP_MAX_PER_MINUTE=30
```

The password file should be `chmod 0400 root:nova` and contain only
the password (a single trailing newline is tolerated).

### 2b. Runtime PUT

```bash
curl -X PUT https://nova/api/v1/notifications/smtp \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "host": "smtp.sendgrid.net",
    "port": 587,
    "username": "apikey",
    "password": "SG.xxx…",
    "fromAddress": "alerts@example.com",
    "tlsMode": "starttls",
    "maxPerMinute": 30
  }'
```

The password is stored only in process memory (it is **not** written
back to disk). To preserve the existing password while editing other
fields, send `"password": "***"`.

### Test it

```bash
curl -X POST https://nova/api/v1/notifications/smtp/test \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"to": "you@example.com"}'
```

A 200 means the relay accepted the message. A 502 with code
`smtp_error` surfaces the relay's error verbatim.

---

## 3. Configure Alertmanager

`deploy/alertmanager/alertmanager.yml` already references
`${SMTP_HOST}` / `${SMTP_USERNAME}` / `${SMTP_PASSWORD}` etc. and ships
an `email-ops` receiver. The systemd unit
(`deploy/systemd/alertmanager.service`) loads the credentials via
systemd `LoadCredential=`:

```bash
sudo install -m 0400 -o alertmanager -g alertmanager /dev/stdin \
  /etc/alertmanager/smtp.env <<'EOF'
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USERNAME=apikey
SMTP_PASSWORD=SG.xxx…
SMTP_FROM=alerts@example.com
EOF

sudo systemctl restart alertmanager
```

Route an alert to the new receiver by editing the `route:` block:

```yaml
route:
  receiver: 'null'
  routes:
    - match_re:
        alertname: 'NovaNAS.*'
      receiver: 'email-ops'   # was: 'null'
```

Reload:

```bash
sudo systemctl reload alertmanager
# or, without systemd:
curl -X POST http://127.0.0.1:9093/-/reload
```

---

## 4. Troubleshooting

### `connection refused`

* The relay is not listening on the port you configured. Confirm with
  `nc -zv smtp.example.com 587`.
* Egress firewall blocks 587/465. Some ISPs/VPCs block 25 outbound;
  switch to 587 (submission) or 465 (SMTPS).
* Wrong `tlsMode`: implicit-TLS port 465 with `starttls` will hang or
  reset the connection. Use `tlsMode: tls` for 465.

### `authentication failed`

* Username case-sensitivity: SendGrid wants the literal string
  `apikey` (not your account email) and the API key as the password.
* Gmail wants an *App password*, not your account password. 2FA must
  be enabled to create one.
* Postmark wants the *server token*, not the account password.

### `tls: failed to verify certificate`

* Corporate CA on a self-hosted relay: install the CA bundle into the
  system trust store (`/usr/local/share/ca-certificates/`,
  `update-ca-certificates`).
* Hostname mismatch: the cert's CN/SAN must match `host:`. Don't
  point at `relay.internal` if the cert says `mail.corp.example.com`.

### `503 Bad sequence of commands` on Gmail

* You're trying to AUTH on a plaintext channel. Set `tlsMode:
  starttls`; Gmail rejects PLAIN over un-upgraded connections.

### Alertmanager: messages queue but never deliver

* Check `journalctl -u alertmanager` for SMTP errors.
* Confirm `${SMTP_*}` expanded at start: `systemctl show alertmanager
  -p Environment` should list the SMTP_* vars.
* If `LoadCredential=` fails, the EnvironmentFile path
  `/run/credentials/alertmanager.service/smtp.env` won't exist — check
  permissions on `/etc/alertmanager/smtp.env` (must be readable by
  root; systemd copies it as root before dropping privileges).

---

## 5. Operational notes

* **Rate limit.** A per-recipient leaky bucket (default 30/min) keeps
  a flooding alert from drowning a recipient. Raise via
  `SMTP_MAX_PER_MINUTE` or PUT.
* **Outbox.** Failures from nova-api's transactional sends are kept in
  an in-process outbox (capped at 1000 entries, FIFO eviction) for
  later retry. The outbox is **not** persisted across restarts; for
  durability rely on the relay's queue.
* **Password handling.** The password is held in process memory and
  never written to disk by nova-api. The redacted GET response always
  echoes `"***"`.
