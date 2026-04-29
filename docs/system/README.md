# System API

The system surface mixes existing nova-api routes with two new pass-through
helpers (`/system/version`, `/system/updates`). The existing routes
(`/system/info`, `/system/time`, `/system/hostname`, `/system/timezone`,
`/system/ntp`, `/system/reboot`, `/system/shutdown`, `/system/cancel-shutdown`)
are unchanged.

## New endpoints

| Method | Path                       | RBAC                |
|--------|----------------------------|---------------------|
| GET    | `/api/v1/system/version`   | `nova:system:read`  |
| GET    | `/api/v1/system/updates`   | `nova:system:read`  |

### `/system/version`

Build metadata for the running nova-api binary. Populated from
`runtime/debug.ReadBuildInfo()` plus optional `-ldflags` stamps:

```
go build -ldflags "-X main.buildCommit=$(git rev-parse HEAD) -X main.buildTime=$(date -u +%FT%TZ)"
```

Cached in-memory after the first call.

### `/system/updates`

A/B image-update state. **v1 stub** — always returns:

```json
{ "available": false, "reason": "image-update-channel not configured", "status": "idle" }
```

The OS image-update layer isn't built yet. The stub lets the UI render
"no updates available" without falling over.

Production shape (the UI may rely on this contract; fields are optional but
the keys are stable):

```json
{
  "currentVersion":   "1.2.3",
  "availableVersion": "1.2.4",
  "channel":          "stable",
  "lastChecked":      "2026-04-29T08:30:00Z",
  "status":           "idle"
}
```

Status values: `idle`, `checking`, `downloading`, `installed-pending-reboot`.

## curl examples

```bash
# Build version
curl -sH "Authorization: Bearer $TOKEN" https://nas.example.com/api/v1/system/version | jq

# Update state (v1: stubbed)
curl -sH "Authorization: Bearer $TOKEN" https://nas.example.com/api/v1/system/updates | jq

# System info (existing route)
curl -sH "Authorization: Bearer $TOKEN" https://nas.example.com/api/v1/system/info | jq

# Reboot (admin-only)
curl -sX POST -H "Authorization: Bearer $TOKEN" \
  https://nas.example.com/api/v1/system/reboot

# Shutdown (admin-only)
curl -sX POST -H "Authorization: Bearer $TOKEN" \
  https://nas.example.com/api/v1/system/shutdown
```

Reboot/shutdown go through the job dispatcher (existing behaviour) and produce
audit-log entries via the global audit middleware.
