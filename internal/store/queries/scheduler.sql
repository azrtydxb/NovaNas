-- =====================================================================
-- Snapshot schedules
-- =====================================================================

-- name: CreateSnapshotSchedule :one
INSERT INTO snapshot_schedules (
    dataset, name, cron, recursive, snapshot_prefix,
    retention_hourly, retention_daily, retention_weekly,
    retention_monthly, retention_yearly, enabled
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: UpdateSnapshotSchedule :one
UPDATE snapshot_schedules
   SET cron              = $2,
       recursive         = $3,
       snapshot_prefix   = $4,
       retention_hourly  = $5,
       retention_daily   = $6,
       retention_weekly  = $7,
       retention_monthly = $8,
       retention_yearly  = $9,
       enabled           = $10,
       updated_at        = now()
 WHERE id = $1
RETURNING *;

-- name: DeleteSnapshotSchedule :exec
DELETE FROM snapshot_schedules WHERE id = $1;

-- name: GetSnapshotSchedule :one
SELECT * FROM snapshot_schedules WHERE id = $1;

-- name: ListSnapshotSchedules :many
SELECT * FROM snapshot_schedules ORDER BY dataset, name;

-- name: ListEnabledSnapshotSchedules :many
SELECT * FROM snapshot_schedules WHERE enabled = true;

-- name: MarkSnapshotScheduleFired :exec
UPDATE snapshot_schedules
   SET last_fired_at = $2,
       updated_at    = now()
 WHERE id = $1;

-- =====================================================================
-- Replication targets
-- =====================================================================

-- name: CreateReplicationTarget :one
INSERT INTO replication_targets (
    name, host, port, ssh_user, ssh_key_path, dataset_prefix
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateReplicationTarget :one
UPDATE replication_targets
   SET host           = $2,
       port           = $3,
       ssh_user       = $4,
       ssh_key_path   = $5,
       dataset_prefix = $6
 WHERE id = $1
RETURNING *;

-- name: DeleteReplicationTarget :exec
DELETE FROM replication_targets WHERE id = $1;

-- name: GetReplicationTarget :one
SELECT * FROM replication_targets WHERE id = $1;

-- name: ListReplicationTargets :many
SELECT * FROM replication_targets ORDER BY name;

-- =====================================================================
-- Replication schedules
-- =====================================================================

-- name: CreateReplicationSchedule :one
INSERT INTO replication_schedules (
    src_dataset, target_id, cron, snapshot_prefix, retention_remote, enabled
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateReplicationSchedule :one
UPDATE replication_schedules
   SET cron             = $2,
       snapshot_prefix  = $3,
       retention_remote = $4,
       enabled          = $5,
       updated_at       = now()
 WHERE id = $1
RETURNING *;

-- name: DeleteReplicationSchedule :exec
DELETE FROM replication_schedules WHERE id = $1;

-- name: GetReplicationSchedule :one
SELECT * FROM replication_schedules WHERE id = $1;

-- name: ListReplicationSchedules :many
SELECT * FROM replication_schedules ORDER BY src_dataset;

-- name: ListEnabledReplicationSchedules :many
SELECT * FROM replication_schedules WHERE enabled = true;

-- name: MarkReplicationScheduleFired :exec
UPDATE replication_schedules
   SET last_fired_at = $2,
       updated_at    = now()
 WHERE id = $1;

-- name: MarkReplicationScheduleResult :exec
UPDATE replication_schedules
   SET last_sync_snapshot = $2,
       last_error         = $3,
       updated_at         = now()
 WHERE id = $1;
