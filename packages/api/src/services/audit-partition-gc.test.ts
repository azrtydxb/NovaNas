import { describe, expect, it, vi } from 'vitest';
import { _internals, startAuditPartitionGc } from './audit-partition-gc.js';
import type { DbClient } from './db.js';

function silentLogger() {
  return {
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
    debug: vi.fn(),
    trace: vi.fn(),
    fatal: vi.fn(),
    level: 'silent',
    silent: vi.fn(),
    child: vi.fn(),
  } as unknown as import('fastify').FastifyBaseLogger;
}

/**
 * Fake `DbClient.execute` that records SQL strings and returns a scripted
 * partition listing when it sees the pg_inherits probe.
 */
function fakeDbWithPartitions(existing: Array<{ name: string; rangeEnd: string | null }>): {
  db: DbClient;
  calls: string[];
} {
  const calls: string[] = [];
  const db = {
    async execute(stmt: unknown) {
      const sqlText = extractSqlText(stmt);
      calls.push(sqlText);
      if (sqlText.includes('pg_inherits')) {
        return existing.map((p) => ({
          partition_name: p.name,
          bound: p.rangeEnd ? `FOR VALUES FROM ('${p.rangeEnd}') TO ('${p.rangeEnd}')` : 'DEFAULT',
        }));
      }
      return [];
    },
  } as unknown as DbClient;
  return { db, calls };
}

function extractSqlText(stmt: unknown): string {
  // drizzle sql nodes have `.queryChunks` that include strings. For sql.raw,
  // the text is stored directly. Fall back to JSON.stringify.
  const obj = stmt as {
    queryChunks?: Array<{ value?: string[] | string } | string>;
    sql?: string;
  };
  if (typeof obj?.sql === 'string') return obj.sql;
  if (Array.isArray(obj?.queryChunks)) {
    return obj.queryChunks
      .map((c) => {
        if (typeof c === 'string') return c;
        const v = (c as { value?: unknown }).value;
        if (Array.isArray(v)) return v.join('');
        if (typeof v === 'string') return v;
        return '';
      })
      .join('');
  }
  try {
    return JSON.stringify(stmt);
  } catch {
    return String(stmt);
  }
}

describe('startAuditPartitionGc', () => {
  it('creates partitions for the current month plus the next 3 ahead', async () => {
    const now = new Date('2026-04-22T12:00:00Z');
    const { db, calls } = fakeDbWithPartitions([]);

    const h = startAuditPartitionGc({
      db,
      logger: silentLogger(),
      intervalMs: 1_000_000,
      runOnStart: false,
      aheadMonths: 3,
      now: () => now,
    });
    const r = await h.runOnce();
    h.stop();

    // Four create-partition calls (current + 3 ahead).
    const creates = calls.filter((c) => c.includes('novanas_create_audit_partition'));
    expect(creates).toHaveLength(4);
    expect(creates[0]).toContain("'2026-04-01'");
    expect(creates[1]).toContain("'2026-05-01'");
    expect(creates[2]).toContain("'2026-06-01'");
    expect(creates[3]).toContain("'2026-07-01'");
    expect(r.created).toHaveLength(4);
  });

  it('drops partitions whose upper bound is older than the retention cutoff', async () => {
    const now = new Date('2026-04-22T00:00:00Z');
    // default retention = 365 days. A partition ending in 2024 is > 365d old.
    const { db, calls } = fakeDbWithPartitions([
      { name: 'audit_log_y2024m01', rangeEnd: '2024-02-01 00:00:00+00' },
      { name: 'audit_log_y2026m04', rangeEnd: '2026-05-01 00:00:00+00' },
      { name: 'audit_log_default', rangeEnd: null }, // DEFAULT, never dropped
    ]);

    const h = startAuditPartitionGc({
      db,
      logger: silentLogger(),
      intervalMs: 1_000_000,
      runOnStart: false,
      now: () => now,
    });
    const r = await h.runOnce();
    h.stop();

    expect(r.dropped).toEqual(['audit_log_y2024m01']);
    const drops = calls.filter((c) => c.includes('DROP TABLE'));
    expect(drops).toHaveLength(1);
    expect(drops[0]).toContain('audit_log_y2024m01');
  });

  it('no-ops when db is null', async () => {
    const h = startAuditPartitionGc({
      db: null,
      logger: silentLogger(),
      intervalMs: 1_000_000,
      runOnStart: false,
    });
    const r = await h.runOnce();
    h.stop();
    expect(r.created).toEqual([]);
    expect(r.dropped).toEqual([]);
  });

  it('date helpers handle month rollover', () => {
    const d = new Date('2026-11-15T00:00:00Z');
    const base = _internals.firstOfMonth(d);
    expect(_internals.toDateLiteral(base)).toBe('2026-11-01');
    const plus2 = _internals.addMonths(base, 2);
    expect(_internals.toDateLiteral(plus2)).toBe('2027-01-01');
  });
});
