-- =====================================================================
-- Unified Notification Center (internal/notifycenter)
-- =====================================================================

-- name: InsertNotification :one
-- Idempotent insert: if (source, source_id) already exists, the existing
-- row is returned untouched. The aggregator relies on this so its
-- polling loop never produces duplicates even across restarts.
INSERT INTO notifications (id, tenant_id, source, source_id, severity, title, body, link)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (source, source_id) DO UPDATE SET source = EXCLUDED.source
RETURNING *;

-- name: GetNotification :one
SELECT * FROM notifications WHERE id = $1;

-- name: ListNotifications :many
-- Filter columns are nullable; NULL means "don't filter". Cursor predicate
-- (created_at, id) yields a stable DESC ordering even under concurrent
-- inserts.
SELECT * FROM notifications
WHERE (sqlc.narg('severity')::text   IS NULL OR severity = sqlc.narg('severity'))
  AND (sqlc.narg('source')::text     IS NULL OR source   = sqlc.narg('source'))
  AND (sqlc.narg('cursor_ts')::timestamptz IS NULL
       OR created_at <  sqlc.narg('cursor_ts')
       OR (created_at = sqlc.narg('cursor_ts') AND id < sqlc.narg('cursor_id')::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('lim');

-- name: GetUserState :one
SELECT * FROM notification_state
WHERE notification_id = $1 AND user_subject = $2;

-- name: ListUserStatesForNotifications :many
-- Returns the per-user state rows for a batch of notifications. Used by
-- the list endpoint to enrich each event with the calling user's state
-- without an N+1 round-trip.
SELECT * FROM notification_state
WHERE user_subject = sqlc.arg('user_subject')
  AND notification_id = ANY(sqlc.arg('ids')::uuid[]);

-- name: UpsertUserStateRead :one
INSERT INTO notification_state (notification_id, user_subject, read_at)
VALUES ($1, $2, now())
ON CONFLICT (notification_id, user_subject) DO UPDATE
   SET read_at = COALESCE(notification_state.read_at, EXCLUDED.read_at)
RETURNING *;

-- name: UpsertUserStateDismiss :one
INSERT INTO notification_state (notification_id, user_subject, dismissed_at, read_at)
VALUES ($1, $2, now(), now())
ON CONFLICT (notification_id, user_subject) DO UPDATE
   SET dismissed_at = EXCLUDED.dismissed_at,
       read_at      = COALESCE(notification_state.read_at, EXCLUDED.read_at)
RETURNING *;

-- name: UpsertUserStateSnooze :one
INSERT INTO notification_state (notification_id, user_subject, snoozed_until)
VALUES ($1, $2, $3)
ON CONFLICT (notification_id, user_subject) DO UPDATE
   SET snoozed_until = EXCLUDED.snoozed_until
RETURNING *;

-- name: MarkAllReadForUser :exec
-- Bulk "mark read" — inserts state rows for every undismissed notification
-- the user hasn't already touched, and bumps existing rows that lack a
-- read_at. Dismissed rows are left alone (already counted as read).
INSERT INTO notification_state (notification_id, user_subject, read_at)
SELECT n.id, sqlc.arg('user_subject')::text, now()
  FROM notifications n
  LEFT JOIN notification_state s
         ON s.notification_id = n.id AND s.user_subject = sqlc.arg('user_subject')::text
 WHERE s.read_at IS NULL AND s.dismissed_at IS NULL
ON CONFLICT (notification_id, user_subject) DO UPDATE
   SET read_at = COALESCE(notification_state.read_at, now());

-- name: UnreadCountForUser :one
-- Counts notifications the user hasn't read or dismissed, and that aren't
-- currently snoozed. Used to populate the bell badge.
SELECT COUNT(*)::bigint AS count
FROM notifications n
LEFT JOIN notification_state s
       ON s.notification_id = n.id AND s.user_subject = sqlc.arg('user_subject')::text
WHERE (s.read_at IS NULL)
  AND (s.dismissed_at IS NULL)
  AND (s.snoozed_until IS NULL OR s.snoozed_until <= now());
