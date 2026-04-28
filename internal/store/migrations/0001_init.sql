-- +goose Up
CREATE TABLE jobs (
    id           uuid        PRIMARY KEY,
    kind         text        NOT NULL,
    target       text        NOT NULL,
    state        text        NOT NULL,
    command      text        NOT NULL DEFAULT '',
    stdout       text        NOT NULL DEFAULT '',
    stderr       text        NOT NULL DEFAULT '',
    exit_code    integer,
    error        text,
    request_id   text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    started_at   timestamptz,
    finished_at  timestamptz,
    CHECK (state IN ('queued','running','succeeded','failed','cancelled','interrupted'))
);
CREATE INDEX jobs_state_idx ON jobs(state);
CREATE INDEX jobs_created_idx ON jobs(created_at DESC);

CREATE TABLE audit_log (
    id           bigserial   PRIMARY KEY,
    ts           timestamptz NOT NULL DEFAULT now(),
    actor        text,
    action       text        NOT NULL,
    target       text        NOT NULL,
    request_id   text        NOT NULL DEFAULT '',
    payload      jsonb,
    result       text        NOT NULL,
    CHECK (result IN ('accepted','rejected'))
);
CREATE INDEX audit_log_ts_idx ON audit_log(ts DESC);

CREATE TABLE resource_metadata (
    id            bigserial   PRIMARY KEY,
    kind          text        NOT NULL,
    zfs_name      text        NOT NULL,
    display_name  text,
    description   text,
    tags          jsonb,
    UNIQUE(kind, zfs_name),
    CHECK (kind IN ('pool','dataset','snapshot'))
);

-- +goose Down
DROP TABLE resource_metadata;
DROP TABLE audit_log;
DROP TABLE jobs;
