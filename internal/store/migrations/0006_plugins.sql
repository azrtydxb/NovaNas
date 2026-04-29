-- +goose Up
-- =====================================================================
-- Tier 2 first-party plugins. The marketplace lives off-box; this table
-- records what's installed locally so nova-api can re-mount API routes
-- and the UI bundle on restart.
-- =====================================================================

CREATE TABLE IF NOT EXISTS plugins (
    id            uuid PRIMARY KEY,
    name          text NOT NULL UNIQUE,
    version       text NOT NULL,
    manifest      jsonb NOT NULL,
    status        text NOT NULL DEFAULT 'installed',
    installed_at  timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS plugins_status_idx ON plugins (status);

-- plugin_resources is the cleanup ledger: every auto-provisioned `needs:`
-- resource is recorded here so uninstall (--purge) can undo them.
CREATE TABLE IF NOT EXISTS plugin_resources (
    id            bigserial PRIMARY KEY,
    plugin_id     uuid NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    resource_type text NOT NULL CHECK (resource_type IN ('dataset','oidcClient','tlsCert','permission')),
    resource_id   text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS plugin_resources_plugin_idx ON plugin_resources (plugin_id);

-- +goose Down
DROP TABLE IF EXISTS plugin_resources;
DROP TABLE IF EXISTS plugins;
