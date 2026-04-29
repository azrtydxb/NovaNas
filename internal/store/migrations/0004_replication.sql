-- +goose Up
--
-- General replication subsystem (internal/replication).
--
-- Supersedes the older scheduler-driven replication path
-- (replication_targets/replication_schedules in 0002_scheduler.sql).
-- Those tables are intentionally left in place for backward compatibility
-- with hosts that already configured them; they are no longer written by
-- the API. See docs/replication/README.md for the migration story.
CREATE TABLE replication_jobs (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text        NOT NULL UNIQUE,
    backend         text        NOT NULL,                    -- zfs|s3|rsync
    direction       text        NOT NULL,                    -- push|pull
    source_json     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    destination_json jsonb      NOT NULL DEFAULT '{}'::jsonb,
    schedule        text        NOT NULL DEFAULT '',         -- empty = manual-only
    retention_json  jsonb       NOT NULL DEFAULT '{}'::jsonb,
    enabled         boolean     NOT NULL DEFAULT true,
    secret_ref      text        NOT NULL DEFAULT '',
    last_snapshot   text        NOT NULL DEFAULT '',
    last_fired_at   timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_replication_jobs_enabled ON replication_jobs(enabled);

CREATE TABLE replication_runs (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id            uuid        NOT NULL REFERENCES replication_jobs(id) ON DELETE CASCADE,
    started_at        timestamptz NOT NULL DEFAULT now(),
    finished_at       timestamptz,
    outcome           text        NOT NULL DEFAULT 'pending',
    bytes_transferred bigint      NOT NULL DEFAULT 0,
    snapshot          text        NOT NULL DEFAULT '',
    error             text        NOT NULL DEFAULT ''
);
CREATE INDEX idx_replication_runs_job_started ON replication_runs(job_id, started_at DESC, id DESC);

-- +goose Down
DROP TABLE replication_runs;
DROP TABLE replication_jobs;
