-- name: InsertJob :one
INSERT INTO jobs (id, kind, target, state, command, request_id)
VALUES ($1, $2, $3, 'queued', $4, $5)
RETURNING *;

-- name: GetJob :one
SELECT * FROM jobs WHERE id = $1;

-- name: ListJobs :many
SELECT * FROM jobs
WHERE (sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state'))
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: MarkJobRunning :exec
UPDATE jobs
   SET state = 'running', started_at = now()
 WHERE id = $1 AND state IN ('queued','interrupted');

-- name: MarkJobFinished :exec
-- Only writes if the row is still 'running'. A user CancelJob between
-- markRunning and finish flips state to 'cancelled' and the host op is
-- already underway — we honor the user's cancel intent in the row even
-- though the host effect can't be unwound.
UPDATE jobs
   SET state = $2,
       stdout = $3,
       stderr = $4,
       exit_code = $5,
       error = $6,
       finished_at = now()
 WHERE id = $1 AND state = 'running';

-- name: MarkRunningInterrupted :exec
UPDATE jobs
   SET state = 'interrupted',
       error = 'process restarted'
 WHERE state IN ('queued','running');

-- name: CancelJob :exec
UPDATE jobs
   SET state = 'cancelled', finished_at = now()
 WHERE id = $1 AND state IN ('queued','running');
