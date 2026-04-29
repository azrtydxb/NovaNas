-- =====================================================================
-- Tier 2 plugin engine queries. Hand-written DAO at
-- internal/plugins/store.go is the canonical caller in v1; sqlc
-- regeneration of these queries lands when the next bulk regen runs.
-- =====================================================================

-- name: CreatePlugin :one
INSERT INTO plugins (id, name, version, manifest, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, name, version, manifest, status, installed_at, updated_at;

-- name: UpdatePlugin :one
UPDATE plugins
SET version = $2,
    manifest = $3,
    status = $4,
    updated_at = now()
WHERE name = $1
RETURNING id, name, version, manifest, status, installed_at, updated_at;

-- name: GetPluginByName :one
SELECT id, name, version, manifest, status, installed_at, updated_at
FROM plugins
WHERE name = $1;

-- name: ListPlugins :many
SELECT id, name, version, manifest, status, installed_at, updated_at
FROM plugins
ORDER BY name;

-- name: DeletePlugin :exec
DELETE FROM plugins WHERE name = $1;

-- name: AddPluginResource :exec
INSERT INTO plugin_resources (plugin_id, resource_type, resource_id)
VALUES ($1, $2, $3);

-- name: ListPluginResources :many
SELECT id, plugin_id, resource_type, resource_id, created_at
FROM plugin_resources
WHERE plugin_id = $1
ORDER BY id;

-- name: DeletePluginResources :exec
DELETE FROM plugin_resources WHERE plugin_id = $1;
