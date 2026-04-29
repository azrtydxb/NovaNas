# Notification Center — Event Stream

The Notification Center is the "bell" subsystem: a unified event log
that aggregates signals from Alertmanager (firing alerts), the jobs
subsystem (failed jobs), and the audit log (rejected outcomes), into
a single per-user-state-tracked stream.

It is **separate** from `/api/v1/notifications/smtp` (the outbound
SMTP relay). The two share a path prefix but are unrelated.

## Endpoints

All endpoints live under `/api/v1/notifications/events`.

| Method | Path | Permission | Description |
| ------ | ---- | ---------- | ----------- |
| GET    | `/notifications/events`               | `nova:notifications.events:read`  | List events visible to the caller. |
| GET    | `/notifications/events/unread-count`  | `nova:notifications.events:read`  | Bell badge count. |
| GET    | `/notifications/events/stream`        | `nova:notifications.events:read`  | SSE push stream. |
| POST   | `/notifications/events/{id}/read`     | `nova:notifications.events:write` | Mark an event read for the caller. |
| POST   | `/notifications/events/{id}/dismiss`  | `nova:notifications.events:write` | Dismiss for the caller. |
| POST   | `/notifications/events/{id}/snooze`   | `nova:notifications.events:write` | Snooze with `{"until": "<RFC3339>"}`. |
| POST   | `/notifications/events/read-all`      | `nova:notifications.events:write` | Bulk mark-read. |

## Event payload

```json
{
  "id": "11111111-2222-3333-4444-555555555555",
  "source": "jobs",
  "sourceId": "9b8e...-...",
  "severity": "warning",
  "title": "Job failed: pool.scrub",
  "body": "scrub of pool 'tank' returned 1 error",
  "link": "/jobs/9b8e...",
  "createdAt": "2026-04-29T10:21:34.123456Z",
  "userState": {
    "read": false,
    "dismissed": false,
    "snoozed": false,
    "snoozedUntil": null
  }
}
```

`source` is one of `alertmanager | jobs | audit | system`. `severity`
is `info | warning | critical`. `userState` is computed for the
calling user at read time; it is independent for every operator.

## Server-Sent Events stream

The bell icon in the GUI subscribes to the stream **once on mount** and
receives push updates for the duration of the page lifetime.

### On-the-wire format

```
GET /api/v1/notifications/events/stream HTTP/1.1
Authorization: Bearer <token>
Accept: text/event-stream
```

The server replies with `Content-Type: text/event-stream` and emits:

1. **An immediate `: connected` heartbeat** on subscribe so the client
   can confirm the connection without waiting on the 15s keepalive
   tick.

   ```
   : connected

   ```

2. **A `: keepalive` comment frame every 15s** while the stream is
   idle. SSE comments (lines starting with `:`) are required by the
   spec to be ignored by clients; they exist to keep proxies (nginx,
   AWS ALB) from idle-closing the connection.

   ```
   : keepalive

   ```

3. **A `notification` event** for every newly recorded notification:

   ```
   event: notification
   data: {"id":"...","source":"alertmanager","severity":"critical", ...}

   ```

   The `data` line is a single JSON object matching the schema above.

### Subscriptions are global, state is per-user

When the aggregator (or any other writer) calls `RecordEvent`, the
event is fanned out to **every** currently-connected SSE subscriber.
The payload's `userState` block is a snapshot for the receiving user
at delivery time. Clients should still call `GET
/notifications/events` on initial render to bootstrap state for events
that landed before the subscription opened.

### Reconnection

Standard `EventSource` semantics apply. If the connection drops the
client should reconnect (browsers do this automatically with a small
backoff). On reconnect the stream resumes pushing **new** events; any
events that fired during the disconnect window are visible via `GET
/notifications/events` and the unread-count endpoint.

## Filtering & cursor pagination

`GET /notifications/events` supports the following query params:

| Param | Default | Notes |
| ----- | ------- | ----- |
| `severity`         | _none_ | `info` \| `warning` \| `critical` |
| `source`           | _none_ | `alertmanager` \| `jobs` \| `audit` \| `system` |
| `unread`           | `false` | Hide events already read by the caller. |
| `includeDismissed` | `false` | By default dismissed events are hidden. |
| `includeSnoozed`   | `false` | By default actively-snoozed events are hidden. |
| `onlySnoozed`      | `false` | Surface ONLY currently-snoozed events. |
| `limit`            | `50`    | 1–500. |
| `cursor`           | _none_  | Opaque; pass `nextCursor` from the previous page. |

The response envelope is:

```json
{
  "items": [ /* NotificationEvent[] */ ],
  "nextCursor": "2026-04-29T10:21:34.123Z|<uuid>"
}
```

`nextCursor` is omitted when the page is short (last page).

## Severity heuristics

| Source        | Mapping |
| ------------- | ------- |
| Alertmanager  | `severity` label maps directly. Unknown labels collapse to `info`. |
| Jobs (failed) | Always `warning`. |
| Audit (rejected) | `info`, **except** auth-related actions which become `warning`. |

## Sources of truth

- The aggregator (`internal/notifycenter/aggregator.go`) polls the
  three sources every 30s and calls `Manager.RecordEvent`.
  `RecordEvent` is **idempotent** on `(source, sourceId)` — re-observing
  the same alert/job/audit row never duplicates the bell entry. As a
  consequence the per-source cursor is in-memory and resets to "now"
  on restart; that's safe because of the idempotency.
- Clients that drive their own signals (e.g. the Web GUI surfacing a
  client-side validation error) must NOT call `RecordEvent` directly
  — that's a server-only API. Instead, post to whichever upstream
  surface is appropriate (audit log, etc.) and let the aggregator
  pick it up.

## Per-user state

State is stored in `notification_state(notification_id, user_subject,
read_at, dismissed_at, snoozed_until)` keyed on the Keycloak `sub`
claim. Two operators viewing the same critical alert have independent
read/dismissed/snoozed state. Dismissing an event hides it from that
operator's bell forever; the underlying notification row is preserved
so the audit trail and other operators are unaffected.
