-- name: GetResourceMetadata :one
SELECT id, kind, zfs_name, display_name, description, tags
FROM resource_metadata
WHERE kind = $1 AND zfs_name = $2;

-- name: ListResourceMetadataByKind :many
SELECT id, kind, zfs_name, display_name, description, tags
FROM resource_metadata
WHERE kind = $1
ORDER BY zfs_name;

-- name: UpsertResourceMetadata :one
INSERT INTO resource_metadata (kind, zfs_name, display_name, description, tags)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (kind, zfs_name) DO UPDATE
   SET display_name = COALESCE(EXCLUDED.display_name, resource_metadata.display_name),
       description  = COALESCE(EXCLUDED.description,  resource_metadata.description),
       tags         = COALESCE(EXCLUDED.tags,         resource_metadata.tags)
RETURNING id, kind, zfs_name, display_name, description, tags;

-- name: DeleteResourceMetadata :exec
DELETE FROM resource_metadata WHERE kind = $1 AND zfs_name = $2;
