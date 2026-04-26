-- 0003_baseline_schema.sql
--
-- Idempotent baseline of every business-critical table. Closes #43:
-- the api was logging warnings about missing tables (metric_rollups,
-- novanas_create_audit_partition function, sessions, jobs, …) on
-- every boot because the prior 0001/0002 migrations only handled
-- the audit-log partitioning + the resources table. This migration
-- creates the rest, plus a partitioned audit_log + helper function
-- for fresh installs that have never seen 0001.
--
-- Every CREATE is IF NOT EXISTS so this is safe on:
--   - Fresh install (creates everything)
--   - Dev environments that ran 0002 manually but skipped 0001
--   - Systems that already ran 0001 (audit_log already partitioned)
--
-- Schema mirrors packages/db/src/schema/*.ts — keep in sync.

BEGIN;

-- Enums --------------------------------------------------------------------

DO $$ BEGIN
  CREATE TYPE job_state AS ENUM ('queued', 'running', 'succeeded', 'failed', 'cancelled');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE notification_severity AS ENUM ('info', 'warning', 'critical');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE audit_actor_type AS ENUM ('user', 'system', 'operator', 'apiToken');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE audit_outcome AS ENUM ('success', 'failure', 'denied');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Identity -----------------------------------------------------------------

CREATE TABLE IF NOT EXISTS users (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  keycloak_id   varchar(128) NOT NULL,
  username      varchar(255) NOT NULL,
  email         varchar(320),
  display_name  varchar(255),
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS users_keycloak_id_idx ON users (keycloak_id);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_idx ON users (username);

CREATE TABLE IF NOT EXISTS groups (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  keycloak_id  varchar(128) NOT NULL,
  name         varchar(255) NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS groups_keycloak_id_idx ON groups (keycloak_id);
CREATE INDEX IF NOT EXISTS groups_name_idx ON groups (name);

CREATE TABLE IF NOT EXISTS user_groups (
  user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  group_id   uuid NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, group_id)
);
CREATE INDEX IF NOT EXISTS user_groups_user_idx ON user_groups (user_id);
CREATE INDEX IF NOT EXISTS user_groups_group_idx ON user_groups (group_id);

-- Sessions / api-tokens ----------------------------------------------------

CREATE TABLE IF NOT EXISTS sessions (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  token_hash  varchar(128) NOT NULL,
  user_agent  varchar(512),
  ip_address  varchar(64),
  created_at  timestamptz NOT NULL DEFAULT now(),
  expires_at  timestamptz NOT NULL,
  revoked_at  timestamptz
);
CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions (user_id);
CREATE INDEX IF NOT EXISTS sessions_token_hash_idx ON sessions (token_hash);
CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions (expires_at);

CREATE TABLE IF NOT EXISTS api_tokens (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  name          varchar(255) NOT NULL,
  token_hash    varchar(128) NOT NULL,
  scopes        jsonb NOT NULL DEFAULT '[]'::jsonb,
  expires_at    timestamptz,
  last_used_at  timestamptz,
  revoked_at    timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS api_tokens_token_hash_idx ON api_tokens (token_hash);
CREATE INDEX IF NOT EXISTS api_tokens_user_idx ON api_tokens (user_id);
CREATE INDEX IF NOT EXISTS api_tokens_expires_idx ON api_tokens (expires_at);

-- Jobs / notifications -----------------------------------------------------

CREATE TABLE IF NOT EXISTS jobs (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  kind              varchar(128) NOT NULL,
  state             job_state NOT NULL DEFAULT 'queued',
  progress_percent  integer NOT NULL DEFAULT 0,
  started_at        timestamptz,
  finished_at       timestamptz,
  params            jsonb NOT NULL DEFAULT '{}'::jsonb,
  result            jsonb,
  error             text,
  owner_id          uuid REFERENCES users (id) ON DELETE SET NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS jobs_state_kind_idx ON jobs (state, kind);
CREATE INDEX IF NOT EXISTS jobs_owner_idx ON jobs (owner_id);
CREATE INDEX IF NOT EXISTS jobs_created_idx ON jobs (created_at);

CREATE TABLE IF NOT EXISTS notifications (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  severity    notification_severity NOT NULL DEFAULT 'info',
  title       varchar(255) NOT NULL,
  body        text NOT NULL,
  link        varchar(1024),
  read_at     timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS notifications_user_read_idx ON notifications (user_id, read_at);
CREATE INDEX IF NOT EXISTS notifications_created_idx ON notifications (created_at);

-- User preferences ---------------------------------------------------------

CREATE TABLE IF NOT EXISTS user_preferences (
  user_id      uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  preferences  jsonb NOT NULL DEFAULT '{}'::jsonb,
  updated_at   timestamptz NOT NULL DEFAULT now()
);

-- App-catalog cache --------------------------------------------------------

CREATE TABLE IF NOT EXISTS app_catalog_cache (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  catalog_name  varchar(255) NOT NULL,
  app_name      varchar(255) NOT NULL,
  version       varchar(64) NOT NULL,
  metadata      jsonb NOT NULL DEFAULT '{}'::jsonb,
  icon          bytea,
  fetched_at    timestamptz NOT NULL DEFAULT now(),
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS app_catalog_cache_catalog_app_version_idx
  ON app_catalog_cache (catalog_name, app_name, version);
CREATE INDEX IF NOT EXISTS app_catalog_cache_app_idx ON app_catalog_cache (app_name);
CREATE INDEX IF NOT EXISTS app_catalog_cache_fetched_idx ON app_catalog_cache (fetched_at);

-- Metric rollups -----------------------------------------------------------

CREATE TABLE IF NOT EXISTS metric_rollups (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  metric_name    varchar(255) NOT NULL,
  resource_kind  varchar(128) NOT NULL,
  resource_name  varchar(253) NOT NULL,
  window_start   timestamptz NOT NULL,
  window_end     timestamptz NOT NULL,
  aggregation    jsonb NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS metric_rollups_metric_resource_window_idx
  ON metric_rollups (metric_name, resource_kind, resource_name, window_start);
CREATE INDEX IF NOT EXISTS metric_rollups_window_idx
  ON metric_rollups (window_start, window_end);

-- Partitioned audit_log + helper -------------------------------------------
--
-- 0001_audit_partitioning.sql creates this on systems that already had a
-- non-partitioned audit_log; for fresh installs that path doesn't fire (the
-- table doesn't exist to drop). Re-create it idempotently here so the api's
-- audit-partition-gc service finds the helper function on first boot.

CREATE TABLE IF NOT EXISTS audit_log (
  id                  uuid NOT NULL DEFAULT gen_random_uuid(),
  "timestamp"         timestamptz NOT NULL DEFAULT now(),
  actor_id            uuid,
  actor_type          audit_actor_type NOT NULL,
  action              varchar(128) NOT NULL,
  resource_kind       varchar(128) NOT NULL,
  resource_name       varchar(253),
  resource_namespace  varchar(253),
  payload             jsonb,
  outcome             audit_outcome NOT NULL,
  source_ip           varchar(64),
  details             jsonb,
  PRIMARY KEY (id, "timestamp"),
  FOREIGN KEY (actor_id) REFERENCES users (id) ON DELETE SET NULL
) PARTITION BY RANGE ("timestamp");

CREATE OR REPLACE FUNCTION novanas_create_audit_partition(month_start date)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE
  part_name   text;
  range_start timestamptz;
  range_end   timestamptz;
BEGIN
  range_start := date_trunc('month', month_start)::timestamptz;
  range_end   := (date_trunc('month', month_start) + interval '1 month')::timestamptz;
  part_name   := format('audit_log_y%sm%s',
                        to_char(range_start, 'YYYY'),
                        to_char(range_start, 'MM'));

  EXECUTE format(
    'CREATE TABLE IF NOT EXISTS %I PARTITION OF audit_log FOR VALUES FROM (%L) TO (%L)',
    part_name, range_start, range_end
  );
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS %I ON %I ("timestamp" DESC, actor_id)',
    part_name || '_ts_actor_idx', part_name
  );
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS %I ON %I (resource_kind, resource_name)',
    part_name || '_resource_idx', part_name
  );
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS %I ON %I (action)',
    part_name || '_action_idx', part_name
  );
END;
$$;

-- Bootstrap partitions: current + 3 months ahead. Idempotent.
SELECT novanas_create_audit_partition(date_trunc('month', now())::date);
SELECT novanas_create_audit_partition((date_trunc('month', now()) + interval '1 month')::date);
SELECT novanas_create_audit_partition((date_trunc('month', now()) + interval '2 month')::date);
SELECT novanas_create_audit_partition((date_trunc('month', now()) + interval '3 month')::date);

CREATE TABLE IF NOT EXISTS audit_log_default PARTITION OF audit_log DEFAULT;

COMMIT;
