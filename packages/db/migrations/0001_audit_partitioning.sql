-- B3-API-Infra: Convert audit_log to a range-partitioned table by timestamp
-- (monthly partitions). The drizzle schema keeps the logical columns; this
-- migration reshapes physical storage so that retention policy GC can drop
-- whole partitions instead of running slow DELETEs.
--
-- NOTE: Partitioned tables in Postgres require the partition key to be part
-- of the primary key; we therefore drop the old single-column PK and replace
-- it with a composite (id, timestamp) PK.

BEGIN;

-- Stash existing rows so we can rehydrate into the partitioned parent.
CREATE TABLE IF NOT EXISTS audit_log__legacy (LIKE audit_log INCLUDING ALL);
INSERT INTO audit_log__legacy SELECT * FROM audit_log;

-- Drop dependent indexes + the original table.
DROP INDEX IF EXISTS audit_log_timestamp_actor_idx;
DROP INDEX IF EXISTS audit_log_resource_idx;
DROP INDEX IF EXISTS audit_log_timestamp_idx;
DROP INDEX IF EXISTS audit_log_action_idx;
DROP TABLE audit_log;

-- Recreate as a PARTITION BY RANGE parent.
CREATE TABLE audit_log (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  "timestamp" timestamptz NOT NULL DEFAULT now(),
  actor_id uuid,
  actor_type audit_actor_type NOT NULL,
  action varchar(128) NOT NULL,
  resource_kind varchar(128) NOT NULL,
  resource_name varchar(253),
  resource_namespace varchar(253),
  payload jsonb,
  outcome audit_outcome NOT NULL,
  source_ip varchar(64),
  details jsonb,
  PRIMARY KEY (id, "timestamp"),
  FOREIGN KEY (actor_id) REFERENCES users (id) ON DELETE SET NULL
) PARTITION BY RANGE ("timestamp");

-- Partition-creation helper. Creates a monthly partition plus the two standard
-- indexes on (timestamp DESC, actor_id) and (action). Idempotent.
CREATE OR REPLACE FUNCTION novanas_create_audit_partition(month_start date)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE
  part_name text;
  range_start timestamptz;
  range_end   timestamptz;
BEGIN
  range_start := date_trunc('month', month_start)::timestamptz;
  range_end   := (date_trunc('month', month_start) + interval '1 month')::timestamptz;
  part_name := format('audit_log_y%sm%s',
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

-- Bootstrap: current month + next 3 months.
SELECT novanas_create_audit_partition(date_trunc('month', now())::date);
SELECT novanas_create_audit_partition((date_trunc('month', now()) + interval '1 month')::date);
SELECT novanas_create_audit_partition((date_trunc('month', now()) + interval '2 month')::date);
SELECT novanas_create_audit_partition((date_trunc('month', now()) + interval '3 month')::date);

-- Rehydrate legacy rows — each one lands in the matching partition. Older
-- rows (outside any existing partition) are routed into a catch-all
-- "default" partition so we don't error on historical data.
CREATE TABLE IF NOT EXISTS audit_log_default PARTITION OF audit_log DEFAULT;

INSERT INTO audit_log
  (id, "timestamp", actor_id, actor_type, action, resource_kind,
   resource_name, resource_namespace, payload, outcome, source_ip, details)
SELECT id, "timestamp", actor_id, actor_type, action, resource_kind,
       resource_name, resource_namespace, payload, outcome, source_ip, details
FROM audit_log__legacy;

DROP TABLE audit_log__legacy;

COMMIT;
