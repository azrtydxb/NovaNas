-- +goose Up
CREATE TABLE scrub_policies (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text        NOT NULL UNIQUE,
    -- "*" means "all pools at fire time"; otherwise a comma-separated list
    -- of pool names. Stored as text to keep the table portable; the
    -- executor splits on ',' and trims whitespace.
    pools         text        NOT NULL DEFAULT '*',
    cron          text        NOT NULL,
    -- priority: low|medium|high. Currently advisory — passed through to
    -- the metrics + audit log; ZFS itself has no scrub priority knob
    -- portable across versions.
    priority      text        NOT NULL DEFAULT 'medium',
    enabled       boolean     NOT NULL DEFAULT true,
    -- Marks the operator-default policy. The bootstrap path inserts one
    -- row with builtin=true on a fresh install; re-running is a no-op
    -- (UNIQUE(name) prevents duplicates).
    builtin       boolean     NOT NULL DEFAULT false,
    last_fired_at timestamptz,
    last_error    text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_scrub_policies_enabled ON scrub_policies(enabled);

-- +goose Down
DROP TABLE scrub_policies;
