# Audit Log Read API

NovaNAS records every state-changing API request in an append-only
`audit_log` table. The Audit middleware (see
`internal/api/middleware/audit.go`) writes a row on every non-GET request
that traverses the API; the read API documented here lets operators and
compliance auditors query, summarise, and export those rows without
direct DB access.

## At a glance

| Endpoint                | Purpose                                    |
| ----------------------- | ------------------------------------------ |
| `GET /api/v1/audit`         | Cursor-paginated event listing             |
| `GET /api/v1/audit/summary` | Aggregate counts by (actor, action, outcome) |
| `GET /api/v1/audit/export`  | Streamed CSV/JSONL dump for compliance     |

All three require `nova:audit:read` (`PermAuditRead`). In the default
role map this is granted to `nova-admin` and `nova-operator`. Viewers
(`nova-viewer`) are intentionally NOT granted access — knowing who
looked at what is itself sensitive reconnaissance signal, and audit
visibility is the difference between a curious viewer and an
investigator.

## Event shape

```json
{
  "id": 12345,
  "timestamp": "2026-04-29T13:14:15.123456Z",
  "actor": "alice@watteel.com",
  "action": "POST /api/v1/pools",
  "resource": "/api/v1/pools",
  "request_id": "01J...",
  "payload": { "name": "tank", "vdevs": [...] },
  "outcome": "accepted"
}
```

Field notes:

- **actor** comes from the verified JWT (`preferredUsername` or
  `subject` fallback). If auth was disabled on the request, actor is
  empty.
- **action** is `METHOD /full/path` — useful for filter-by-verb.
- **resource** is the request path; treat it as the subject of the
  action. Filtering on resource is a server-side prefix match
  (`?resource=/api/v1/datasets/tank` matches everything under that
  dataset).
- **payload** is the redacted JSON request body. Secrets (`password`,
  `token`, etc.) are scrubbed by `middleware.RedactSecrets` before the
  row is written.
- **outcome** is `accepted` (HTTP < 400) or `rejected` (HTTP >= 400).

`source_ip` is not a column on the `audit_log` table. The
`?source_ip=` filter is applied post-query against
`payload->>'source_ip'` if present. Rows without a source_ip in the
payload are excluded by the filter, not by the absence of a column.

## Filtering

All filters are AND-combined and parameterised at the SQL layer (no
string concatenation):

```
?actor=alice
?action=POST%20/api/v1/pools
?resource=/api/v1/datasets/tank        # prefix match
?outcome=accepted|rejected
?since=2026-04-01T00:00:00Z            # RFC3339
?until=2026-04-30T00:00:00Z            # RFC3339, exclusive
?source_ip=10.0.0.0/8                  # bare IP or CIDR (v4 or v6)
?limit=100                             # default 100, max 1000
?cursor=<opaque-token>                 # from previous next_cursor
```

Malformed inputs return `400 bad_request` with the field name in the
message.

## Cursor pagination

The cursor is a base64url-encoded `<timestamp_unix_nano>:<id>` tuple of
the last row returned. The server then asks SQL for rows where
`(ts, id) < (cursor_ts, cursor_id)` ordered `ts DESC, id DESC`. This is
stable under concurrent inserts: a new event written after you started
paging always sorts BEFORE your cursor and thus never injects into a
later page.

```bash
curl -s -H "Authorization: Bearer $TOK" \
  "$API/api/v1/audit?actor=alice&limit=100" \
  | jq '{count: (.items | length), next: .next_cursor}'
```

Empty `next_cursor` means you've reached the end. Pass it back
verbatim:

```bash
curl -s -H "Authorization: Bearer $TOK" \
  "$API/api/v1/audit?actor=alice&limit=100&cursor=$NEXT"
```

## Streaming export

The export endpoint streams results using chunked transfer encoding;
the server never holds more than one DB page (~500 rows) in memory.
Use it for compliance dumps and forensic timelines.

```bash
# CSV
curl -sN -H "Authorization: Bearer $TOK" \
  "$API/api/v1/audit/export?format=csv&since=2026-01-01T00:00:00Z" \
  > audit-2026-q1.csv

# JSONL (one JSON event per line; pipes well into jq, ClickHouse, etc.)
curl -sN -H "Authorization: Bearer $TOK" \
  "$API/api/v1/audit/export?format=jsonl&since=2026-01-01T00:00:00Z" \
  | jq -c 'select(.outcome == "rejected")'
```

Behaviour:

- Only one export per user runs at a time; a second call returns `429
  rate_limited` until the first finishes. This protects the DB from
  multi-GB scans by a misbehaving compliance script.
- Aborting the client (Ctrl-C, `ctx.Cancel()`) terminates the server
  loop on the next iteration; no orphan queries.
- The act of exporting is itself audited. A row with
  `action="GET /api/v1/audit/export"` is inserted on completion, with a
  payload containing the filter set and the row count. This means the
  audit log is self-witnessing: nobody can quietly walk the table.

## Summary endpoint

`GET /api/v1/audit/summary?since=...&until=...` returns aggregate
counts grouped by `(actor, action, outcome)`. It powers
"who did what last week" overviews:

```bash
curl -s -H "Authorization: Bearer $TOK" \
  "$API/api/v1/audit/summary?since=$(date -u -d '-7 days' +%FT%TZ)" \
  | jq -r '.[] | "\(.count)\t\(.actor)\t\(.action)\t\(.outcome)"' \
  | sort -rn | head -20
```

## Retention story

