-- +goose Up
-- =====================================================================
-- Tier 2 plugin engine — marketplace registry. The locked
-- novanas-official entry is seeded by nova-api at boot if missing
-- (using MARKETPLACE_INDEX_URL + MARKETPLACE_TRUST_KEY_PATH for
-- backward compat). Operators add other marketplaces (TrueCharts,
-- third-party publishers, internal mirrors) via the API; each carries
-- its own pinned trust key and is fetched independently.
-- =====================================================================

CREATE TABLE IF NOT EXISTS marketplaces (
    id            uuid PRIMARY KEY,
    name          text NOT NULL UNIQUE,
    index_url     text NOT NULL,
    trust_key_url text NOT NULL,
    trust_key_pem text NOT NULL,
    locked        boolean NOT NULL DEFAULT false,
    enabled       boolean NOT NULL DEFAULT true,
    added_by      text,
    added_at      timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS marketplaces_enabled_idx ON marketplaces (enabled);

-- +goose Down
DROP TABLE IF EXISTS marketplaces;
