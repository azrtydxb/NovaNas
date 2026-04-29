# Logs API

Pass-through to a loopback-bound Loki (`http://127.0.0.1:3100` by default,
overridable via `LOKI_URL`). All endpoints are read-only and require
`nova:logs:read` (operator+). Range queries stream chunked so very long ranges
don't buffer in memory; cancellation propagates via the request context.

## Endpoints

| Method | Path                                       | RBAC               |
|--------|--------------------------------------------|--------------------|
| GET    | `/api/v1/logs/query`                       | `nova:logs:read`   |
| GET    | `/api/v1/logs/query/instant`               | `nova:logs:read`   |
| GET    | `/api/v1/logs/labels`                      | `nova:logs:read`   |
| GET    | `/api/v1/logs/labels/{name}/values`        | `nova:logs:read`   |
| GET    | `/api/v1/logs/series`                      | `nova:logs:read`   |
| GET    | `/api/v1/logs/tail`                        | `nova:logs:read`   |

`/logs/tail` is documented but returns 501 in v1 — Loki tails over WebSocket and
nova-api does not currently terminate WS clients itself. UIs that need live tail
should target Loki via a network-level reverse proxy.

## curl examples

LogQL range query for nova-api errors over the last hour:

```bash
START=$(date -u -d '1 hour ago' +%s)000000000
END=$(date -u +%s)000000000
curl -sG -H "Authorization: Bearer $TOKEN" \
  --data-urlencode 'query={app="nova-api"} |= "level=error"' \
  --data-urlencode "start=$START" --data-urlencode "end=$END" \
  --data-urlencode 'limit=500' \
  https://nas.example.com/api/v1/logs/query | jq
```

Instant query:

```bash
curl -sG -H "Authorization: Bearer $TOKEN" \
  --data-urlencode 'query=count_over_time({app="nova-api"} |= "panic" [5m])' \
  https://nas.example.com/api/v1/logs/query/instant | jq
```

List label keys / values:

```bash
curl -sH "Authorization: Bearer $TOKEN" https://nas.example.com/api/v1/logs/labels
curl -sH "Authorization: Bearer $TOKEN" https://nas.example.com/api/v1/logs/labels/app/values
```

List streams matching one or more matchers:

```bash
curl -sG -H "Authorization: Bearer $TOKEN" \
  --data-urlencode 'match[]={app="nova-api"}' \
  https://nas.example.com/api/v1/logs/series
```
