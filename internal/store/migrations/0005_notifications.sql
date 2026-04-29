-- +goose Up
--
-- Unified Notification Center (internal/notifycenter).
--
-- The notifications table is the canonical event log: one row per
-- aggregated signal from Alertmanager, the jobs subsystem, the audit
-- log, or the system itself. Rows are append-only; a row is never
-- mutated after insert. (source, source_id) is unique so the
-- aggregator's polling loop is naturally idempotent — re-observing
-- the same alert/job/audit row does not duplicate.
--
-- Per-user state lives in a separate table so a critical alert is
-- read/dismissed/snoozed independently for each operator. The PRIMARY
-- KEY is (notification_id, user_subject); rows are upserted by the
-- handler when a user marks-read / dismisses / snoozes.
CREATE TABLE notifications (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text,                                       -- nullable; reserved for multi-tenancy
    source      text        NOT NULL,                       -- alertmanager|jobs|audit|system
    source_id   text        NOT NULL,                       -- fingerprint/job_id/audit_id/etc
    severity    text        NOT NULL,                       -- info|warning|critical
    title       text        NOT NULL,
    body        text        NOT NULL DEFAULT '',
    link        text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source, source_id)
);
CREATE INDEX idx_notifications_created ON notifications(created_at DESC, id DESC);
CREATE INDEX idx_notifications_severity ON notifications(severity);
CREATE INDEX idx_notifications_source ON notifications(source);

CREATE TABLE notification_state (
    notification_id uuid        NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    user_subject    text        NOT NULL,
    read_at         timestamptz,
    dismissed_at    timestamptz,
    snoozed_until   timestamptz,
    PRIMARY KEY (notification_id, user_subject)
);
CREATE INDEX idx_notification_state_user ON notification_state(user_subject);

-- +goose Down
DROP TABLE notification_state;
DROP TABLE notifications;
