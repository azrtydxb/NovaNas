-- =====================================================================
-- Replication jobs + runs (internal/replication subsystem)
-- =====================================================================

-- name: CreateReplicationJob :one
INSERT INTO replication_jobs (
    id, name, backend, direction, source_json, destination_json,
    schedule, retention_json, enabled, secret_ref, last_snapshot
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: UpdateReplicationJob :one
UPDATE replication_jobs
   SET name             = $2,
       backend          = $3,
       direction        = $4,
       source_json      = $5,
       destination_json = $6,
       schedule         = $7,
       retention_json   = $8,
       enabled          = $9,
       secret_ref       = $10,
       last_snapshot    = $11,
       updated_at       = now()
 WHERE id = $1
RETURNING *;

-- name: DeleteReplicationJob :exec
DELETE FROM replication_jobs WHERE id = $1;

-- name: GetReplicationJob :one
SELECT * FROM replication_jobs WHERE id = $1;

-- name: ListReplicationJobs :many
SELECT * FROM replication_jobs ORDER BY name;

-- name: ListEnabledReplicationJobs :many
SELECT * FROM replication_jobs WHERE enabled = true;

-- name: MarkReplicationJobFired :exec
UPDATE replication_jobs
   SET last_fired_at = $2,
       updated_at    = now()
 WHERE id = $1;

-- name: InsertReplicationRun :one
INSERT INTO replication_runs (
    id, job_id, started_at, outcome, bytes_transferred, snapshot, error
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateReplicationRun :one
UPDATE replication_runs
   SET finished_at       = $2,
       outcome           = $3,
       bytes_transferred = $4,
       snapshot          = $5,
       error             = $6
 WHERE id = $1
RETURNING *;

-- name: ListReplicationRuns :many
SELECT * FROM replication_runs
 WHERE job_id = $1
 ORDER BY started_at DESC, id DESC
 LIMIT $2;

-- name: ListReplicationRunsAfter :many
SELECT * FROM replication_runs
 WHERE job_id = $1
   AND (started_at, id) < ($2, $3)
 ORDER BY started_at DESC, id DESC
 LIMIT $4;
