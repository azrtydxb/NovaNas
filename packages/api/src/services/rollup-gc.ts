import { metricRollups } from '@novanas/db';
import { and, lt, sql } from 'drizzle-orm';
import type { FastifyBaseLogger } from 'fastify';
import type { DbClient } from './db.js';

/**
 * Metric-rollup retention.
 *
 * Three retention tiers, keyed by window duration:
 *   - 5-minute windows  → kept  30 days
 *   - hourly windows    → kept  90 days
 *   - daily  windows    → kept 365 days
 *
 * Windows are classified by `window_end - window_start`. Anything else falls
 * under the most generous (daily) bucket so we never blow away rows whose
 * classification we don't understand.
 */

export interface RollupGcOptions {
  db: DbClient | null | undefined;
  logger: FastifyBaseLogger;
  /** Override retention defaults. All values in days. */
  retention?: {
    fiveMinuteDays?: number;
    hourlyDays?: number;
    dailyDays?: number;
  };
  /** Re-run cadence in ms. Default 24h. */
  intervalMs?: number;
  /** Run a pass immediately on start. Default true. */
  runOnStart?: boolean;
  /** Clock injection for tests. */
  now?: () => Date;
}

export interface RollupGcHandle {
  runOnce(): Promise<{ deleted: number }>;
  stop(): void;
}

const DEFAULT_RETENTION = {
  fiveMinuteDays: 30,
  hourlyDays: 90,
  dailyDays: 365,
} as const;

export function startRollupGc(opts: RollupGcOptions): RollupGcHandle {
  const { db, logger, retention, intervalMs = 24 * 3600 * 1000, now = () => new Date() } = opts;

  const rFive = retention?.fiveMinuteDays ?? DEFAULT_RETENTION.fiveMinuteDays;
  const rHour = retention?.hourlyDays ?? DEFAULT_RETENTION.hourlyDays;
  const rDay = retention?.dailyDays ?? DEFAULT_RETENTION.dailyDays;

  async function deleteWindow(widthSec: number, days: number): Promise<number> {
    if (!db) return 0;
    const cutoff = new Date(now().getTime() - days * 24 * 3600 * 1000);
    // Use raw SQL fragment for the width predicate so we don't need an extra
    // computed column. `EXTRACT(EPOCH FROM (window_end - window_start))` is
    // computed on the filtered rows only; the `created_at` filter keeps the
    // scan small.
    const res = (await db
      .delete(metricRollups)
      .where(
        and(
          lt(metricRollups.createdAt, cutoff),
          sql`EXTRACT(EPOCH FROM (${metricRollups.windowEnd} - ${metricRollups.windowStart})) = ${widthSec}`
        )
      )
      .returning({ id: metricRollups.id })) as Array<{ id: string }>;
    return res.length;
  }

  async function deleteLongTail(): Promise<number> {
    if (!db) return 0;
    // Catch-all for daily+ windows (>= 1h width but not exactly 1h): treat as
    // daily-tier retention.
    const cutoff = new Date(now().getTime() - rDay * 24 * 3600 * 1000);
    const res = (await db
      .delete(metricRollups)
      .where(
        and(
          lt(metricRollups.createdAt, cutoff),
          sql`EXTRACT(EPOCH FROM (${metricRollups.windowEnd} - ${metricRollups.windowStart})) > 3600`
        )
      )
      .returning({ id: metricRollups.id })) as Array<{ id: string }>;
    return res.length;
  }

  async function runOnce(): Promise<{ deleted: number }> {
    if (!db) return { deleted: 0 };
    let deleted = 0;
    try {
      deleted += await deleteWindow(300, rFive); // 5m
      deleted += await deleteWindow(3600, rHour); // 1h
      deleted += await deleteLongTail();
      logger.info({ deleted }, 'rollup_gc.pass');
    } catch (err) {
      logger.error({ err }, 'rollup_gc.failed');
    }
    return { deleted };
  }

  if (opts.runOnStart !== false) void runOnce();
  const timer = setInterval(() => {
    void runOnce();
  }, intervalMs);
  if (typeof (timer as { unref?: () => void }).unref === 'function') {
    (timer as unknown as { unref: () => void }).unref();
  }

  return {
    runOnce,
    stop: () => clearInterval(timer),
  };
}
