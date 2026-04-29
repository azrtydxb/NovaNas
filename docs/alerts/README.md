# Alerts API

Pass-through to a loopback-bound Alertmanager (`http://127.0.0.1:9093` by default,
overridable via `ALERTMANAGER_URL`). nova-api gates each route with the operator's
RBAC (`nova:alerts:read` / `nova:alerts:write`) before forwarding the request — the
caller's bearer token is not propagated to AM.

## Endpoints

| Method | Path                                | RBAC                |
|--------|-------------------------------------|---------------------|
| GET    | `/api/v1/alerts`                    | `nova:alerts:read`  |
| GET    | `/api/v1/alerts/{fingerprint}`      | `nova:alerts:read`  |
| GET    | `/api/v1/alert-silences`            | `nova:alerts:read`  |
| POST   | `/api/v1/alert-silences`            | `nova:alerts:write` |
| DELETE | `/api/v1/alert-silences/{id}`       | `nova:alerts:write` |
| GET    | `/api/v1/alert-receivers`           | `nova:alerts:read`  |

## curl examples

List active alerts:

```bash
curl -sH "Authorization: Bearer $TOKEN" https://nas.example.com/api/v1/alerts | jq
```

Get a specific alert by fingerprint:

```bash
curl -sH "Authorization: Bearer $TOKEN" \
  https://nas.example.com/api/v1/alerts/4a8b9c2d3e4f5a6b
```

Create a 2-hour silence:

```bash
curl -sX POST -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  https://nas.example.com/api/v1/alert-silences \
  -d '{
    "matchers":[{"name":"alertname","value":"PoolDegraded","isRegex":false}],
    "startsAt":"2026-04-29T12:00:00Z",
    "endsAt":  "2026-04-29T14:00:00Z",
    "createdBy":"alice",
    "comment":"Maintenance window"
  }'
```

Expire a silence:

```bash
curl -sX DELETE -H "Authorization: Bearer $TOKEN" \
  https://nas.example.com/api/v1/alert-silences/03a8d2e2-...
```

List configured receivers (read-only — receiver editing happens out-of-band by
editing AM's config file and reloading):

```bash
curl -sH "Authorization: Bearer $TOKEN" https://nas.example.com/api/v1/alert-receivers
```
