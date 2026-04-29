-- +goose Up
CREATE TABLE snapshot_schedules (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset         text        NOT NULL,
    name            text        NOT NULL,
    cron            text        NOT NULL,
    recursive       boolean     NOT NULL DEFAULT false,
    snapshot_prefix text        NOT NULL DEFAULT 'auto',
    retention_hourly  int       NOT NULL DEFAULT 0,
    retention_daily   int       NOT NULL DEFAULT 0,
    retention_weekly  int       NOT NULL DEFAULT 0,
    retention_monthly int       NOT NULL DEFAULT 0,
    retention_yearly  int       NOT NULL DEFAULT 0,
    enabled         boolean     NOT NULL DEFAULT true,
    last_fired_at   timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(dataset, name)
);
CREATE INDEX idx_snapshot_schedules_enabled ON snapshot_schedules(enabled);

CREATE TABLE replication_targets (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name           text        NOT NULL UNIQUE,
    host           text        NOT NULL,
    port           int         NOT NULL DEFAULT 22,
    ssh_user       text        NOT NULL,
    ssh_key_path   text        NOT NULL,
    dataset_prefix text        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE replication_schedules (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    src_dataset        text        NOT NULL,
    target_id          uuid        NOT NULL REFERENCES replication_targets(id) ON DELETE CASCADE,
    cron               text        NOT NULL,
    snapshot_prefix    text        NOT NULL DEFAULT 'repl',
    retention_remote   int         NOT NULL DEFAULT 0,
    enabled            boolean     NOT NULL DEFAULT true,
    last_fired_at      timestamptz,
    last_sync_snapshot text,
    last_error         text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_replication_schedules_enabled ON replication_schedules(enabled);

-- +goose Down
DROP TABLE replication_schedules;
DROP TABLE replication_targets;
DROP TABLE snapshot_schedules;