The current schema has no built-in retention. Operators choose how long
to keep events; recommended postures:

- **HIPAA**: 6 years from creation or last effective date of the record.
  Run a nightly `DELETE FROM audit_log WHERE ts < now() - interval '6 years'`,
  *after* archiving the deleted rows to immutable storage (S3 Object
  Lock, write-once volume, etc.). Document the archival proof chain in
  your audit policy.
- **SOC 2**: 1 year online plus 6 years cold. A common implementation
  is the export-then-delete pattern: a weekly job calls
  `/audit/export?format=jsonl` to a WORM bucket, then trims rows older
  than 1 year from the live table.
- **PCI-DSS 4.0 (req. 10.5)**: 1 year minimum, with the most recent 3
  months immediately available. Keep online; archive nightly.

A future migration may add `retention_class` and a background trimmer.
Until then, retention is operator-driven and the export endpoint is the
canonical way to ship rows to long-term storage.

## Compliance examples

### HIPAA — produce an access trail for ePHI

Auditors typically want "every read or write to dataset X by every
user, in chronological order, between dates Y and Z."

```bash
TOK=$(./novanas-cli auth token)
curl -sN -H "Authorization: Bearer $TOK" \
  "$API/api/v1/audit/export?format=jsonl&resource=/api/v1/datasets/ehr&since=2026-01-01T00:00:00Z&until=2026-04-01T00:00:00Z" \
  | jq -s 'sort_by(.timestamp)' \
  > ehr-q1-access.json
```

The output is a single sorted timeline with actor, action, outcome,
request_id, and payload for every operation against the `ehr` dataset.

### SOC 2 CC7.2 — change tracking for production storage

"All production configuration changes are logged with the requester,
the time, and the change content."

```bash
curl -sN -H "Authorization: Bearer $TOK" \
  "$API/api/v1/audit/export?format=csv&action=POST%20/api/v1/pools" \
  > pool-create-events.csv
```

The CSV's `payload` column carries the redacted request body — i.e. the
exact PoolCreateSpec — which is the change content. Pair with a
deterministic JWT subject claim and you have the whole CC7.2
requirement in one row per change.

### PCI-DSS 10.2.1 — individual user accesses to cardholder data

```bash
# Anything an operator did against the PCI dataset family in the last
# 24 hours — useful for daily review (req. 10.4.1).
curl -s -H "Authorization: Bearer $TOK" \
  "$API/api/v1/audit?resource=/api/v1/datasets/pci&since=$(date -u -d '-1 day' +%FT%TZ)" \
  | jq '.items[] | {time: .timestamp, actor, action, outcome}'
```

### Per-user activity report

```bash
curl -s -H "Authorization: Bearer $TOK" \
  "$API/api/v1/audit/summary?since=$(date -u -d '-30 days' +%FT%TZ)" \
  | jq 'group_by(.actor) | map({actor: .[0].actor, total: (map(.count) | add)})'
```

### Detect rejected operations from a subnet

```bash
curl -s -H "Authorization: Bearer $TOK" \
  "$API/api/v1/audit?outcome=rejected&source_ip=10.42.0.0/16&limit=200" \
  | jq '.items[] | "\(.timestamp) \(.actor // "?") \(.action)"'
```

## SDK usage (Go)

The Go SDK at `clients/go/novanas` exposes the read API as three
methods plus a row-by-row iterator. Memory stays bounded because the
streaming export decodes one row at a time.

```go
import "github.com/novanas/nova-nas/clients/go/novanas"

c, _ := novanas.New(novanas.Config{BaseURL: api, Token: tok})

// One page.
page, err := c.ListAudit(ctx, novanas.AuditFilter{Actor: "alice"}, "", 100)

// Walk every page.
err = c.IterateAudit(ctx, novanas.AuditFilter{Outcome: "rejected"}, 500,
    func(ev novanas.AuditEvent) error {
        log.Printf("%s %s %s", ev.Timestamp, ev.Actor, ev.Action)
        return nil
    })

// Stream a CSV export to a long-term archive.
err = c.ExportAudit(ctx, novanas.AuditExportCSV,
    novanas.AuditFilter{Since: monthStart},
    func(ev novanas.AuditEvent) error {
        return archive.Append(ev)
    })
if errors.Is(err, novanas.ErrAuditExportBusy) {
    // back off — another export is in flight for this user.
}
```

## Operational runbook

- **Filter must be parameterised**: never embed user input into a
  hand-written SQL filter. The handler uses sqlc-generated queries
  (`internal/store/queries/audit_log.sql`) which take all filters as
  named, type-checked parameters.
- **Cursor stability**: don't synthesise a cursor by hand. The token
  encodes `(ts_unix_nano, id)` and the server validates both. Tampered
  cursors return `400 invalid cursor`.
- **Time zones**: timestamps are stored in UTC; clients should pass
  filters in RFC3339 with an explicit zone (`Z` or `+00:00`).
- **Self-audit cardinality**: every export adds one row. Don't loop the
  exporter inside a script — page through the list endpoint instead, or
  the export trail will dominate the live table.

## File map

- Read handler:    `internal/api/handlers/audit_read.go`
- Read handler tests: `internal/api/handlers/audit_read_test.go`
- SQL queries:     `internal/store/queries/audit_log.sql`
- OpenAPI spec:    `api/openapi.yaml` (paths under `/audit`)
- Permission:      `internal/auth/rbac.go` (`PermAuditRead`)
- Go SDK:          `clients/go/novanas/audit.go`
