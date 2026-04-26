-- B3-API-Infra: Convert audit_log to a range-partitioned table by timestamp
-- (monthly partitions). The drizzle schema keeps the logical columns; this
-- migration reshapes physical storage so that retention policy GC can drop
-- whole partitions instead of running slow DELETEs.
--
-- NOTE: Partitioned tables in Postgres require the partition key to be part
-- of the primary key; we therefore drop the old single-column PK and replace
-- it with a composite (id, timestamp) PK.
--
-- Made idempotent (#43): on fresh installs there is no pre-existing
-- audit_log to migrate, so the legacy stash + DROP are skipped. The
-- baseline migration in 0003_baseline_schema.sql creates the
-- partitioned shape directly for fresh installs.



-- Only run the legacy-to-partitioned conversion if a non-partitioned
-- audit_log exists. PARTITIONED tables show up in pg_partitioned_table;
-- their absence + presence in pg_class means "legacy table to migrate."
DO $migrate$
DECLARE
  has_legacy boolean;
BEGIN
  SELECT EXISTS(
    SELECT 1 FROM pg_class c
    LEFT JOIN pg_partitioned_table p ON p.partrelid = c.oid
    WHERE c.relname = 'audit_log' AND c.relkind = 'r' AND p.partrelid IS NULL
  ) INTO has_legacy;

  IF NOT has_legacy THEN
    RETURN;
  END IF;

  CREATE TABLE IF NOT EXISTS audit_log__legacy (LIKE audit_log INCLUDING ALL);
  INSERT INTO audit_log__legacy SELECT * FROM audit_log;

  DROP INDEX IF EXISTS audit_log_timestamp_actor_idx;
  DROP INDEX IF EXISTS audit_log_resource_idx;
  DROP INDEX IF EXISTS audit_log_timestamp_idx;
  DROP INDEX IF EXISTS audit_log_action_idx;
  DROP TABLE audit_log;
END
$migrate$;

-- Recreate as a PARTITION BY RANGE parent. IF NOT EXISTS so a fresh
-- install (where 0003 might run first, depending on journal order)
-- doesn't double-create.
CREATE TABLE IF NOT EXISTS audit_log (
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

-- Catch-all "default" partition for rows whose timestamp falls outside
-- the bootstrapped range (e.g. legacy rows from before partitioning).
CREATE TABLE IF NOT EXISTS audit_log_default PARTITION OF audit_log DEFAULT;

-- Rehydrate legacy rows only if the legacy stash exists (skipped on
-- fresh installs by the DO block above).
DO $rehydrate$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'audit_log__legacy') THEN
    INSERT INTO audit_log
      (id, "timestamp", actor_id, actor_type, action, resource_kind,
       resource_name, resource_namespace, payload, outcome, source_ip, details)
    SELECT id, "timestamp", actor_id, actor_type, action, resource_kind,
           resource_name, resource_namespace, payload, outcome, source_ip, details
    FROM audit_log__legacy;

    DROP TABLE audit_log__legacy;
  END IF;
END
$rehydrate$;


