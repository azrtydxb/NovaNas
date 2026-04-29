# Sessions / login history API

Pass-through to the Keycloak admin REST API. nova-api authenticates each call with
its own `client_credentials` token (cached in-process and refreshed on demand);
the caller's bearer token is never sent to Keycloak.

## Configuration

Set the following env on nova-api:

| Variable                              | Default                                         | Notes |
|---------------------------------------|-------------------------------------------------|-------|
| `KEYCLOAK_ADMIN_URL`                  | derived from `OIDC_ISSUER_URL`                  | Realm-scoped admin URL, e.g. `https://kc.example.com/admin/realms/novanas` |
| `KEYCLOAK_ADMIN_CLIENT_ID`            | (unset)                                          | client_credentials client; recommended: reuse `nova-krb5-sync` (already has `view-users` + `view-events` from `create-krb5-sync-client.sh`) |
| `KEYCLOAK_ADMIN_CLIENT_SECRET_FILE`   | (unset)                                          | File with the client secret on disk (avoid leaking via /proc/<pid>/environ) |

When client_id or secret file is missing, all sessions/login-history routes
return `503 keycloak_admin_unconfigured`.

### Choice: reuse krb5sync client

We reuse the krb5sync client because it already carries `view-users` and
`view-events` realm-management roles. Adding `manage-users` (needed for
session revocation) keeps the client surface narrow rather than provisioning
a second client. If your deployment treats those concerns as separate trust
domains, provision a dedicated `nova-api-admin` client and grant it
`view-users`, `view-events`, `manage-users` only.

## Endpoints

| Method | Path                                          | RBAC                  |
|--------|-----------------------------------------------|-----------------------|
| GET    | `/api/v1/auth/sessions`                       | `nova:sessions:read`  |
| DELETE | `/api/v1/auth/sessions/{id}`                  | `nova:sessions:read`  |
| GET    | `/api/v1/auth/users/{id}/sessions`            | `nova:sessions:admin` |
| DELETE | `/api/v1/auth/users/{id}/sessions`            | `nova:sessions:admin` |
| GET    | `/api/v1/auth/login-history`                  | `nova:sessions:read`  |
| GET    | `/api/v1/auth/users/{id}/login-history`       | `nova:sessions:admin` |

The "own sessions" routes resolve the caller's `sub` from the verified JWT;
revoke ownership is enforced by listing the caller's sessions first and
rejecting with `404 not_found` when the supplied id doesn't belong to them.

Sessions surfaced by Keycloak come in two flavours: regular **user sessions**
(login sessions) and **offline sessions** (refresh-only). The wire shape exposes
a `type` field so the UI can render both lists.

## curl examples

```bash
# List the caller's active sessions
curl -sH "Authorization: Bearer $TOKEN" https://nas.example.com/api/v1/auth/sessions

# Revoke one of the caller's sessions
curl -sX DELETE -H "Authorization: Bearer $TOKEN" \
  https://nas.example.com/api/v1/auth/sessions/3a4d0b...

# Caller's recent LOGIN events (last 50 by default)
curl -sG -H "Authorization: Bearer $TOKEN" --data-urlencode 'max=50' \
  https://nas.example.com/api/v1/auth/login-history

# Admin: list another user's sessions
curl -sH "Authorization: Bearer $TOKEN" \
  https://nas.example.com/api/v1/auth/users/$USER_ID/sessions

# Admin: log a user out everywhere
curl -sX DELETE -H "Authorization: Bearer $TOKEN" \
  https://nas.example.com/api/v1/auth/users/$USER_ID/sessions
```
