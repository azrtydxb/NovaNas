-- name: GetResourceMetadata :one
SELECT id, kind, zfs_name, display_name, description, tags
FROM resource_metadata
WHERE kind = $1 AND zfs_name = $2;

-- name: ListResourceMetadataByKind :many
SELECT id, kind, zfs_name, display_name, description, tags
FROM resource_metadata
WHERE kind = $1
ORDER BY zfs_name;
