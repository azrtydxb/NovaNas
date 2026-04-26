-- 0002_resources.sql — polymorphic NovaNas business-object table.
--
-- This is the storage backing for the CRD-to-Postgres migration. Each row
-- holds one resource of one `kind` keyed by `(kind, namespace, name)`. The
-- jsonb columns mirror the Kubernetes CRD envelope (labels / annotations
-- / spec / status) so the API's existing Zod schemas and the SPA's data
-- shapes work unchanged.
--
-- See packages/db/src/schema/resources.ts for the design rationale.



CREATE TABLE IF NOT EXISTS resources (
    kind         varchar(64)  NOT NULL,
    name         varchar(253) NOT NULL,
    namespace    varchar(253) NOT NULL DEFAULT '',
    labels       jsonb        NOT NULL DEFAULT '{}'::jsonb,
    annotations  jsonb        NOT NULL DEFAULT '{}'::jsonb,
    spec         jsonb        NOT NULL DEFAULT '{}'::jsonb,
    status       jsonb        NOT NULL DEFAULT '{}'::jsonb,
    revision     text         NOT NULL DEFAULT '1',
    created_at   timestamptz  NOT NULL DEFAULT now(),
    updated_at   timestamptz  NOT NULL DEFAULT now(),
    deleted_at   timestamptz
);

-- Primary access path: get/patch/delete by (kind, namespace, name).
-- Cluster-scoped resources use namespace='' so this index serves both.
CREATE UNIQUE INDEX IF NOT EXISTS resources_kind_namespace_name_idx
    ON resources (kind, namespace, name);

-- LIST <kind> [in <namespace>]
CREATE INDEX IF NOT EXISTS resources_kind_idx ON resources (kind);

-- Watch / change-feed support: ORDER BY updated_at for consumers polling
-- for changes since a known timestamp.
CREATE INDEX IF NOT EXISTS resources_updated_idx ON resources (updated_at);


