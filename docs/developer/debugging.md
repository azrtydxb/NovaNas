# Debugging

How to investigate a misbehaving NovaNas appliance or dev environment.

## Logs

All NovaNas services emit **structured JSON** — pino in TypeScript, zap in
Go, `tracing` with a JSON subscriber in Rust. Common fields per
[`docs/12-observability.md`](../12-observability.md): `component`,
`request_id`, `user`, `resource_kind`, `resource_name`, `severity`.

Useful `jq` one-liners when tailing a file or `kubectl logs` output:

```sh
# Errors only, compact
jq -c 'select(.severity=="error")' < log.jsonl

# Follow one request across services
jq -c 'select(.request_id=="req_abc123")' < log.jsonl

# Top 10 error messages by frequency
jq -r 'select(.severity=="error") | .msg' < log.jsonl | sort | uniq -c | sort -rn | head
```

In a running appliance, logs are shipped to Loki via Alloy; query them in
Grafana with LogQL.

## CRD inspection

Use the NovaNas API (or `novanasctl`) in normal flows rather than raw
`kubectl`. This matches how operators, UI, and audit logging actually see
the system, per
[decisions U4/U7/U15 and T5](../14-decision-log.md).

```sh
novanasctl get pools
novanasctl describe dataset media
novanasctl logs operator dataset
```

`kubectl` is an **escape hatch** — fine for deep debugging, but anything
you do with it bypasses the API's fine-grained authorization and audit.
Use it read-only when possible; avoid mutating CRDs from `kubectl` on a
live appliance.

## Tracing

OpenTelemetry spans flow to Tempo. Default sample rate is **1%**; flip
the **debug window** toggle to force full sampling for a bounded time
period when investigating a specific issue. Trace IDs are included in
log entries so you can jump from a log line to the corresponding trace
in Grafana.

Cross-layer traces (UI → API → operator → chunk engine) are the main
payoff here — slow operations almost always involve more than one layer.

## Common issues

- **Operator reconcile loop not converging** — check controller logs for
  repeating errors; look at `.status.conditions` on the CR; verify RBAC
  for the controller's service account.
- **API returns 401 unexpectedly** — token expiry; check Keycloak client
  clock skew and session lifetimes.
- **Storage write stalls** — inspect chunk engine metrics in Grafana
  (write latency, quorum wait, placement queue); check disk state machine
  in the Disks view.
- **UI live updates stop** — WebSocket channel dropped; browser devtools
  Network tab will show the close code. The UI retries with backoff; if
  it doesn't recover, check the API's Redis connection.
- **Dev build is stale** — `pnpm --filter ... build` only rebuilds the
  named package; run a top-level `pnpm -r build` if generated types are
  out of date.

## Tool choice

| Use | Tool |
|---|---|
| Daily admin / normal debugging | `novanasctl`, UI |
| Metrics dashboards, log search, trace explore | Grafana |
| Cluster-level poking when all else fails | `kubectl` (read-only first) |
| Chunk engine / dataplane internals | engine's admin gRPC + metrics |

The order matters: reach for `kubectl` last, not first.
