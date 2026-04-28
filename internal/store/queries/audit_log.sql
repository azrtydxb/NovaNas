-- name: InsertAudit :exec
INSERT INTO audit_log (actor, action, target, request_id, payload, result)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListAudit :many
SELECT * FROM audit_log
ORDER BY ts DESC
LIMIT $1 OFFSET $2;
