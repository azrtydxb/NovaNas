import { sql } from 'drizzle-orm';
import type { FastifyBaseLogger } from 'fastify';
import type { DbClient } from './db.js';

/**
 * Audit-log partition maintenance.
 *
 * Each audit_log partition holds one calendar month of rows (see
 * `0001_audit_partitioning.sql`). This service:
 *
 *   1. Ensures the current month plus the next 3 months have partitions
 *      pre-created — the insert path must never block on DDL.
 *   2. Drops partitions whose upper bound is older than the configured
 *      retention window (default 1 year).
 *
 * It is designed to run on API boot and then daily via `setInterval`.
 */

export interface AuditPartitionGcOptions {
  db: DbClient | null | undefined;
  logger: FastifyBaseLogger;
  /** How many months of future partitions to keep ahead of `now`. Default 3. */
  aheadMonths?: number;
  /** Retention window in days. Default 365. */
  retentionDays?: number;
  /** Re-run cadence in ms. Default 24h. */
  intervalMs?: number;
  /** Run a pass immediately on start. Default true. */
  runOnStart?: boolean;
  /** Clock injection for tests. */
  now?: () => Date;
}

export interface AuditPartitionGcHandle {
  /** Run one pass immediately. Resolves when the pass finishes. */
  runOnce(): Promise<{ created: string[]; dropped: string[] }>;
  /** Stop the scheduled interval. */
  stop(): void;
}

interface PartitionRow {
  partition_name: string;
  range_end: string;
}

const MONTH_MS = 31 * 24 * 3600 * 1000;

function firstOfMonth(d: Date): Date {
  return new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), 1));
}

function addMonths(d: Date, n: number): Date {
  return new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + n, 1));
}

function toDateLiteral(d: Date): string {
  // YYYY-MM-01, suitable for ::date casts
  const y = d.getUTCFullYear();
  const m = String(d.getUTCMonth() + 1).padStart(2, '0');
  return `${y}-${m}-01`;
}

/** Introspect the current partitions of audit_log. */
export async function listAuditPartitions(db: DbClient): Promise<PartitionRow[]> {
  const res = (await db.execute(sql`
    SELECT c.relname AS partition_name,
           pg_get_expr(c.relpartbound, c.oid) AS bound
      FROM pg_inherits i
      JOIN pg_class parent ON parent.oid = i.inhparent
      JOIN pg_class c ON c.oid = i.inhrelid
     WHERE parent.relname = 'audit_log'
  `)) as unknown as Array<{ partition_name: string; bound: string | null }>;

  const rows: PartitionRow[] = [];
  for (const r of res) {
    // bound: "FOR VALUES FROM ('2026-04-01 00:00:00+00') TO ('2026-05-01 00:00:00+00')"
    // or    "DEFAULT"
    const m = /TO \('([^']+)'\)/.exec(r.bound ?? '');
    rows.push({ partition_name: r.partition_name, range_end: m?.[1] ?? '' });
  }
  return rows;
}

export function startAuditPartitionGc(opts: AuditPartitionGcOptions): AuditPartitionGcHandle {
  const {
    db,
    logger,
    aheadMonths = 3,
    retentionDays = 365,
    intervalMs = 24 * 3600 * 1000,
    now = () => new Date(),
  } = opts;

  async function runOnce(): Promise<{ created: string[]; dropped: string[] }> {
    const created: string[] = [];
    const dropped: string[] = [];
    if (!db) return { created, dropped };

    try {
      // 1) Ensure current + next N months exist.
      const base = firstOfMonth(now());
      for (let i = 0; i <= aheadMonths; i++) {
        const monthStart = addMonths(base, i);
        const literal = toDateLiteral(monthStart);
        await db.execute(sql.raw(`SELECT novanas_create_audit_partition('${literal}'::date)`));
        created.push(
          `audit_log_y${monthStart.getUTCFullYear()}m${String(monthStart.getUTCMonth() + 1).padStart(2, '0')}`
        );
      }

      // 2) Drop partitions whose upper bound is older than the retention cutoff.
      const cutoff = new Date(now().getTime() - retentionDays * 24 * 3600 * 1000);
      const partitions = await listAuditPartitions(db);
      for (const p of partitions) {
        if (!p.range_end) continue; // skip DEFAULT partition
        const end = new Date(p.range_end);
        if (Number.isNaN(end.getTime())) continue;
        if (end.getTime() < cutoff.getTime()) {
          await db.execute(sql.raw(`DROP TABLE IF EXISTS ${p.partition_name}`));
          dropped.push(p.partition_name);
        }
      }

      logger.info({ created: created.length, dropped: dropped.length }, 'audit_partition_gc.pass');
    } catch (err) {
      logger.error({ err }, 'audit_partition_gc.failed');
    }

    return { created, dropped };
  }

  // Kick off an immediate pass (unless the caller opts out, e.g. in tests),
  // then a repeating one. We intentionally swallow the returned promise —
  // errors are logged inside `runOnce`.
  if (opts.runOnStart !== false) void runOnce();
  const timer = setInterval(() => {
    void runOnce();
  }, intervalMs);
  // Don't keep the process alive.
  if (typeof (timer as { unref?: () => void }).unref === 'function') {
    (timer as unknown as { unref: () => void }).unref();
  }

  return {
    runOnce,
    stop: () => clearInterval(timer),
  };
}

/** Exported for test introspection. */
export const _internals = { firstOfMonth, addMonths, toDateLiteral, MONTH_MS };
