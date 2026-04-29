-- =====================================================================
-- Marketplaces registry queries (Tier 2 plugin engine).
-- =====================================================================

-- name: CreateMarketplace :one
INSERT INTO marketplaces (id, name, index_url, trust_key_url, trust_key_pem, locked, enabled, added_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, name, index_url, trust_key_url, trust_key_pem, locked, enabled, added_by, added_at, updated_at;

-- name: GetMarketplace :one
SELECT id, name, index_url, trust_key_url, trust_key_pem, locked, enabled, added_by, added_at, updated_at
FROM marketplaces
WHERE id = $1;

-- name: GetMarketplaceByName :one
SELECT id, name, index_url, trust_key_url, trust_key_pem, locked, enabled, added_by, added_at, updated_at
FROM marketplaces
WHERE name = $1;

-- name: ListMarketplaces :many
SELECT id, name, index_url, trust_key_url, trust_key_pem, locked, enabled, added_by, added_at, updated_at
FROM marketplaces
ORDER BY locked DESC, added_at ASC;

-- name: ListEnabledMarketplaces :many
SELECT id, name, index_url, trust_key_url, trust_key_pem, locked, enabled, added_by, added_at, updated_at
FROM marketplaces
WHERE enabled = true
ORDER BY locked DESC, added_at ASC;

-- name: UpdateMarketplaceEnabled :one
UPDATE marketplaces
SET enabled = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, name, index_url, trust_key_url, trust_key_pem, locked, enabled, added_by, added_at, updated_at;

-- name: UpdateMarketplaceTrustKey :one
UPDATE marketplaces
SET trust_key_pem = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, name, index_url, trust_key_url, trust_key_pem, locked, enabled, added_by, added_at, updated_at;

-- name: DeleteMarketplace :exec
DELETE FROM marketplaces WHERE id = $1;
