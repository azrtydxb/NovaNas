-- name: InsertAudit :exec
INSERT INTO audit_log (actor, action, target, request_id, payload, result)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListAudit :many
SELECT * FROM audit_log
ORDER BY ts DESC
LIMIT $1 OFFSET $2;

-- name: SearchAudit :many
-- Filter columns are nullable; NULL means "don't filter on this column".
-- The (ts, id) cursor predicate yields a stable DESC ordering even under
-- concurrent inserts — new rows always sort before any returned cursor.
-- target prefix match uses LIKE with an explicit anchor ('foo%').
SELECT id, ts, actor, action, target, request_id, payload, result
FROM audit_log
WHERE (sqlc.narg('actor')::text   IS NULL OR actor  = sqlc.narg('actor'))
  AND (sqlc.narg('action')::text  IS NULL OR action = sqlc.narg('action'))
  AND (sqlc.narg('result')::text  IS NULL OR result = sqlc.narg('result'))
  AND (sqlc.narg('target_prefix')::text IS NULL OR target LIKE sqlc.narg('target_prefix') || '%')
  AND (sqlc.narg('since')::timestamptz IS NULL OR ts >= sqlc.narg('since'))
  AND (sqlc.narg('until')::timestamptz IS NULL OR ts <  sqlc.narg('until'))
  AND (sqlc.narg('cursor_ts')::timestamptz IS NULL
       OR ts <  sqlc.narg('cursor_ts')
       OR (ts = sqlc.narg('cursor_ts') AND id < sqlc.narg('cursor_id')::bigint))
ORDER BY ts DESC, id DESC
LIMIT $1;

-- name: SummaryAudit :many
-- Aggregate counts grouped by (actor, action, result) within an optional
-- time window. Used by the /audit/summary endpoint.
SELECT
  COALESCE(actor, '') AS actor,
  action,
  result,
  COUNT(*)::bigint    AS count
FROM audit_log
WHERE (sqlc.narg('since')::timestamptz IS NULL OR ts >= sqlc.narg('since'))
  AND (sqlc.narg('until')::timestamptz IS NULL OR ts <  sqlc.narg('until'))
GROUP BY COALESCE(actor, ''), action, result
ORDER BY count DESC, actor, action, result;
