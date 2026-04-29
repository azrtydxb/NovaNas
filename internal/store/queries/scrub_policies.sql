-- =====================================================================
-- Scrub policies
-- =====================================================================

-- name: CreateScrubPolicy :one
INSERT INTO scrub_policies (
    name, pools, cron, priority, enabled, builtin
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateScrubPolicy :one
UPDATE scrub_policies
   SET pools      = $2,
       cron       = $3,
       priority   = $4,
       enabled    = $5,
       updated_at = now()
 WHERE id = $1
RETURNING *;

-- name: DeleteScrubPolicy :exec
DELETE FROM scrub_policies WHERE id = $1;

-- name: GetScrubPolicy :one
SELECT * FROM scrub_policies WHERE id = $1;

-- name: GetScrubPolicyByName :one
SELECT * FROM scrub_policies WHERE name = $1;

-- name: ListScrubPolicies :many
SELECT * FROM scrub_policies ORDER BY name;

-- name: ListEnabledScrubPolicies :many
SELECT * FROM scrub_policies WHERE enabled = true;

-- name: MarkScrubPolicyFired :exec
UPDATE scrub_policies
   SET last_fired_at = $2,
       last_error    = $3,
       updated_at    = now()
 WHERE id = $1;
